import { useEffect, useState, type CSSProperties } from 'react'
import type { Game } from '../types'
import { ballTone } from '../data/games'
import { Icon } from '../components/Icon'
import { MarkSixDrawBall } from '../components/MarkSixBall'
import { useGameDraws } from '../hooks/useGameDraws'
import { lotteryResultSummary, lotteryRuleProfile, markSixBallClass, markSixWaveLabel } from '../utils/lotteryRules'
import './draw-results-rules.css'

type ResultMode = 'numbers' | 'size' | 'parity' | 'trend'

const resultModes: Array<{ id: ResultMode; label: string }> = [
  { id: 'numbers', label: '号码' },
  { id: 'size', label: '大小' },
  { id: 'parity', label: '单双' },
  { id: 'trend', label: '冠亚/龙虎' },
]

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

function ResultCells({ mode, numbers, gameId, ruleVersion = '', drawAt }: { mode: ResultMode; numbers: number[]; gameId: string; ruleVersion?: string; drawAt: string }) {
  const profile = lotteryRuleProfile(gameId)
  const summary = lotteryResultSummary(gameId, numbers, ruleVersion)
  // Long official results wrap into complete rows instead of squeezing twenty
  // balls into the width reserved for ten or losing numbers off screen.
  const cellColumns = { '--result-count': Math.min(numbers.length, 10) } as CSSProperties
  if (!numbers.length) return <div className="draw-result-cells result-unavailable">暂无号码</div>

  if (mode === 'numbers' || !summary) {
    const isMarkSix = profile.family === 'mark-six'
    return <div className={`draw-result-cells number-cells${isMarkSix ? ' mark-six-result-balls' : ''}`} style={cellColumns}>{numbers.map((number, index) => isMarkSix
      ? <MarkSixDrawBall drawAt={drawAt} index={index} key={index} length={numbers.length} number={number} />
      : <b className={ballTone(number)} key={index}>{number}</b>)}</div>
  }

  if (mode === 'size') {
    return <div className="draw-result-cells result-word-cells" style={cellColumns}>{numbers.map((number, index) => {
      if (profile.family === 'mark-six' && number === 49) return <b className="result-neutral" key={index}>和</b>
      const large = number >= profile.numberThreshold
      return <b className={large ? 'result-blue' : 'result-orange'} key={index}>{large ? '大' : '小'}</b>
    })}</div>
  }

  if (mode === 'parity') {
    return <div className="draw-result-cells result-word-cells" style={cellColumns}>{numbers.map((number, index) => {
      if (profile.family === 'mark-six' && number === 49) return <b className="result-neutral" key={index}>和</b>
      const odd = number % 2 === 1
      return <b className={odd ? 'result-blue' : 'result-orange'} key={index}>{odd ? '单' : '双'}</b>
    })}</div>
  }

  if (profile.family === 'mark-six') {
    return <div className="draw-result-cells trend-cells mark-six-special-summary">
      <strong>{summary.total}</strong>
      <b className={summary.size === '和' ? 'result-neutral' : summary.size === '大' ? 'result-blue' : 'result-orange'}>{summary.size}</b>
      <b className={summary.parity === '和' ? 'result-neutral' : summary.parity === '单' ? 'result-blue' : 'result-orange'}>{summary.parity}</b>
      <b className={markSixBallClass(summary.total)}>{markSixWaveLabel(summary.total)}</b>
      <em className="sr-only">{summary.label}：{summary.text}</em>
    </div>
  }

  return <div className="draw-result-cells trend-cells">
    <strong>{summary.total}</strong>
    <b className={summary.size === '大' ? 'result-blue' : 'result-orange'}>{summary.size}</b>
    <b className={summary.parity === '单' ? 'result-blue' : 'result-orange'}>{summary.parity}</b>
    <i aria-hidden="true" />
    {summary.dragons.map((dragon, index) => <b className={dragon === '龙' ? 'result-blue' : dragon === '虎' ? 'result-orange' : 'result-neutral'} key={index}>{dragon}</b>)}
    <em className="sr-only">{summary.label}：{summary.text}，{summary.dragonLabel}：{summary.dragonText}</em>
  </div>
}

