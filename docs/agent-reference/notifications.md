# 通知状态、到货与迁移参考

改通知、持久化、游标、迁移、合并或生命周期时读取。先读 [目录规则](../../internal/notify/AGENTS.md)，改文案另读 [界面规则](flow.md#界面与文案)。

本文从原根指南按主题迁出，保留当时的实现说明与上游版本背景，不代表已对当前线上服务重新核验。涉及目标版本变化时，按根指南核验来源。

## 到货聚合与画质

Arrivals: `notify.Notifier` polls `GET /api/v1/history/transfer` → `arrive` records matches as pending (media
+ season + episode range, plus one source file name per download, at most 3) → `flush` announces once settled
→ each download's quality is read from MoviePilot's own release-name recognizer (`GET
/api/v1/media/recognize?title=<file>`, what its WebUI shows; also a TMDB lookup, ~2 s, cosmetic within 10 s,
unrecognized answers `meta_info: null`) and shown once as small print, e.g. `1080p · BluRay · x265 · FLAC 2.0
· FROGE` → `telegram.Bot.Notify` posts in the requesting chat, mentioning the requester in groups. Only the
file name of `src` is kept, never the server path. Watches of the same media and season (a subscription and a
download picked for it) match the same transfer, so `flush` tells them together: the first to settle announces
every pending arrival of all of them in one notice, to all their requesters, once the media server shows every
one of them (`due` checks the whole group, `settling.holds`); the notice says all wanted episodes are in only
when that holds for every watch it tells, in a download's words only when every watch it tells is a download;
a stuck download is likewise told once to every watch awaiting it (`withStalls`).

A download's watch (`Request.Download`, `watch.Download`) matches transfers by `download_hash` and its season,
takes every file of that download and ends when the whole season arrived (not the release's episode range:
MoviePilot also reads episodes from the description, so a 25-episode pack saying 「修复第9集章节」 is E09 in its
search results and download history, and `/tasks` shows that label), else settles after quiet, or
`orphanGrace` after the download left `GET /api/v1/download/`; stall notices match it by hash

每个订阅或下载保留自己的 watch；同媒体同季的待发到货由 `due` / `settling.holds` / `flush` 一起等待和投递。独立 watch 不意味着每个下载必须各发一条消息。

## 频道与请求者投递

With `CRYCHIC_TELEGRAM_NOTIFY_CHAT_ID` set, `notify.Options.Destination` replaces requester delivery: one
notice per watch delivery to the channel, with no requester identity and no fallback to the originating chats.
Blank keeps the original behavior. The destination is a negative chat ID or `@channelusername`; the bot needs
channel posting rights. Delivery still persists before sending and does not replay failed sends.

## 完成日期游标与迁移

MoviePilot has no outgoing webhook, so arrivals come from polling successful transfer history (one record per
file, `seasons` "S03", `episodes` "E01" or "E01-E03"). v3.1.0 sorts by local completion `date` (`YYYY-MM-DD
HH:MM:SS`), not ID, and a failed transfer succeeding on retry updates the same row's date/status.
`TransferCursor` persists that date and all successful IDs seen in its second; reads include the entire
boundary second across pages. The first poll records the whole newest second as baseline, so earlier history
is never announced. The bot compares MoviePilot's civil dates as written, not with its own clock. The catch-up
remains bounded; an incomplete newest second at that bound is an error, never a committed checkpoint.

Old notification files with only `baseline`/`last_transfer` migrate in one atomic state update: still-unseen
IDs above the old watermark are processed once, all currently read successes seed the date cursor, and pending
notices, requests and digest entries are preserved. Ambiguous older IDs are not replayed during migration;
low-ID successes already missed before upgrading cannot be reliably distinguished from previously processed
history. An initialized empty cursor is persisted too, so the first later arrival is not swallowed as another
baseline.

状态入口为 `internal/notify/state.go`，游标入口为
`internal/notify/cursor.go`。`withRequest`、`arrive`、`flush`、`withActivity` 保持纯状态转换，写盘原子完成。

## 订阅身份范围与关闭

A MoviePilot subscription may close before files are transferred: a closed subscription must not immediately
end a watch. A completed download watch ends after its notice; a subscription watch keeps its requesters and
delivered episodes until `orphanGrace` (72h) after its subscription disappears. This preserves controls for
active upgrade subscriptions and notices for seasons whose episode counts grow. A watch whose row was last
read gone is `closed`: its last total bounds episodes again, other downloads of a delivered movie or a
complete season no longer count for stall notices, and a new request reusing its id (nothing pending) replaces
it instead of inheriting its requesters.

Before collecting new transfers or completing pending arrivals, the notifier reads their subscriptions and
refreshes the current total and `best_version`. A failed read preserves the cursor and pending state for a
read-only retry; a successful empty/replaced row preserves the last known scope. The initial
`Target.BestVersion` is persisted, so upgrades still notify when the backend closes the row before transfer.
New upgrade transfers may announce an already-delivered episode again, but the transfer cursor prevents
replay; TV upgrade notices never infer complete upgrading from previously held episodes. Ordinary movies
retain a delivered marker and notify once.

A subscription watch is identified by its row id **and** media source, media id and season; SQLite can reuse a
completed row id while its older files still await arrival. `withRequest` keeps those targets separate, and
derived `watch.key` values keep their library waits and activity independent. `Feed.Subscription` reads the
current row's identity from `GET /api/v1/subscribe/{id}`: a replacement does not keep the older watch active.
`Watcher.Forget` takes the cancelled `Subscription`, not just its id, so it removes only that target. The
persisted fields are unchanged; existing watches need no migration, but a new target already lost by the old
id-only merge cannot be reconstructed from that state.

旧指南曾把关闭时点简写为“下载完成”，同时记录了“种子加入后即计为取得”的机制。这里保留可验证的应用约束：关闭不等于转移入库，watch 不可立即结束。上游精确触发条件须按目标版本核验，不能仅据旧简写推断。

## 静默媒体库等待与持久化

MoviePilot transfers episodes one by one as their downloads finish, often minutes apart. A show's arrivals
stay pending (persisted) until nothing new came for `CRYCHIC_NOTIFY_QUIET`, then go out as one notice; a watch
whose every wanted episode is in (and any movie) is announced at once.

A settled notice is held until the media server shows the arrival (`exists_remote`/`notexists`; rclone/alist
dir caches can delay Emby by minutes), at most `CRYCHIC_NOTIFY_LIBRARY_WAIT`; the notice then links to the
item from `GET /api/v1/mediaserver/latest` (matched by title + year; `link` is the server's public web URL,
item `image` is an internal URL and is not used).

`LibraryWait=0` bypasses media-server checks. Movies and complete seasons start that wait immediately;
incomplete seasons start after quiet. Unchanged notifier state is not rewritten; necessary writes remain
serialized under the state lock

Notifier state is saved before notices are sent: a crash may drop a notice, never repeat one. Polling calls
are kept out of e2e transcripts; the notices they cause are recorded

## 洗版已持有集数与停滞

MoviePilot counts episodes as obtained once their torrent is added and closes the subscription then; it never
checks progress (`chain/subscribe/completion.py`), so a dead torrent would end in silence. Each poll also
reads `GET /api/v1/download/`: a non-paused download of a watched media/season (and, when known, a
still-missing episode) whose progress has not changed for `CRYCHIC_NOTIFY_STALL` is told about once
(`internal/notify/stall.go`). MoviePilot v3.1.0 collapses queuedDL into downloading, so the reminder says long
without progress and names queuing as a possibility, not a proven dead torrent. Progress marks live in memory
(a restart restarts the clocks, so no state write per poll); the told torrent hashes persist in the watch
(`stalled`), so a restart never repeats a stall notice

Held episodes never appear in transfer history, so a `Request` carries `Held`. Subscription watches count them
as delivered; download watches persist them separately (`held`) and use their union with actual arrivals only
for completion, so re-downloaded episodes still announce. An already fully held season does not seed Held for
a replacement download. Completion that used prior holdings says so, but does not retire the download watch
before its own files cover the season (or orphan grace expires), so later replacements of previously held
episodes still notify; old requests without Held are readable but cannot have the missing observation
reconstructed

## 每周汇总

`internal/notify/digest.go` - weekly digest (`Options.Digest`, a `Schedule`): while enabled, every successful
transfer of each poll (requested or not, title and year from the record) is kept in `state.Digest`; once the
schedule's latest moment is after `state.LastDigest`, the entries are cleared and `LastDigest` set before one
notice goes to `Destination` or each of `DigestTo` (title + season merged in first-arrival order, linked via
`GET /api/v1/mediaserver/latest`, at most 30 listed); the first check after enabling only sets `LastDigest`,
and an empty week sends nothing. Transitions keep fields they do not touch (`state.emptied`)

## 相关实现与测试

请求记录由 flow 交给 watcher；订阅历史重建不能借用之前卡片的季数或 Held，见 [历史规则](flow.md#历史与洗版)。

媒体库延迟、同 ID 转移重试、完整轮询屏障、日期分页和状态迁移的 fake 语义见 [E2E 参考](e2e.md#轮询日期与通知假服务)。
