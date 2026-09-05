import { useMemo, useState } from 'react'
import type { PlanGameSummary, PlanRecommendation, RacingPlanRecommendation, RacingPlanSelection } from '../api/plans'
import { Icon } from '../components/Icon'
import { ballTone } from '../data/games'
import type { Game } from '../types'
import { usePlanCatalog, usePlanDetail, useRacingPlanStream } from '../hooks/usePlanFeed'
import { displayedPlanMasters, planIsCurrent, planMasterIdentity, planResultLabel, recentPlanHistory } from '../utils/planPresentation'
import { PlanSelectionSheet } from '../components/PlanSelectionSheet'
import { isRacingPlanGame, racingPlanResultLabel, racingPlanDirection, racingPlanHistory, racingPlanIsCurrent, racingPlanMasters, racingPlanPositionLabel, racingPlanProgress } from '../utils/racingPlans'
import { markSixDrawBallClass, markSixBallClass, usesMarkSixDrawPresentation } from '../utils/lotteryRules'

const PC_PLAN_GAMES = new Set(['pc-canada', 'canada-28', 'canada-20'])

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
  if (PC_PLAN_GAMES.has(game.id)) {
    const numbers = game.balls.slice(0, 3)
    const sum = numbers.reduce((total, number) => total + number, 0)
    return <div className="plan-result-balls plan-canada-result" aria-label={`上期号码 ${numbers.join('、')}，和值 ${sum}`}>
      {numbers.map((number, index) => <span key={`${game.id}-${index}`}><b className={`canada-tone-${index + 1}`}>{number}</b>{index < numbers.length - 1 && <i>+</i>}</span>)}
      <i>=</i><strong>{sum}</strong>
    </div>
  }
  if (usesMarkSixDrawPresentation(game.id)) {
    return <div className="plan-result-balls" aria-label={`上期号码 ${game.balls.join('、')}`}>
      {game.balls.map((number, ballIndex) => <b className={markSixDrawBallClass(number, ballIndex, game.balls.length)} key={`${game.id}-${ballIndex}`}>{String(number).padStart(2, '0')}</b>)}
    </div>
  }
  return <div className="plan-result-balls" aria-label={`上期号码 ${game.balls.join('、')}`}>
    {game.balls.map((number, ballIndex) => <b className={ballTone(number)} key={`${game.id}-${ballIndex}`}>{number}</b>)}
  </div>
}

function planPickNumberClass(gameId: string, number: number) {
  return usesMarkSixDrawPresentation(gameId) ? markSixBallClass(number) : undefined
}

function PlanPickDisplay({ row, mode }: { row: PlanRecommendation; mode: PlanMode }) {
  const showSize = (mode === 'combo' || mode === 'size') && row.size
  const showParity = (mode === 'combo' || mode === 'parity') && row.parity
  return <span className="plan-history-pick has-numbers">
    {(mode === 'combo' || mode === 'numbers') && <strong aria-label={`推荐号码 ${row.numbers.join('、')}`}>{row.numbers.map(number => <i className={planPickNumberClass(row.game_id, number)} key={number}>{usesMarkSixDrawPresentation(row.game_id) ? String(number).padStart(2, '0') : number}</i>)}</strong>}
    {(showSize || showParity) && <small>
      {showSize && <b>{row.size}</b>}
      {showSize && showParity && <em>·</em>}
      {showParity && <b>{row.parity}</b>}
    </small>}
  </span>
}

