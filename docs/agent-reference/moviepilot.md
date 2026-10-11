# MoviePilot 接口参考

改请求、字段转换、响应 fake 或排查上游行为时读取。先读 [目录规则](../../internal/moviepilot/AGENTS.md)。

本文从原根指南按主题迁出，保留当时的实现说明与上游版本背景，不代表已对当前线上服务重新核验。涉及目标版本变化时，按根指南核验来源。

## v3.1.1 API 核验基线

2026-10-09 核对主人提供的 `MoviePilot/OpenAPI/openapi.json`，`info.version` 为 `v3.1.1`，
SHA-256 为 `c31b46882465e8e046ef405872e3e389c04c4e64e2ada6239ebe3ed27c364680`。
该文件是开发核验材料，不是运行依赖；接口行为另对照官方 `v3.1.1` 源码。

- 含媒体下载仍为单项 `POST /api/v1/download/`，必填 `media_in`、`torrent_in`，结果为 `data.download_id`。
  没有手动勾选种子的批量提交 API；Crychic 只协调逐项调用，不直接操作下载器或 MoviePilot 数据库。
- `/subscribe/execution/batches` 及其取消接口属于订阅搜索，不用于停止手动下载批次；
  停止后续提交只阻止尚未发出的 POST，不调用下载 stop/delete。
- `/download/` 与 `/transfer/queue` 可省略 `page/count` 保持完整结果；不能仅取第一页并据此宣称任务结束。
  `/transfer/queue/page` 的轻量 schema 缺少 `media.type` 和任务 `meta` 集数，当前详情仍用完整队列。
- 资源搜索流已支持可选 `MoviePilot` 资源 Cookie；未配置仍走 `X-API-KEY` JSON 搜索。系统进度、实时消息和 Agent 流未接入。没有自动登录或续期；真实服务禁写、禁 PT 搜索及凭据隐私边界不变。
- `DownloadAddedData.requires_confirmation` 是未识别资源入口的提示；当前已有媒体的 `/download/` 路径不
  自动改走 `/download/add`，也不因缺少 download ID 重试提交。
- Bangumi 简介使用 `GET /api/v1/bangumi/{bangumiid}` 的 `data.overview`；空对象、null 或空简介不算有效
  注释，允许在整页剩余预算内使用原公共 API 回退。没有完整星期分组契约的 MP 放送列表不替换公共日历。

源码依据：[download.py](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/download.py)、
[transfer.py](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/transfer.py)。

## v3.1.3 兼容性核验

2026-10-10 对照官方 `v3.1.3` 提交 `7aa5d613b8641b94624f8879ad9596dbcd4edffa`。第 3 项使用的订阅列表、下载列表、完整整理队列和失败历史契约与 `v3.1.1` 兼容（第 3 项「我的请求」已于 2026-10-11 删除）。版本记录用于追溯源码，客户端不检测或锁定小版本，也没有逐版本适配分支；只解码使用的字段。

