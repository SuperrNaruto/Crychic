# Crychic

Go chat bot that searches media and creates subscriptions in MoviePilot (v3 API). Telegram ships; Discord is next. Scope: subscribe only, whitelist auth.

## Commands

| Command | Description |
|---------|-------------|
| `go build ./cmd/crychic` | Build the binary |
| `go vet ./... && gofmt -l .` | Lint; both must print nothing |
| `go test -race ./...` | Run all tests (they live only in `e2e/`) |
| `go test ./e2e/ -run TestSubscribeMovie` | Run one scenario |
| `docker build -t crychic .` | Build the image (distroless, uid 65532, data at `/data`); `compose.example.yaml` shows a deployment |
| `go test ./e2e/ -update` | Regenerate golden transcripts after an intended behavior change |

## Architecture

```
cmd/crychic/         # main: config.Load(os.Getenv) → app.Run
internal/app/        # the only wiring point; shared by main and e2e
internal/flow/       # platform-agnostic conversation: search → pick (library check) → season → confirm → subscribe
internal/moviepilot/ # minimal MoviePilot client, implements flow.Backend
internal/bangumi/    # Bangumi's public API (no key): airing calendar and synopses, implements flow.Calendar
internal/telegram/   # renders flow.Reply as messages + inline keyboards; whitelist check; delivers notices
internal/notify/     # remembers requests (data dir JSON), polls MoviePilot transfer history, notifies requesters
internal/config/     # env vars → config.Config
e2e/                 # behavior tests: real app vs fake Telegram Bot API + fake MoviePilot + fake Bangumi
```

Arrivals: `notify.Notifier` polls `GET /api/v1/history/transfer` → `arrive` records matches as pending (media + season + episode range) → `flush` announces once settled → `telegram.Bot.Notify` posts in the requesting chat, mentioning the requester in groups.

Data flow: Telegram update → `telegram.adapter` → `flow.Engine.Start/Choose` → `flow.Backend` (MoviePilot) → `flow.Reply` → `sendMessage` (command) or `editMessageText` (button) → `answerCallbackQuery` last.

## Key Files

- `internal/flow/model.go` - `Backend`, `Reply`, `UserError`: the contract every platform and backend uses
- `internal/flow/engine.go` - conversation steps; `internal/flow/text.go` - all user-facing copy (result lines, media card)
- `internal/flow/tasks.go` - `/tasks`: list downloads and transfer jobs, follow one live (`Reply.Follow`); `internal/telegram/follow.go` re-asks the flow every `CRYCHIC_PROGRESS_INTERVAL` and edits only on change
- `internal/flow/home.go` - `/start` home menu (`features`), typed title search; an empty task list opened from it returns to the menu; `internal/flow/charts.go` - `/hot` charts, paging, pick → subscribable result; 取消 on a pick returns to its chart page
- `internal/flow/charts.go` calendar: 新番放送 opens on today's weekday (China time, `calendarZone`), one weekday per view with a 一…日 button row, picks sorted by first air date within the day
- `internal/flow/notes.go` - `/hot` calendar pages: each pick looked up on TMDB (title, then original title) for link + identity, synopsis from TMDB else Bangumi's own `/v0/subjects/{id}` (`flow.Calendar.Summary`, kept on `Media.CalendarID`); all three lookups of every pick on a page run at once (one round trip); a pick found with a synopsis is done for the session, one without is looked up again whenever its page shows (MoviePilot answers an upstream failure with an empty `MediaInfo`, HTTP 200, which is why synopses skip it). Other charts show the synopsis the chart carries (Douban's is a region / genre / director / cast line), collapsed
- `internal/flow/related.go` - 相似推荐 / 同系列 on a media card (season picker, movie confirm, held, already subscribed): lists related media or the movie's series as results to pick from; nothing found or a failed lookup only flashes a notice and keeps the card
- `internal/flow/latest.go` - `/new`: newest media server items linked to their web page
- `internal/flow/subs.go` - `/subs`: every subscription, cancel only those the user requested (`Watcher.Requested`), then `Watcher.Forget`
- `internal/flow/seasons.go` - multi-season picker (「多选季…」): tick seasons, subscribe each from episode 1, report per season
- `internal/flow/episodes.go` - start-episode choices for seasons and typed answers (`Engine.Answer`)
- `internal/flow/rich.go` - `flow.Text`: formatting by meaning (bold, italic, code, link, collapsible quote)
- `internal/telegram/html.go` - renders `flow.Text` to Telegram HTML, escaping everything
- `internal/flow/store.go` - in-memory sessions (10 min TTL)
- `internal/flow/pages.go` - bounded pages for subscriptions, tasks and latest items; page buttons keep item indices stable
- `internal/telegram/lanes.go` - serializes each message's state transition and edit, including background refreshes
- `internal/notify/state.go` - pure state transitions (`withRequest`, `arrive`, `flush`, `withActivity`) and atomic persistence
- `e2e/harness_test.go` - `start`, `say`, `answer`, `answerQuoting`, `chatter`, `tap`, `tapData`, `shows`, `arrives`, `transfers`, `reports`, `restart`
- `e2e/testdata/transcripts/*.txt` - golden transcripts, one per scenario

