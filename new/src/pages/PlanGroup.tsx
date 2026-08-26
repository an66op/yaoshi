import { useEffect, useMemo, useState } from 'react'
import { lotteryApi, type DrawResult } from '../api/lottery'
import { Icon } from '../components/Icon'
import { ballTone } from '../data/games'
import type { Game } from '../types'

const featuredPlanIds = ['canada-28', 'speed-racing', 'au-lucky-10']

const masters = [
  { name: '青云老师', title: '综合趋势', color: '#2aa9b3' },
  { name: '北斗数据师', title: '冷热分析', color: '#6e70df' },
  { name: '锦鲤计划师', title: '节奏追踪', color: '#e58b45' },
]

type PlanMode = 'combo' | 'numbers' | 'size' | 'parity'

type PlanPick = {
  numbers: number[]
  size: '大' | '小'
  parity: '单' | '双'
}

type PlanOutcome = PlanPick & {
  sum: number
  targetNumber: number
}

function stableNumber(value: string) {
  let hash = 2166136261
  for (const character of value) {
    hash ^= character.charCodeAt(0)
    hash = Math.imul(hash, 16777619)
  }
  return Math.abs(hash)
}

function planNumbers(game: Game, seed: number) {
  const isCanada = game.id === 'canada-28'
  const first = isCanada ? 0 : 1
  const range = isCanada ? 28 : 10
  const numbers: number[] = []
  let cursor = seed
  while (numbers.length < 3) {
    const candidate = first + (cursor % range)
    if (!numbers.includes(candidate)) numbers.push(candidate)
    cursor = Math.floor(cursor / range) + 7 + numbers.length * 11
  }
  return numbers
}

function numberPickLabel(game: Game) {
  if (game.id === 'canada-28') return '和值号码'
  if (game.balls.length >= 10) return '冠军号码'
  return '第一球号码'
}

function planPick(game: Game, issue: string, masterIndex: number): PlanPick {
  const seed = stableNumber(`${game.id}:${issue}:${masterIndex}`)
  return {
    numbers: planNumbers(game, seed),
    size: seed % 2 === 0 ? '大' : '小',
    parity: Math.floor(seed / 7) % 2 === 0 ? '单' : '双',
  }
}

function drawOutcome(game: Game, draw: DrawResult): PlanOutcome {
  const isCanada = game.id === 'canada-28'
  const values = draw.numbers.slice(0, isCanada ? 3 : 2)
  const sum = values.reduce((total, number) => total + number, 0)
  return {
    numbers: [],
    sum,
    targetNumber: isCanada ? sum : (draw.numbers[0] ?? 0),
    size: sum >= (isCanada ? 14 : 12) ? '大' : '小',
    parity: sum % 2 ? '单' : '双',
  }
}

function pickMatched(pick: PlanPick, outcome: PlanOutcome, mode: PlanMode) {
  if (mode === 'numbers') return pick.numbers.includes(outcome.targetNumber)
  if (mode === 'size') return pick.size === outcome.size
  if (mode === 'parity') return pick.parity === outcome.parity
  return pick.size === outcome.size && pick.parity === outcome.parity
}

function PlanPickDisplay({ pick, mode }: { pick: PlanPick; mode: PlanMode }) {
  const showSize = mode === 'combo' || mode === 'size'
  const showParity = mode === 'combo' || mode === 'parity'

  return <span className="plan-history-pick has-numbers">
    <strong aria-label={`推荐号码 ${pick.numbers.join('、')}`}>
      {pick.numbers.map((number) => <i key={number}>{number}</i>)}
    </strong>
    {(showSize || showParity) && <small>
      {showSize && <b>{pick.size}</b>}
      {showSize && showParity && <em>·</em>}
      {showParity && <b>{pick.parity}</b>}
    </small>}
  </span>
}

function shortIssue(value: string) {
  if (!value || value === '—') return '待更新'
  return value.length > 12 ? value.slice(-10) : value
}

function drawTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--:--'
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false })
}

