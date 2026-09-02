import { markSixDrawBallClass, markSixZodiacLabel } from '../utils/lotteryRules'
import './mark-six-ball.css'

type MarkSixDrawBallProps = {
  number: number
  index: number
  length: number
  drawAt: string | number | Date | null | undefined
  showZodiac?: boolean
}

/** A result ball with the draw-date zodiac label used by Mark Six screens. */
export function MarkSixDrawBall({ number, index, length, drawAt, showZodiac = true }: MarkSixDrawBallProps) {
  const special = index === 6 && length === 7
  return <span className={`mark-six-draw-cell${special ? ' is-special' : ''}`}>
    <b className={markSixDrawBallClass(number, index, length)}>{String(number).padStart(2, '0')}</b>
    {showZodiac && <small>{markSixZodiacLabel(number, drawAt)}</small>}
  </span>
}