## Environment

- `CRYCHIC_MOVIEPILOT_URL`, `CRYCHIC_MOVIEPILOT_API_KEY` (MoviePilot `API_TOKEN`), `CRYCHIC_TELEGRAM_TOKEN` - required
- `CRYCHIC_TELEGRAM_ALLOWED_USERS` - required, comma-separated user IDs; empty refuses to start
- `CRYCHIC_TELEGRAM_API_URL` - optional, defaults to `https://api.telegram.org`
- `CRYCHIC_BANGUMI_API_URL` - optional, defaults to `https://api.bgm.tv`; the 新番放送 calendar is read from it
- `CRYCHIC_DATA_DIR` - optional, defaults to `data`; holds `requests.json` (pending requests, last seen transfer)
- `CRYCHIC_NOTIFY_INTERVAL` - optional, defaults to `1m`, minimum `100ms`; how often transfer history is polled
- `CRYCHIC_NOTIFY_LIBRARY_WAIT` - optional, defaults to `30m`, `0s` disables; how long a settled notice waits for the media server to show the arrival
- `CRYCHIC_PROGRESS_INTERVAL` - optional, defaults to `5s`, minimum `100ms`; how often a followed `/tasks` view refreshes (Telegram throttles frequent edits)
- `CRYCHIC_NOTIFY_QUIET` - optional, defaults to `3m`, `0s` disables; how long a show's arrivals settle before one notice covers them

## Code Style

- `flow` must never import a platform package; a new platform is a sibling of `internal/telegram` that consumes `flow` through a small interface
- Inject dependencies (`Backend`, `Now`, `*http.Client`, `*slog.Logger`); construct concrete types only in `internal/app`
- User-facing copy is `flow.Text` built with `Strong`/`Emphasis`/`Mono`/`Linked`/`Quote`; never put platform markup in flow strings, adapters render and escape. Emoji sparingly: 🔍 search, 🎬/📺 card, ✅ done, ℹ️ already subscribed, ⬇️/⏸️ download progress, 📦 transfer job (⏳ ▶️ ✅ ⚠️ per file), 📋 task list, 📥 arrival, ⚠️ errors, ⌛ expired, 🚫 refused
- Errors safe to show users are `*flow.UserError`; others get logged and shown as the generic outage text
- Hard limits: functions ≤ 50 lines, nesting ≤ 3, ≤ 3 positional params (use a struct, e.g. `moviepilot.call`), complexity ≤ 10, no magic numbers

## Testing

- **Agents never write to a real MoviePilot**: no `POST /api/v1/subscribe/` and no bot runs that end in a subscription. A real subscription makes MoviePilot search and download from the owner's PT sites and can get the account banned. Agents' live calls are limited to read-only metadata endpoints; write paths are verified through the fakes and MoviePilot's source. The owner may test the live bot by hand
- End-to-end only: no unit tests, no mocks of internal packages, no redundant or change-detection tests
- Every scenario ends with `h.tr.verify(t)`; after `-update`, review the transcript diff line by line and commit it with the code
- Press buttons with `tap(user, msgID, label)` so a button missing from screen fails; use `tapData` only for deliberately stale buttons
- The fake MoviePilot answers the library and downloader checks as an idle server (`idleServer`); scenarios override `libraryShowPath`/`libraryMoviePath`/`downloadsPath`
- Recordings of `/api/v1/download/` carry tracker URLs with passkeys, the site name, the release name and the owner's username: keep only the fields Crychic reads
- Fakes must behave like the real services (long-poll `getUpdates`, `X-API-KEY` check, real error shapes); extend them, don't shortcut them
- `e2e/testdata/bangumi/calendar.json` is a trimmed recording of `api.bgm.tv/calendar` (all seven weekday groups, a few shows, only fields Crychic reads); `subject_<id>.json` record `/v0/subjects/<id>` the same way and `not_found.json` is Bangumi's live 404. `h.bgm.setDown(id, true)` makes a subject answer 503. Scenarios start on `epoch`, Monday 2026-10-05 noon China time, and the clock runs at real speed from there (`app.Deps.Now`), so "today" is stable
- `e2e/testdata/moviepilot/*.json` are trimmed recordings from a live v3.1.0 instance (`curl -H "X-API-KEY: $KEY" "$MP/api/v1/media/search?title=沙丘&type=media&count=10"`), except `subscribe_rejected.json`, `server_error.json`, `library_movie_held.json` (the live library holds no movie yet), `subscriptions.json`, `subscribe_deleted.json` (no live subscription may be created to record them; `subscriptions.json` copies the shape of recorded subscription history); `latest.json` is a live recording with the owner's Emby domain replaced by `emby.example.com`; `subscribe_created.json` came from a one-off live subscription that must not be repeated (see above); never commit real keys or tokens

