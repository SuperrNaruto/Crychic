# Crychic

Go chat bot that searches media and creates subscriptions in MoviePilot (v3 API). Telegram ships; Discord is next. Scope: subscribe only, whitelist auth.

## Commands

| Command | Description |
|---------|-------------|
| `go build ./cmd/crychic` | Build the binary |
| `go vet ./... && gofmt -l .` | Lint; both must print nothing |
| `go test -race ./...` | Run all tests (they live only in `e2e/`) |
| `go test ./e2e/ -run TestSubscribeMovie` | Run one scenario |
| `go test ./e2e/ -update` | Regenerate golden transcripts after an intended behavior change |

## Architecture

```
cmd/crychic/         # main: config.Load(os.Getenv) → app.Run
internal/app/        # the only wiring point; shared by main and e2e
internal/flow/       # platform-agnostic conversation: search → pick → season → confirm → subscribe
internal/moviepilot/ # minimal MoviePilot client, implements flow.Backend
internal/telegram/   # renders flow.Reply as messages + inline keyboards; whitelist check; delivers notices
internal/notify/     # remembers requests (data dir JSON), polls MoviePilot transfer history, notifies requesters
internal/config/     # env vars → config.Config
e2e/                 # behavior tests: real app vs fake Telegram Bot API + fake MoviePilot
```

Arrivals: `notify.Notifier` polls `GET /api/v1/history/transfer` → `arrive` records matches as pending (media + season + episode range) → `flush` announces once settled → `telegram.Bot.Notify` posts in the requesting chat, mentioning the requester in groups.

Data flow: Telegram update → `telegram.adapter` → `flow.Engine.Start/Choose` → `flow.Backend` (MoviePilot) → `flow.Reply` → `sendMessage` (command) or `editMessageText` (button) → `answerCallbackQuery` last.

## Key Files

- `internal/flow/model.go` - `Backend`, `Reply`, `UserError`: the contract every platform and backend uses
- `internal/flow/engine.go` - conversation steps; `internal/flow/text.go` - all user-facing copy (result lines, media card)
- `internal/flow/episodes.go` - start-episode choices for seasons and typed answers (`Engine.Answer`)
- `internal/flow/rich.go` - `flow.Text`: formatting by meaning (bold, italic, code, link, collapsible quote)
- `internal/telegram/html.go` - renders `flow.Text` to Telegram HTML, escaping everything
- `internal/flow/store.go` - in-memory sessions (10 min TTL)
- `internal/notify/state.go` - pure state transitions (`withRequest`, `arrive`, `flush`, `withActivity`) and atomic persistence
- `e2e/harness_test.go` - `start`, `say`, `answer`, `chatter`, `tap`, `tapData`, `shows`, `arrives`, `transfers`, `restart`
- `e2e/testdata/transcripts/*.txt` - golden transcripts, one per scenario

## Environment

- `CRYCHIC_MOVIEPILOT_URL`, `CRYCHIC_MOVIEPILOT_API_KEY` (MoviePilot `API_TOKEN`), `CRYCHIC_TELEGRAM_TOKEN` - required
- `CRYCHIC_TELEGRAM_ALLOWED_USERS` - required, comma-separated user IDs; empty refuses to start
- `CRYCHIC_TELEGRAM_API_URL` - optional, defaults to `https://api.telegram.org`
- `CRYCHIC_DATA_DIR` - optional, defaults to `data`; holds `requests.json` (pending requests, last seen transfer)
- `CRYCHIC_NOTIFY_INTERVAL` - optional, defaults to `1m`, minimum `100ms`; how often transfer history is polled
- `CRYCHIC_NOTIFY_QUIET` - optional, defaults to `3m`, `0s` disables; how long a show's arrivals settle before one notice covers them

## Code Style

- `flow` must never import a platform package; a new platform is a sibling of `internal/telegram` that consumes `flow` through a small interface
- Inject dependencies (`Backend`, `Now`, `*http.Client`, `*slog.Logger`); construct concrete types only in `internal/app`
- User-facing copy is `flow.Text` built with `Strong`/`Emphasis`/`Mono`/`Linked`/`Quote`; never put platform markup in flow strings, adapters render and escape. Emoji sparingly: 🔍 search, 🎬/📺 card, ✅ done, ℹ️ already subscribed, ⚠️ errors, ⌛ expired, 🚫 refused
- Errors safe to show users are `*flow.UserError`; others get logged and shown as the generic outage text
- Hard limits: functions ≤ 50 lines, nesting ≤ 3, ≤ 3 positional params (use a struct, e.g. `moviepilot.call`), complexity ≤ 10, no magic numbers