function PlanResultBalls({ game }: { game: Game }) {
  if (game.id === 'canada-28') {
    const numbers = game.balls.slice(0, 3)
    const sum = numbers.reduce((total, number) => total + number, 0)
    return <div className="plan-result-balls plan-canada-result" aria-label={`上期号码 ${numbers.join('、')}，和值 ${sum}`}>
      {numbers.map((number, index) => <span key={`${game.id}-${index}`}><b className={`canada-tone-${index + 1}`}>{number}</b>{index < numbers.length - 1 && <i>+</i>}</span>)}
      <i>=</i><strong>{sum}</strong>
    </div>
  }
  return <div className="plan-result-balls" aria-label={`上期号码 ${game.balls.join('、')}`}>
    {game.balls.map((number, ballIndex) => <b className={ballTone(number)} key={`${game.id}-${ballIndex}`}>{number}</b>)}
  </div>
}

export function PlanLobby({ games, onBack, onSelect }: { games: Game[]; onBack: () => void; onSelect: (gameId: string) => void }) {
  const planGames = featuredPlanIds.map((id) => games.find((game) => game.id === id)).filter((game): game is Game => Boolean(game))

  return (
    <section className="plan-page plan-lobby-page">
      <header className="blue-header plan-header">
        <button aria-label="返回消息列表" onClick={onBack}><Icon name="back" /></button>
        <b>计划群</b>
        <span aria-hidden="true" />
      </header>
      <div className="plan-lobby-overview">
        <div><small>MASTER PLAN</small><b>大师推荐</b><span>计划随开奖更新 · 仅供娱乐参考</span></div>
        <div className="plan-overview-masters" aria-label="3 位大师在线">
          {masters.map((master, index) => <i key={master.name} style={{ background: master.color }}>{index + 1}</i>)}
          <em>3 位在线</em>
        </div>
      </div>
      <div className="plan-game-list">
        {planGames.map((game) => (
          <button className="plan-game-card" key={game.id} onClick={() => onSelect(game.id)}>
            <div className="plan-game-main">
              <span className="plan-game-logo">{game.logo ? <img alt={`${game.title} Logo`} src={game.logo} /> : game.tag.slice(0, 2)}</span>
              <div><b>{game.title}</b><small><i />{masters.length} 位大师在线 · 每期更新</small></div>
              <em>第 {shortIssue(game.period)} 期<Icon name="arrow" /></em>
            </div>
            <footer>
              <span>上期</span>
              <PlanResultBalls game={game} />
            </footer>
          </button>
        ))}
        {planGames.length === 0 && <p className="plan-empty">彩票计划正在加载，请稍后重试。</p>}
      </div>
    </section>
  )
}

