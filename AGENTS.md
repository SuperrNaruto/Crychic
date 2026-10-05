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
internal/telegram/   # renders flow.Reply as messages + inline keyboards; whitelist check
internal/config/     # env vars → config.Config
e2e/                 # behavior tests: real app vs fake Telegram Bot API + fake MoviePilot
```

Data flow: Telegram update → `telegram.adapter` → `flow.Engine.Start/Choose` → `flow.Backend` (MoviePilot) → `flow.Reply` → `sendMessage` (command) or `editMessageText` (button) → `answerCallbackQuery` last.

## Key Files

- `internal/flow/model.go` - `Backend`, `Reply`, `UserError`: the contract every platform and backend uses
- `internal/flow/engine.go` - conversation steps and all user-facing copy
- `internal/flow/store.go` - in-memory sessions (10 min TTL)
- `e2e/harness_test.go` - `start`, `say`, `tap`, `tapData`, `shows`
- `e2e/testdata/transcripts/*.txt` - golden transcripts, one per scenario

## Environment

- `CRYCHIC_MOVIEPILOT_URL`, `CRYCHIC_MOVIEPILOT_API_KEY` (MoviePilot `API_TOKEN`), `CRYCHIC_TELEGRAM_TOKEN` - required
- `CRYCHIC_TELEGRAM_ALLOWED_USERS` - required, comma-separated user IDs; empty refuses to start
- `CRYCHIC_TELEGRAM_API_URL` - optional, defaults to `https://api.telegram.org`

## Code Style

- `flow` must never import a platform package; a new platform is a sibling of `internal/telegram` that consumes `flow` through a small interface
- Inject dependencies (`Backend`, `Now`, `*http.Client`, `*slog.Logger`); construct concrete types only in `internal/app`
- Errors safe to show users are `*flow.UserError`; others get logged and shown as the generic outage text
- Hard limits: functions ≤ 50 lines, nesting ≤ 3, ≤ 3 positional params (use a struct, e.g. `moviepilot.call`), complexity ≤ 10, no magic numbers

## Testing

- End-to-end only: no unit tests, no mocks of internal packages, no redundant or change-detection tests
- Every scenario ends with `h.tr.verify(t)`; after `-update`, review the transcript diff line by line and commit it with the code
- Press buttons with `tap(user, msgID, label)` so a button missing from screen fails; use `tapData` only for deliberately stale buttons
- Fakes must behave like the real services (long-poll `getUpdates`, `X-API-KEY` check, real error shapes); extend them, don't shortcut them
- `e2e/testdata/moviepilot/*.json` are hand-written from v3.1.0 models, not yet recorded from a live instance; replace with real responses when one is available (`curl -H "X-API-KEY: $KEY" "$MP/api/v1/media/search?title=沙丘&type=media&count=10"`), never committing real keys or tokens

## Gotchas

- The API key authenticates as MoviePilot's **superuser**; every subscription is owned by the admin, not the chat user
- Media `type` is a Chinese enum: `电影`, `电视剧`; results without `media_source`/`media_id` are dropped
- `GET /api/v1/subscribe/media/{id}` returns an object with `id: null` when nothing is subscribed, not a 404
- `POST /api/v1/subscribe/` reports failure as HTTP 200 with `success: false`; relay its `message`
- Button data is `<session>:<action>:<arg>` and must stay ≤ 64 bytes (Telegram limit)
- Confirm and cancel use `store.take`, so a double tap acts once; keep that for any step that ends a conversation
- A `Reply` with `Notice` must not edit the message (another group member tapping your buttons)
- When MoviePilot behavior is unclear, read its source (`app/api/endpoints/{media,subscribe}.py`) instead of guessing

## Workflow

- Commit each finished task on its own; never push unless asked
