import { useEffect, useMemo, useState } from 'react'
import { planApi, type PlanDetail as PlanDetailData, type PlanGameSummary, type PlanRecommendation } from '../api/plans'
import { Icon } from '../components/Icon'
import { ballTone } from '../data/games'
import type { Game } from '../types'

type PlanMode = 'combo' | 'numbers' | 'size' | 'parity'

function shortIssue(value: string) {
  if (!value || value === '—') return '—'
  return value.length > 12 ? value.slice(-10) : value
}

function updateTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--:--'
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false })
}

function PlanResultBalls({ game }: { game: Game }) {
  if (!game.balls.length) {
    return <span className="plan-result-empty">等待开奖</span>
  }
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

function PlanPickDisplay({ row, mode }: { row: PlanRecommendation; mode: PlanMode }) {
  const showSize = (mode === 'combo' || mode === 'size') && row.size
  const showParity = (mode === 'combo' || mode === 'parity') && row.parity
  return <span className="plan-history-pick has-numbers">
    {(mode === 'combo' || mode === 'numbers') && <strong aria-label={`推荐号码 ${row.numbers.join('、')}`}>{row.numbers.map(number => <i key={number}>{number}</i>)}</strong>}
    {(showSize || showParity) && <small>
      {showSize && <b>{row.size}</b>}
      {showSize && showParity && <em>·</em>}
      {showParity && <b>{row.parity}</b>}
    </small>}
  </span>
}

export function PlanLobby({ games, onBack, onSelect }: { games: Game[]; onBack: () => void; onSelect: (gameId: string) => void }) {
  const [catalog, setCatalog] = useState<PlanGameSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    void planApi.catalog().then(rows => {
      if (!cancelled) setCatalog(Array.isArray(rows) ? rows : [])
    }).catch(reason => {
      if (!cancelled) { setCatalog([]); setError(reason instanceof Error ? reason.message : '读取计划失败') }
    }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [])

  const planGames = useMemo(() => catalog.map(summary => {
    const game = games.find(item => item.id === summary.game_id)
    return game ? { game, summary } : null
  }).filter((item): item is { game: Game; summary: PlanGameSummary } => Boolean(item)), [catalog, games])

  return <section className="plan-page plan-lobby-page">
    <header className="blue-header plan-header"><button aria-label="返回消息列表" onClick={onBack}><Icon name="back" /></button><b>计划群</b><span aria-hidden="true" /></header>
    <div className="plan-lobby-overview">
      <div><small>MASTER PLAN</small><b>大师推荐</b><span>房间人工计划 · 仅供娱乐参考</span></div>
      <div className="plan-overview-masters"><em>{planGames.length} 个彩种</em></div>
    </div>
    <div className="plan-game-list">
      {planGames.map(({ game, summary }) => <button className="plan-game-card" key={game.id} onClick={() => onSelect(game.id)}>
        <div className="plan-game-main">
          <span className="plan-game-logo">{game.logo ? <img alt={`${game.title} Logo`} src={game.logo} /> : game.tag.slice(0, 2)}</span>
          <div><b>{game.title}</b><small><i />{summary.master_count} 位大师 · 后台发布</small></div>
          <em>第 {shortIssue(summary.current_issue)} 期<Icon name="arrow" /></em>
        </div>
        <footer><span>上期</span><PlanResultBalls game={game} /></footer>
      </button>)}
      {loading && <p className="plan-empty">正在加载房间计划…</p>}
      {!loading && error && <p className="plan-empty">{error}</p>}
      {!loading && !error && planGames.length === 0 && <p className="plan-empty">当前房间暂无计划推荐</p>}
    </div>
  </section>
}

export function PlanDetail({ games, gameId, onBack }: { games: Game[]; gameId?: string; onBack: () => void }) {
  const game = games.find(item => item.id === gameId)
  const [data, setData] = useState<PlanDetailData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [masterIndex, setMasterIndex] = useState(0)
  const [mode, setMode] = useState<PlanMode>('combo')

  useEffect(() => {
    let cancelled = false
    setMasterIndex(0); setData(null); setError('')
    if (!gameId) { setLoading(false); return }
    setLoading(true)
    void planApi.detail(gameId).then(result => {
      if (!cancelled) setData({ ...result, recommendations: Array.isArray(result.recommendations) ? result.recommendations : [], history: Array.isArray(result.history) ? result.history : [] })
    }).catch(reason => {
      if (!cancelled) setError(reason instanceof Error ? reason.message : '读取计划失败')
    }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [gameId])

  const masters = data?.recommendations ?? []
  const activeMaster = masters[masterIndex]
  const history = useMemo(() => activeMaster ? (data?.history ?? []).filter(row => row.master_name === activeMaster.master_name) : [], [activeMaster, data?.history])

  if (!game) return <section className="plan-page"><header className="blue-header"><button aria-label="返回计划群" onClick={onBack}><Icon name="back" /></button><b>计划详情</b><span /></header><p className="plan-empty">该彩票计划暂未开放。</p></section>

  return <section className="plan-page plan-detail-page">
    <header className="blue-header plan-header"><button aria-label="返回计划群" onClick={onBack}><Icon name="back" /></button><b>{game.title}</b><span aria-hidden="true" /></header>
    <div className="plan-current">
      <span className="plan-current-logo">{game.logo ? <img alt={`${game.title} Logo`} src={game.logo} /> : game.tag.slice(0, 2)}</span>
      <div><small>{data?.current_issue ? `第 ${shortIssue(data.current_issue)} 期` : '暂无受理期'}</small><b>{loading ? '正在读取' : masters.length ? '本期计划' : '暂无计划'}</b></div>
      <time>{activeMaster ? `更新 ${updateTime(activeMaster.updated_at)}` : ''}</time>
    </div>
    {error && <p className="plan-empty">{error}</p>}
    {!loading && !error && masters.length === 0 && <p className="plan-empty">当前彩种暂无已发布计划</p>}
    {masters.length > 0 && <>
      <div className="plan-master-tabs" role="tablist" aria-label="选择计划大师">
        {masters.map((master, index) => <button aria-selected={masterIndex === index} className={masterIndex === index ? 'active' : ''} key={master.id} onClick={() => setMasterIndex(index)} role="tab"><span style={{ background: master.master_color }}>{index + 1}</span><b>{master.master_name}</b><small>{master.master_hit_rate === null ? (master.master_title || '暂无统计') : `历史 ${master.master_hit_rate.toFixed(0)}%`}</small></button>)}
      </div>
      <div className="plan-mode-tabs">{([['combo', '综合'], ['numbers', '号码'], ['size', '大小'], ['parity', '单双']] as const).map(([value, label]) => <button className={mode === value ? 'active' : ''} key={value} onClick={() => setMode(value)}>{label}</button>)}</div>
      {activeMaster && <article className="plan-recommendation">
        <header><div><small>{activeMaster.master_name}</small><b>第 {shortIssue(activeMaster.issue)} 期推荐</b></div><em>{updateTime(activeMaster.updated_at)} 更新</em></header>
        <div className="plan-picks">
          {(mode === 'combo' || mode === 'numbers') && <span className="plan-number-pick"><small>推荐号码</small><strong>{activeMaster.numbers.map(number => <i key={number}>{number}</i>)}</strong></span>}
          {(mode === 'combo' || mode === 'size') && activeMaster.size && <span className={activeMaster.size === '大' ? 'blue' : 'orange'}><small>大小</small><b>{activeMaster.size}</b></span>}
          {(mode === 'combo' || mode === 'parity') && activeMaster.parity && <span className={activeMaster.parity === '单' ? 'blue' : 'orange'}><small>单双</small><b>{activeMaster.parity}</b></span>}
        </div>
        <p>{activeMaster.note || '本推荐由房间后台发布，仅供娱乐参考。'}</p>
      </article>}
      <section className="plan-history">
        <header><b>计划发布记录</b><span>{activeMaster?.master_hit_rate === null ? '暂无已结算命中率' : `历史命中率 ${activeMaster?.master_hit_rate?.toFixed(0)}%`}</span></header>
        <div className="plan-history-head"><span>期号/时间</span><span>推荐号码</span><span>方向</span><span>结果</span></div>
        {history.map(row => <div className="plan-history-row" key={row.id}>
          <span><b>{shortIssue(row.issue)}</b><small>{updateTime(row.updated_at)}</small></span>
          <span><b>{row.numbers.join('、')}</b><small>{row.master_title || row.master_name}</small></span>
          <PlanPickDisplay row={row} mode={mode} />
          <em className={row.result === 'hit' ? 'hit' : row.result === 'miss' ? 'miss' : ''}>{row.result === 'hit' ? '中' : row.result === 'miss' ? '未中' : '待开奖'}</em>
        </div>)}
        {history.length === 0 && <p className="plan-history-loading">暂无计划发布记录</p>}
      </section>
    </>}
  </section>
}
