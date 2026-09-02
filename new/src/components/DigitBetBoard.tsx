import { useEffect, useState } from 'react'
import type { Game } from '../types'
import type { GameOdds } from '../api/portal'
import { formatBetAmount } from '../utils/betAmount'
import { controlSurfaceProps } from '../utils/controlSurface'
import { digitBallLabel, digitChoice, digitCommandLengthError, digitDragonLabel, digitDragonPositions, digitDragonSelections, digitNumbers, digitPatternPositions, digitPatterns, digitPatternTarget, digitSelectionCommand, digitSelectionGroups, digitSelectionKey, digitSides, toggleDigitChoice, type DigitBetKind, type DigitSelection } from '../utils/digitBetSelection'
import { boardAmountCents } from '../utils/fullBetSelection'
import { roomBettingTarget } from '../utils/gameRoomBetting'
import { canSubmitPlayWithOddsResponse, oddsForPlayCode, oddsLabel, type PlayOdds } from '../utils/gameRoomSafety'
import { isDigit5V3Game } from '../utils/lotteryRules'
import { Icon } from './Icon'
import './digit-bet-board.css'

type Props = {
  game: Game
  ballCount: 3 | 5
  submitting?: boolean
  embedded?: boolean
  active?: boolean
  surfaceId?: string
  /** Exact authoritative version returned by the odds/status endpoint. */
  ruleVersion?: string
  odds: PlayOdds
  oddsHidden: boolean
  oddsResponseReady: boolean
  oddsInfo?: GameOdds | null
  onConfirm: (content: string) => void
  onClose: () => void
}
const baseTabs: Array<{ id: DigitBetKind; label: string }> = [
  { id: 'ball', label: '单球' }, { id: 'sum', label: '总和' }, { id: 'sum_tail', label: '总和尾' }, { id: 'pattern', label: '前三形态' }, { id: 'dragon_tiger', label: '龙虎' },
]

