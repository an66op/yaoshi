import type { AdminGame } from '../api'

const beijingFormatter = new Intl.DateTimeFormat('en-GB', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
})

type TimeValue = string | number | null | undefined

function timestamp(value: TimeValue): number | null {
  if (value == null || value === '' || (typeof value === 'string' && /^0001-/.test(value))) return null
  const result = new Date(value).getTime()
  return Number.isFinite(result) ? result : null
}

/** One exact, explicit timezone format for clocks, draw records and exports. */
export function formatBeijingDateTime(value: TimeValue, fallback = '—'): string {
  const time = timestamp(value)
  if (time == null) return fallback
  const parts = Object.fromEntries(beijingFormatter.formatToParts(time).map(part => [part.type, part.value]))
  return `${parts.year}/${parts.month}/${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`
}

/** This is the next collection attempt, never the draw-period countdown. */
export function formatFeedCountdown(value: TimeValue, now: number): string {
  const target = timestamp(value)
  if (target == null || !Number.isFinite(now) || now <= 0) return '等待调度'
  return `${Math.max(0, Math.ceil((target - now) / 1000))} 秒后检查`
}

export function formatDurationSeconds(value: number | undefined): string | null {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) return null
  const seconds = value % 60
  const minutes = Math.floor(value / 60) % 60
  const hours = Math.floor(value / 3600) % 24
  const days = Math.floor(value / 86400)
  if (days) return `${days}天${hours ? `${hours}小时` : ''}${minutes ? `${minutes}分` : ''}${seconds ? `${seconds}秒` : ''}`
  if (hours) return `${hours}小时${String(minutes).padStart(2, '0')}分${String(seconds).padStart(2, '0')}秒`
  if (minutes) return `${minutes}分${String(seconds).padStart(2, '0')}秒`
  return `${seconds}秒`
}

type GameSchedule = Pick<AdminGame, 'schedule_mode' | 'draw_interval' | 'seal_seconds' | 'timing_source'>

export function describeGameSchedule(game: GameSchedule | undefined): { interval: string; seal: string; source: string } {
  const interval = formatDurationSeconds(game?.draw_interval)
  const seal = formatDurationSeconds(game?.seal_seconds)
  const source = game?.timing_source
  const fromFeed = source === 'upstream' || source === 'observed'
  return {
    // Daily/weekly official calendars are not fixed three-day countdowns.
    // A supplied next-draw timestamp does not make a daily/weekly calendar
    // periodic. The backend only observes fixed high-frequency cadences.
    interval: game?.schedule_mode === 'official-feed' && (!fromFeed || (game.draw_interval ?? 0) > 6 * 3600)
      ? '按官方日程开奖'
      : interval && game?.draw_interval
        ? `每期 ${interval}`
        : '开奖周期未配置',
    seal: seal == null ? '封盘提前量未配置' : game?.seal_seconds === 0 ? '开奖时封盘' : `提前 ${seal} 封盘`,
    source: source === 'upstream' ? '源站时序' : source === 'observed' ? '实际开奖间隔' : source === 'configured' ? '配置周期' : '',
  }
}

/** Respect error/settlement states; wall time only refines an active period. */
export function describeIssueState(game: Pick<AdminGame, 'issue_status' | 'accept_at' | 'seal_at' | 'next_draw_at'> | undefined, now: number): string {
  if (!game) return '等待期号'
  switch (game.issue_status) {
    case 'error': return '开奖异常'
    case 'settling': return '结算中'
    case 'settled': return '已结算'
  }
  const acceptAt = timestamp(game.accept_at)
  const sealAt = timestamp(game.seal_at)
  const drawAt = timestamp(game.next_draw_at)
  if (Number.isFinite(now) && now > 0) {
    if (drawAt != null && now >= drawAt) return '等待开奖'
    if (sealAt != null && now >= sealAt) return '封盘中'
    if (acceptAt != null && now < acceptAt) return '未开始受理'
  }
  switch (game.issue_status) {
    case 'accepting': return '受理中'
    case 'sealed': return '封盘中'
    case 'awaiting_draw': return '等待开奖'
    case 'pending': return '未开始受理'
    default: return '等待期号'
  }
}