export function PlanLobby({ games, onBack, onSelect }: { games: Game[]; onBack: () => void; onSelect: (gameId: string) => void }) {
  // Chats is keyed by the authenticated room, so room switches unmount this feed.
  const { data: catalog, loading, error } = usePlanCatalog()

  const planGames = useMemo(() => (catalog ?? []).map(summary => {
    const game = games.find(item => item.id === summary.game_id)
    return game ? { game, summary } : null
  }).filter((item): item is { game: Game; summary: PlanGameSummary } => Boolean(item)), [catalog, games])

  return <section className="plan-page plan-lobby-page">
    <header className="blue-header plan-header"><button aria-label="返回消息列表" onClick={onBack}><Icon name="back" /></button><b>计划群</b><span aria-hidden="true" /></header>
    <div className="plan-lobby-overview">
      <div><small>EXPERT PLAN</small><b>专家推荐</b><span>访问时更新 · 最近 6 期 · 仅供娱乐参考</span></div>
      <div className="plan-overview-masters"><em>{planGames.length} 个彩种</em></div>
    </div>
    <div className="plan-game-list">
      {planGames.map(({ game, summary }) => <button className="plan-game-card" key={game.id} onClick={() => onSelect(game.id)}>
        <div className="plan-game-main">
          <span className="plan-game-logo">{game.logo ? <img alt={`${game.title} Logo`} src={game.logo} /> : game.tag.slice(0, 2)}</span>
          <div><b>{game.title}</b><small><i />{summary.latest_issue ? <>{summary.master_count} 位专家 · {summary.history_only ? '查看最近发布' : '本期计划'}</> : '可切换计划 · 等待首次发布'}</small></div>
          <em>{summary.latest_issue ? <>{summary.history_only ? '最近 ' : '第 '}{shortIssue(summary.history_only ? summary.latest_issue : summary.current_issue)} 期</> : '选择计划'}<Icon name="arrow" /></em>
        </div>
        <footer><span>上期</span><PlanResultBalls game={game} /></footer>
      </button>)}
      {loading && <p className="plan-empty">正在加载房间计划…</p>}
      {!loading && error && <p className="plan-empty">{error}</p>}
      {!loading && !error && planGames.length === 0 && <p className="plan-empty">当前房间暂无计划推荐</p>}
    </div>
  </section>
}

type PlanDetailProps = { games: Game[]; gameId?: string; onBack: () => void }

export function PlanDetail(props: PlanDetailProps) {
  return props.gameId && isRacingPlanGame(props.gameId) ? <RacingPlanDetail {...props} /> : <LegacyPlanDetail {...props} />
}

function RacingPlanPick({ row, compact = false }: { row: RacingPlanRecommendation; compact?: boolean }) {
  const direction = racingPlanDirection(row)
  return row.kind === 'numbers'
    ? <strong className={`racing-plan-numbers${compact ? ' is-compact' : ''}`} aria-label={`推荐号码 ${row.numbers.join('、')}`}>{row.numbers.map((number, index) => <b className={ballTone(number)} key={`${number}-${index}`}>{number}</b>)}</strong>
    : <strong className={`racing-plan-direction${compact ? ' is-compact' : ''}`}>{direction || '等待发布'}</strong>
}

