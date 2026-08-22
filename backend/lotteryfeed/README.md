# Official draw feed

`lotteryfeed` is the provider-neutral scheduler for official draw results. It
is intentionally isolated from Gin and GORM so the runtime can be extracted
without rewriting its timing and retry behaviour.

## Runtime modes

- The main backend embeds the scheduler and exposes the read-only feed under
  `/api/public`.
- `go run ./cmd/drawfeed` starts a results-only service on port `8081` (or
  `DRAWFEED_PORT`). It does not register account, admin, or betting routes.

## Public contract

- `GET /api/public/clock`
- `GET /api/public/lottery/status`
- `GET /api/public/lottery/games`
- `GET /api/public/lottery/games/:id/draws?limit=30`
- `GET /api/public/lottery/latest`

The browser calibrates its clock against `/clock`. During official draw
windows, provider jobs switch to short polling; outside those windows they use
a low-frequency backfill check. Every insert is idempotent on `(game_id,
issue)`, so retries and multiple readers cannot create duplicate draws.

Official websites publish results after their draw process completes. The
service therefore follows the provider publication time rather than inventing
numbers at a locally predicted second.
