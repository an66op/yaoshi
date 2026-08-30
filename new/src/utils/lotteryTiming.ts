export type LotteryPhase = 'accepting' | 'sealed' | 'awaiting_draw' | 'settling' | 'settled' | 'pending' | 'error' | 'unavailable'

/** The server owns periods and their boundaries; a browser never rolls a period forward. */
export type LotteryTimingInput = {
  next_draw_at?: string | null
  seal_at?: string | null
  accept_at?: string | null
  draw_interval?: number | null
  seal_seconds?: number | null
  issue_status?: string | null
  source_healthy?: boolean
  enabled?: boolean
}

export type LotteryTiming = {
  phase: LotteryPhase
  phaseLabel: string
  statusLabel: string
  accepting: boolean
  due: string
  remainingSeconds: number | null
  drawAtMs: number | null
  sealAtMs: number | null
  acceptAtMs: number | null
  intervalSeconds: number | null
  sealSeconds: number | null
}

function timestampMillis(value?: string | null) {
  // The API emits RFC3339. Locale-dependent timestamps and Go's zero value
  // must not turn into a plausible betting deadline on a different device.
  if (!value || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/i.test(value)) return null
  const result = Date.parse(value)
  return Number.isFinite(result) && result > 0 ? result : null
}

function finiteSeconds(value: number | null | undefined, allowZero = false) {
  return typeof value === 'number' && Number.isFinite(value) && (allowZero ? value >= 0 : value > 0) ? value : null
}

export function formatLotteryCountdown(seconds: number | null) {
  if (seconds === null || !Number.isFinite(seconds)) return '--:--'
  const total = Math.max(0, Math.ceil(seconds))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const remainder = total % 60
  const clock = `${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`
  return hours > 0 ? `${String(hours).padStart(2, '0')}:${clock}` : clock
}

/**
 * Count down to seal while accepting, then to the draw while sealed. An
 * expired timestamp never starts a synthetic next round. Non-accepting server
 * states always win, including a previous reconciliation/source error.
 */
export function resolveLotteryTiming(input: LotteryTimingInput, nowMs: number): LotteryTiming {
  const drawAtMs = timestampMillis(input.next_draw_at)
  const configuredSealSeconds = finiteSeconds(input.seal_seconds, true)
  const explicitSeal = input.seal_at != null && input.seal_at !== ''
  const sealAtMs = explicitSeal
    ? timestampMillis(input.seal_at)
    : drawAtMs !== null && configuredSealSeconds !== null
      ? drawAtMs - configuredSealSeconds * 1000
      : null
  const acceptAtMs = timestampMillis(input.accept_at)
  const intervalSeconds = finiteSeconds(input.draw_interval)
  const validBoundaries = drawAtMs !== null && sealAtMs !== null && sealAtMs > 0 && sealAtMs <= drawAtMs
    && (!(input.accept_at != null && input.accept_at !== '') || (acceptAtMs !== null && acceptAtMs <= sealAtMs))
  const sealSeconds = validBoundaries ? (drawAtMs - sealAtMs) / 1000 : configuredSealSeconds
  const secondsUntil = (atMs: number) => Math.max(0, Math.ceil((atMs - nowMs) / 1000))
  const result = (phase: LotteryPhase, phaseLabel: string, statusLabel: string, remainingSeconds: number | null = null): LotteryTiming => ({
    phase, phaseLabel, statusLabel, accepting: phase === 'accepting',
    due: formatLotteryCountdown(remainingSeconds), remainingSeconds,
    drawAtMs, sealAtMs, acceptAtMs, intervalSeconds, sealSeconds,
  })

  if (input.enabled === false) return result('unavailable', '已停盘', '彩种已关闭')
  if (input.source_healthy === false) return result('error', '已停盘', '开奖源异常 · 已停盘')
  if (input.issue_status === 'error') return result('error', '已停盘', '本期异常 · 已停盘')
  if (input.issue_status === 'settling') return result('settling', '正在结算', '正在结算', 0)
  if (input.issue_status === 'settled') return result('settled', '等待下一期', '正在切换下一期', 0)
  if (input.issue_status === 'awaiting_draw') return result('awaiting_draw', '开奖中', '开奖中', 0)
  if (input.source_healthy !== true || !Number.isFinite(nowMs) || nowMs <= 0) return result('unavailable', '时间同步中', '状态同步中')
  if (!validBoundaries) return result('unavailable', '时间待同步', '开奖时间待同步')

  // A sealed/pending/unknown server snapshot must never become accepting just
  // because its countdown is positive. Only explicit accepting is eligible.
  if (input.issue_status !== 'accepting' && input.issue_status !== 'sealed' && input.issue_status !== 'pending') {
    return result('unavailable', '状态同步中', '状态同步中')
  }
  if (nowMs >= drawAtMs) return result('awaiting_draw', '开奖中', '开奖中', 0)
  if (input.issue_status === 'sealed' || nowMs >= sealAtMs) return result('sealed', '封盘倒计时', '已封盘', secondsUntil(drawAtMs))
  if (input.issue_status === 'pending' || (acceptAtMs !== null && nowMs < acceptAtMs)) {
    return result('pending', '距开始受理', '即将开始受理', acceptAtMs !== null && nowMs < acceptAtMs ? secondsUntil(acceptAtMs) : null)
  }
  return result('accepting', '受理倒计时', '正在受理', secondsUntil(sealAtMs))
}

export type ServerClockSample = {
  serverTimeMs: number
  monotonicAtMs: number
  roundTripMs: number
}

/** Measure the clock request itself, not the slower catalog request beside it. */
export function sampleServerClock(serverTimeMs: number, sentAtMs: number, receivedAtMs: number): ServerClockSample | null {
  if (!Number.isFinite(serverTimeMs) || serverTimeMs <= 0 || !Number.isFinite(sentAtMs) || !Number.isFinite(receivedAtMs) || receivedAtMs < sentAtMs) return null
  const roundTripMs = receivedAtMs - sentAtMs
  return { serverTimeMs: serverTimeMs + roundTripMs / 2, monotonicAtMs: receivedAtMs, roundTripMs }
}

/** A local wall-clock change cannot reopen a sealed round between syncs. */
export function readServerClock(sample: ServerClockSample | null, monotonicNowMs: number): number | null {
  if (!sample || !Number.isFinite(monotonicNowMs) || monotonicNowMs < sample.monotonicAtMs) return null
  return sample.serverTimeMs + monotonicNowMs - sample.monotonicAtMs
}
