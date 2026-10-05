# Notes for coding agents

Crychic is a chat bot that subscribes media in MoviePilot. Telegram ships
today; Discord is next. Read README.md for usage and layout; this file holds
what the code doesn't say.

## Commands

```sh
go vet ./... && gofmt -l .          # must be clean
go test -race ./...                 # all tests live in e2e/
go test ./e2e/ -update              # regenerate golden transcripts
```

## Architecture rules

- **`internal/flow` knows no platform.** It speaks `Reply{Text, Buttons,
  Notice}` and opaque button data. Anything Telegram- or Discord-specific
  (message editing, keyboards, user IDs' meaning, whitelists) belongs in the
  adapter. A new platform is a new package beside `internal/telegram` that
  depends on `flow` through a small interface; `flow` itself should not change.
- **`internal/app` is the only wiring point.** `main` and the e2e harness both
  call `app.Run`, so tests exercise what ships. Don't construct parts
  elsewhere, and don't add test-only branches to `app`.
- **Dependencies are injected** (`flow.Backend`, `telegram.Flow`, `Now`,
  `*http.Client`, `*slog.Logger`). Keep it that way.
- **Sessions are values.** `flow/store.go` copies sessions in and out; steps
  that end a conversation use `take` so a double tap acts once. Button data is
  `<session>:<action>:<arg>` and must stay under Telegram's 64-byte limit.
- **User-facing errors** are `*flow.UserError`; anything else is logged and
  shown as the generic "MoviePilot 暂时不可用".
- **Notice vs. edit:** a `Reply` with `Notice` must not touch the
  conversation message (used when someone else taps your buttons in a group).

## MoviePilot facts (verified against v3.1.0 source, not docs)

- `X-API-KEY: <API_TOKEN>` authenticates as the superuser
  (`app/adapters/web/security/access.py: verify_token`). Subscriptions are
  therefore owned by the admin, not the chat user.
- Search: `GET /api/v1/media/search?title=&type=media&count=`. Results carry
  `type` as Chinese enum values (`电影`, `电视剧`), plus `media_source` /
  `media_id` as the identity. Items without an identity are dropped.
- Seasons: `GET /api/v1/media/seasons?media_source=&media_id=`.
- Existing subscription: `GET /api/v1/subscribe/media/{media_id}?media_source=…`.
  "None" is an **empty subscription object with `id: null`**, not a 404.
- Create: `POST /api/v1/subscribe/` → `{success, message, data:{id}}`.
  `success: false` is a 200; relay `message` to the user.
- 401/403 means a wrong API key and is shown to the user as such.

When behavior here is in doubt, read the MoviePilot source
(`app/api/endpoints/{media,subscribe}.py`) rather than guessing.

## Testing rules

- **End-to-end only.** Tests drive real user actions (`say`, `tap`) through
  the real app against `httptest` fakes of the Telegram Bot API and
  MoviePilot. No unit tests, no mocks of internal packages.
- **Golden transcripts are the artifact.** Every scenario ends with
  `h.tr.verify(t)`. After an intended behavior change, run `-update`, read the
  transcript diff line by line, and commit it with the code. Never run
  `-update` to silence a failure you haven't understood.
- **One scenario per distinct behavior.** No redundant or change-detection
  tests; no regression test for a bug unless it covers a genuine gap.
- **`tap` by label, not data.** A button that isn't on screen fails the test.
  Use `tapData` only to simulate stale buttons on purpose.
- The fakes must behave like the real services (long-poll `getUpdates`,
  API-key check, MoviePilot's error shapes). Extend them, don't shortcut them.

### Fixtures

`e2e/testdata/moviepilot/*.json` were hand-written from MoviePilot v3.1.0
response models and **have not yet been checked against a live instance**.
To calibrate, record real responses and replace the fixtures:

```sh
curl -s -H "X-API-KEY: $KEY" "$MP/api/v1/media/search?title=沙丘&type=media&count=10"
curl -s -H "X-API-KEY: $KEY" "$MP/api/v1/media/seasons?media_source=themoviedb&media_id=1396"
curl -s -H "X-API-KEY: $KEY" "$MP/api/v1/subscribe/media/438631?media_source=themoviedb"
```

Trim to the fields worth keeping, keep the shape exact, then rerun the tests.
Never put a real API key or bot token in fixtures or transcripts.

## Code limits

Functions ≤ 50 lines, nesting ≤ 3, ≤ 3 positional parameters (use a struct,
see `moviepilot.call`), cyclomatic complexity ≤ 10, no magic numbers. Comments
only for non-obvious intent. Remove dead code rather than keeping fallbacks.

## Commits

Commit each finished task on its own; don't push unless asked.