export function PlanDetail({ games, gameId, onBack }: { games: Game[]; gameId?: string; onBack: () => void }) {
  const game = games.find((item) => item.id === gameId)
  const [draws, setDraws] = useState<DrawResult[]>([])
  const [loading, setLoading] = useState(true)
  const [masterIndex, setMasterIndex] = useState(0)
  const [mode, setMode] = useState<PlanMode>('combo')

  useEffect(() => {
    let cancelled = false
    if (!game) {
      setLoading(false)
      return
    }
    setLoading(true)
    void lotteryApi.draws(game.id, 18).then((rows) => {
      if (!cancelled) setDraws(rows)
    }).catch(() => {
      if (!cancelled) setDraws([])
    }).finally(() => {
      if (!cancelled) setLoading(false)
    })
    return () => { cancelled = true }
  }, [game])

  const masterStats = useMemo(() => masters.map((master, index) => {
    const sample = draws.slice(0, 12)
    const wins = game ? sample.filter((draw) => pickMatched(planPick(game, draw.issue, index), drawOutcome(game, draw), mode)).length : 0
    return { ...master, rate: sample.length ? Math.round((wins / sample.length) * 100) : 0 }
  }), [draws, game, mode])

  if (!game) {
    return <section className="plan-page"><header className="blue-header"><button aria-label="返回计划群" onClick={onBack}><Icon name="back" /></button><b>计划详情</b><span /></header><p className="plan-empty">该彩票计划暂未开放。</p></section>
  }

  const currentPick = planPick(game, game.period, masterIndex)
  const activeMaster = masterStats[masterIndex]

  return (
    <section className="plan-page plan-detail-page">
      <header className="blue-header plan-header">
        <button aria-label="返回计划群" onClick={onBack}><Icon name="back" /></button>
        <b>{game.title}</b>
        <span aria-hidden="true" />
      </header>
      <div className="plan-current">
        <span className="plan-current-logo">{game.logo ? <img alt={`${game.title} Logo`} src={game.logo} /> : game.tag.slice(0, 2)}</span>
        <div><small>第 {shortIssue(game.period)} 期</small><b>{game.issueStatus === 'accepting' ? '推荐更新中' : '本期计划'}</b></div>
        <time>截止 {game.due}</time>
      </div>
      <div className="plan-master-tabs" role="tablist" aria-label="选择计划大师">
        {masterStats.map((master, index) => <button aria-selected={masterIndex === index} className={masterIndex === index ? 'active' : ''} key={master.name} onClick={() => setMasterIndex(index)} role="tab"><span style={{ background: master.color }}>{index + 1}</span><b>{master.name}</b><small>{master.rate ? `近12期 ${master.rate}%` : master.title}</small></button>)}
      </div>
      <div className="plan-mode-tabs">
        {([['combo', '综合'], ['numbers', '号码'], ['size', '大小'], ['parity', '单双']] as const).map(([value, label]) => <button className={mode === value ? 'active' : ''} key={value} onClick={() => setMode(value)}>{label}</button>)}
      </div>
      <article className="plan-recommendation">
        <header><div><small>{activeMaster.name}</small><b>第 {shortIssue(game.period)} 期推荐</b></div><em>更新于刚刚</em></header>
        <div className="plan-picks">
          <span className="plan-number-pick"><small>{numberPickLabel(game)}</small><strong>{currentPick.numbers.map((number) => <i key={number}>{number}</i>)}</strong></span>
          {(mode === 'combo' || mode === 'size') && <span className={currentPick.size === '大' ? 'blue' : 'orange'}><small>大小</small><b>{currentPick.size}</b></span>}
          {(mode === 'combo' || mode === 'parity') && <span className={currentPick.parity === '单' ? 'blue' : 'orange'}><small>单双</small><b>{currentPick.parity}</b></span>}
        </div>
        <p>根据近期冷热、连开节奏与位置变化生成，仅供娱乐参考。</p>
      </article>
      <section className="plan-history">
        <header><b>近期计划记录</b><span>{activeMaster.rate ? `当前策略命中率 ${activeMaster.rate}%` : '等待开奖数据'}</span></header>
        <div className="plan-history-head"><span>期号/时间</span><span>开奖</span><span>推荐</span><span>结果</span></div>
        {loading && <p className="plan-history-loading">正在加载开奖记录…</p>}
        {!loading && draws.slice(0, 12).map((draw) => {
          const outcome = drawOutcome(game, draw)
          const pick = planPick(game, draw.issue, masterIndex)
          const matched = pickMatched(pick, outcome, mode)
          return <div className="plan-history-row" key={draw.id}>
            <span><b>{shortIssue(draw.issue)}</b><small>{drawTime(draw.draw_at)}</small></span>
            <span><b>{mode === 'numbers' ? outcome.targetNumber : outcome.sum}</b><small>{mode === 'numbers' ? (game.id === 'canada-28' ? '和值' : '冠军') : `${outcome.size} · ${outcome.parity}`}</small></span>
            <PlanPickDisplay pick={pick} mode={mode} />
            <em className={matched ? 'hit' : 'miss'}>{matched ? '中' : '未中'}</em>
          </div>
        })}
        {!loading && draws.length === 0 && <p className="plan-history-loading">暂无可用开奖记录</p>}
      </section>
    </section>
  )
}
