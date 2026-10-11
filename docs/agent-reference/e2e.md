# E2E、fake 与录制参考

改场景、fake、fixture、golden 或排查时钟和交互顺序时读取。编写规则见 [测试目录指南](../../e2e/AGENTS.md)。

本文从原根指南按主题迁出，保留当时的实现说明与上游版本背景，不代表已对当前线上服务重新核验。涉及目标版本变化时，按根指南核验来源。

## Harness 与可复验产物

`e2e/harness_test.go` - `start`, `say`, `answer`, `answerQuoting`, `chatter`, `tap`, `tapData`, `shows`,
`arrives`, `transfers`, `reports`, `restart`

`e2e/testdata/transcripts/*.txt` - golden transcripts, one per scenario

`h.tr.verify(t)` 在普通运行中检查消息标题并比较已提交 transcript，只有 `-update` 才写 golden。未通过标题检查时即使更新了文件，测试仍失败；不能用更新跳过行为判断。

`transcript.verify` also fails any message the bot sends or edits whose content (posters aside) does not open
with a `<h3>` heading: every message, endings, expiries and empty states included, opens with a `Heading`
(`titled` adds one to a bare failure)

普通 bot send/edit 的标题检查不覆盖 `answerInlineQuery` 中经典 HTML article；inline 使用下面的独立协议检查。所有交互入口与 fake 均走真实 app 装配。

## Telegram 与 MoviePilot 协议仿真

The fake Telegram accepts `answerInlineQuery` (results must be an array of articles with a 1-64 byte id, title
and message text) and records every result with its text and buttons; `h.inline(user, query)` sends an
`inline_query` update and waits for the answer

`PUT /api/v1/subscribe/status/{id}` sets R/P/S for a subscription in the current list, which then lists it in
that state (`fake_substate_test.go`), else answers 订阅不存在. The fake MoviePilot answers the library and
downloader checks as an idle server (`idleServer`); scenarios override
`libraryShowPath`/`libraryMoviePath`/`downloadsPath`. `DELETE /api/v1/download/{hash}?name=...` removes only
that instance's task; omitting `name` selects the default qBittorrent, whose deletion succeeds even for an
absent hash. A route for the exact path overrides that

Fakes must behave like the real services (long-poll `getUpdates`, `X-API-KEY` check, real error shapes);
extend them, don't shortcut them

For ordinary bot messages, the fake Telegram accepts only `sendRichMessage` and rich `editMessageText`, refuses rich HTML with
undocumented tags or unbalanced markup, and records each message's HTML a block or `<br>` per transcript line.
`h.tg.refuseImage(url)` fails media blocks with that URL; `h.tg.seedPhoto` leaves a pre-rich-message photo
menu in a chat

The fake numbers subscriptions created from `subscribe_created.json` as consecutive positive integers; unlike an ever-increasing
counter, MoviePilot's SQLite row ids can be reused after deletion, which `subidentity_test.go` models by
replacing the listed record and `notify_identity_test.go` by returning the reused id to a new request. Reads
by subscription id return the created/found identity (empty when absent); exact routes can model completion or
replacement. The fake likewise answers each added download with its own hash (`addedHash`, then
`addedHashN(n)`)

## 消息顺序与图像身份

`h.tg.holdNextResponse(method)` delays a response after the fake displays its message; release is idempotent
and cancellation ends the wait. `TestSubscribeMovie` presses a result before releasing the initial send, so
callbacks must work before the sender learns poster file_ids; the transcript records this delivery ordering
without requiring one poster transport representation.

`loseNextResponse` stores a displayed message but returns an incomplete HTTP body, distinguishing unknown
send results from explicit media refusals. `rejectNextEdit` can return Telegram's 429 with `retry_after`;
delivery scenarios verify the deadline and retained file_id, as well as successful-receipt recovery without
another MoviePilot write. Late initial responses and whitelist revocation are exercised at the app boundary.

