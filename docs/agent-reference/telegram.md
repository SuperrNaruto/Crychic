# Telegram 接入与投递参考

改输入、消息、inline、海报、Follow 或并发时读取。先读 [目录规则](../../internal/telegram/AGENTS.md)；文案另读 [界面规则](flow.md#界面与文案)。

本文从原根指南按主题迁出，保留当时的实现说明与上游版本背景，不代表已对当前线上服务重新核验。涉及目标版本变化时，按根指南核验来源。

## 启动命令与更新订阅

The bot registers its command menu (`setMyCommands`, `telegram.commands`) on every start, for the default
scope and for each whitelisted user's chat (a chat-scoped menu outranks the default; the owner's token carried
a stale one from an earlier program); keep it in step with `parseCommand`'s commands and `help`

Telegram keeps the last `allowed_updates` a `getUpdates` sent and go-telegram/bot omits it by default, so the
bot sends its own list (`telegram.allowedUpdates`: message, callback_query, inline_query); the owner's token
was left at message + callback_query by an earlier program, which silently dropped inline queries. The fake
Telegram starts with that stale list and honors `allowed_updates` like Telegram; a new update type must be
added there or it never arrives

## 聊天话题与输入

Explicit commands for another bot (`/search@other_bot`) are ignored before dispatch or refusal. Forum
destinations persist in `Actor.Address` as `chat/topic`; ordinary numeric addresses and channel usernames
remain compatible. Only a positive `message_thread_id` on `is_topic_message` selects a forum topic. Commands,
callbacks, typed answers and notices preserve it; pending input is keyed by chat, user and topic.
`Actor.Private` is set (`callbackActor`/`messageActor`) only when the message really sits in a private chat; a
callback whose message is inaccessible falls back to the user's ID as destination but is not private, so it
never revives an expired page.

`internal/telegram/input.go` - unquoted private text answers the newest pending prompt; an explicit reply must
quote that prompt in every chat. A mismatched private reply gets a warning without editing either conversation
or replacing the pending prompt; unmatched group replies stay silent

Typed answers: a `Reply` with `Input` makes the Telegram adapter remember (chat, user, topic) → message in
`adapter.pending`, including initial sends. Private chats take plain text as an answer while pending,
otherwise search it as a title; every recognized private command clears the old pending input. An expired
prompt leaves unquoted private text free to start a new title search immediately; an explicit quoted answer
only reports expiry. `Reply.Expired` carries this semantic distinction without inspecting rendered text. Typed
episode numbers accept full-width digits and the optional 第...集 wrapper, then use the same range checks.
Private text/commands share an input lane so old answers cannot restore a cleared prompt. Groups only take a
reply quoting the prompt

## Inline 与深链

`internal/flow/inline.go`, `internal/telegram/inline.go` - inline mode (BotFather `/setinline` must be on):
`Conversation.Find` searches media for a query of at least 2 runes (no session, at most 10, deduplicated by
`MediaRef`); the adapter answers whitelisted users only (others get `[]`), personal, 10 s cache, cancelling
the user's previous search as they type (handlers run at once, so among searches under way the newest query is
the highest update id, and an older one starting late is left unanswered; a finished search is forgotten,
since after a week without updates Telegram picks the next update id at random, which the fake models with
`h.tg.idleWeek`). Each result is an article whose message is **classic HTML** (`<b>` title, facts, 查看详情页
link): go-telegram/bot v1.27.0's `InputRichMessageContent` does not implement `InputMessageContent`, so inline
results are the one place without rich messages; its URL button 在机器人里打开 is a deep link
`t.me/<bot>?start=<MediaRef>` (`m_tv_t_224207`: kind `tv`/`mv`, source `t`/`d`/`b`, id). `/start <ref>` calls
`Conversation.Open`, which reads the media by identity (`Backend.Lookup`, `GET /api/v1/media/{id}`, the same
endpoint details read again for the card) and opens it like a unique search result; an unreadable ref opens
home. The bot calls `getMe` itself at start (library check skipped) to learn its username

## 富文本海报与旧消息