function RacingPlanDetail({ games, gameId, onBack }: PlanDetailProps) {
  const game = games.find(item => item.id === gameId)
  const { data, loading, error, selection, activate, activating, activationError, clearActivationError } = useRacingPlanStream('', gameId ?? '')
  const [selectedMasterIdentity, setSelectedMasterIdentity] = useState('')
  const [selectorOpen, setSelectorOpen] = useState(false)
  const masters = useMemo(() => racingPlanMasters(data, selection), [data, selection])
  const activeMaster = masters.find(master => planMasterIdentity(master) === selectedMasterIdentity) ?? masters[0]
  const history = useMemo(() => racingPlanHistory(data, selection, activeMaster), [data, selection, activeMaster])
  const current = racingPlanIsCurrent(data, selection, activeMaster)
  const position = data?.positions.find(item => item.position === selection.position)
  const option = data?.options.find(item => item.key === selection.plan_key)
  const positionLabel = position?.label ?? racingPlanPositionLabel(selection.position)
  const planLabel = option?.label ?? (selection.plan_key === 'four-period-five-codes' ? '四期五码' : selection.plan_key)
  const statsLabel = (master?: RacingPlanRecommendation) => master && master.master_sample_count > 0 && master.master_hit_rate !== null
    ? `命中 ${master.master_hit_rate.toFixed(0)}% · ${master.master_sample_count}期`
    : '暂无开奖样本'
  const confirmSelection = async (next: RacingPlanSelection) => { if (await activate(next)) setSelectorOpen(false) }
  const openSelector = () => { clearActivationError(); setSelectorOpen(true) }

  if (!game) return <section className="plan-page"><header className="blue-header"><button aria-label="返回计划群" onClick={onBack}><Icon name="back" /></button><b>计划详情</b><span /></header><p className="plan-empty">该彩票计划暂未开放。</p></section>

  return <section className="plan-page plan-detail-page racing-plan-page">
    <header className="blue-header plan-header"><button aria-label="返回计划群" onClick={onBack}><Icon name="back" /></button><b>{game.title}</b><button type="button" className="plan-switch-button" aria-label="切换计划" aria-haspopup="dialog" aria-expanded={selectorOpen} disabled={!data || activating} onClick={openSelector}><Icon name="switch" /><span>切换计划</span></button></header>
    <div className="plan-current">
      <span className="plan-current-logo">{game.logo ? <img alt={`${game.title} Logo`} src={game.logo} /> : game.tag.slice(0, 2)}</span>
      <div><small>当前计划</small><b>{positionLabel} · {planLabel}</b><small>{loading ? '正在读取独立计划…' : current ? `第 ${shortIssue(activeMaster.issue)} 期` : '等待本期计划'}</small></div>
      <time>{activeMaster ? `更新 ${updateTime(activeMaster.updated_at)}` : ''}</time>
    </div>
    {data && <div className="racing-plan-stream-status"><span>{!data.stream.allowed ? '当前计划暂未开放 · 只读记录' : data.automation_enabled ? '本页可见时每 15 秒更新' : '自动推荐已关闭 · 只读记录'}</span><small>访问中 {data.stream.active_count} / {data.stream.max_active}</small></div>}
    {error && <p className="racing-plan-message" role="alert">{error}</p>}
    {data && !data.stream.allowed && <p className="racing-plan-message">当前名次或计划类型未开放，请切换其他已开放计划，或联系房间管理员。</p>}
    {!selectorOpen && activationError && <p className="racing-plan-message" role="alert">{activationError}</p>}
    {!loading && !error && !masters.length && <p className="plan-empty">当前计划正在准备中。{data?.automation_enabled && data.stream.allowed ? '浏览本页时，开放期内会自动更新。' : ''}</p>}
    {masters.length > 0 && <>
      <div className="plan-master-tabs" role="tablist" aria-label="选择计划专家">
        {masters.map((master, index) => <button type="button" aria-selected={activeMaster ? planMasterIdentity(activeMaster) === planMasterIdentity(master) : false} className={activeMaster && planMasterIdentity(activeMaster) === planMasterIdentity(master) ? 'active' : ''} key={planMasterIdentity(master)} onClick={() => setSelectedMasterIdentity(planMasterIdentity(master))} role="tab"><span style={{ background: master.master_color }}>{index + 1}</span><b>{master.master_name}</b><small>{statsLabel(master)}</small></button>)}
      </div>
      {activeMaster && <article className="plan-recommendation racing-plan-recommendation">
        <header><div><small>{activeMaster.master_name} · {positionLabel}</small><b>{planLabel}</b></div><em>{current ? '本期计划' : '历史计划'}</em></header>
        <div className="racing-plan-cycle"><b>{racingPlanProgress(activeMaster)}</b><span className={`plan-outcome ${activeMaster.result}`}>{racingPlanResultLabel(activeMaster)}</span><small>起始期号 {shortIssue(activeMaster.cycle_start_issue)}</small></div>
        {!current && <p className="racing-plan-history-warning" role="note">历史计划，非本期推荐；仅展示当前所选名次与类型的最近发布。</p>}
        <div className="racing-plan-picks"><small>第 {shortIssue(activeMaster.issue)} 期 · {activeMaster.kind === 'numbers' ? `${activeMaster.numbers.length}码推荐` : option?.kind === 'dragon_tiger' ? `${positionLabel} vs ${racingPlanPositionLabel(position?.opponent_position ?? 11 - selection.position)}` : '推荐方向'}</small><RacingPlanPick row={activeMaster} /></div>
        {activeMaster.draw_numbers?.length === 10 && <div className="racing-plan-draw"><small>本期开奖号码</small><div className="racing-plan-numbers" aria-label={`开奖号码 ${activeMaster.draw_numbers.join('、')}`}>{activeMaster.draw_numbers.map((number, index) => <b className={`${ballTone(number)}${index + 1 === selection.position ? ' is-target' : ''}`} key={index}>{number}</b>)}</div></div>}
      </article>}
      <section className="plan-history racing-plan-history">
        <header><b>最近 6 期发布记录</b><span>{statsLabel(activeMaster)}</span></header>
        <div className="plan-history-head"><span>期号 / 周期</span><span>推荐内容</span><span>结果</span></div>
        {history.map(row => <div className="plan-history-row" key={row.id}>
          <span><b>{shortIssue(row.issue)}</b><small>{racingPlanProgress(row)}</small><small>{updateTime(row.updated_at)}</small></span>
          <RacingPlanPick row={row} compact />
          <em className={`plan-outcome ${row.result}`}>{racingPlanResultLabel(row)}</em>
        </div>)}
        {!history.length && <p className="plan-history-loading">当前所选专家暂无此计划的发布记录</p>}
      </section>
    </>}
    {selectorOpen && data && <PlanSelectionSheet selection={selection} positions={data.positions} options={data.options} allowedPositions={data.allowed_positions} allowedPlanKeys={data.allowed_plan_keys} submitting={activating} error={activationError} onCancel={() => setSelectorOpen(false)} onConfirm={next => void confirmSelection(next)} onEdit={clearActivationError} />}
  </section>
}

