import { useEffect, useState, type CSSProperties } from 'react'
import type { Game } from '../types'
import { ballTone } from '../data/games'
import { Icon } from '../components/Icon'
import { useGameDraws } from '../hooks/useGameDraws'

type ResultMode = 'numbers' | 'size' | 'parity' | 'trend'

const resultModes: Array<{ id: ResultMode; label: string }> = [
  { id: 'numbers', label: '号码' },
  { id: 'size', label: '大小' },
  { id: 'parity', label: '单双' },
  { id: 'trend', label: '冠亚/龙虎' },
]

function shortIssue(issue: string) {
  return issue.match(/^(\d{8})-/)?.[1] ?? issue
}

function crownMeta(balls: number[]) {
  if (balls.length < 2) return { crownResult: '—', dragonTiger: '—' }
  const sum = balls[0] + balls[1]
  return {
    crownResult: `${sum}${sum >= 12 ? '大' : '小'}${sum % 2 ? '单' : '双'}`,
    dragonTiger: balls.slice(0, 5).map((ball, index) => (balls[9 - index] !== undefined && ball > balls[9 - index] ? '龙' : '虎')).join(''),
  }
}

function drawTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--:--'
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

function isLargeNumber(number: number, balls: number[]) {
  const usesTenRanks = !balls.includes(0) && Math.max(...balls, 0) >= 10
  return number >= (usesTenRanks ? 6 : 5)
}

function ResultCells({ mode, numbers, gameBalls }: { mode: ResultMode; numbers: number[]; gameBalls: number[] }) {
  const cellColumns = { '--result-count': numbers.length } as CSSProperties

  if (mode === 'numbers') {
    return <div className="draw-result-cells number-cells" style={cellColumns}>{numbers.map((number, index) => <b className={ballTone(number)} key={index}>{number}</b>)}</div>
  }

  if (mode === 'size') {
    return <div className="draw-result-cells result-word-cells" style={cellColumns}>{numbers.map((number, index) => {
      const large = isLargeNumber(number, gameBalls)
      return <b className={large ? 'result-blue' : 'result-orange'} key={index}>{large ? '大' : '小'}</b>
    })}</div>
  }

  if (mode === 'parity') {
    return <div className="draw-result-cells result-word-cells" style={cellColumns}>{numbers.map((number, index) => {
      const odd = number % 2 === 1
      return <b className={odd ? 'result-blue' : 'result-orange'} key={index}>{odd ? '单' : '双'}</b>
    })}</div>
  }

  const meta = crownMeta(numbers)
  const sum = numbers[0] + numbers[1]
  const dragons = numbers.slice(0, 5).map((number, index) => number > numbers[numbers.length - 1 - index])
  return <div className="draw-result-cells trend-cells">
    <strong>{sum}</strong>
    <b className={sum >= 12 ? 'result-blue' : 'result-orange'}>{sum >= 12 ? '大' : '小'}</b>
    <b className={sum % 2 ? 'result-blue' : 'result-orange'}>{sum % 2 ? '单' : '双'}</b>
    <i aria-hidden="true" />
    {dragons.map((dragon, index) => <b className={dragon ? 'result-blue' : 'result-orange'} key={index}>{dragon ? '龙' : '虎'}</b>)}
    <em className="sr-only">{meta.crownResult}，{meta.dragonTiger}</em>
  </div>
}

export function DrawResults({ games, initialGameId, onBack, onSelectGame }: { games: Game[]; initialGameId?: string; onBack: () => void; onSelectGame: (gameId: string) => void }) {
  const [selectedGameId, setSelectedGameId] = useState(initialGameId ?? games[0]?.id ?? '')
  const [selectedDate, setSelectedDate] = useState('')
  const [gamePickerOpen, setGamePickerOpen] = useState(false)
  const [resultMode, setResultMode] = useState<ResultMode>('numbers')
  const selectedGame = games.find((item) => item.id === selectedGameId) ?? games[0]
  const { draws, loading } = useGameDraws(selectedGame?.id ?? '', 100)
  const drawDate = (value: string) => new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai' }).format(new Date(value))
  const visibleDraws = selectedDate ? draws.filter((draw) => drawDate(draw.draw_at) === selectedDate) : draws
  const availableModes = selectedGame?.balls.length >= 10 ? resultModes : resultModes.filter((item) => item.id !== 'trend')

  useEffect(() => {
    if (initialGameId) {
      setSelectedGameId(initialGameId)
      setSelectedDate('')
      setResultMode('numbers')
    }
  }, [initialGameId])

  // `games` is rebuilt every second so the lobby countdown can tick. Keep a
  // valid manual selection across those refreshes; only fall back when that
  // game was actually disabled or removed.
  useEffect(() => {
    if (!games.length) return
    setSelectedGameId((current) => games.some((game) => game.id === current) ? current : games[0].id)
  }, [games])

  if (!selectedGame) return null

  return <main className="draw-results-page">
    <header className="draw-results-header">
      <button aria-label="返回游戏" onClick={onBack}><Icon name="back" /></button>
      <div><b>开奖结果</b><small>{selectedGame.title} · 历史开奖记录</small></div>
      <span aria-hidden="true" />
    </header>
    <section className="draw-results-filters">
      <div className="draw-game-picker"><button className="draw-game-picker-trigger" aria-expanded={gamePickerOpen} onClick={() => setGamePickerOpen((open) => !open)}><span>彩种</span><b>{selectedGame.title}</b><i>⌄</i></button>{gamePickerOpen && <div className="draw-game-picker-menu">{games.map((item) => <button className={item.id === selectedGameId ? 'active' : ''} key={item.id} onClick={() => { setSelectedGameId(item.id); setSelectedDate(''); setResultMode('numbers'); setGamePickerOpen(false); onSelectGame(item.id) }}><span style={{ background: item.color }}>{item.tag.slice(0, 2)}</span><b>{item.title}</b>{item.id === selectedGameId && <i>✓</i>}</button>)}</div>}</div>
      <label className="draw-date-picker"><span>日期</span><input aria-label="选择开奖日期" type="date" value={selectedDate} onChange={(event) => setSelectedDate(event.target.value)} />{!selectedDate && <i aria-hidden="true">年 / 月 / 日</i>}<em aria-hidden="true" /></label>
      {selectedDate && <button onClick={() => setSelectedDate('')}>全部日期</button>}
    </section>
    <nav className="draw-result-modes" aria-label="开奖显示方式">
      <span>期数 / 时间</span>
      <div>{availableModes.map((item) => <button className={resultMode === item.id ? 'active' : ''} aria-pressed={resultMode === item.id} key={item.id} onClick={() => setResultMode(item.id)}>{item.label}</button>)}</div>
      <small>{selectedDate || `最近 ${visibleDraws.length} 期`}</small>
    </nav>
    <section className={`draw-results-table result-mode-${resultMode}${selectedGame.balls.length > 5 ? ' racing-results-table' : ''}`}>
      {loading && <p className="draw-results-empty">正在加载开奖结果…</p>}
      {!loading && visibleDraws.map((draw) => {
        const balls = draw.numbers.length ? draw.numbers : selectedGame.balls
        return <article key={draw.issue}>
          <span><b>{shortIssue(draw.issue)}</b><time>{drawTime(draw.draw_at)}</time></span>
          <ResultCells mode={resultMode} numbers={balls} gameBalls={selectedGame.balls} />
        </article>
      })}
      {!loading && !visibleDraws.length && <p className="draw-results-empty">该日期暂无开奖结果</p>}
    </section>
  </main>
}
