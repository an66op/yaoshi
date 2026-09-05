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

The current 163 endpoint is plain HTTP. Its anonymous request signature checks
request parameters but does not provide TLS response-transport integrity. The
strict history and cross-source gates below fail closed on detectable data
errors, but they do not remove that infrastructure risk.

## 163 high-frequency mother source

Seven matched products use the 163 directory's verified 168-mirror IDs as the
production mother source: `56`, `61`, `58`, `38`, `33`, `55` and `31`. These
IDs preserve the existing product results; the similarly named 163-official
IDs are different lotteries and must not be substituted. Each poll verifies a
fresh latest row against bounded history before an import. Existing 168 rows
keep their original timestamp and source revision; a same-issue number conflict
fails closed. The old `168-highfreq` group is not scheduled or exposed as a
manual sync group, so two writers cannot race.

SG SSC has its own versioned writer described below. It uses 163 ID `64`, not
the different lottery exposed as ID `169`.

## 163 Canada28 mother source

`pc-canada`, `canada-28` and `canada-20` are three rule/odds versions of the
same Canada28 draw. The `163-pc28` job fetches ID `57` once and writes the same
ordered three digits to all three games with independent immutable source
snapshots. Production validation requires 0-9 digits and an observed interval
of exactly 210 seconds; the former 120-second inference for “2.0” is retired.

## 163 Taiwan Bingo mother source

The seven Bingo-derived products use 163 ID `185` as the ordered 20-ball mother
source and ID `135` as the same-period sorted-set check. Every issue must have
the same draw time and exact 20-ball set on both endpoints before any derived
game can be written. The four SSC games consume positions 1-5, 6-10, 11-15 and
16-20 respectively; Racing A ranks the first ten balls and Racing B ranks the
last ten. Bingo Mark Six keeps the first seven ordered values in 1-49. The
former `jyb.one` path and sorted-offset conversions are historical only.

The `163-bingo` scheduler is the sole production writer. The old `168-bingo`
group is neither scheduled nor accepted by the manual sync endpoint. Every new
draw stores a source and conversion revision; old rows keep their original
provenance, and any same-issue conflict with unresolved financial evidence
fails closed.

## 163 direct Mark Six sources

Hong Kong, Happy8, New Macau and Old Macau Mark Six use direct seven-ball 163
IDs `18`, `141`, `140` and `70`. Each product has its own immutable source,
conversion and rules revisions even though the current validated play catalogue
is shared. The `163-marksix` reader requires the latest result to occur in at
least seven bounded history rows, seven unique values in 1-49, consecutive
issues, increasing timestamps and an explicit next issue/open time. Happy8 and
both Macau products additionally require exact 24-hour boundaries. A database
source binding mismatch is unhealthy for betting even if `sync_status` still
says `ok`; no manual, random or platform fallback is permitted.

## SG SSC verified feed

SG SSC uses a separate `sg-ssc-verified` job. 163 ID `64` is the only source
allowed to supply numbers; 115 product `sgssc` is a verifier which can reject a
batch but can never fill one. The latest result, next-period metadata and a
continuous 24-period history window must match exactly before import. A
timeout, mismatch, missing period or stale schedule pauses betting; there is no
single-source or platform-generated fallback. ID `169` is a different result
system and must never be mixed with ID `64`.

See [the source and cutover contract](../../docs/SG时时彩双站接入-2026-09-03.md)
for midnight numbering, legacy-ticket isolation, limits and the opt-in live test.

Historical debt is handled separately by `services.StartSGSSCBackfill`, not by
relaxing this live window. It uses a durable per-issue queue/attempt journal,
up to 48 targets across two dates, and requires both endpoints for each result.
Requests outside the finite history window that 163 can actually prove are
blocked for manual handling instead of being presented as a 30-day guarantee.
It cannot update live Game health or next-period scheduling.
See [historical recovery](../../docs/SG历史补采与恢复-2026-09-03.md) for restart
fencing, settlement guards, admin-only queueing and the read-only live probe.