`internal/telegram/rich.go` - renders `flow.Text` as a Telegram rich message (Bot API 10.1 rich HTML:
`h3`/`h4`, `p` with `br`, `ol`/`li value`, `blockquote expandable` (card synopsis), plain `blockquote` (`Callout`, consecutive lines joined by `br`), top-level `details`/`summary` (`Fold`, closed), `li` > `details`/`summary`
(an entry with a body; closed by default, the summary keeps name and facts), `table bordered striped compact`, `ul`
with `input type=checkbox`, `mark`, `tg-button-row`/`tg-button type=url`, `footer`
(remarks, and `Small` print mid-message: rich HTML has no font size below headings, and the owner chose a
footer over `h6`/`sub` on a live sample), `hr`, `img`), escaping everything

Ordinary bot messages use `sendRichMessage`; edits pass `rich_message`, and the library's empty `text` field
is accepted. Inline articles use the classic HTML exception described above. Media
posters are an `<img>` block on top; list pages (search results, charts, 相似推荐, 同系列, 演职人员作品) carry no
poster block: slides had no numbers to match entries and eight Douban posters cost about 2.4s to upload
(主人 2026-10-13 决定去掉)。Telegram fetches posters itself the first time; the poster's `file_id` is then read from
Telegram's copy of the message and later messages send it by `file_id` (`tg://photo?id=pN` media; transcripts
normalize to the displayed image identity), so Telegram need not fetch it again. Only an explicit invalid
file identifier or failed image-URL rejection forgets reused file_ids and retries with fresh posters, then
without posters. Network failures, rate limits and other refusals keep the cache. A new message whose send
result is unknown is not resent: it may already be visible. Douban images (`*.doubanio.com`, all Douban
charts and 正在热映) answer 418 without a Douban referer, so Telegram cannot fetch them:
`internal/telegram/posters.go` downloads them with the referer and its own User-Agent (`img3` answers 403 to
Go's default one; 5 s budget, 5 MB cap, 4 at a time), uploads them as `attach://` media (file_ids: memory
only, LRU of 1000, gone on restart); a poster that fails to download is dropped alone. Downloaded bytes wait in a second in-memory LRU
(`posterbytes.go`: at most 32 posters and 16 MB, successes only, dropped once a file_id is learnt); one download
per URL runs at a time, on the bot's own context rather than a caller's, and later requests for it join the
flight. Reply.Prefetch (the poster URLs of a list page's entries, not displayed) is downloaded in the background
(2 at a time, separate from the 4 of a message being sent) after the page is sent or edited, so a later pick
finds the bytes; a chart pick whose search result is the same Douban media (Source and ID) keeps the chart
item's poster for its card so that prefetched poster is the one uploaded (MoviePilot's title search otherwise
returns another signed Douban URL or a TMDB one, which Telegram fetches itself) MoviePilot's WebUI
shows them through its own image proxy (`/api/v1/system/img`), which Telegram cannot reach. Rich HTML
collapses newlines, so the renderer turns them into `<br>`. The home menu alone sets `Reply.Banner`
(`flow.BannerHome`; every name in `flow.Banners` needs a bundled JPEG or the bot stops at start); the banner
goes through the poster file_id cache as `banner:<name>`, uploaded once and then reused like a poster. Empty
feature buttons only flash a notice and leave the home banner untouched. Telegram clients often keep showing
the old top image after an edit changes it until the chat is reopened; the bot's requests are right, and the
owner chose not to work around it. Photo messages (home menus from before rich messages) cannot become rich
text, so a press on one sends a new message and deletes the photo; photos are recognized from callback
metadata; `Reply.Notice` keeps them untouched

缓存实现入口为 `internal/telegram/posters.go`，首页发送/编辑入口为 `internal/telegram/home.go`。只有首页携带
`assets/home.jpg`；旧照片菜单替换成新消息后删除，不能把照片原地编辑成 rich message。

## 跟随者投递与停止