`fakeMoviePilot.holdCalls` blocks selected external API requests before processing, with explicit entered,
release and exited barriers. Batch scenarios use it to observe each submission, stop before the next POST,
and terminate/restart during an in-flight write. No internal flow state is mocked; every normal run compares
the recorded progress and terminal receipt, including mixed success/refusal/unknown outcomes.

`fakeImages` is Douban's image host: 418 without the Douban referer, 403 to Go's default User-Agent, a tiny
JPEG per path otherwise, `h.img.setMissing(path)` answers 404; `app.Deps.Images` routes poster downloads to
it. The fake Telegram hands out a file_id per upload, returns the top photo or slideshow with file_ids,
accepts known file_ids only, and records each displayed image by its complete stable identity via
`message.seen()` (URL hash or uploaded-byte SHA-256), not its URL/upload/file_id transport; the cache recovery
scenario separately asserts actual file_id reuse

## 轮询日期与通知假服务

The fake media server shows every transfer on top of the scenario's library fixture; `scenario.lagging` +
`h.catchUp` model a server that has not scanned yet. Library and latest reads are polled, so they stay out of
transcripts

The fake answers `recognize` per file name (`scenario.recognized`, others get `recognize_none.json` like
live); fake transfers carry `Src`/`Hash` only where a scenario needs qualities. History is filtered by status
and paged by `Date`, with no assumed ID order between completion times. `failedTransfer`/`retryTransfer` model
a failed row becoming successful without changing its ID; failed records are not visible in the fake media
library. Only page-one reads advance the harness's full-poll barrier.

The E2E quiet default is `300ms`; `transfers` waits until a whole poll handled the records, so a scenario can
act between polls. Polling calls stay out of transcripts while their emitted notices remain recorded.

身份复用场景分别通过替换列出的订阅和向新请求返回复用 row ID 建模，不得以只增不减的假 ID 排除这个生产失败方式。

## 并行发现读取