本次相关变化是手动音乐整理新增可选 `music_cue_enable`，以及下载目录别名说明。认证、订阅条件 schema 和更新实现未变。资源搜索流新增默认关闭的 `manual_paging` 及可选分页参数，Crychic 未发送这些参数，仍使用兼容的完整搜索流。源码见 [队列模型](https://github.com/jxxghp/MoviePilot/blob/v3.1.3/app/schemas/transfer.py)、[下载端点](https://github.com/jxxghp/MoviePilot/blob/v3.1.3/app/api/endpoints/download.py)、[搜索流](https://github.com/jxxghp/MoviePilot/blob/v3.1.3/app/api/endpoints/search.py)。Context7 仍指向 `v3` 主线，具体差异以上述固定版本源码为准。

## 认证类型与响应

The API key authenticates as MoviePilot's **superuser**; every subscription is owned by the admin, not the
chat user

Media `type` is a Chinese enum: `电影`, `电视剧`; results without `media_source`/`media_id` are dropped

Every JSON endpoint answers `{success, message, data}`, even where the route's `response_model` says
`List[...]` (wrapped by `app/api/response.py`); `moviepilot.Client.do` unwraps it, and `success: false` on
HTTP 200 becomes a `UserError` with `message`

`GET /api/v1/subscribe/media/{id}` returns `data` with `id: null` when nothing is subscribed, not a 404

## 媒体详情与图片

Poster URLs from MoviePilot are TMDB `original` size (MBs); `moviepilot` rewrites them to `w500` (`posterURL`)

`GET /api/v1/media/{id}` details are cosmetic: on failure the card falls back to search metadata and the flow
continues; its `directors` field mixes in producers and episode directors, so it isn't shown

## 媒体库读取

Library checks go through MoviePilot to its media server and are cosmetic like details (failure does not block
confirmation, but the card marks the library state unknown instead of claiming absence). Shows use `POST
/api/v1/mediaserver/exists_remote` (`{"3":[1,2,...]}`, `{}` when absent; needs `title`+`year`, ids alone
answer `{}`). Its episode lists are sorted and deduplicated at the client boundary: media servers can return
several unmerged versions of one episode, which must not count as several episodes or make a missing season
look complete. Movies use `POST /api/v1/mediaserver/notexists` (`[]` = held), because `exists_remote` answers
`{}` for movies either way. `GET /api/v1/mediaserver/exists` reads MoviePilot's own sync table, which is empty
unless library sync runs, so it isn't used

这里的 POST 是只读媒体库查询；根禁令按业务副作用判断。GET `/api/v1/search/*` 仍是禁止的真实站点搜索，不能据 HTTP 方法判定安全。

缺集方案会阻止库读取失败后的自动建议。TV 响应整体 `data:null` 或任一季条目 `null` 不符合 v3.1.1 的字典／集数数组契约，客户端返回通用错误；空对象 `{}` 与合法季条目中的 `[]` 保持合法。不过上游媒体库模块可能将提供方失败也折叠为空结果，合法 `{}` 不能证明媒体库健康，因此空库存预览必须提示先核对连接与扫描状态。参见 [端点](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/mediaserver.py#L129-L150)、[模型](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/schemas/mediaserver.py#L42-L43) 与 [提供方聚合](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/modules/_base/mediaserver.py#L138-L156)。

## 资源搜索与原始记录

Resource search is `GET /api/v1/search/media/{id}?media_source=&mtype=&season=` (MoviePilot searches every
indexer site while the request waits, tens of seconds; `torrentSearchTimeout`), answering `Context.to_dict()`
items; nothing found is `success: false`, 未搜索到任何资源. Their `torrent_info` carries the site's cookie, user agent
and download link: `moviepilot` keeps `media_info` + `torrent_info` raw as `flow.Torrent.Record`, which is
never shown, logged or persisted, and `POST /api/v1/download/` gets it back unchanged as `{media_in,
torrent_in}` like the WebUI; the answer's `download_id` is the torrent hash.

Release-name technical numbers are masked without consuming their boundaries (`moviepilot.withoutTechnical`);
adjacent audio tracks such as `FLAC.2.0+5.1` share a separator, and neither may become an episode token.
Numeric boundaries are checked against the original title so fragments of years or resolutions are not
removed.

补集建议额外投影 `end_season`、`season_list` 及 meta/media 的 `episode_group` 为安全标记 `RangeUnsure`，不展开或持久化组信息。多季、季列表与明确季号冲突或任一非默认组均不参与建议。`season_list` 在未识别季号的 TV 中可能默认 `[1]`，不能补出缺失的 `begin_season`；`episode_list` 仍只是识别结果，并非种子文件清单。参见 [MetaBase](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/domain/meta/metabase.py#L471-L511)、[MetaInfo](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/schemas/context.py#L14-L97)、[结果投影](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/chain/search/result.py#L241-L264) 与 [clear 行为](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/domain/context.py#L1961-L1980)。

含媒体的 [下载接口](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/download.py#L219-L255) 没有按集裁剪参数；缺集组合仍逐个原样提交整个资源，不把重复已有或重叠的文件暗示为不会下载。`size` 为零或未知时不按零成本推荐，不把已知部分相加冒充完整总大小。第 4 项 Context7 结果指向 `v3` 主线且未命中上述细节，故关键结论按固定 `v3.1.1` 源码核验。

## 资源搜索流

`searchstream.go` 只向 `GET /api/v1/search/media/{id}/stream` 发送资源 Cookie，JSON 仍使用 API Key。专用 client 复用 app transport，清空 Cookie jar，拒绝全部重定向，错误不携带响应正文或目标 URL。配置只接受未加引号的资源令牌值，公开域名要求 HTTPS；不会签发、续期或持久化 Cookie。[v3.1.1 认证](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/adapters/web/security/access.py#L176-L301)

`sse.go` 按完整帧读取 UTF-8，支持 BOM、CR/LF/CRLF、心跳注释和多行 data，EOF 不补发缺少空行的帧。`streamresults.go` 将早期候选与最终匹配分开，按 `replace_batch` 的连续批次和总数核对，收到完整 `done` 后一次发布终态；半批、缺终态、错误帧或超时均保留不完整状态。每帧上限 8 MiB，每个搜索阶段最多 10000 个结果，超限按不完整处理，不静默截断。[帧与批次](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/search.py#L97-L447)

早期候选可能没有媒体身份，投影为 `Unverified`，不把搜索目标填回 `media_info`。最终资源核对来源、ID、类型与已知季号，只接受已匹配内容。稳定身份使用资源定位及选择字段的摘要，动态做种数与优先级不改变该身份；原始下载记录仍只存在 `Torrent.Record`，不显示或保存。[候选和终态](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/chain/search/execution.py#L129-L244)、[最终匹配投影](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/chain/search/result.py#L250-L262)

同一搜索只有一个请求，不自动重连或改发 JSON。Go 会重试可重放 GET，SSE 请求因此使用不可重建的空 Body 禁止已发请求重放，同时保持空 GET 语义；首字节丢失由外部 fake 验证。Go 模块为 1.25，本次工具链为 1.26.0，两版本相关分支已对照源码。[Go 1.25 请求重放判定](https://github.com/golang/go/blob/go1.25.0/src/net/http/request.go#L1534-L1547)、[HTTP/2 重试](https://github.com/golang/go/blob/go1.25.0/src/net/http/h2_bundle.go#L7988-L8017)

搜索前站点列表读取 `GET /api/v1/site/media/{电影|电视剧}`，只投影有效、不重复的 ID 与名称。选择后 JSON 和 SSE 均传逗号分隔的 `sites`，不把本地结果筛选当成上游范围。[站点接口](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/site.py#L218-L276)

## 下载进度、暂停恢复与任务删除

资源搜索的 Cookie 流仅用于获取候选，不改变以下任务轮询与完成判断。

`GET /api/v1/download/` lists unfinished torrents; `media` (source, id, `season` "S01", `episode` "E10-E12" or
`E02、E04-E05`, parsed segment by segment without filling gaps) comes from MoviePilot's download history and is
null for torrents added by hand; `progress` is a percentage, `size` the bytes of the files being downloaded
(qBittorrent `total_size`; shown in `/tasks` as e.g. `17.5 GiB`), `state` is `downloading` or `paused`,
`left_time` is Chinese text like `1时5分3秒` (empty while stalled). The card shows the chosen target's downloads,
cosmetic like details

`/tasks` follows a task by re-reading `GET /api/v1/download/` (by downloader instance and torrent hash,
`flow.DownloadRef`) or `GET /api/v1/transfer/queue` (by media + season; tasks are
`waiting`/`running`/`completed`/`failed`, each file covers `meta.begin_episode` through `meta.end_episode`,
inclusive; a missing end means one episode). Byte progress of a transfer is only an SSE stream behind a
browser cookie (`/system/progress/filetransfer`), so transfers show per-file states. A view stops when its
task leaves the list, when its owner presses 停止刷新, or after `flow.FollowFor` (10 min); a callback first
suspends the follower, with rejected callbacks resuming it as described in the Telegram reference. Every task
view has 返回 (`l`), which re-reads both lists in the same session (a finished view keeps its session for that).
Both lists are polled, so the fake keeps them out of transcripts and `reports` swaps their fixture

`GET /api/v1/download/` aggregates tasks from all configured downloaders. Deletion must preserve each task's
`downloader` and send `DELETE /api/v1/download/{hash}?name=<instance>`; omitting `name` selects the default
instance. It always deletes files too (`delete_file=True` in v3.1.0) and answers only `success`; qBittorrent
reports success even for an absent hash, so that response cannot correct a missing instance selector. Instance
names stay out of messages and buttons. Deleting does not undo a subscription: its `note` recorded the
episodes as obtained when the torrent was added, and nothing in MoviePilot's core listens for the deletion, so
the subscription never fetches them again

### 暂停与恢复（v3.1.4 核验）

2026-10-12 对照 MP 最新稳定版 `v3.1.4`（部署同版本）源码：`app/api/endpoints/download.py` 的 `update_task`、
`app/application/download/tasks.py` 的 `DownloadTaskMutationService`、`app/schemas/download.py` 及
`app/modules/{qbittorrent,transmission}`。`PATCH /api/v1/download/{hash}` 认证为 `get_current_active_user`（API key
即超级用户），请求体只发 `{"action": "stop"|"start", "downloader": <实例>}`。hash 须为 40 位十六进制，否则
`success=false`；省略 `downloader` 时 MP 自行按 hash 查找实例，Crychic 始终显式传任务自己的实例，实例缺失或同
hash 多实例时不提供控制。响应 `data` 为 `{hash, downloader, results: [{operation, success, message}]}`，
`success` 是逐项结果的合取；空 `results` 在上游也算成功，所以客户端要求恰好一项、`operation` 与请求一致且
hash 和实例匹配，否则按结果未知处理。

下载器对不存在的 hash 也报告成功（qBittorrent `torrents_pause/resume` 无异常即真），未配置或停用的实例使该项
失败。因此确认前和写入前都重读 `GET /api/v1/download/`，写后只给只读「查看任务」。qBittorrent 的
`downloading` 过滤包含 `StoppedDownloading`（qBittorrent `TorrentImpl::isDownloading`），暂停的未完成任务仍以
`state=paused` 留在列表中，可以恢复；Transmission 的列表只取 `downloading`/`download_pending`，暂停后任务离开
列表，恢复需到 MP 或下载器中进行。2026-10-12 只读查询 `/api/v1/download/clients`，目标部署只有一个 qBittorrent。

写请求复用禁止重定向、无 Cookie jar 的 client，不回显响应正文。HTTP 200 的 `success=false`、400/404/405/409/
422/429 投影为固定拒绝文案；401/403 为 API key 拒绝；其他状态、超时、解码失败、缺 `success` 或结果不匹配都
按未知结果处理，绝不自动再次 PATCH。旧的 `GET /download/start|stop/{hash}` 会改状态却是 GET，不使用。

## 订阅默认项与执行状态

`GET /api/v1/subscribe/` lists all subscriptions for the superuser key (`state` N/R/P/S, `lack_episode`,
`total_episode`); Crychic filters it to movie/TV records, never treating music or unknown types as movies.
`Backend.FindSubscription` retains both ID and state, so an existing paused subscription card says 目前已暂停;
`DELETE /api/v1/subscribe/{id}` answers 404 for one already gone, which `Unsubscribe` treats as done.
Cancelling a subscription stops future subscription work; do not describe it as deleting an existing torrent;
the bot's other live writes are subscribing (and resubscribing), pausing/resuming a subscription, downloading
a picked release and deleting a download

Subscription details show what applies where a subscription sets nothing: no quality/resolution/effect is 不限,
and no `filter_groups` falls back to the global `SubscribeFilterRuleGroups` setting for ordinary subscriptions,
or `BestVersionFilterRuleGroups` for upgrades (`GET /api/v1/system/setting/{name}`;
the per-kind default subscription configs only seed new
subscriptions). MoviePilot schedules no search per subscription (`execution_status.next_run_at` is null after
a run), so 自动查找 reads its scheduler (`GET /api/v1/dashboard/schedule`, the WebUI's 后台服务): `subscribe_refresh`
browses the sites' newest releases (`SUBSCRIBE_MODE`, every `SUBSCRIBE_RSS_INTERVAL` minutes) and
`subscribe_search` only exists while `SUBSCRIBE_SEARCH` is on; `next_run` is Chinese text such as 19分钟, status
等待 or 正在运行. Both reads are cosmetic, inside the details' budget (`flow.SubscriptionUpkeep`); `schedule.json`
and `subscribe_filter_groups.json` are trimmed live recordings

Only the subscription list attaches `execution_status`; `GET /api/v1/subscribe/{id}` does not, and Crychic does not read it. `last_search`
and execution `next_run_at` are UTC timestamps, displayed in China time; missing/unrecognized timestamps and
states stay unknown. MoviePilot v3.1.0's execution-error projection only flattens/truncates text, so it can
still contain paths or credentials: `moviepilot` passes only `HasError` to flow, never the original error.
`subscriptions_details.json`, `downloads_subscription.json`, `queue_subscription.json` and
`library_subscription.json` are synthetic v3.1.0-shaped e2e fixtures, including an intentionally
private-looking error to prove it is not shown

订阅身份核验与只写一次见 [会话参考](flow.md#订阅列表详情与身份)。`start_episode` 的范围及下一集条件见 [季与起始集](flow.md#季与起始集)。

## v3.1.1 订阅条件更新

`PUT /api/v1/subscribe/` 接受 `id` 和明确改动的公共字段。端点调用 `to_public_write_payload(exclude_unset=True)`；未提交的字段不写，空字符串或显式 null 可清空 `quality/resolution/effect/include/exclude`。Crychic 使用显式 null，集合清空使用 `[]`。成功为 `success: true, data: null`，不能要求返回订阅对象。[端点](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/subscribe.py#L299-L379) · [模型及排除字段](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/schemas/subscribe.py#L129-L358)

公共接口排除 `note`、`lack_episode`、`state`、`current_priority`、`episode_priority`、`date`、`username` 等运行事实。Crychic 不提交这些字段，也不通过改总集数或调用 reset 来调整起始集／洗版。关闭洗版仅改 `best_version`，保留暂时不生效的 `best_version_full`；电影不修改这个电视剧字段。[写服务](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/application/subscription/mutation.py)

规则读取使用 `GET /rule/groups?include_usage=false` 的 `data.rule_groups`，以及 `/rule/builtin`、`/rule/custom?include_group_refs=false` 的 `data.rules`。只投影唯一名称、类型、分类和可用性，内存中计算规则组及其引用规则的摘要以核对并发修改；不向 flow 传递规则正文或使用者信息。缺少集合、未知引用、重名和错误语法不可当成可信可选组。[目录投影](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/application/filtering.py#L187-L225) · [查询](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/application/filtering.py#L423-L488)

规格匹配值来自固定版本内置规则；自定义包含／排除仅生成转义后的字面关键词表达式，不运行用户正则。MoviePilot 对资源的标题、描述、标签匹配包含／排除，对标题匹配规格；空规则组按普通／洗版选择不同全局组。[匹配](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/application/torrent/download.py#L740-L783) · [内置规则](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/application/rules.py#L510-L606) · [全局组选择](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/chain/subscribe/match.py#L732-L745)

更新使用 API key，无资源 Cookie；复用禁止重定向的 client 与共享 transport，防止 307/308 重放和认证外送。拒绝信息固定投影，不展示写请求或响应原文；未知 HTTP 状态、408、解码错误或缺少 `success` 均按结果未知处理。成功后只读核对，绝不自动再次 PUT。当前模块声明 Go 1.25，验证工具链为 1.26.0；Context7 虽指定 Go 1.25，仍给出主线 JSON v2 片段，不能用其 `omitempty` 语义解释本项目 JSON v1，已另外读取安装工具链的 `encoding/json` 与 `net/http` 源码。

## 单订阅搜索与执行批次

`POST /api/v1/subscribe/search/{subscribe_id}?type={电影|电视剧}` 只提交一个既有订阅，无请求体。使用 API key，不需要资源 Cookie。`data` 包含 `batch_id`、`batch_ids`、`target_count`、`queued_count`、`ongoing_count` 和 `single`；HTTP 成功或有批次 ID 都不足以证明本次新建了批次。

`subsearch.go` 仅在目标数与新建数均为 1、合并数为 0、`single=true`、唯一批次 ID 一致时向 flow 返回可跟踪身份。缺失／null 计数不按零处理。新建数为 0、合并数为 1 表示已有任务，客户端不向 flow 暴露其批次 ID。上游合并可能提升排队任务优先级或清除其补查游标，但仍保留原任务与批次身份；不把它当作机器人新建的搜索。

`GET /api/v1/subscribe/execution/batches/{batch_id}` 返回统一包裹的批次投影，不包含完整任务列表。只读取安全字段，原始执行错误仅投影为 `HasError`。总数、已处理数、完成数、失败数和停止数必须存在；跳过数沿用上游模型的零默认值，所有计数仍须相互一致。终态必须处理完全部目标，活动状态不能已全部处理。未知枚举显示未知，不提供停止入口。flow 进一步核对本地新建回执、`manual` 来源、单订阅范围、当前订阅 ID 与创建时间；终态的当前订阅 ID 为空是正常情况。字段要求见[批次响应模型](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/schemas/subscribe.py#L96-L116)。

`PUT /api/v1/subscribe/execution/batches/{batch_id}/cancel` 无请求体，成功只返回 `success=true, data=null`。排队任务可以立即停止，运行中的任务先标记正在停止；已越过下载提交边界的结果不会撤销。手动搜索不要求订阅处于自动调度状态，暂停订阅也不会取消已接纳任务。不得用全局批次列表或订阅最近一次执行 ID 建立聊天请求者的控制权。

三个端点均复用共享传输，但拒绝全部重定向且不共享 Cookie jar，错误不带 URL、响应原文或站点信息。搜索和停止不自动重试；408、跳转、缺少成功标记及半响应属于未知写入。批次 ID 只允许有界的 ASCII 字母、数字、下划线和连字符，不拼接不可信路径。

关键依据为 `v3.1.1` 的[提交端点](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/submaintenance.py#L184-L217)、[入队装配](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/dependencies/subscription.py#L138-L177)、[合并实现](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/db/oper/subscriptionsearch.py#L29-L145)、[批次投影](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/application/subscription/status.py#L179-L220)、[停止端点](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/subexecution.py#L88-L117)与[提交后的结果处理](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/application/subscription/observability.py#L105-L156)。Context7 返回的是 `v3` 主线资料，接口细节以上述固定源码及本地 OpenAPI 为准。

2026-10-10 另对照 `v3.1.3`，提交、读取、取消端点及状态投影、手动入队装配与 `v3.1.1` 相同。该版本增加分页检查点清理、站点请求间隔，以及增量搜索完成标记参与收尾；客户端只展示实际返回的状态与计数，不推断已提交下载的终态，也不按版本分支。见[队列实现](https://github.com/jxxghp/MoviePilot/blob/v3.1.3/app/db/oper/subscriptionsearch.py)和[执行收尾](https://github.com/jxxghp/MoviePilot/blob/v3.1.3/app/application/subscription/observability.py#L105-L158)。

Go Context7 查询虽指定 1.25，HTTP 片段仍引用主线，故另核对[Go 1.25 请求重放判定](https://github.com/golang/go/blob/go1.25.0/src/net/http/request.go#L1534-L1547)、[传输重试](https://github.com/golang/go/blob/go1.25.0/src/net/http/transport.go#L808-L860)及本机 1.26 工具链源码。没有幂等键的 POST／PUT 不会在已发请求的模糊失败后按可重放请求处理；尚未写出请求的安全重连与应用重复提交不是同一件事。

## 超时与敏感错误

MoviePilot calls are bounded per call (`moviepilot.callTimeout`, 30 s; resource search `torrentSearchTimeout`,
2 min), so its `http.Client` sets no `Timeout`. A call whose body carries site credentials (`call.secret`, the
download post) never quotes MoviePilot's error answer, which can echo the request.

上游拒绝原样转述只适用于安全的 `flow.UserError`；`call.secret` 和执行错误的原文可能带请求载荷、路径或凭据，安全过滤优先。

## 关联接口

推荐、日历与合集的响应限制见 [发现参考](discovery.md)。Crychic 自 2026-10-11 起不再读取整理历史（`/api/v1/history/transfer`，含失败记录）或单个订阅（`GET /api/v1/subscribe/{id}`）；这两类读取只服务已删除的本地请求归属。订阅、下载和整理列表的 `data: null` 视为来源未读到，不当作成功空列表。历史契约仍可参考 [历史端点](https://github.com/jxxghp/MoviePilot/blob/v3.1.3/app/api/endpoints/history.py#L512-L542)。

When MoviePilot behavior is unclear, read its source (`app/api/endpoints/{media,subscribe}.py`) instead of
guessing

原资料的上游路径还包括 `app/api/response.py`（统一响应封装）与
`chain/subscribe/completion.py`（订阅取得集数）。版本变更时核对对应源码，不在真实服务上制造订阅或下载来验证。

## 原生自定义通知接入

原生通知使用 `/api/v1/plugin/Crychic/status`、`notifications` 和 `ack`，复用 API key，不发资源 Cookie、不跟随重定向、不回显错误响应。独立超时 35 秒覆盖 25 秒长轮询，每批最多 32 条；协议缺失或范围不完整不推进。ACK 只写插件消费进度，不代表平台发送成功。

本次匹配最新 MP v3.1.4 的自定义渠道、公开插件 SDK、自有数据库与窄范围 `_MessageBase`/durable 事件身份适配。Context7 主线资料仅作导航，实际契约用固定版本源码及隔离的官方镜像端到端验证。源码入口为 `app/sdk/plugin/base.py`、`app/sdk/database.py`、`app/modules/__init__.py`、`app/chain/_messaging.py` 和前端通知配置页面。见 [通知详情](notifications.md)。
