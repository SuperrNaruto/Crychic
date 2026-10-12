# 会话、交互与文案参考

改会话、订阅、下载、导航或任意目录的用户可见内容时，读取相关小节。先读 [目录规则](../../internal/flow/AGENTS.md)。

本文从原根指南按主题迁出，保留当时的实现说明与上游版本背景，不代表已对当前线上服务重新核验。涉及目标版本变化时，按根指南核验来源。

## 界面与文案

Flow choice buttons only choose: no emoji, no titles or facts in choice labels. A notice may have a
`flow.Open` URL (e.g. 在 Emby 中观看), and inline articles have their separate deep-link URL button. Picks are number buttons
(`gridColumns` a row) or short names (第 N 季), with the details in the message text; menus share rows
(`menuColumns`)

Button rows run in one order: choices (numbers and their pager, seasons, 确认X on a row of its own), switches
(电视剧/电影, weekdays; the shown one marked `·X·` by `markShown`), onward actions (搜索资源/相似推荐/同系列 via `onward`,
订阅历史, 重新搜索 on its own row), and last the one navigation row

Navigation has one vocabulary. A request (search, subscribe, download, resubscribe) ends with 取消; a browsing
screen (lists, details, charts, task views) ends with `browseRow`: 首页 then 关闭 (`c`, generic). Every way back
is labelled 返回 (`labelBack`): navigate's history (`bk`, put in front of 取消/首页/关闭 by `withBack`) or `backTo` a
list read afresh (tasks `l`, subscriptions `j`, history `hf`, charts `b`); a screen never shows two.
Destructive confirmations are 确认X over 返回 alone