Fake routes are keyed by path, so a scenario needing two searches swaps `searchPath` with `h.mp.setRoute`, or
answers per title with `scenario.searches`. Reads the bot makes at once (calendar lookups) arrive in any
order, so the transcript sorts each run of consecutive reads of one route (`transcript.settle`; media searches
and details count as one route, searches first; the calendar's TMDB season reads count as one route too)

The fake answers collection searches per title (`collectionSearch(title)`, `[]` otherwise, like MoviePilot),
and the transcript sorts them like other simultaneous reads

Calendar annotations first finish their parallel MoviePilot searches/Bangumi-detail reads, then read direct
Bangumi only for missing synopses. Transcript normalization keeps these as separate read groups, preserving
the provider fallback order while sorting only concurrent requests within a group. Availability scenarios
cover successful MP overview, explicit failure fallback, the shared five-second deadline, and later retry;
task-list scenarios independently fail/recover downloader and queue responses without any write API call.

## Bangumi 录制与时钟

`e2e/testdata/bangumi/calendar.json` is a trimmed recording of `api.bgm.tv/calendar` (all seven weekday
groups, a few shows, only fields Crychic reads); `subject_<id>.json` record `/v0/subjects/<id>` the same way
and `not_found.json` is Bangumi's live 404. `h.bgm.setDown(id, true)` makes a subject answer 503. Scenarios
start on `epoch`, Monday 2026-10-05 noon China time, and the clock runs at real speed from there
(`app.Deps.Now`), so "today" is stable; `h.advance` adds an atomic clock offset for expiry scenarios without
changing real timer cadence

## MoviePilot 录制来源与脱敏

`subsearch*_test.go` 通过真实订阅详情和确认入口验证单订阅搜索、条件变化后的重新确认、只读自动跟踪、批次归属与计数、停止副作用边界、未知写入、投递恢复、话题和重启。`fake_subsearch_test.go` 按官方 `v3.1.1` 契约合成入队、旧批次合并及排队／运行中取消，未访问真实站点或录制真实批次。写入之后响应丢失和应用退出场景会先在 fake 中实际创建批次，不把未执行的请求冒充未知结果。

重复批次读取像下载与整理轮询一样不记入 transcript；fake 独立统计实际 HTTP 请求，产物保留用户操作、全部写入及显示状态。敏感错误、路径和站点名仅用于证明不泄露，不进入产物。终态与计数矛盾、缺少创建时间的订阅均不得获得确认或停止权限；既有批次正在取消时仍属于原任务，不能被 fake 误作新建。

自动跟踪场景用现有 `holdCalls` 入站屏障挂起批次 GET，再点击当前可见的「停止本次搜索」。它必须进入确认，不能因本地取消 Follow 读取而变成陈旧按钮；返回后仍可继续刷新，搜索只提交一次且没有停止写入。该扩展在修复前确因未进入确认而失败，未增加内部测试入口。

`torrentplan*_test.go` 的 7 个真实 app 场景覆盖自动缺集建议的可信身份与不连续范围、重复/重叠/未知大小/H&R、筛选全集及排序稳定性、失败和 null 库读取恢复、确认前库存与总集数变化、SSE 完整终态、起始集与 20 项/100 集边界、他人/话题隔离及重启旧确认。沿用真实批量 worker 的混合回执，不复制已有停止和退出场景。范围、季信息及候选均为合成协议资料，不录制真实资源搜索。

`fake_library_test.go` 仅在成功响应具有有效非 null 季字典和集数数组时叠加已扫描整理文件；整体或单季 null 保留原响应，不能让 fake 把未知库重写为空库。测试在实现入口前因缺按钮失败；起始集扩展在修复前因仍从 E01 计算失败。产物均为固定脱敏 transcript，普通运行逐字比对。

`myrequests*_test.go` 覆盖请求归属、筛选与稳定编号、下载实例关联、安全失败阶段、部分读取和页数上限、完成历史与重启。fake 按 `status=false` 独立注入失败页，不推进通知的成功历史轮询屏障；没有录制真实请求或错误。合成字段同时对照 `v3.1.1` 与 `v3.1.3` 的相同影视读取契约，不按版本号复制同一套测试。

共享预算场景用 HTTP 入站屏障确认下载和媒体库在慢订阅读取结束前已开始，实际等待生产五秒预算，只保留成功来源。transcript 只归一化同一批相邻的并行订阅和失败历史读取，不跨用户动作或写操作排序。已有下载整包与洗版到货场景扩展验证完成历史，不再复制其通知过程；每个场景仍以固定产物比对结束。

`subedit*_test.go` 经真实 `/subscribe` 详情和输入流程验证条件草稿、最小 PUT、null/空集合、并发修改、目录解析与规则摘要、旧编号/票据、401/403、拒绝/408/半响应/缺失成功标记/重定向、投递重试、话题、过期、重启及编辑后的到货。`fake_subedit_test.go` 按 v3.1.1 更新模型只应用明确字段，不重写未提交设置；允许“已提交但丢响应”和“已保存但后续读取不可用”。`subscribe_upgrade_groups.json` 为合成的洗版全局规则配置。

`holdSubscriptionRead` 在 fake 已读取并序列化旧订阅后阻塞 HTTP 响应，与通用 `holdCalls` 的请求入站屏障不同。它验证本地编辑完成以后，陈旧的通知读取不会覆盖起始集和洗版模式，也不会错误推进整理游标。固定 transcript 记录用户可见结果，目录定义、后台路径和敏感旧条件不进入产物。

`subedit_ux_test.go` 通过真实聊天入口覆盖退出草稿保护、继续输入与放弃后订阅控制、单字段恢复、只读逐字段核对、后台冲突保留与采用后台、处理期间再次变更、最新起始集上限。每个场景保留不重复 PUT、陈旧按钮无效和敏感文本不泄露的断言。既有目录变更场景扩展为保留其他草稿并完成重新选择；条件 PUT 计数按完整路由行计算，不能把暂停路由误算为条件更新。

`fake_search_stream_test.go` 的 SSE 来自合成数据，帧形状对照 MoviePilot v3.1.1 `search.py` 与 `chain/search/execution.py`，早期候选明确没有媒体身份。复用既有合成种子记录，不录制真实流。HTTP admission、逐帧 flush 和连接退出屏障约束可重复时序；不排序有副作用的搜索请求来掩盖乱序。Cookie 仅在 fake 内校验，其他服务和普通 JSON 路径收到 Cookie 会使场景失败。

`search_stream*_test.go`、`search_sites_test.go` 与 `cookie_config_test.go` 覆盖资源身份、完整帧和批次、读取总预算、断线不重搜、终态投递恢复、初始消息和输入、话题、到期和重启、搜索范围及启动拒绝。总预算场景实际等待两分钟，因此完整测试相应变长；不通过缩短生产预算或注入内部 mock 加速。每个场景仍以 transcript 比对结束。

Telegram fake 的消息可见状态和对应 transcript 在同一锁内发布，避免测试已经读到新消息、记录却晚于后续取消的假乱序。发送响应仍可单独延迟，保留「消息已显示但响应尚未到达」的协议行为。

Recordings of `/api/v1/download/` carry tracker URLs with passkeys, the site name, the release name and the
owner's username: keep only the fields Crychic reads

`e2e/testdata/moviepilot/*.json` are trimmed recordings from a live v3.1.0 instance (`curl -H "X-API-KEY:
$KEY" "$MP/api/v1/media/search?title=沙丘&type=media&count=10"`), except `subscribe_rejected.json`,
`server_error.json`, `library_movie_held.json` (the live library holds no movie yet), `subscriptions.json`,
`subscribe_deleted.json`, `subscribe_gone.json` (no live subscription may be created to record them;
`subscriptions.json` copies the shape of recorded subscription history); `latest.json` is a live recording
with the owner's Emby domain replaced by `emby.example.com`; `subscribe_created.json` came from a one-off live
subscription that must not be repeated (see above); `torrents_*.json`, `download_*.json`,
`downloads_dune_mygo.json` are synthetic, shaped after v3.1.0's `Context.to_dict()` and download endpoint,
with a fake site cookie and passkey the fake checks come back in `POST /api/v1/download/` (recorded as a
summary) and scenarios check never reach a chat; never commit real keys or tokens

`subscriptions_details.json`、`downloads_subscription.json`、`queue_subscription.json` 和
`library_subscription.json` 是 v3.1.0 形状的合成资料，包含看似私密的执行错误，用于验证原文不会到达聊天。`schedule.json` 和
`subscribe_filter_groups.json` 是裁剪后的真实录制。

真实服务只允许根指南限定的只读元数据；不要为了刷新既有创建订阅录制而再次产生真实订阅。示例里的合成 cookie/passkey 是协议校验数据，真实值不得记录。

## 原生通知与插件宿主

原生通知 HTTP fake 独立于转移历史，h.transfers 不生成通知；h.nativeNotice 明确模拟 MP 的投递记录。旧 h.arrives 仅用于既有媒体交互场景，同时明确产生独立 MP 通知，不验证已退役的本地通知规则。基线 ACK 是启动屏障。

主场景验证富文本、明确目标和话题、白名单、未知发送、重启、坏批次及只有事实无通知；每个场景结束比对 transcript。`run_mp_plugin_e2e.sh` 在 `--network none` 的官方 MP v3.1.4 镜像中，用合成临时状态启动真实宿主和插件，以认证 HTTP 读写插件状态。验证 native post_message 路由、类型开关、发送时段、durable 身份、持久缓冲及不启动 Telegram 客户端。普通运行比对 MPPluginNativeHost.txt，只有 --update 才更新。