export function DrawResults({ games, initialGameId, onBack, onSelectGame }: { games: Game[]; initialGameId?: string; onBack: () => void; onSelectGame: (gameId: string) => void }) {
  const [selectedGameId, setSelectedGameId] = useState(initialGameId ?? games[0]?.id ?? '')
  const [selectedDate, setSelectedDate] = useState('')
  const [gamePickerOpen, setGamePickerOpen] = useState(false)
  const [resultMode, setResultMode] = useState<ResultMode>('numbers')
  const selectedGame = games.find((item) => item.id === selectedGameId) ?? games[0]
  const profile = lotteryRuleProfile(selectedGame?.id ?? '')
  const { draws, loading } = useGameDraws(selectedGame?.id ?? '', 100)
  const drawDate = (value: string) => {
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? '' : new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai' }).format(date)
  }
  const visibleDraws = draws.filter(draw => draw.game_id === selectedGame?.id && (!selectedDate || drawDate(draw.draw_at) === selectedDate))
  const availableModes = profile.family === 'unknown' ? resultModes.filter(item => item.id === 'numbers')
    : resultModes.map(item => item.id === 'trend' ? { ...item, label: profile.family === 'mark-six' ? '特码属性' : `${profile.sumLabel}/龙虎` } : item)
  const activeMode = availableModes.some(item => item.id === resultMode) ? resultMode : 'numbers'

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
      <div className="draw-game-picker"><button className="draw-game-picker-trigger" aria-expanded={gamePickerOpen} onClick={() => setGamePickerOpen((open) => !open)}><span>彩种</span><b>{selectedGame.title}</b><i>⌄</i></button>{gamePickerOpen && <div className="draw-game-picker-menu">{games.map((item) => <button className={item.id === selectedGameId ? 'active' : ''} key={item.id} onClick={() => { setSelectedGameId(item.id); setSelectedDate(''); setResultMode('numbers'); setGamePickerOpen(false); onSelectGame(item.id) }}><span className={item.logo ? 'has-image' : ''} style={{ background: item.logo ? undefined : item.color }}>{item.logo ? <img alt={`${item.title} Logo`} src={item.logo} /> : item.tag.slice(0, 2)}</span><b>{item.title}</b>{item.id === selectedGameId && <i>✓</i>}</button>)}</div>}</div>
      <label className="draw-date-picker"><span>日期</span><input aria-label="选择开奖日期" type="date" value={selectedDate} onChange={(event) => setSelectedDate(event.target.value)} />{!selectedDate && <i aria-hidden="true">年 / 月 / 日</i>}<em aria-hidden="true" /></label>
      {selectedDate && <button onClick={() => setSelectedDate('')}>全部日期</button>}
    </section>
    <nav className="draw-result-modes" aria-label="开奖显示方式">
      <span>期数 / 时间</span>
      <div>{availableModes.map((item) => <button className={activeMode === item.id ? 'active' : ''} aria-pressed={activeMode === item.id} key={item.id} onClick={() => setResultMode(item.id)}>{item.label}</button>)}</div>
      <small>{selectedDate || `最近 ${visibleDraws.length} 期`}</small>
    </nav>
    <section className={`draw-results-table result-mode-${activeMode}${profile.family === 'racing' ? ' racing-results-table' : ''}`}>
      {loading && <p className="draw-results-empty">正在加载开奖结果…</p>}
      {!loading && visibleDraws.map((draw) => {
        return <article key={draw.issue}>
          <span><b>{draw.issue}</b><time dateTime={draw.draw_at}>{drawTime(draw.draw_at)}</time></span>
          <ResultCells mode={activeMode} numbers={draw.numbers} gameId={selectedGame.id} ruleVersion={selectedGame.ruleVersion} drawAt={draw.draw_at} />
        </article>
      })}
      {!loading && !visibleDraws.length && <p className="draw-results-empty">该日期暂无开奖结果</p>}
    </section>
  </main>
}