Data with columns (season picker, multi-season results, a task's files or progress) is a `flow.Table`; the
multi-season picker shows its ticks as `Ticked` lines

Numbered list entries are `flow.entry` (an `Item` block): bold name, then facts (original title, year, kind,
rating, source, state, progress) in italics below it (chart/related `Brief` entries put facts beside the name
instead; so do latest items and subscription lists, 8 a page, owner's choice 2026-10-12), rendered as `<li value="n">` so numbers match the buttons on every page. Each ordinary bot message
opens with a `Heading`; inline articles use their separate classic HTML format; list sections and calendar
dates are `Group`s, page numbers and refresh state are `Remark`s (footer), a media card's original title, year
/ kind / rating / source, genres / length and cast share one line of `Small` print (joined by ` · `) under its heading,
and a media card is separated from its question by a `Divider`

User-facing copy is `flow.Text` built with `Strong`/`Emphasis`/`Mono`/`Linked`/`Quote`; never put platform
markup in flow strings, adapters render and escape. Emoji sparingly, one scene emoji per ordinary heading, from
this table only:

| Emoji | Scene |
| --- | --- |
| 🔍 | search, results, 缺集方案, 停止本次搜索, 立即搜索订阅, filters, sites, empty results (🔍 呜…什么都没找到) |
| 🎬 / 📺 | a movie card / a TV card; 🎬 also the help screen, 同系列作品 and 补齐同系列 |
| 📚 | subscriptions: list, detail, history, cancel, and the whole subscription-edit family (menu, field pickers, 确认修改订阅条件, conflict, leave, 保存结果待核对, 订阅修改核对结果, 修改已受理…); the saved outcome stays ✅ 订阅条件已保存 |
| 📋 | 正在忙的任务 and 暂停下载 / 恢复下载 / 删除下载任务 confirm and failure screens |
| ⬇️ / ⏸️ | a download's live view heading (downloading / paused) |
| 📦 | a transfer job (per file ⏳ ▶️ ✅ ⚠️) |
| 📥 / 🔔 | an arrival notice / any other MoviePilot notice; MP notice headings pick ⚠️ 📥 ⬇️ 📚 🔔 from the title's words, see [通知详情](notifications.md#受众与展示) |
| ✅ | done: subscribed, cancelled subscription, deleted, saved |
| ℹ️ / ⚠️ | a Callout line: already subscribed / a warning or failure; also table cell states |
| ⌛ | expired |
| 🚫 | refused (no permission) |
| 🔥 🎌 🎯 🎭 🗂️ 🆕 📅 🎟️ | 发现好片 and chart icons (🔥 trending, 📺 shows, 🎬 movies, 🎟️ 正在热映, 🎌 新番放送), 条件找片 (🎯), 演职人员 and a person's works (🎭), 剧集组 (🗂️), 新鲜入库 (🆕), 追剧日历 (📅) |

已关闭 and 已取消 are the neutral terminal exception: no emoji. Media names in headings and sentences use 《》;
「」 stays for text the user typed and for button names. Toasts (`Reply.Notice`) carry no emoji and no kaomoji;
table cells and facts use the neutral `msgBackendUnavailable` instead of `msgBackendDown`.

`flow.Callout` and `flow.Fold` (rendered as a plain `<blockquote>` and a closed top-level `<details>`, summary
说明). A Callout is an in-message status or warning line (⚠️/ℹ️ stays inside it): consecutive lines form one
quote; never in headings, toasts, table cells, entry bodies or notifications (the requester mention needs a
`Para`). Pre-write risk notes on confirm screens are Callouts. A Fold holds explanation only: whatever it says
about safety or status must also be visible elsewhere on the screen (the edit-menu note repeats on the confirm
screen, subscription-detail read failures are Callouts and the omitted count a Remark beside the folded
explanation). Never fold on a screen edited by `Reply.Follow`; those keep a short visible Remark. Safety text on
confirmation screens, page numbers, 更新时间, refresh state and 点编号… hints stay visible. How a Callout and a
Fold look in the Telegram client is pending the owner's live check

Voice: the bot is cheerful and cute, calls itself 我 and the user 你, ends sentences with 啦/哦/呀/～. At most one
kaomoji per message, only on home, success, empty/failed results and arrival notices (never in headings,
toasts, tables or facts), from this set: `(๑•̀ㅂ•́)و✧` `ヾ(≧▽≦*)o` `(ﾉ>ω<)ﾉ` `(｡•́︿•̀｡)` ``(´-ω-`)``. Data stays
neutral (titles, facts, table heads, subscription states, 第 N 季, page numbers, file states, chart names), as do button labels and the command menu. Safety meaning never softens: an unknown subscription
result still sends the user to `/subscribe` first, cancelling still says MoviePilot will stop
searching/downloading for that subscription, without implying an existing torrent is deleted, an API key
refusal still names the admin; MoviePilot's safe refusal messages are relayed verbatim; secret-bearing
responses and execution errors are never relayed raw

文案用正面陈述说明会发生什么、结果在哪里核对（主人 2026-10-11 要求）。不写「会 X，而不会 Y」「不会 Y」「不代表／不等于／不是 Z」「不保证／不承诺」这类用否定排除误解的免责句；安全含义改用正面说法保留，例如「已提交的下载会保留」「订阅保持暂停」「是否已经停下请刷新核对」「需要时请手动重新搜索」。直接的输入纠错和权限提示（如「这是别人的请求哦，不能替 TA 点」）不受此限。

Errors safe to show users are `*flow.UserError`; others get logged and shown as the generic outage text

## 语义文本模型

`internal/flow/rich.go` - `flow.Text`: blocks by meaning (`Heading`, `Line` paragraphs, numbered `entry` items
(a `Body` folds behind the name), `Group` section heads, `Quote`, `Table` (columns empty in every row are
dropped), `Ticked` checklist lines, `Open` link button, `Small` print, `Remark` notes, `Callout` status/warning
lines, `Fold` (summary plus a `Body`, counted by `textSize`), `Divider`) of spans
(bold, italic, code, link); an entry's `Tag` is a highlighted fact

平台标签与 HTML 渲染见 [Telegram 参考](telegram.md#富文本海报与旧消息)。原有页码、标题、颜文字与图像呈现契约保留，不因拆文档而修改。

## 搜索意图与首页

`internal/flow/engine.go` - conversation choices; `internal/flow/search.go` - ordinary searches retain all
returned results (a leading 订阅/搜索/下载 plus optional colon or space is stripped like MoviePilot's own bot,
`intent.go`; read where a title is typed (`searchTyped`; 重试 repeats the bare search; correcting a title keeps
its download intent unless an explicit 订阅/搜索/下载 prefix overrides it); 下载 sets `session.download`, so
`confirmation` and `held` start the resource search as soon as a target is chosen, already in the library or
not (`huntNow`), except a target numbered by an episode group (picking a group clears the intent, and `huntNow` and
`searching` refuse group targets; 返回 restores the title's intent), and a person's works list clears it too; `navigate` records the skipped card (`session.skipped`) in place of the search under way like
any step's screen, so 返回 leads to it from the releases and a download of a list pick leads back to that list),
page 8 at a time, open unique results directly, and replace a query under a fresh session ID;
`internal/flow/text.go` - result lines and media cards; search results and cards name the media source
(TMDB / 豆瓣 / Bangumi, an unknown source as given) after the rating, whose source it is (owner's request 2026-10-12)

`internal/flow/home.go` - `/start` home menu (`features`), typed title search; 首页 clears the previous title's
download intent, so subsequent discovery picks ask to subscribe normally; a feature with nothing to list
(tasks, subscriptions, latest) only flashes a toast (`Reply.Notice`) and the menu stays, while the typed
commands answer with a message; `internal/flow/charts.go` - `/trending` charts, paging, pick → subscribable
result; 返回 on a pick redraws its chart page (retrying missing synopses); an empty/failed pick keeps the chart
conversation and its return path. 取消 always ends the conversation

## 屏幕导航与读取回执

`internal/flow/nav.go` - 返回: every shown reply is kept with the session state behind it (`screen`); forward
steps (`forward`: pick, season, 相似/系列, multi-season, start-episode prompt, chart pick, typed title) push the
previous screen, same-level steps (paging, ticking, weekday, a typed episode) replace it, any other step
clears the history; 返回 normally restores a screen exactly; a subscription list preceding a successful
pause/resume is read afresh, preserving its kind and page (clamped if the list shrank). Forward picks must
belong to the current screen; the immediately repeated accepted read replays its generated reply without
backend calls or extra history, allowing a failed edit to recover. Every later transition clears that read
receipt; background polls and writes are never cached as accepted read buttons. A request picked from a list
(search results, chart page, 相似/系列; `listPicks`) keeps its session past the write (`settle` clears the
confirmable target instead of taking it), and its outcome (subscribed, seasons, download, batch; `finishing`)
offers 返回 to that list beside 首页/关闭; a request opened directly retains an immutable receipt without navigation
until TTL, so a duplicate confirmation cannot replace success with expiry. `grid` lays out button rows

Cancel/close and replacing a search use `store.take`. Confirmations use `settle`, which clears the target
under the session's step lock before writing. Direct requests keep a terminal receipt without buttons until
TTL; requests from lists keep navigation. A second tap only flashes a notice and cannot replace the outcome.
重新订阅 keeps its session for 返回 to the history: steps of a session run one at a time and it clears the
confirmable target before writing, so a second tap only gets a notice. Failed writes consume the target too;
unknown outcomes use the operation-specific read path described under recovery and downloads.

Session IDs are random across restarts; never replace them with a counter that resets on startup.
`Choose`/`Answer` validate both user and chat and serialize steps per session. Calendar annotations clone both
mutable slices before writing. E2E transcripts normalize opaque session IDs only when recording, never in
updates sent to the app

`internal/flow/store.go` - in-memory sessions (10 min TTL)

`internal/flow/pages.go` - bounded pages for subscriptions, tasks and latest items; page buttons keep item
indices stable

## 确认票据与过期

Button data is `<session>:<action>:<arg>` and must stay ≤ 64 bytes (Telegram limit). Guarded write buttons
(`needsTicket`: `finishing`'s `ok`, `y`, `td`, `tz`, `bo`, plus subscription `sp`, `sr`, `z`, `uw`, `qc`, `qy`
and download `kq`, `ks`; series `jy` is in `finishing`) carry a fourth
part, a ticket `navigate` hands the shown screen (`ticketed`); only a press with the shown screen's ticket
writes, and 返回 restores a screen with its ticket, so a stale 确认 from an earlier pick of the same list never
confirms the next pick. Subscription mutations do not enter `finishing`, which controls request navigation. An
expired button only has its action to go on: task-specific actions point to `/tasks`, subscription-specific
actions to `/subscribe`; ambiguous 返回/翻页 (`bk`/`p`) point to `/start` to reopen the relevant feature rather
than claiming the page was a search

## 只读恢复与未知写结果

`internal/flow/recovery.go` - failed search, season and subscription reads keep their session and offer a
read-only retry (`rt`); retry replaces the failed screen without adding a navigation step. Subscription writes
go through `submit` once; transport/response failures or a missing positive ID point to `/subscribe`, never to
a write retry. Failed reads clear the confirmable target while keeping retry context; a successful retry clears
that retry state.

未知结果按操作区分：订阅创建、重新订阅与取消先查 `/subscribe`；下载创建与删除先查 `/tasks`；暂停或恢复先刷新。明确的安全拒绝保留，不能把未收到结果解释成写入失败。

## 季与起始集

Subscriptions take `start_episode` (per season): MoviePilot skips earlier episodes. "只追新集" uses details'
`next_episode_to_air` and only shows when it is in the chosen season and > 1; button data `ok:<n>` carries the
start, validated against the season's episode count before the confirmable target is cleared

Unique ordinary search results open directly, but chart identity disambiguation still requires a pick. The
season picker is skipped for the season a search term named (`Media.Season`, from MoviePilot's search
`season`, when the show has it; 青之芦苇 第二季 opens on 第 2 季) or a sole season whose number is positive; specials
are never selected implicitly. TMDB may fold a sequel into season 1 (青之箱 第二季 continues at S1E27), which then
opens like any sole season. Both shortcuts keep final confirmation. `重新搜索` (`rs`) replaces the old session
when the new title is submitted; empty results keep a typed-title prompt. Generic paging records its screen,
so 返回 restores the page selected from

`internal/flow/seasons.go` - multi-season picker (「多选季…」): tick seasons, subscribe each from episode 1, report
per season

`internal/flow/episodes.go` - start-episode choices for seasons and typed answers (`Engine.Answer`)

## 订阅列表详情与身份

`internal/flow/subs.go` - `/subscribe`: subscriptions one kind at a time (电视剧/电影 buttons, `sf`, same-level; TV
first when there are any); number buttons open read-only details (`sd`); one shared 取消订阅 (`sc`, whenever the
shown kind holds any) lists them to pick and confirm. Every whitelisted user manages every subscription,
including ones created in MoviePilot's web UI (see [用户与权限模型](architecture.md#用户与权限模型))

`internal/flow/subdetail.go`, `subdetailtext.go`, `subprogress.go` - subscription details re-read the
subscription list on open and 刷新 (`su`, same-level), keeping its original page for 返回; successful state
mutations and successful refreshes observing a changed state invalidate the source subscription-list snapshot,
so 返回 reads it afresh; one that has ended offers 订阅历史 of its own kind (a calendar list chose none). Every
subscription shows 暂停订阅 (`sp`) or, when its state is S, 恢复订阅 (`sr`) beside 刷新
(`subpause.go`, same-level): `PUT /api/v1/subscribe/status/{id}?state=S|R` once, then the details read afresh;
a refusal (订阅不存在) is relayed as a notice and an unknown result asks to 刷新, never a retry. Settings and
last-search execution state come from that list; library, downloads and transfer queue are read concurrently
within the cosmetic budget. Exact kind + media identity + season associates tasks (movie and TV IDs can
collide); unknown season/episode tasks only add a note. Library visibility is independent of task completion;
a download percentage describes the whole torrent, not each file. Consecutive equal episode rows merge; at
most 100 requested episodes, 20 table rows a page with explicit omissions. No resource names, sites,
downloaders or file paths

Subscription buttons (`sd`, `su`, `sp`, `sr`, `v`, `z`) carry the subscription id, never an index into the
session's list (`session.subAt`). Row ids can also be reused: `subidentity.go` checks the displayed
subscription's source, media id, kind, season, episode group and original creation `date`
(`Subscription.Created`) against a fresh list before pause, resume or cancellation; an unavailable or changed
identity only flashes a notice, never writes. Details use the same identity comparison. MoviePilot's write API has no conditional identity parameter:
these are client-side preflight checks, not an atomic compare-and-write guarantee against concurrent backend
replacement.

### 订阅条件编辑

`subedit.go`、`subeditchoices.go`、`subeditdirectory.go`、`subeditinput.go`、`subedittext.go` 和
`subeditsave.go` 维护当前订阅的原始条件和内存草稿。每个订阅详情页都提供「编辑条件」，打开和保存均按复合身份核验。保存前比较全部可编辑字段和总集数，普通进度变化不阻断，也不随草稿回传。

字段页、关键词输入和预览均替换同一编辑层级；字段页返回保留草稿，编辑菜单返回或取消在有改动时先确认放弃。退出确认用「返回」继续原字段／输入，用「放弃修改」丢弃草稿；无改动不多问。退出确认保存独立的展示快照，不放进可恢复写目标的导航历史，恢复时重新签发写票据。选项、恢复原值、放弃与输入代次均不因返回复用，旧按钮不能影响新字段或新草稿。

编辑菜单显示未保存项数及逐项「已修改」，单字段页显示当前／原值与草稿，`恢复原值` 只恢复这一项且保留原始后端表达式。用户可见「质量／特效」调整为「资源类型／画面格式」，取消字段限制称「不限」。订阅详情的规格也使用安全的人类可读名称，不展示内部匹配表达式。

规格选择是固定且经过 v3.1.1 源码核对的匹配表达式。包含／排除采用单行字面关键词（256 字、12 项上限），`|` 分隔任一匹配，特殊字符转义；不使用 Go 正则解释旧 Python 表达式。原后台条件只显示已设置，输入经过检查的本次字面关键词在预览中显示。既有复杂正则只能保留或清空，编辑复杂表达式留在 MoviePilot 后台。

规则组保留选择顺序，使用已有目录和引用规则摘要核对；不显示自定义规则正文、使用者、服务路径或错误原文。站点复用媒体类型目录，仅投影 ID 和名称，不保留或展示凭据。清空规则组沿用普通／洗版各自全局设置；空站点沿用后台默认。只有草稿改动了某个目录的选择且结果非空时，保存前才重新读取该目录核对；目录失败保留原草稿，不发出更新。

后台条件或总集数变化进入 `subeditconflict.go` 的保留草稿比较流程。只有用户和后台都改动同字段且值不同才要求逐项选择；未修改字段明确说明沿用后台新值，其他草稿保留。「完成处理」再次读取同一身份和快照，若又变更则重新处理；选择完成后以最新快照为基线，重新生成预览和确认，不能自动写入。目录变更返回编辑菜单保留草稿，仍须重新选项并经原目录预检。

`uw` 绑定当前屏幕票据，写前消费草稿并移除编辑历史，同时保存独立的只读 `session.subReceipt`。`subeditcheck.go` 比较本次实际修改的每个字段，显示与期望一致／仍为原值／其他值／未知；未知身份或读取失败均不猜测。包含／排除原文不展开。核对及返回只能持有不可提交的摘要，不能把旧确认恢复成草稿；摘要只保留本会话最近一次提交，仍遵守 TTL，不持久化。

写结果未知（含超时、半响应、缺少 `success`、跳转）不能重试。明确成功后再读当前订阅；当前读失败只提示稍后核对。回执及同会话对应详情提供「核对结果」，仅只读比较当前值，不能据相等追认当时请求成功，不承诺客户端预检是后端原子 compare-and-write。

### 单订阅立即搜索与执行跟踪

`subsearch.go` 在订阅详情中提供「立即搜索」。进入确认和最终提交均读取订阅，核对复合身份；缺失创建时间不能发起搜索。条件、暂停状态或总集数发生变化时重新确认。提示明确说明会访问站点、可能按执行时的条件自动下载；暂停中的订阅仍可经明确确认搜索一次，不恢复自动调度。

`qc` 只接受当前屏幕票据，写前清空 `subSearchAsk`，建立结果尚未确定的本地回执。明确的新建单订阅回执才保留批次 ID；合并到已有批次或响应不完整时不取得控制权。`session.subSearch` 只保留当前会话最近一次尝试，返回保留最新值，不从旧屏幕恢复。批次序号区分同一会话前后的搜索，旧按钮不能转而控制新批次。

状态页复用 `Reply.Follow`，每次只读本地记录的批次 ID，沿用现有刷新间隔与十分钟跟踪上限。用户点击先中断在途 Follow 读取时，丢弃该次读取副本，不覆盖当前屏幕；读取本身超时或失败仍保留标明非当前状态的上次快照，撤下停止入口，等待显式刷新。范围不符或未知状态不开放控制。批次完成只表示搜索执行事实，不推断资源、下载或媒体库状态。返回重新读取订阅详情，停止刷新、关闭与会话过期均不取消上游。

「停止本次搜索」有独立确认，`qy` 写前再核对订阅复合身份，以及批次 ID、创建时间、`manual` 来源、单订阅总数、当前订阅 ID、计数和 `can_cancel`。预检不能代替后端原子比较并写入。停止确认在 PUT 前消费，明确拒绝和未知结果都只保留只读核对，不从返回或投递恢复中重复 PUT。

停止回执与批次终态分开显示，受理不等于已经停止；批次最后标为已停止时，也可能已有搜索任务提交了下载。已提交下载不撤销，订阅自动调度不暂停。回执、批次 ID 和停止状态不持久化，重启不自动恢复控制权或重放写入。

## 历史与洗版

`internal/flow/history.go` - 订阅历史 (`hs` from the subscription list, `hf` switches kind and reads page 1 afresh):
the kind's past subscriptions newest first, 10 per upstream page (`GET /api/v1/subscribe/history/{电视剧|电影}?page=N&count=10`).
更多历史 (`hm`, same level) appends the next upstream page under continuing numbers and shows the display page
holding the first new record; records already listed (by history ID) are not repeated, since a subscription that
finishes meanwhile pushes older records onto the next page. The note gives the latest `X-Total-Count`, how many are
listed and how many lack a media identity (counted by ID, never listed); 更多历史 shows while the last page read was
full (unlisted records count) and short of the total. Records newly added at the top appear when the history is
opened again; a record deleted elsewhere between reads can shift one unread record past the offset, which the
counts then show. A pick (`hp`) already
subscribed only flashes a notice (lookup failure still asks), otherwise a card asks; 确认重新订阅 (`ho`) posts the
history record back unchanged like MoviePilot's WebUI (MoviePilot drops its id/state fields and keeps its
settings); the receipt only says MoviePilot sends notifications by its own settings

`internal/flow/upgrade.go` - 洗版订阅 (`bv`, forward): offered on the movie confirm card, a season's start choices
and a held card (movie or whole season in the library), never on an already subscribed card or after a typed
start episode; it reads the target's subscription again (MoviePilot's create answers a subscribed target with
the existing row unchanged, `exist_ok=True`), so an existing one or a failed read only flashes a notice; 确认洗版
(`bo`, ticketed, `finishing`) posts the whole target with `best_version: 1` once (omitted otherwise, so
MoviePilot's per-kind default config applies to ordinary subscriptions)

## 资源搜索筛选与批量下载

### 可选流式搜索

配置资源 Cookie 后，`torrenthunt.go` 在独立代次中消费 SSE，前台只取只读快照。资源列表先展示未匹配候选，完整终态到齐前不开放单项或批量下载；空最终结果与不完整结果分别显示。每个资源分配固定身份，按钮只携带该身份，排序、过滤、返回和重搜都不能复用旧身份；列表显示的序号与 JSON 搜索一样按当前位置从 1 编号（主人 2026-10-11 要求，最终匹配剔除候选后不再跳号），最终顺序仍使用 MoviePilot 优先级。详情页固定已选资源，不受后台事件覆盖；「刷新结果」只读该资源最新状态，消失或选择字段改变则要求返回重选。

`Reply.Wake` 只在首批可用结果和终态唤醒既有 Follow，其他更新继续用 `CRYCHIC_PROGRESS_INTERVAL`。Telegram 投递失败只重投保存的 Reply，不重搜。返回媒体卡片、取消、会话清理和 app 退出关闭并等待 worker；旧代次 Follow 不能覆盖新列表、详情或回执。Cookie 缺省仍走下述 JSON 流程。

卡片的「选择搜索站点」先读取媒体类型对应站点，编号勾选后「开始搜索」才访问站点；空选择表示全部，读取失败或无站点不会启动搜索。「筛选站点」继续只过滤已有结果。新动作码为 `ss/st/sa/sb`，流式只读刷新为 `tu`，显式重复搜索为 `te`。

`internal/flow/torrents.go`, `torrenttext.go` - 搜索资源 on a card with a chosen target (movie confirm, held,
already subscribed, a season's start-episode prompt; `session.focus`): `ts`/`tr` start the slow site search in
the background (`torrenthunt.go`, `session.hunt`; 取消 stops it) and show the card with a searching line and
`Reply.Follow` (`tg` leads on from the card, `tq` replaces a failure): each refresh only checks the search,
saying how many seconds it has run (from `Now`, so `h.advance` moves it) until the results replace it;
releases list in MoviePilot's order (8 a page, number buttons `tp` carrying each release's own index),
re-sortable like the WebUI (`torrentsort.go`: 默认/做种/时间/大小/站点, `to`, same-level) and filtered to one site (筛选站点
`tf` lists sites with counts, `tw` picks one or all; its 返回 re-shows the list); 组合筛选 (`torrentfilter.go`, `ti`
panel, `tj` opens a field and fixes its offered values, `tl` ticks one of them (its arg carries that opening's generation, from a session counter 返回 never rewinds, so
a button from another field or an earlier opening is refused), `tn` clears a field or all; all
same-level, 返回 re-shows the list) narrows by 分辨率/编码/制作组/大小/促销/H&R like the WebUI's filterForm: values
come from the results, ticks within a field widen, fields narrow each other, an empty value (and MP's 未知
promotion) is offered as 未知, and each value's count is what it would show under the site, missing and other
conditions; the list note says 符合 N 个，共 M 个 with the conditions, a new search clears them, and 缺集方案
takes only releases inside the current site and combined filters; for a season partly in the
library (`torrentmissing.go`) each release says what it adds (补 E01-E12 when only part of it is missing, 已都有
when none) and 只看缺集 (`tx`, arg 1 on / 0 off, same-level, offered while some release adds nothing) hides the
releases adding nothing; MoviePilot reads episodes from a description only when the title names none, so a
release whose title names none of its episodes (`Torrent.EpisodesUnsure`, `moviepilot/episodename.go`, which
ignores audio channels like DDP5.1 and H.264; a whole pack saying 修复第9集章节 reads E09) gets no missing-episode
note and is never hidden; lists/details say 集数识别不确定, and success does not repeat an uncertain episode range;
all in the session without searching again, a pick shows the release under the card (two tables, H&R warning)
and 确认下载 (`td`) writes once (the release to confirm is cleared before the write; a refusal is relayed, an
unknown result points to `/tasks`, never a retry);
`torrentbatch.go`: 多选… (`tm`, forward) turns the number buttons into ticks (`tk`, same-level, stays on the
ticked one's page, at most `maxTickedReleases`) kept across paging, sort and filters; 下载所选 N 个 (`tb`) lists
the ticked releases under the card and 确认下载 (`tz`) clears the batch before writing each once, tabling each
release's outcome (refusal relayed, unknown → `/tasks`); arrival notices come from MoviePilot, so separate
downloads do not promise separate messages

Batch confirmation consumes its target before starting one background worker (`submission.go`). The worker
serially posts each selected release to MoviePilot's download API; `br` only reads submission progress, and
`bs` stops future submissions of that batch token. It does not cancel the current HTTP request or pause/delete
existing downloads. While running, only progress and stop are accepted on that conversation; the final
result restores its original list/terminal navigation. Requests can still be made in other messages.

Progress outcomes contain titles and statuses, not site records. The live session remains available through
execution and for the normal TTL after completion; in-memory batches are not persisted or resumed on restart.
`app.Run` injects the run context and waits for flow workers after platform handlers drain. Shutdown cancels
the in-flight call as an unknown result and starts no remaining items. Per-call MoviePilot budgets stay in force.

资源原始记录、集数识别和删除实例参数见 [MoviePilot 参考](moviepilot.md)。

### 缺集下载方案

`torrentplan.go`、`torrentplanread.go`、`torrentplanselect.go`、`torrentplanmodel.go` 和 `torrentplantext.go` 复用资源结果及批量提交。完整 TV 单季结果提供「缺集方案」，当前站点筛选与组合筛选作用于全部结果而非当前页；显示排序不影响确定性建议。生成与重新计算并行只读媒体库及季信息，共用现有五秒预算，不访问资源搜索端点。

所需范围为当前起始集至季末，季总数须明确且不超过 100。只用同来源、ID、类型、明确单季、非空且未越界的可信识别集数；`Unverified`、`EpisodesUnsure`、`RangeUnsure` 均排除，不从空列表推断整季，也不将非默认剧集组映射成普通季。按新增覆盖量、无 H&R、少重复、已知且较小体积、MoviePilot 顺序贪心选取，最多 20 个，不承诺最优或种子文件核验。

摘要保留完整集数汇总，资源按既有文本预算分页。重复已有、资源间重叠及起始集以前三个集合可相交；大小未知单列，不把部分已知大小称为总量。下载完整资源，不提供种子内文件裁剪。空库存必须提示上游可能隐藏媒体库连接或扫描问题；方案不把正在下载的内容算作已入库。

`session.plan` 是不可变内存预览；确认仍使用原 `tz` 当前屏幕票据。确认前再次读取并比较本季库存、总数及起始集；变化用只读 `pc` 转移替换预览，不按写入完成处理导航，重新签发确认票据。错误页撤下确认，重算不重放写入。成功预检后交给原串行 worker 并消费方案，返回不能恢复已提交目标；客户端预检仍非后端原子事务。

## 任务最新入库与内容预算

`internal/flow/tasks.go` - `/tasks`: list downloads and transfer jobs, follow one live (`Reply.Follow`);
`taskdelete.go`: a download's view offers 删除任务 (`kd`), which stops following, re-reads the download and asks,
warning that its files go too and, when a subscription of the same media and season exists, that the
subscription will not fetch it again; 确认删除 (`ky`) clears the download to confirm before the write, relays a
refusal and points an unknown result to `/tasks`; `internal/telegram/follow.go`
re-asks the flow every `CRYCHIC_PROGRESS_INTERVAL` and edits only on change

`seriesplan.go`: 同系列 tags each part (`Block.Tag`) with its state and offers 补齐缺少的作品… (`jq`, forward) to tick
missing parts (`jt`, same-level) and subscribe them (`jy`, finishing and ticketed), see
[发现参考](discovery.md#相关推荐与同系列)

`groups.go`: a TMDB show's season picker offers 剧集组… (`eg`, forward) listing groups with seasons and episodes;
`ep` (forward) reads the group's seasons (`/media/group/seasons/{id}`, episode counts zero dropped) and shows them
through the ordinary season picker with `session.group` set. Targets carry `EpisodeGroup` and its name; under
a group the library is replaced by a note, downloads are not matched (`Target.has`), and multi-season, the
next-episode start, 搜索资源, 选择搜索站点 and 洗版 are left out because they work in standard seasons; picking a
group also drops the 下载 intent, since resources would be searched by the standard season. A new
media pick clears the group

`discoverfilter.go` and `people.go`: the condition search panel (`ya`, same-level `yk`/`yf`/`yo`, forward
`ys`, same-level `ym`) and a card's 演职人员 (`ca` → `cw` → `cp`) list picks like a chart; see
[发现参考](discovery.md#条件找片与演职人员)

`taskprogress.go`: a transfer view with a running file reads the plugin's progress records once per refresh
and adds a percentage to the running file exactly one active record names (media, season, episodes,
download hash and instance); twin running files, several records, an ended record or an unavailable read
leave the states as they are

`taskpause.go`: a download view offers 暂停下载 (`kp`) or 恢复下载 (`kr`) by the state of its last successful read;
both stop following, read the download afresh and ask on a card with 确认暂停 (`kq`) or 确认恢复 (`ks`) over 返回.
The confirm code carries the direction, the ticket binds it to the shown screen, and the asked target
(`session.toggle`) is consumed before anything else. The download is read again right before the one write:
gone, already in the asked state, or without one known instance (`InstanceAmbiguous`) shows it as it is and
writes nothing. A task without one known instance offers neither pause/resume nor 删除任务. An accepted
write goes straight to the task followed live, as 查看任务 (`k`) would (owner's choice 2026-10-12), with a
footer that it was submitted; the downloader's state reaches the view by the read and the follow refreshes.
A refusal is relayed, an unknown result offers only 查看任务, a read, never a retry

`internal/flow/latest.go` - `/newly_added`: newest media server items (20 per server) linked to their web
page, grouped by kind in the order kinds first appear, one line each with the year; a list entry's `section`
puts the `Group` above it whenever it opens a page or a new section, so a group continued on the next page is
headed again. Subscription lines leave the usual 订阅中 unsaid and name other states

List pages budget text and item buttons together (up to 20 entries and 3000 content runes). Large transfer
views show at most 100 file lines, retaining the full aggregate count and explicitly naming omitted files

Task lists keep downloader and transfer-queue reads independent (`taskreads.go`). A failed source is named
as unknown while the successful source remains selectable. Any source failure retains the conversation and
offers read-only 刷新; only two successful empty reads can claim there are no tasks. Refresh reconstructs
task references from the available records and never starts a download, search or transfer.

下载及转移任务的接口字段、SSE 限制、停止跟随和删除副作用见 [MoviePilot 任务参考](moviepilot.md#下载进度与任务删除)。
投递失败只重试消息，见 [Telegram 跟随参考](telegram.md#跟随者投递与停止)。

## 相关推荐

`internal/flow/related.go` - 相似推荐 / 同系列 on a media card (season picker, movie confirm, held, already
subscribed): lists related media or the movie's series like a chart (`pickEntry`, opening to synopses; generic
`listPages` paging, which is a same-level step for 返回) with number buttons picking `actionMedia` directly;
entering a related/series list clears the preceding title's download intent, while 返回 restores that title's
own state; nothing found or a failed lookup only flashes a notice and keeps the card

跨来源身份、日历、播出日期与合集 API 见 [发现参考](discovery.md)。

## 回调动作索引

Callback action strings are unique across every flow handler. Resource search is `ts`/`tg`/`tr`/`tq`, release
pick `tp`, ticking releases `tm`/`tk`/`tb`/`tz`, release order `to`, site filter `tf`/`tw`, combined filter `ti`/`tj`/`tl`/`tn`, missing-only
filter `tx`, download `td`, download deletion `kd`/`ky`; 洗版订阅 is `bv`/`bo`, subscription return is `j`,
subscription kind `sf`, shared cancel `sc`, subscription details/refresh `sd`/`su`, pause/resume `sp`/`sr`,
history `hs`/`hf`/`hm`/`hp`/`ho`, calendar weekday is `w`, generic list paging is `p`, 相似推荐 is `r`, 同系列 is `n`,
重新搜索 is `rs`, read-only 重试 is `rt` and 返回 is `bk`.
订阅编辑为 `ue/uf/uv/uc/ua/uq/uw/ui`，依次对应入口、字段、选择、清空、返回菜单、预览、确认写入和 typed input。
交互增强为 `uo/ud/uy`（恢复原值、确认放弃、只读核对）及 `up/uk/ub/ug/ur`（冲突字段、保留草稿、采用后台、完成处理、重新读取）。
缺集方案为 `pl/pc`（生成／重新计算），确认复用 `tz`；确认发现库存变化时按只读 `pc` 更新屏幕。
单订阅搜索为 `qs/qc`（确认页／提交），`qv/qr/qf/qu`（打开或继续刷新／手动读取／Follow 读取／停止刷新），`qx/qy`（停止确认页／提交停止）。只有 `qc/qy` 写入并绑定当前票据，所有动作只接受当前页面的按钮或 Follow。

这是现有动作的导航索引；新增动作时核对源码中的全部 handler，不能把该表当作无需核验的注册中心。