export function DigitBetBoard({ game, ballCount, submitting, embedded = false, active = true, surfaceId, ruleVersion, odds, oddsHidden, oddsResponseReady, oddsInfo, onConfirm, onClose }: Props) {
  const effectiveRuleVersion = ruleVersion ?? game.ruleVersion ?? ''
  const context = `${game.id}:${effectiveRuleVersion}:${ballCount}`
  const [cart, setCart] = useState<{ context: string; items: DigitSelection[] }>({ context, items: [] })
  const [tab, setTab] = useState<DigitBetKind>('ball')
  const [position, setPosition] = useState(1)
  const [patternPosition, setPatternPosition] = useState(1)
  const [dragonPosition, setDragonPosition] = useState(1)
  const [amount, setAmount] = useState('20')
  const [selectionOpen, setSelectionOpen] = useState(false)
  useEffect(() => {
    setCart({ context, items: [] })
    setTab('ball')
    setPosition(1)
    setPatternPosition(1)
    setDragonPosition(1)
    setSelectionOpen(false)
  }, [context])
  // A route change must not offer the previous game's cart even before effects run.
  const selections = cart.context === context ? cart.items : []
  const v3 = isDigit5V3Game(game.id, effectiveRuleVersion)
  // The supplied speed/AU v3 manual does not define the legacy local sum or
  // sum-tail extensions. Keep them available to v2 products, but do not
  // advertise or serialize them under the stricter v3 contract.
  const tabs = baseTabs
    .filter(item => !v3 || (item.id !== 'sum' && item.id !== 'sum_tail'))
    .map(item => item.id === 'pattern' && v3 ? { ...item, label: '三段形态' } : item)
  const activePosition = position <= ballCount ? position : 1
  const activePatternPosition = v3 && digitPatternTarget(patternPosition) ? patternPosition : 1
  const dragonPositions = digitDragonPositions(ballCount, game.id, effectiveRuleVersion)
  const dragonSelections = digitDragonSelections(game.id, effectiveRuleVersion)
  const activeDragonPosition = dragonPositions.includes(dragonPosition) ? dragonPosition : 1
  const groups = digitSelectionGroups(selections, ballCount)
  const { issue, timing } = roomBettingTarget(game)
  const rulesReady = game.rulesReady !== false
  const cents = boardAmountCents(amount)
  const totalCents = cents === null ? null : cents * selections.length
  const validAmount = totalCents !== null && Number.isSafeInteger(totalCents)
  const total = validAmount ? formatBetAmount(totalCents / 100) : '—'
  const command = digitSelectionCommand(selections, amount, ballCount, game.id, effectiveRuleVersion)
  const lengthError = digitCommandLengthError(command)
  const playHasOdds = (playCode: string) => oddsInfo !== undefined
    ? canSubmitPlayWithOddsResponse(playCode, oddsInfo)
    : oddsHidden || oddsForPlayCode(playCode, odds) !== null
  const hasOdds = oddsResponseReady && selections.every(item => playHasOdds(item.playCode))
  const canSubmit = !submitting && rulesReady && timing.accepting && validAmount && selections.length > 0 && command !== '' && !lengthError && hasOdds
  const updateSelections = (update: (previous: DigitSelection[]) => DigitSelection[]) => {
    setCart(previous => ({ context, items: update(previous.context === context ? previous.items : []) }))
  }
  const option = (value: string) => {
    const optionPosition = tab === 'dragon_tiger' ? activeDragonPosition : tab === 'pattern' ? activePatternPosition : activePosition
    const item = digitChoice(tab, value, optionPosition, game.id, effectiveRuleVersion)
    if (!item) return null
    const selected = selections.some(choice => digitSelectionKey(choice) === digitSelectionKey(item))
    const disabled = Boolean(submitting) || !rulesReady || !oddsResponseReady || !playHasOdds(item.playCode)
    const label = tab === 'ball' ? digitBallLabel(activePosition) : tab === 'dragon_tiger' ? digitDragonLabel(activeDragonPosition, ballCount) : tab === 'pattern' ? digitPatternTarget(activePatternPosition)!.label : tabs.find(item => item.id === tab)!.label
    return <button type="button" key={value} className={`board-choice${selected ? ' selected' : ''}`} data-choice={value} aria-label={`${label}${value}`} aria-pressed={selected} disabled={disabled} onClick={() => { if (!disabled) updateSelections(previous => toggleDigitChoice(previous, item)) }}>
      <b>{value}</b><small>{oddsLabel(oddsForPlayCode(item.playCode, odds), 3, oddsHidden)}</small>
    </button>
  }
  const confirmLabel = submitting ? '提交中…' : !rulesReady ? '规则待配置' : !timing.accepting ? timing.statusLabel : !validAmount ? '金额格式不正确' : lengthError ? '请减少所选项' : !hasOdds && selections.length ? '赔率待配置' : '立即投注'
  return <div aria-hidden={!active || undefined} className={`full-bet-layer digit-bet-layer${embedded ? ' full-bet-drawer' : ''}`} hidden={!active} id={surfaceId} onClick={embedded || submitting ? undefined : onClose}>
    <section className={`full-bet-board digit-bet-board${embedded ? ' embedded' : ''}`} aria-label="数字彩投注面板" {...controlSurfaceProps} onClick={event => event.stopPropagation()}>
      {!embedded && <header className="full-bet-header">
        <button type="button" aria-label="返回游戏聊天室" disabled={submitting} onClick={onClose}><Icon name="back" /></button>
        <div><b>{game.title}</b><small>第 {issue} 期 · {timing.statusLabel}</small></div>
        <button type="button" className="full-bet-close" aria-label="关闭投注面板" disabled={submitting} onClick={onClose}>×</button>
      </header>}
      {!embedded && <div className="full-bet-current"><span>{timing.due}</span><i className={`full-bet-acceptance ${timing.accepting ? 'open' : 'closed'}`}>{timing.statusLabel}</i><div aria-label="最近开奖号码">{game.balls.map((ball, index) => <b key={index}>{ball}</b>)}</div></div>}
      <div className="full-bet-workspace">
        <aside aria-label="投注玩法">{tabs.map(item => <button type="button" key={item.id} aria-pressed={tab === item.id} disabled={submitting} className={tab === item.id ? 'active' : ''} onClick={() => { if (!submitting) setTab(item.id) }}>{item.label}</button>)}</aside>
        <section className="full-bet-content">
          <header><div><b>{tabs.find(item => item.id === tab)!.label}</b><small>{tab === 'ball' ? '各球位独立选择' : tab === 'dragon_tiger' ? (v3 ? '第一球与第五球比较，相同为和' : '各组独立选择') : tab === 'pattern' ? (v3 ? '前三、中三、后三独立选择' : '仅第一至第三球') : `全部 ${ballCount} 球${tab === 'sum_tail' ? '总和的个位数' : '之和'}`}</small></div>{embedded && <button type="button" className="detail-panel-collapse" aria-label="收起详细投注，返回聊天" disabled={submitting} onClick={onClose}><Icon name="arrow" /></button>}</header>
          {!rulesReady && <p className="digit-board-notice" role="status">该彩种规则尚未就绪，暂不可下注。</p>}
          {tab === 'ball' && <>
            <div className={`rank-selector digit-ball-selector digit-ball-selector-${ballCount}`} aria-label="切换球位（独立编辑）">{Array.from({ length: ballCount }, (_, index) => index + 1).map(value => {
              const count = selections.filter(item => item.kind === 'ball' && item.position === value).length
              return <button type="button" key={value} aria-label={`编辑${digitBallLabel(value)}`} aria-pressed={activePosition === value} disabled={submitting} className={activePosition === value ? 'active' : ''} onClick={() => { if (!submitting) setPosition(value) }}>{digitBallLabel(value)}{count > 0 && <small className="rank-count">{count}</small>}</button>
            })}</div>
            <p className="board-section-title">{digitBallLabel(activePosition)} · 号码 0–9</p>
            <div className="full-bet-numbers">{digitNumbers.map(option)}</div>
            <p className="board-section-title">{digitBallLabel(activePosition)} · 两面</p>
            <div className="full-bet-options">{digitSides.map(option)}</div>
          </>}
          {tab === 'sum' && <div className="full-bet-options">{digitSides.map(option)}</div>}
          {tab === 'sum_tail' && <div className="full-bet-numbers">{digitNumbers.map(option)}</div>}
          {tab === 'pattern' && <>
            {v3 && <div className="rank-selector digit-pattern-selector" aria-label="切换三段形态（独立编辑）">{digitPatternPositions.map(item => {
              const count = selections.filter(selection => selection.kind === 'pattern' && selection.position === item.position).length
              return <button type="button" key={item.position} aria-label={`编辑${item.label}`} aria-pressed={activePatternPosition === item.position} disabled={submitting} className={activePatternPosition === item.position ? 'active' : ''} onClick={() => { if (!submitting) setPatternPosition(item.position) }}>{item.target}{count > 0 && <small className="rank-count">{count}</small>}</button>
            })}</div>}
            <p className="board-section-title">{digitPatternTarget(activePatternPosition)!.label}</p>
            <div className="full-bet-options digit-pattern-options">{digitPatterns.map(item => option(item.selection))}</div>
          </>}
          {tab === 'dragon_tiger' && <>
            <div className={`rank-selector digit-dragon-selector digit-dragon-selector-${dragonPositions.length}`} aria-label="切换龙虎组（独立编辑）">{dragonPositions.map(value => {
              const count = selections.filter(item => item.kind === 'dragon_tiger' && item.position === value).length
              return <button type="button" key={value} aria-label={`编辑${digitDragonLabel(value, ballCount)}`} aria-pressed={activeDragonPosition === value} disabled={submitting} className={activeDragonPosition === value ? 'active' : ''} onClick={() => { if (!submitting) setDragonPosition(value) }}>{digitDragonLabel(value, ballCount)}{count > 0 && <small className="rank-count">{count}</small>}</button>
            })}</div>
            <p className="board-section-title">{digitDragonLabel(activeDragonPosition, ballCount)}</p>
            <div className="full-bet-options">{dragonSelections.map(option)}</div>
          </>}
        </section>
      </div>
      <footer className="full-bet-footer">
        <div className="full-bet-summary"><button type="button" disabled={submitting || !selections.length} onClick={() => { if (!submitting) updateSelections(() => []) }}>清空选择</button><button type="button" className="full-bet-selection-toggle" aria-expanded={selectionOpen} onClick={() => setSelectionOpen(previous => !previous)}><span aria-live="polite">已选 <b>{groups.length}</b> 组 · <b>{selections.length}</b> 注</span><i>{selectionOpen ? '收起' : '查看清单'}</i></button></div>
        {selections.length > 0 && <div className="board-selected-preview" aria-label="已选投注">{groups.map(group => <span key={group.rank}>{group.label} <b>{group.choices.map(item => item.selection).join('、')}</b></span>)}</div>}
        {selectionOpen && <div className="full-bet-selection-list"><header><b>本次投注清单</b><span>合计 ¥ {total}</span></header>{groups.length ? groups.map(group => <article key={group.rank}><div><b>{group.label}</b><div className="board-selection-chips">{group.choices.map(item => <button type="button" key={digitSelectionKey(item)} disabled={submitting} aria-label={`移除${group.label}${item.selection}`} onClick={() => { if (!submitting) updateSelections(previous => previous.filter(choice => digitSelectionKey(choice) !== digitSelectionKey(item))) }}>{item.selection}<span aria-hidden="true"> ×</span></button>)}</div></div></article>) : <p>暂未选择玩法或号码</p>}</div>}
        <div className="amount-pills" aria-label="单注金额">{[20, 50, 100, 200].map(value => <button type="button" key={value} aria-pressed={cents === value * 100} className={cents === value * 100 ? 'active' : ''} disabled={submitting} onClick={() => { if (!submitting) setAmount(String(value)) }}>{value}</button>)}</div>
        <p className="digit-board-amount-note">每个所选项均按单注金额计费</p>
        {!validAmount && <p className="digit-board-error" role="alert">金额须大于 0，最多 2 位小数，且合计不能超出安全范围。</p>}
        {lengthError && <p className="digit-board-error" role="alert">{lengthError}</p>}
        <div className="board-submit-row"><label className="board-custom-amount">单注<input aria-label="自定义单注金额" aria-invalid={!validAmount} inputMode="decimal" autoComplete="off" placeholder="金额" disabled={submitting} value={amount} onChange={event => { if (!submitting) setAmount(event.target.value) }} /></label><button type="button" className="full-bet-confirm" disabled={!canSubmit} onClick={() => { if (canSubmit) onConfirm(command) }}>{confirmLabel} <small>¥ {total}</small></button></div>
      </footer>
    </section>
  </div>
}
