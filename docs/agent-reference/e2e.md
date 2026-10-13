# E2E、fake 与录制参考

改场景、fake、fixture、golden 或排查时钟和交互顺序时读取。编写规则见 [测试目录指南](../../e2e/AGENTS.md)。

本文从原根指南按主题迁出，保留当时的实现说明与上游版本背景，不代表已对当前线上服务重新核验。涉及目标版本变化时，按根指南核验来源。

## Harness 与可复验产物

`e2e/harness_test.go` - `start`, `say`, `answer`, `answerQuoting`, `chatter`, `tap`, `tapData`, `shows`,
`arrives`, `transfers`, `reports`, `restart`

`e2e/testdata/transcripts/*.txt` - golden transcripts, one per scenario（按钮渲染为 `[标签 | 数据]`，带颜色的按钮追加 ` | primary` 或 ` | danger`）

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
without requiring one poster transport representation. Scenarios default `answerLead` to `orderedAnswers`
(longer than any action wait), so a fake edit always lands before its press is answered and transcripts keep
that order; `TestSlowEditAnswersAlongside` sets a 1ms lead and `holdNextRequest` keeps the edit from being
applied, so the press must be answered before the edit shows.

`sendChatAction` is accepted by the fake (chat must exist, a topic must belong to a forum, only `typing`) and
recorded as `<< sendChatAction chat=N [topic=T] typing`. Scenarios disable the typing action by default
(`noTyping`) so `-race` timing never adds lines; `scenario.typingLead` enables it, and `TestSlowSearchShowsTyping`
holds the title search (`holdCalls`), waits on `expect("action:<chat>")`, then releases it, so the transcript
shows the action before the reply in a private chat and a forum topic.

`loseNextResponse` stores a displayed message but returns an incomplete HTTP body, distinguishing unknown
send results from explicit media refusals. `rejectNextEdit` can return Telegram's 429 with `retry_after`;
delivery scenarios verify the deadline and retained file_id, as well as successful-receipt recovery without
another MoviePilot write. Late initial responses and whitelist revocation are exercised at the app boundary.

`fakeMoviePilot.holdCalls` blocks selected external API requests before processing, with explicit entered,
release and exited barriers. Batch scenarios use it to observe each submission, stop before the next POST,
and terminate/restart during an in-flight write. No internal flow state is mocked; every normal run compares
the recorded progress and terminal receipt, including mixed success/refusal/unknown outcomes.

`fakeImages` is Douban's image host: 418 without the Douban referer, 403 to Go's default User-Agent, a tiny
JPEG per path otherwise, `h.img.setMissing(path)` answers 404; `h.img.downloads(path)` counts requests, `firstRequest(path)` is a barrier closed at a path's first request, `hold(path)` keeps answering until released or the client disconnects (`TestChartPostersDownloadAheadAndCardKeepsThePoster` uses them: background prefetch is asserted through counters, never recorded in transcripts, and a restart with a held download must finish quickly; `search_abyss_douban.json` is a synthetic Douban-sourced search result shaped after a read-only look at MoviePilot's title search); `app.Deps.Images` routes poster downloads to
it. The fake Telegram hands out a file_id per upload, returns the top photo with its file_id,
accepts known file_ids only, and records each displayed image by its complete stable identity via
`message.seen()` (URL hash or uploaded-byte SHA-256), not its URL/upload/file_id transport; the cache recovery
scenario separately asserts actual file_id reuse

## 整理记录与媒体库假服务

The fake media server shows every transfer added with `h.transfers` on top of the scenario's library fixture;
nothing in Crychic reads the transfer history itself, so `transfers` does not wait for any poll. Library and
latest reads are polled, so they stay out of transcripts. Notices come only from `h.nativeNotice` (and
`h.arrives`, which adds transfers and then a native notice).

身份复用场景分别通过替换列出的订阅和向新请求返回复用 row ID 建模，不得以只增不减的假 ID 排除这个生产失败方式。

## 并行发现读取