## Testing

- **Agents never write to a real MoviePilot**: no `POST /api/v1/subscribe/` and no bot runs that end in a subscription. A real subscription makes MoviePilot search and download from the owner's PT sites and can get the account banned. Agents' live calls are limited to read-only metadata endpoints; write paths are verified through the fakes and MoviePilot's source. The owner may test the live bot by hand
- End-to-end only: no unit tests, no mocks of internal packages, no redundant or change-detection tests
- Every scenario ends with `h.tr.verify(t)`; after `-update`, review the transcript diff line by line and commit it with the code
- Press buttons with `tap(user, msgID, label)` so a button missing from screen fails; use `tapData` only for deliberately stale buttons
- Fakes must behave like the real services (long-poll `getUpdates`, `X-API-KEY` check, real error shapes); extend them, don't shortcut them
- `e2e/testdata/moviepilot/*.json` are trimmed recordings from a live v3.1.0 instance (`curl -H "X-API-KEY: $KEY" "$MP/api/v1/media/search?title=沙丘&type=media&count=10"`), except `subscribe_rejected.json` and `server_error.json`, which follow the source; `subscribe_created.json` came from a one-off live subscription that must not be repeated (see above); never commit real keys or tokens

## Gotchas

- The API key authenticates as MoviePilot's **superuser**; every subscription is owned by the admin, not the chat user
- Media `type` is a Chinese enum: `电影`, `电视剧`; results without `media_source`/`media_id` are dropped
- Every JSON endpoint answers `{success, message, data}`, even where the route's `response_model` says `List[...]` (wrapped by `app/api/response.py`); `moviepilot.Client.do` unwraps it, and `success: false` on HTTP 200 becomes a `UserError` with `message`
- `GET /api/v1/subscribe/media/{id}` returns `data` with `id: null` when nothing is subscribed, not a 404
- Posters render as a large link preview above the text (`telegram.preview`): text messages can't carry photos and can't be edited into photo messages; a reply without `Image` explicitly disables the preview so an old poster doesn't linger
- Poster URLs from MoviePilot are TMDB `original` size (MBs); `moviepilot` rewrites them to `w500`
- `GET /api/v1/media/{id}` details are cosmetic: on failure the card falls back to search metadata and the flow continues; its `directors` field mixes in producers and episode directors, so it isn't shown
- Subscriptions take `start_episode` (per season): MoviePilot skips earlier episodes. "只追新集" uses details' `next_episode_to_air` and only shows when it is in the chosen season and > 1; button data `ok:<n>` carries the start, validated against the season's episode count before the session is taken
- Typed answers: a `Reply` with `Input` makes the Telegram adapter remember (chat, user) → message in `adapter.pending`; private chats take any plain text from that user, groups only a reply quoting the message
- MoviePilot has no outgoing webhook, so arrivals come from polling transfer history (one record per file, newest first, `seasons` "S03", `episodes` "E01" or "E01-E03"). The first poll only records the latest id (baseline), so history from before Crychic is never announced
- MoviePilot closes a subscription when downloads finish, before files are transferred: a closed subscription must not end a watch. Watches end when every wanted episode arrived (movies: first arrival) or after `orphanGrace` (72h) with the subscription gone
- MoviePilot transfers episodes one by one as their downloads finish, often minutes apart. A show's arrivals stay pending (persisted) until nothing new came for `CRYCHIC_NOTIFY_QUIET`, then go out as one notice; a watch whose every wanted episode is in (and any movie) is announced at once. The e2e default is `300ms`; `transfers` waits until a whole poll handled the records, so a test can act between polls
- Notifier state is saved before notices are sent: a crash may drop a notice, never repeat one. Polling calls are kept out of e2e transcripts; the notices they cause are recorded
- Button data is `<session>:<action>:<arg>` and must stay ≤ 64 bytes (Telegram limit)
- Confirm and cancel use `store.take`, so a double tap acts once; keep that for any step that ends a conversation
- A `Reply` with `Notice` must not edit the message (another group member tapping your buttons)
- When MoviePilot behavior is unclear, read its source (`app/api/endpoints/{media,subscribe}.py`) instead of guessing

## Workflow

- Commit each finished task on its own; never push unless asked