资源搜索仍走语义 `Reply.Follow`，适配器不解析 MoviePilot 帧。可选 `Reply.Wake` 通道在首批可用结果及终态关闭，提前触发一次只读刷新；普通中途结果沿用配置间隔。待投递 Reply 的限流和重试期限优先于唤醒，失败时移除本轮 wake，避免关闭通道触发忙循环。初始发送、typed edit 和 callback edit 都保留同一唤醒语义。

A sent reply (not only an edited one) with `Follow` gets a follower (`adapter.replyTo`), and so does a typed
answer's edit (`adapter.show`, shared with button presses), e.g. `/search 下载 <title>` with a unique result
whose resource search starts at once; 重新搜索 is not offered on a search under way

A follower keeps one unsent `Reply` until its edit succeeds (including `message is not modified`). Delivery
failures retry that reply, never the flow transition or resource search, on the existing refresh cadence and
respecting Telegram's `retry_after`; retries stop after `FollowFor` from the first failed delivery. Owner
navigation or cancellation discards the pending reply. A callback suspends its follower before waiting for the
message lock, but a rejected callback resumes the same follower and pending reply; an intervening accepted
navigation invalidates that identity.

Ordinary callback and typed-answer edits use the same pending-reply mechanism, including successful write
receipts. Repeating the callback that produced a pending reply only retries delivery, never the business
step; flood-control delays and the original retry deadline remain in force. Permanent Telegram refusals
stop retries. This applies to editing an existing rich message, not uncertain photo-replacement sends or
arrival notices, whose save-before-send policy is unchanged.

Initial sends with input or follow information record navigation accepted while their response is in flight.
When the send returns, installation runs under the message lane and ignores superseded initial state. These
records live only for that send; rejected callbacks do not supersede it. Requester-bearing notices recheck
the current whitelist at delivery; identity-free configured channel broadcasts remain independent.

An owner's button press suspends the message's follower before waiting for the message lane, so a slow refresh
never holds the press

正常导航或取消丢弃 pending Reply；被拒 callback 恢复同一 follower，恢复时仍须核对身份，不能跨过已经接受的新导航。任务离开列表、用户停止刷新或达到 `flow.FollowFor`
时停止，具体接口见 [任务参考](moviepilot.md#下载进度与任务删除)。

## 消息顺序与网络预算

Telegram non-poll requests have a 10-second deadline including response-body reads; `getUpdates` uses the HTTP
client's 70-second timeout. Metadata card enrichment uses a 5-second cosmetic budget; independent read-only
calls overlap (a show's seasons are read alongside its details and library state). The Telegram adapter counts
running handlers and followers and `Bot.Run` waits for them after polling stops, since the library does not,
subscription writes do not. All clients share one transport keeping 16 idle connections per host

A `Reply` with `Notice` must not edit the message (another group member tapping your buttons)

命令发送用 `sendRichMessage`，按钮编辑用带 `rich_message` 的 `editMessageText`，需编辑的按下在回答前先预热回复的防盗链海报（`warmBudget` 2s，下载进入字节缓存；超时则放弃等待，下载继续供编辑加入），客户端转圈因此覆盖海报下载（线上实测 Douban 海报下载 1.4–3.7s）；随后 `answerCallbackQuery` 在编辑完成或编辑已运行 `answerLead`（`telegram.Config.AnswerLead`，默认 50ms）时发出，以先到者为准；Notice 不编辑，直接回答。2026-10-12 线上计时：「返回」flow 0ms，编辑约 380ms，原先串行的回答约 340ms，到 Telegram RTT 约 160ms；并行后每次按下省掉约一次回答往返。超过 500ms 的按下记 `slow button press`（只记动作码与分段耗时，含 `posters` 预热段），海报准备超过 200ms 另记一行。
内联键盘按钮把 `flow.Button.Style` 映射为 Bot API 的 `style`（`primary` 蓝、`danger` 红，零值为默认色）。

`internal/telegram/lanes.go` 串行每条消息的状态变更及编辑；初始发送尚未返回时，已经显示的按钮也必须可用。

开启 follow 的初始发送和 typed edit 与按钮编辑共用语义；图像显示身份和响应先后由 [E2E 参考](e2e.md#消息顺序与图像身份) 中的 fake 覆盖。