## Gotchas

- The API key authenticates as MoviePilot's **superuser**; every subscription is owned by the admin, not the chat user
- Media `type` is a Chinese enum: `电影`, `电视剧`; results without `media_source`/`media_id` are dropped
- Every JSON endpoint answers `{success, message, data}`, even where the route's `response_model` says `List[...]` (wrapped by `app/api/response.py`); `moviepilot.Client.do` unwraps it, and `success: false` on HTTP 200 becomes a `UserError` with `message`
- `GET /api/v1/subscribe/media/{id}` returns `data` with `id: null` when nothing is subscribed, not a 404
- Posters render as a large link preview above the text (`telegram.preview`): text messages can't carry photos and can't be edited into photo messages; a reply without `Image` explicitly disables the preview so an old poster doesn't linger
- Poster URLs from MoviePilot are TMDB `original` size (MBs); `moviepilot` rewrites them to `w500`
- `GET /api/v1/media/{id}` details are cosmetic: on failure the card falls back to search metadata and the flow continues; its `directors` field mixes in producers and episode directors, so it isn't shown
- Subscriptions take `start_episode` (per season): MoviePilot skips earlier episodes. "只追新集" uses details' `next_episode_to_air` and only shows when it is in the chosen season and > 1; button data `ok:<n>` carries the start, validated against the season's episode count before the session is taken
- The bot registers its command menu (`setMyCommands`, `telegram.commands`) on every start, for the default scope and for each whitelisted user's chat (a chat-scoped menu outranks the default; the owner's token carried a stale one from an earlier program); keep it in step with `parseCommand`'s commands and `help`
- Typed answers: a `Reply` with `Input` makes the Telegram adapter remember (chat, user) → message in `adapter.pending`; private chats take any plain text from that user, groups only a reply quoting the message
- MoviePilot has no outgoing webhook, so arrivals come from polling transfer history (one record per file, newest first, `seasons` "S03", `episodes` "E01" or "E01-E03"). The first poll only records the latest id (baseline), so history from before Crychic is never announced
- MoviePilot closes a subscription when downloads finish, before files are transferred: a closed subscription must not end a watch. Watches end when every wanted episode arrived (movies: first arrival) or after `orphanGrace` (72h) with the subscription gone
- MoviePilot transfers episodes one by one as their downloads finish, often minutes apart. A show's arrivals stay pending (persisted) until nothing new came for `CRYCHIC_NOTIFY_QUIET`, then go out as one notice; a watch whose every wanted episode is in (and any movie) is announced at once. The e2e default is `300ms`; `transfers` waits until a whole poll handled the records, so a test can act between polls
- A settled notice is held until the media server shows the arrival (`exists_remote`/`notexists`; rclone/alist dir caches can delay Emby by minutes), at most `CRYCHIC_NOTIFY_LIBRARY_WAIT`; the notice then links to the item from `GET /api/v1/mediaserver/latest` (matched by title + year; `link` is the server's public web URL, item `image` is an internal URL and is not used). The fake media server shows every transfer on top of the scenario's library fixture; `scenario.lagging` + `h.catchUp` model a server that has not scanned yet. Library and latest reads are polled, so they stay out of transcripts
- Notifier state is saved before notices are sent: a crash may drop a notice, never repeat one. Polling calls are kept out of e2e transcripts; the notices they cause are recorded
- Button data is `<session>:<action>:<arg>` and must stay ≤ 64 bytes (Telegram limit). A stale button only has its action to go on, so `/tasks` buttons use their own actions (`k f u l c`) and `flow.expired` points them back to `/tasks`, everything else to `/request`
- The fake numbers subscriptions created from `subscribe_created.json` 1, 2, 3, ... like MoviePilot; two watches sharing an id would merge
- Confirm and cancel use `store.take`, so a double tap acts once; keep that for any step that ends a conversation
- Session IDs are random across restarts; never replace them with a counter that resets on startup. `Choose`/`Answer` validate both user and chat and serialize steps per session. Calendar annotations clone both mutable slices before writing. E2E transcripts normalize opaque session IDs only when recording, never in updates sent to the app
- Callback action strings are unique across every flow handler. Subscription return is `j`, calendar weekday is `w`, generic list paging is `p`, 相似推荐 is `r` and 同系列 is `n`
- Telegram non-poll requests have a 10-second deadline including response-body reads; `getUpdates` uses the HTTP client's 70-second timeout. Metadata card enrichment uses a 5-second cosmetic budget; independent read-only calls overlap, subscription writes do not
- List pages budget text and item buttons together (up to 20 entries and 3000 content runes). Large transfer views show at most 100 file lines, retaining the full aggregate count and explicitly naming omitted files
- `LibraryWait=0` bypasses media-server checks. Movies and complete seasons start that wait immediately; incomplete seasons start after quiet. Unchanged notifier state is not rewritten; necessary writes remain serialized under the state lock
- A `Reply` with `Notice` must not edit the message (another group member tapping your buttons)
- Library checks go through MoviePilot to its media server and are cosmetic like details (failure = nothing held). Shows use `POST /api/v1/mediaserver/exists_remote` (`{"3":[1,2,...]}`, `{}` when absent; needs `title`+`year`, ids alone answer `{}`). Movies use `POST /api/v1/mediaserver/notexists` (`[]` = held), because `exists_remote` answers `{}` for movies either way. `GET /api/v1/mediaserver/exists` reads MoviePilot's own sync table, which is empty unless library sync runs, so it isn't used
- `GET /api/v1/download/` lists unfinished torrents; `media` (source, id, `season` "S01", `episode` "E10-E12") comes from MoviePilot's download history and is null for torrents added by hand; `progress` is a percentage, `state` is `downloading` or `paused`, `left_time` is Chinese text like `1时5分3秒` (empty while stalled). The card shows the chosen target's downloads, cosmetic like details
- `/tasks` follows a task by re-reading `GET /api/v1/download/` (by torrent hash) or `GET /api/v1/transfer/queue` (by media + season; tasks are `waiting`/`running`/`completed`/`failed`, episode in `meta.begin_episode`). Byte progress of a transfer is only an SSE stream behind a browser cookie (`/system/progress/filetransfer`), so transfers show per-file states. A view stops when its task leaves the list, when its owner presses 停止刷新, or after `flow.FollowFor` (10 min); any button press on a followed message cancels its follower first. Every task view has 返回任务列表, which re-reads both lists in the same session (a finished view keeps its session for that). Both lists are polled, so the fake keeps them out of transcripts and `reports` swaps their fixture
- `GET /api/v1/subscribe/` lists all subscriptions for the superuser key (`state` N/R/P/S, `lack_episode`, `total_episode`); `DELETE /api/v1/subscribe/{id}` answers 404 for one already gone, which `Unsubscribe` treats as done. Deleting only reduces downloads, so it is the one live write the bot offers besides subscribing
- Chart picks (`/api/v1/recommend/*`) are search-shaped but Douban/Bangumi ones carry Douban/Bangumi ids, while transfer history only carries TMDB ids; a pick is therefore searched by title and only taken directly when the identity or a unique title+year+kind matches, else the user picks. Douban season titles like 「流人 第六季」 often find nothing right. MoviePilot's Bangumi calendar (`/api/v1/recommend/bangumi_calendar`) loses the weekday grouping and serves 30 items a page (Monday to Wednesday), so the bot reads `api.bgm.tv/calendar` itself (summaries there are always empty; synopses come from TMDB, else Bangumi's subject API directly). A TMDB twin keeps its pick's `Weekday` and air date. Fake routes are keyed by path, so a scenario needing two searches swaps `searchPath` with `h.mp.setRoute`, or answers per title with `scenario.searches`. Reads the bot makes at once (calendar lookups) arrive in any order, so the transcript sorts each run of consecutive reads of one route (`transcript.settle`; media searches and details count as one route, searches first)
- 相似推荐 reads `GET /api/v1/tmdb/recommend/{tmdbid}/{电影|电视剧}` (TMDB media only; `type_name` must be the Chinese enum, `movie` answers 未知错误); `/tmdb/similar` is not used, its picks are poor. MoviePilot's details leave `collection_id` null (it reads TMDB's `belongs_to_collection` under the wrong name and drops `tmdb_info`), so 同系列 searches collections (`/api/v1/media/search?type=collection`, items `type` 系列) by the movie's titles and their stems (沙丘2 → 沙丘, `Dune: Part Two` → `Dune`), opens up to 3 candidates per name (`/api/v1/tmdb/collection/{id}`) and takes the one listing the movie; a series named unlike its films (哈利·波特与魔法石) is not found. The fake answers collection searches per title (`collectionSearch(title)`, `[]` otherwise, like MoviePilot), and the transcript keeps them in call order
- Held episodes never appear in transfer history, so a `Request` carries `Held` and the watch counts them as delivered; MoviePilot's subscriptions themselves only download missing episodes
- When MoviePilot behavior is unclear, read its source (`app/api/endpoints/{media,subscribe}.py`) instead of guessing

## Workflow

- Commit each finished task on its own; never push unless asked
