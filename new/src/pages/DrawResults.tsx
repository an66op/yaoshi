import { useEffect, useState } from 'react'
import type { Game } from '../types'
import { ballTone } from '../data/games'
import { Icon } from '../components/Icon'
import { useGameDraws } from '../hooks/useGameDraws'

const positionNames = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十']

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

export function DrawResults({ games, initialGameId, onBack }: { games: Game[]; initialGameId?: string; onBack: () => void }) {
  const [selectedGameId, setSelectedGameId] = useState(initialGameId ?? games[0]?.id ?? '')
  const [selectedDate, setSelectedDate] = useState('')
  const [gamePickerOpen, setGamePickerOpen] = useState(false)
  const selectedGame = games.find((item) => item.id === selectedGameId) ?? games[0]
  const { draws, loading } = useGameDraws(selectedGame?.id ?? '', 100)
  const positions = positionNames.slice(0, Math.max(selectedGame?.balls.length ?? 5, 5))
  const drawDate = (value: string) => new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai' }).format(new Date(value))
  const visibleDraws = selectedDate ? draws.filter((draw) => drawDate(draw.draw_at) === selectedDate) : draws

  useEffect(() => {
    if (initialGameId && games.some((game) => game.id === initialGameId)) {
      setSelectedGameId(initialGameId)
      setSelectedDate('')
    }
  }, [games, initialGameId])

  if (!selectedGame) return null

  return <main className="draw-results-page">
    <header className="draw-results-header">
      <button aria-label="返回游戏" onClick={onBack}><Icon name="back" /></button>
      <div><b>开奖结果</b><small>{selectedGame.title} · 历史开奖记录</small></div>
      <span aria-hidden="true" />
    </header>
    <section className="draw-results-filters">
      <div className="draw-game-picker"><button className="draw-game-picker-trigger" aria-expanded={gamePickerOpen} onClick={() => setGamePickerOpen((open) => !open)}><span>彩种</span><b>{selectedGame.title}</b><i>⌄</i></button>{gamePickerOpen && <div className="draw-game-picker-menu">{games.map((item) => <button className={item.id === selectedGameId ? 'active' : ''} key={item.id} onClick={() => { setSelectedGameId(item.id); setSelectedDate(''); setGamePickerOpen(false) }}><span style={{ background: item.color }}>{item.tag.slice(0, 2)}</span><b>{item.title}</b>{item.id === selectedGameId && <i>✓</i>}</button>)}</div>}</div>
      <label className="draw-date-picker"><span>日期</span><input aria-label="选择开奖日期" type="date" value={selectedDate} onChange={(event) => setSelectedDate(event.target.value)} />{!selectedDate && <i aria-hidden="true">年 / 月 / 日</i>}<em aria-hidden="true" /></label>
      {selectedDate && <button onClick={() => setSelectedDate('')}>全部日期</button>}
    </section>
    <section className="draw-results-intro"><b>{selectedGame.title}</b><span>{selectedDate || '全部日期'} · {visibleDraws.length} 期</span></section>
    <section className="draw-results-table">
      <header><span>期数</span><b>{positions.map((position) => <i key={position}>{position}</i>)}</b><em>结果</em></header>
      {loading && <p className="draw-results-empty">正在加载开奖结果…</p>}
      {!loading && visibleDraws.map((draw) => {
        const balls = draw.numbers.length ? draw.numbers : selectedGame.balls
        const meta = crownMeta(balls)
        return <article key={draw.issue}><span>{shortIssue(draw.issue)}</span><div>{balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div><em>{meta.crownResult} · {meta.dragonTiger}</em></article>
      })}
      {!loading && !visibleDraws.length && <p className="draw-results-empty">该日期暂无开奖结果</p>}
    </section>
  </main>
}
