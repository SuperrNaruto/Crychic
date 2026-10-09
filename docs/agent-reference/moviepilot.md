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
- 资源搜索、系统进度与实时消息 SSE 只声明 `MoviePilot` Cookie 认证；现有代码仍使用 `X-API-KEY` JSON API。
  2026-10-09 主人已允许增加 Cookie 认证，并明确不接入 Agent；后续范围见
  [MoviePilot API 功能改进计划](../plans/moviepilot-api-improvements.md)。计划首版采用可选资源 Cookie，
  不增加自动登录；Cookie 与 SSE 尚未实现。此决定不改变真实服务禁写、禁 PT 搜索及凭据隐私边界。
- `DownloadAddedData.requires_confirmation` 是未识别资源入口的提示；当前已有媒体的 `/download/` 路径不
  自动改走 `/download/add`，也不因缺少 download ID 重试提交。
- Bangumi 简介使用 `GET /api/v1/bangumi/{bangumiid}` 的 `data.overview`；空对象、null 或空简介不算有效
  注释，允许在整页剩余预算内使用原公共 API 回退。没有完整星期分组契约的 MP 放送列表不替换公共日历。

源码依据：[download.py](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/download.py)、
[transfer.py](https://github.com/jxxghp/MoviePilot/blob/v3.1.1/app/api/endpoints/transfer.py)。

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

## 下载进度与任务删除

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

## 订阅默认项与执行状态

`GET /api/v1/subscribe/` lists all subscriptions for the superuser key (`state` N/R/P/S, `lack_episode`,
`total_episode`); Crychic filters it to movie/TV records, never treating music or unknown types as movies.
`Backend.FindSubscription` retains both ID and state, so an existing paused subscription card says 目前已暂停;
`DELETE /api/v1/subscribe/{id}` answers 404 for one already gone, which `Unsubscribe` treats as done.
Cancelling a subscription stops future subscription work; do not describe it as deleting an existing torrent;
the bot's other live writes are subscribing (and resubscribing), pausing/resuming a subscription, downloading
a picked release and deleting a download

Subscription details show what applies where a subscription sets nothing: no quality/resolution/effect is 不限,
and no `filter_groups` falls back to the global `SubscribeFilterRuleGroups` setting (`GET
/api/v1/system/setting/SubscribeFilterRuleGroups`; the per-kind default subscription configs only seed new
subscriptions). MoviePilot schedules no search per subscription (`execution_status.next_run_at` is null after
a run), so 自动查找 reads its scheduler (`GET /api/v1/dashboard/schedule`, the WebUI's 后台服务): `subscribe_refresh`
browses the sites' newest releases (`SUBSCRIBE_MODE`, every `SUBSCRIBE_RSS_INTERVAL` minutes) and
`subscribe_search` only exists while `SUBSCRIBE_SEARCH` is on; `next_run` is Chinese text such as 19分钟, status
等待 or 正在运行. Both reads are cosmetic, inside the details' budget (`flow.SubscriptionUpkeep`); `schedule.json`
and `subscribe_filter_groups.json` are trimmed live recordings

Only the subscription list attaches `execution_status`; `GET /api/v1/subscribe/{id}` does not. `last_search`
and execution `next_run_at` are UTC timestamps, displayed in China time; missing/unrecognized timestamps and
states stay unknown. MoviePilot v3.1.0's execution-error projection only flattens/truncates text, so it can
still contain paths or credentials: `moviepilot` passes only `HasError` to flow, never the original error.
`subscriptions_details.json`, `downloads_subscription.json`, `queue_subscription.json` and
`library_subscription.json` are synthetic v3.1.0-shaped e2e fixtures, including an intentionally
private-looking error to prove it is not shown

订阅身份核验与只写一次见 [会话参考](flow.md#订阅列表详情与身份)。`start_episode` 的范围及下一集条件见 [季与起始集](flow.md#季与起始集)。

## 超时与敏感错误

MoviePilot calls are bounded per call (`moviepilot.callTimeout`, 30 s; resource search `torrentSearchTimeout`,
2 min), so its `http.Client` sets no `Timeout`. A call whose body carries site credentials (`call.secret`, the
download post) never quotes MoviePilot's error answer, which can echo the request.

上游拒绝原样转述只适用于安全的 `flow.UserError`；`call.secret` 和执行错误的原文可能带请求载荷、路径或凭据，安全过滤优先。

## 关联接口与转移历史

推荐、日历与合集的响应限制见 [发现参考](discovery.md)。成功转移的按秒游标、失败记录重试、初始化和迁移见 [通知参考](notifications.md#完成日期游标与迁移)。

When MoviePilot behavior is unclear, read its source (`app/api/endpoints/{media,subscribe}.py`) instead of
guessing

原资料的上游路径还包括 `app/api/response.py`（统一响应封装）与
`chain/subscribe/completion.py`（订阅取得集数）。版本变更时核对对应源码，不在真实服务上制造订阅或下载来验证。