Fake routes are keyed by path, so a scenario needing two searches swaps `searchPath` with `h.mp.setRoute`, or
answers per title with `scenario.searches`. Reads the bot makes at once (calendar lookups) arrive in any
order, so the transcript sorts each run of consecutive reads of one route (`transcript.settle`; media searches
and details count as one route, searches first, and so does the subscription list read beside them for
subscribed marks, last; the calendar's TMDB season reads count as one route too)

The fake answers collection searches per title (`collectionSearch(title)`, `[]` otherwise, like MoviePilot),
and the transcript sorts them like other simultaneous reads

Calendar annotations first finish their parallel MoviePilot searches/Bangumi-detail reads, then read direct
Bangumi only for missing synopses. Transcript normalization keeps these as separate read groups, preserving
the provider fallback order while sorting only concurrent requests within a group. Availability scenarios
cover successful MP overview, explicit failure fallback, the shared five-second deadline, and later retry;
task-list scenarios independently fail/recover downloader and queue responses without any write API call.

## 日历注释场景

`scenario.noteGrace` (harness default `actionTimeout`, like `subsMarksGrace`, so normal calendar pages are complete when shown)
and `scenario.calendarWarm` (off by default, as its lookups interleave with the transcript) are the only knobs.
`calendar_notes_test.go`: `TestCalendarPageFollowsItsNotes` holds MoviePilot's Bangumi details (`holdCalls`) under a
300ms grace and sets `progress` to an hour, so the page shows 简介补充中… with Follow and only the wake-up at the end of
the round redraws it (`h.tg.expect("edit:1")`), buttons unchanged; `TestCalendarRemembersNotes` shows a day again
without lookups and advances the clock past the miss TTL (new session, since sessions expire in 10 minutes) to see a
miss retried while a found synopsis is not; `TestCalendarWarmsTheWeek` hides other days' reads from the transcript
(`transcript.hideReads`, counted instead) and waits on the count of their direct Bangumi reads, then switches weekday
and checks no read was made; `TestCalendarCardDoesNotWaitOnSlowDetails` delays the card details beyond the 2s budget.

## 慢按下场景

`slow_press_test.go`: `scenario.answerDeadline` (empty keeps the 10s default) is set to 500ms in `TestSlowPressIsAnsweredBeforeItsReply`, which holds the card's details read
(`holdCalls`), waits for the press's answer (the `callback:` waiter, closed by `answerCallbackQuery`), checks the card is not shown yet, releases the call and
waits for `edit:1`; a following press on the same message waits for the slow handler's lock, so the answer count of two proves no second answer.

## Bangumi 条目类型场景

`bangumi_filter_test.go`: `TestTitleSearchKeepsBangumiAnimeOnly` uses the synthetic `search_sato_bangumi.json` (TMDB, Douban and three Bangumi
results shaped like the live 2026-10-13 answer) and synthetic `subject_{634708,312453,296610}.json` carrying `type`; a subject made unavailable
(`h.bgm.setDown`) stays listed, and the second search shows only that subject is asked again. The transcript sorts the concurrent subject reads
with the lookups group (last), so they stay ordered beside the search and the subscription list read.

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

`myrequests*_test.go` 与失败历史 fake 随 `/requests` 于 2026-10-11 删除；订阅、下载与整理进度由订阅详情和任务场景覆盖。

`seriesplan_test.go` 经 `/search` 卡片的同系列覆盖状态标注（订阅列表 503 时只信媒体库、未知不可勾选；恢复后已入库/已订阅/未获取）、勾选一次写入与旧确认票据。沙丘2 的订阅由 `subscriptions.json` 首条改写合成；`TestBrowseRecommendationsAndSeries` 与 `TestDownloadIntentStopsAtRelatedMedia` 补上空订阅列表路由，使原场景保持正常状态。

`subscribed_marks_test.go` 经真实 `/search` 入口验证搜索结果标注已订阅电影、已暂停电影与剧集已订阅/已暂停季，选季表的「订阅」列（剧集组订阅不标注标准季），以及订阅列表读取失败时结果与季表都不带标记。订阅行由 `subscriptions.json` 首条改写合成。其余搜索 transcript 因并行读取订阅列表多一行 `GET /api/v1/subscribe/`，未配置该路由的场景得到 404 并显示无标记。

`groups_test.go` 经真实 `/search` 入口按剧集组订阅：空组不列出、组内季号与集数、标准第 2 季订阅不被当作组内第 2 季、组内第 2 季订阅也不被当作标准第 2 季（`TestStandardSeasonIgnoresGroupSubscription`）、下载意图在选剧集组时停下且不发起资源搜索（`TestDownloadIntentStopsAtEpisodeGroup`）、标准分季功能不提供、POST 携带 `episode_group`。`search_kimetsu.json`、`detail_kimetsu.json`、`seasons_kimetsu.json`、`groups_kimetsu.json` 与 `group_seasons_kimetsu.json` 为 2026-10-12 只读录制并裁剪；标准第 2 季订阅由 `subscriptions.json` 首条改写合成。

`intent_test.go` 的 `TestDownloadIntentStopsAtPersonWorks` 从带下载意图的标题进入演职人员作品，选中作品只给订阅确认、不再发起资源搜索。`people_test.go` 与 `discoverfilter_test.go` 的 3 个场景经真实 `/search`、`/trending` 入口覆盖 TMDB 演员作品、Bangumi 配音演员（角色名、重复作品去重、对不上时说明）以及条件找片（逐项选择、TMDB 下界参数与日期、翻页去重与原编号、返回保留条件、换类型清空题材、无结果提示）。`credits_dune.json`、`person_works_tmdb.json`、`discover_anime_p1.json`、`credits_mygo_bangumi.json`、`person_works_bangumi.json`、`detail_mygo_bangumi.json` 与 `seasons_mygo_bangumi.json` 是 2026-10-12 从 v3.1.4 只读录制并裁剪的字段；`discover_anime_p2.json` 为真实第 2 页前插入第 1 页末条，模拟 TMDB 排名在翻页间变动；`search_mygo_bangumi.json` 把 `/api/v1/bangumi/428735` 的录制改成搜索结果形状，模拟搜索来源包含 Bangumi 的部署。fake 按 `page` 区分条件找片的后续页（`discoverPage`）。

`torrents_test.go` 的 `TestCombineReleaseFilters` 经 `/search` 卡片覆盖组合筛选：字段内多选、字段间相交、「未知」促销选项、按其他条件计算的选项计数、筛选后编号仍打开对应资源、另一字段留下的旧选项按钮被拒、清除全部条件；全程只有一次资源搜索请求。

`history_test.go` 的 `TestBrowseOlderHistory` 经 `/subscribe` 覆盖订阅历史翻页：无媒体身份的记录计数不列出且仍算满页、翻页间新完成的订阅把上一页末条推到下一页时不重复、短页结束、较早记录原样回传重新订阅、返回重新读取第 1 页。`TestResubscribeGroupSeasonBesideStandardOne` 用合成的剧集组历史记录与标准季订阅，确认按剧集组的季查重并在确认时说明按剧集组编排。`fake_history_test.go` 按 v3.1.4 语义把路由 fixture 当作该类完整历史，按 `page`/`count` offset 切页并给出 `X-Total-Count`；非 200 路由照常返回。`subscribe_history_tv_long.json` 在原录制 7 条后追加 5 条按同形状合成的较早记录（含一条媒体身份为空），`subscribe_history_tv_grown.json` 再在最前插入一条新完成记录。

`taskprogress_test.go` 的 2 个场景经真实 `/tasks` 跟随整理任务：插件路由缺失（404）时只显示状态，报告后仅对应运行文件显示百分比，他片记录与已完成文件不显示；同一身份的两个运行文件或已结束记录不显示。fake 的插件进度由 `nativeMP.setProgress` 设定，形状按 v1.2.0 插件投影合成，宿主侧形状由 `MPPluginNativeHost.txt` 核对。

`taskpause_test.go` 的 3 个场景经真实 `/tasks` 入口覆盖下载暂停与恢复：非默认实例上的分别确认、写后只读核对、旧确认票据不再写入；确认后目标已被他处暂停或已消失时不写，同 hash 多实例时撤下暂停、恢复和删除；HTTP 200 拒绝、503 与不确认所请求操作的成功响应。`fake_download_control_test.go` 按 v3.1.4 `update_task` 合成：未配置实例使该项失败，不存在的 hash 仍成功，`success` 为逐项合取，缺实例直接使场景失败；暂停状态反映到之后的下载列表。`download_control_empty.json` 是合成的空结果成功响应，未对真实下载器执行暂停或恢复。

`subedit*_test.go` 经真实 `/subscribe` 详情和输入流程验证条件草稿、最小 PUT、null/空集合、并发修改、目录解析与规则摘要、旧编号/票据、401/403、拒绝/408/半响应/缺失成功标记/重定向、投递重试、话题、过期、重启及编辑后的到货。`fake_subedit_test.go` 按 v3.1.1 更新模型只应用明确字段，不重写未提交设置；允许“已提交但丢响应”和“已保存但后续读取不可用”。`subscribe_upgrade_groups.json` 为合成的洗版全局规则配置。

`classification_policy.json` 是按 v3.1.4 分类策略形状合成的资料（含停用分类与音乐 `Album / Compilation`），不是真实录制；`fake_categories_test.go` 按源码语义校验创建与更新的 `media_category_id`（未知、停用、类型不符返回 HTTP 200 的 `订阅分类设置无效…`，空串清除 ID 与路径，有效 ID 写入路径快照）。`category_test.go` 的两个场景经 `/search` 选分类（含旧确认票据与旧选项代次）和 `/subscribe` 编辑旧路径分类后核对结果。

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
with the owner's Emby domain replaced by `emby.example.com`, and `latest_mixed.json` extends it with
synthetic TV and movie items in its shape; `subscribe_created.json` came from a one-off live
subscription that must not be repeated (see above); `torrents_*.json`, `download_*.json`,
`downloads_dune_mygo.json` are synthetic, shaped after v3.1.0's `Context.to_dict()` and download endpoint,
with a fake site cookie and passkey the fake checks come back in `POST /api/v1/download/` (recorded as a
summary) and scenarios check never reach a chat; never commit real keys or tokens

`subscriptions_details.json`、`downloads_subscription.json`、`queue_subscription.json` 和
`library_subscription.json` 是 v3.1.0 形状的合成资料，包含看似私密的执行错误，用于验证原文不会到达聊天。`schedule.json` 和
`subscribe_filter_groups.json` 是裁剪后的真实录制。

真实服务只允许根指南限定的只读元数据；不要为了刷新既有创建订阅录制而再次产生真实订阅。示例里的合成 cookie/passkey 是协议校验数据，真实值不得记录。

## 原生通知与插件宿主

原生通知 HTTP fake 独立于转移历史，h.transfers 不生成通知；h.nativeNotice 明确模拟 MP 的投递记录。h.arrives 仅用于既有媒体交互场景，同时明确产生独立 MP 通知。基线 ACK 是启动屏障。

主场景验证富文本、明确目标和话题、白名单、未知发送、重启、坏批次、整理失败路径隐藏（标签后整行、标点后路径，「国语/粤语/英语」保留）及只有事实无通知；每个场景结束比对 transcript。`run_mp_plugin_e2e.sh` 在 `--network none` 的官方 MP v3.1.4 镜像中，用合成临时状态启动真实宿主和插件，以认证 HTTP 读写插件状态。验证 native post_message 路由、类型开关、发送时段、durable 身份、持久缓冲、宿主渲染的整理失败路径在入队与旧队列读取时隐藏、33 条最长四字节正文按字节分批且每次响应都在 Go 的 2 MiB 上限内及不启动 Telegram 客户端。`search_stream_protocol_test.go` 的协议表另含 `replace` 与 `done` 都不给总数的情况，结果保持不完整。普通运行比对 MPPluginNativeHost.txt，只有 --update 才更新。
