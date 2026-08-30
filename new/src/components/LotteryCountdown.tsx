import type { LotteryTiming } from '../utils/lotteryTiming'

/** Lobby cards reserve the caption row even while accepting shows only digits.
 * Compact room headers keep other states beside the seconds; screen readers
 * always receive the full phase description. */
export function LotteryCountdown({ timing, compact = false }: { timing: LotteryTiming; compact?: boolean }) {
  const inlineLabel = timing.phase === 'accepting' ? null : timing.phase === 'sealed' ? '封盘中' : timing.phaseLabel
  return <div className={`lottery-countdown phase-${timing.phase}${compact ? ' is-compact' : ''}`} aria-label={`${timing.phaseLabel} ${timing.due}`} title={timing.statusLabel}>
    {!compact && <span className="lottery-countdown-label" aria-hidden={timing.phase === 'accepting' || undefined}>{timing.phase === 'accepting' ? null : timing.phaseLabel}</span>}
    <b className="lottery-countdown-digits" aria-hidden="true">{timing.due.split('').map((character, index) => <i className={character === ':' ? 'separator' : undefined} key={index}>{character}</i>)}</b>
    {compact && inlineLabel && <span className="lottery-countdown-label">{inlineLabel}</span>}
  </div>
}