function LegacyPlanDetail({ games, gameId, onBack }: PlanDetailProps) {
  const game = games.find(item => item.id === gameId)
  const { data, loading, error } = usePlanDetail(gameId ?? '')
  const [selectedMasterIdentity, setSelectedMasterIdentity] = useState('')
  const [mode, setMode] = useState<PlanMode>('combo')
  const masters = useMemo(() => displayedPlanMasters(data), [data])
  const hasDirections = useMemo(() => [...masters, ...(data?.history ?? [])].some(row => Boolean(row.size || row.parity)), [data?.history, masters])
  const displayMode = hasDirections ? mode : 'numbers'
  const activeMaster = masters.find(master => planMasterIdentity(master) === selectedMasterIdentity) ?? masters[0]
  const isCurrent = planIsCurrent(data, activeMaster)
  const isAutomatic = activeMaster?.source === 'demo'
  const history = useMemo(() => activeMaster ? recentPlanHistory((data?.history ?? []).filter(row => row.game_id === activeMaster.game_id && row.master_name === activeMaster.master_name && row.source === activeMaster.source)) : [], [activeMaster, data?.history])
  const statsLabel = (master?: PlanRecommendation) => master && master.master_sample_count > 0 && master.master_hit_rate !== null
    ? `命中 ${master.master_hit_rate.toFixed(0)}% · ${master.master_sample_count}期`
    : '暂无开奖样本'

  if (!game) return <section className="plan-page"><header className="blue-header"><button aria-label="返回计划群" onClick={onBack}><Icon name="back" /></button><b>计划详情</b><span /></header><p className="plan-empty">该彩票计划暂未开放。</p></section>

  return <section className="plan-page plan-detail-page is-simple-plan">
    <header className="blue-header plan-header"><button aria-label="返回计划群" onClick={onBack}><Icon name="back" /></button><b>{game.title}</b><span aria-hidden="true" /></header>
    <div className="plan-current">
      <span className="plan-current-logo">{game.logo ? <img alt={`${game.title} Logo`} src={game.logo} /> : game.tag.slice(0, 2)}</span>
      <div><small>{activeMaster ? `第 ${shortIssue(activeMaster.issue)} 期` : '等待发布'}</small><b>{loading ? '正在读取' : activeMaster ? (isCurrent ? '本期计划' : '最近发布 · 历史计划') : '暂无计划'}</b></div>
      <time>{activeMaster ? `更新 ${updateTime(activeMaster.updated_at)}` : ''}</time>
    </div>
    {error && <p className="plan-empty">{error}</p>}
    {!loading && !error && masters.length === 0 && <p className="plan-empty">当前彩种计划正在准备中，请稍后刷新。</p>}
    {masters.length > 0 && <>
      <div className="plan-master-tabs" role="tablist" aria-label="选择计划专家">
        {masters.map((master, index) => <button aria-selected={activeMaster ? planMasterIdentity(activeMaster) === planMasterIdentity(master) : false} className={activeMaster && planMasterIdentity(activeMaster) === planMasterIdentity(master) ? 'active' : ''} key={planMasterIdentity(master)} onClick={() => setSelectedMasterIdentity(planMasterIdentity(master))} role="tab"><span style={{ background: master.master_color }}>{index + 1}</span><b>{master.master_name}</b><small>{statsLabel(master)}</small></button>)}
      </div>
      {hasDirections && <div className="plan-mode-tabs">{([['combo', '综合'], ['numbers', '号码'], ['size', '大小'], ['parity', '单双']] as const).map(([value, label]) => <button className={mode === value ? 'active' : ''} key={value} onClick={() => setMode(value)}>{label}</button>)}</div>}
      {activeMaster && <article className="plan-recommendation">
        <header><div><small>{activeMaster.master_name}</small><b>第 {shortIssue(activeMaster.issue)} 期{isCurrent ? '推荐' : '历史计划'}</b></div><em>{updateTime(activeMaster.updated_at)} 更新</em></header>
        {!isCurrent && <p role="note">历史计划，非本期推荐；新一期发布后自动更新。</p>}
        <div className="plan-picks">
          {(displayMode === 'combo' || displayMode === 'numbers') && <span className="plan-number-pick"><small>推荐号码</small><strong aria-label={`推荐号码 ${activeMaster.numbers.join('、')}`}>{activeMaster.numbers.map(number => <i className={planPickNumberClass(activeMaster.game_id, number)} key={number}>{usesMarkSixDrawPresentation(activeMaster.game_id) ? String(number).padStart(2, '0') : number}</i>)}</strong></span>}
          {(displayMode === 'combo' || displayMode === 'size') && activeMaster.size && <span className={activeMaster.size === '大' ? 'blue' : 'orange'}><small>大小</small><b>{activeMaster.size}</b></span>}
          {(displayMode === 'combo' || displayMode === 'parity') && activeMaster.parity && <span className={activeMaster.parity === '单' ? 'blue' : 'orange'}><small>单双</small><b>{activeMaster.parity}</b></span>}
        </div>
        <p>{isAutomatic ? (activeMaster.note || '系统自动生成，仅供娱乐参考，不保证命中。') : activeMaster.note || '本推荐由后台发布，仅供娱乐参考。'}</p>
      </article>}
      <section className="plan-history">
        <header><b>最近 6 期发布记录</b><span>{statsLabel(activeMaster)}</span></header>
        <div className="plan-history-head"><span>期号/时间</span><span>推荐内容</span><span>结果</span></div>
        {history.map(row => <div className="plan-history-row" key={row.id}>
          <span><b>{shortIssue(row.issue)}</b><small>{updateTime(row.updated_at)}</small></span>
          <PlanPickDisplay row={row} mode={displayMode} />
          <em className={row.result === 'hit' ? 'hit' : row.result === 'miss' ? 'miss' : ''}>{planResultLabel(row)}</em>
        </div>)}
        {history.length === 0 && <p className="plan-history-loading">暂无计划发布记录</p>}
      </section>
    </>}
  </section>
}
