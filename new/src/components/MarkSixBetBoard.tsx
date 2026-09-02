import { useEffect, useMemo, useState } from 'react'
import type { AssistantBetResult, WebBetBatchItem } from '../api/bets'
import type { GameOdds } from '../api/portal'
import type { Game } from '../types'
import { formatBetAmount } from '../utils/betAmount'
import { controlSurfaceProps } from '../utils/controlSurface'
import { boardAmountCents } from '../utils/fullBetSelection'
import {
  markSixBatchError,
  markSixBatchItems,
  markSixCategories,
  markSixComboTicket,
  markSixMarketsForCategory,
  markSixNumberFilters,
  markSixNumberFilterValues,
  markSixOddsItem,
  markSixOptionPlayCode,
  markSixSingleTicket,
  markSixTicketGroups,
  markSixTicketKey,
  toggleMarkSixDraft,
  toggleMarkSixTicket,
  toggleMarkSixTickets,
  type MarkSixCategoryID,
  type MarkSixMarketSpec,
  type MarkSixTicket,
} from '../utils/markSixBetSelection'
import { markSixBallClass, markSixZodiac } from '../utils/lotteryRules'
import { roomBettingTarget } from '../utils/gameRoomBetting'
import { oddsLabel } from '../utils/gameRoomSafety'
import './mark-six-ball.css'
import './mark-six-bet-board.css'

type Props = {
  game: Game
  oddsInfo: GameOdds | null
  rulesReady: boolean
  rulesMessage: string
  submitting?: boolean
  active?: boolean
  surfaceId?: string
  onConfirm: (items: WebBetBatchItem[]) => Promise<AssistantBetResult | null>
}

type ContextCart = { context: string; items: MarkSixTicket[] }
type ComboDraft = { marketId: string; values: string[] }

export function MarkSixBetBoard({ game, oddsInfo, rulesReady, rulesMessage, submitting, active = true, surfaceId, onConfirm }: Props) {
  const bettingTarget = roomBettingTarget(game)
  const context = `${game.id}:${bettingTarget.issue}:mark-six`
  const [cart, setCart] = useState<ContextCart>({ context, items: [] })
  const [category, setCategory] = useState<MarkSixCategoryID>('special_a')
  const [marketId, setMarketId] = useState('special_a_number')
  const [position, setPosition] = useState(1)
  const [comboDraft, setComboDraft] = useState<ComboDraft>({ marketId: '', values: [] })
  const [amount, setAmount] = useState('20')
  const [selectionOpen, setSelectionOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [accepted, setAccepted] = useState<AssistantBetResult | null>(null)

  useEffect(() => {
    setCart({ context, items: [] })
    setCategory('special_a')
    setMarketId('special_a_number')
    setPosition(1)
    setComboDraft({ marketId: '', values: [] })
    setSelectionOpen(false)
    setAccepted(null)
  }, [context])

  const tickets = cart.context === context ? cart.items : []
  const categoryMarkets = markSixMarketsForCategory(category)
  const activeMarket = categoryMarkets.find(item => item.id === marketId) ?? categoryMarkets[0]
  const draftValues = comboDraft.marketId === activeMarket?.id ? comboDraft.values : []
  const quote = activeMarket?.playCode ? markSixOddsItem(activeMarket.playCode, oddsInfo) : null
  const usesOptionPlayCodes = Boolean(activeMarket?.options.some(option => option.playCode))
  const hasPlayCode = Boolean(activeMarket?.playCode) || usesOptionPlayCodes
  const hasConfiguredOption = Boolean(activeMarket?.options.some(option => {
    const playCode = markSixOptionPlayCode(activeMarket, option.value)
    return playCode && markSixOddsItem(playCode, oddsInfo)
  }))
  const busy = Boolean(submitting) || confirming
  const marketBlocked = Boolean(activeMarket?.blockedReason) || !hasPlayCode
  const marketReady = rulesReady && !busy && !marketBlocked && (usesOptionPlayCodes || quote !== null)
  const groups = markSixTicketGroups(tickets)
  const cents = boardAmountCents(amount)
  const totalCents = cents === null ? null : cents * tickets.length
  const total = totalCents !== null && Number.isSafeInteger(totalCents) ? formatBetAmount(totalCents / 100) : '—'
  const batchItems = markSixBatchItems(tickets, amount)
  const batchError = markSixBatchError(tickets, amount, oddsInfo)
  const canSubmit = rulesReady && !busy && bettingTarget.timing.accepting && tickets.length > 0 && batchItems.length === tickets.length && !batchError
  const zodiacFiltersReady = markSixZodiac(1, bettingTarget.timing.drawAtMs) !== null

  const chooseCategory = (next: MarkSixCategoryID) => {
    if (busy) return
    const nextMarket = markSixMarketsForCategory(next)[0]
    setCategory(next)
    setMarketId(nextMarket?.id ?? '')
    setComboDraft({ marketId: '', values: [] })
    setAccepted(null)
  }

  const chooseMarket = (next: MarkSixMarketSpec) => {
    if (busy) return
    setMarketId(next.id)
    setComboDraft({ marketId: '', values: [] })
    setAccepted(null)
  }

  const updateTickets = (update: (previous: MarkSixTicket[]) => MarkSixTicket[]) => {
    setCart(previous => ({ context, items: update(previous.context === context ? previous.items : []) }))
    setAccepted(null)
  }

  const chooseOption = (value: string) => {
    if (!activeMarket || !marketReady) return
    const playCode = markSixOptionPlayCode(activeMarket, value)
    if (!playCode || !markSixOddsItem(playCode, oddsInfo)) return
    if (activeMarket.pickCount > 1) {
      setComboDraft(previous => ({
        marketId: activeMarket.id,
        values: toggleMarkSixDraft(previous.marketId === activeMarket.id ? previous.values : [], value, activeMarket),
      }))
      return
    }
    const ticket = markSixSingleTicket(activeMarket, value, position)
    if (ticket) updateTickets(previous => toggleMarkSixTicket(previous, ticket))
  }

  const addCombo = () => {
    if (!activeMarket || !marketReady) return
    const ticket = markSixComboTicket(activeMarket, draftValues, position)
    if (!ticket) return
    updateTickets(previous => previous.some(item => markSixTicketKey(item) === markSixTicketKey(ticket)) ? previous : [...previous, ticket])
    setComboDraft({ marketId: activeMarket.id, values: [] })
  }

  const applyNumberFilter = (filterId: typeof markSixNumberFilters[number]['id']) => {
    if (!activeMarket || !marketReady || activeMarket.pickCount !== 1 || activeMarket.optionKind !== 'number') return
    const filteredTickets = markSixNumberFilterValues(filterId, bettingTarget.timing.drawAtMs)
      .map(value => markSixSingleTicket(activeMarket, value, position))
      .filter((ticket): ticket is MarkSixTicket => ticket !== null)
    updateTickets(previous => toggleMarkSixTickets(previous, filteredTickets))
  }

  const submit = async () => {
    if (!canSubmit) return
    setConfirming(true)
    try {
      const result = await onConfirm(batchItems)
      if (!result) return
      setAccepted(result)
      setCart({ context, items: [] })
      setComboDraft({ marketId: '', values: [] })
      setSelectionOpen(false)
    } finally {
      setConfirming(false)
    }
  }

  const optionButtons = useMemo(() => activeMarket?.options ?? [], [activeMarket])
  const selectionLabel = activeMarket?.pickCount && activeMarket.pickCount > 1
    ? `请选择 ${activeMarket.pickCount} 项组成一注`
    : activeMarket?.positionMode === 'regular-position'
      ? `正${position} · 每个号码一注`
      : '每个所选项均按单注金额计费'

  return <div aria-hidden={!active || undefined} className="full-bet-layer full-bet-drawer mark-six-bet-layer" hidden={!active} id={surfaceId}>
    <section className="full-bet-board embedded mark-six-bet-board" aria-label="宾果六合彩网投面板" {...controlSurfaceProps}>
      <div className="full-bet-workspace">
        <aside aria-label="六合彩投注玩法">{markSixCategories.map(item => <button type="button" key={item.id} aria-pressed={category === item.id} className={category === item.id ? 'active' : ''} disabled={busy} onClick={() => chooseCategory(item.id)}>{item.label}</button>)}</aside>
        <section className="full-bet-content">
          <header><div><b>{markSixCategories.find(item => item.id === category)?.label}</b><small>{selectionLabel}</small></div><span>第 <b>{bettingTarget.issue}</b> 期</span></header>
          {!rulesReady && <p className="mark-six-rule-notice" role="status">{rulesMessage}</p>}
          {categoryMarkets.length > 1 && <nav className="mark-six-market-tabs" aria-label={`${markSixCategories.find(item => item.id === category)?.label}子玩法`}>{categoryMarkets.map(item => <button type="button" key={item.id} aria-pressed={activeMarket?.id === item.id} className={activeMarket?.id === item.id ? 'active' : ''} disabled={busy} onClick={() => chooseMarket(item)}>{item.label}{item.blockedReason && <small>待核验</small>}</button>)}</nav>}
          {activeMarket?.positionMode === 'regular-position' && <div className="rank-selector mark-six-position-selector" aria-label="切换正码位置（独立编辑）">{Array.from({ length: 6 }, (_, index) => index + 1).map(value => <button type="button" key={value} aria-label={`编辑正${value}`} aria-pressed={position === value} className={position === value ? 'active' : ''} disabled={busy} onClick={() => { if (!busy) { setPosition(value); setComboDraft({ marketId: '', values: [] }) } }}>正{value}</button>)}</div>}
          {(category === 'special_a' || category === 'special_b') && activeMarket?.optionKind === 'number' && <div className="mark-six-number-filters" aria-label="批量筛选特码号码">{markSixNumberFilters.map(filter => {
            const zodiacFilter = filter.id === 'domestic' || filter.id === 'wild'
            const unavailable = zodiacFilter && !zodiacFiltersReady
            return <button type="button" key={filter.id} disabled={!marketReady || unavailable} title={unavailable ? '生肖年份暂时无法识别' : undefined} onClick={() => applyNumberFilter(filter.id)}>{filter.label}</button>
          })}</div>}
          {activeMarket?.blockedReason && <p className="mark-six-market-notice" role="status">{activeMarket.blockedReason}，当前仅展示入口，不会生成注单。</p>}
          {!activeMarket?.blockedReason && hasPlayCode && !hasConfiguredOption && <p className="mark-six-market-notice" role="status">该玩法赔率待配置，当前不可选择或提交。</p>}
          {activeMarket && <div className={activeMarket.optionKind === 'number' ? 'full-bet-numbers mark-six-number-grid' : 'full-bet-options mark-six-value-grid'}>{optionButtons.map(option => {
            const optionPlayCode = markSixOptionPlayCode(activeMarket, option.value)
            const optionQuote = optionPlayCode ? markSixOddsItem(optionPlayCode, oddsInfo) : null
            const single = activeMarket.pickCount === 1 ? markSixSingleTicket(activeMarket, option.value, position) : null
            const selected = activeMarket.pickCount > 1
              ? draftValues.includes(option.value)
              : Boolean(single && tickets.some(ticket => markSixTicketKey(ticket) === markSixTicketKey(single)))
            const maxed = activeMarket.pickCount > 1 && draftValues.length >= activeMarket.pickCount && !selected
            return <button type="button" key={option.value} data-choice={option.value} aria-label={`${activeMarket.label}${activeMarket.positionMode === 'regular-position' ? `正${position}` : ''}${option.label}`} aria-pressed={selected} className={`board-choice${selected ? ' selected' : ''}`} disabled={!marketReady || !optionQuote || maxed} onClick={() => chooseOption(option.value)}>
              <b className={activeMarket.optionKind === 'number' ? markSixBallClass(Number(option.value)) : ''}>{option.label}</b>
              <small>{activeMarket.blockedReason ? '待核验' : !optionQuote ? '待配置' : oddsLabel(optionQuote.odds, 3, oddsInfo?.show_odds === false)}</small>
            </button>
          })}</div>}
          {activeMarket && activeMarket.pickCount > 1 && <div className="mark-six-combo-bar"><span>已选 {draftValues.length}/{activeMarket.pickCount}</span><button type="button" disabled={!marketReady || draftValues.length !== activeMarket.pickCount || busy} onClick={addCombo}>加入{activeMarket.label}清单</button></div>}
        </section>
      </div>
      <footer className="full-bet-footer">
        {accepted && <p className="mark-six-accepted" role="status">第 {accepted.issue} 期已受理 {accepted.bet_count} 注，合计 ¥ {formatBetAmount(accepted.total)}</p>}
        {(batchError || (!rulesReady && tickets.length > 0)) && <p className="board-command-error" role="alert">{!rulesReady ? rulesMessage : batchError}</p>}
        <div className="full-bet-summary"><button type="button" disabled={busy || !tickets.length} onClick={() => updateTickets(() => [])}>清空选择</button><button type="button" className="full-bet-selection-toggle" aria-expanded={selectionOpen} onClick={() => setSelectionOpen(previous => !previous)}><span aria-live="polite">已选 <b>{groups.length}</b> 组 · <b>{tickets.length}</b> 注</span><i>{selectionOpen ? '收起' : '查看清单'}</i></button></div>
        {tickets.length > 0 && <div className="board-selected-preview" aria-label="已选投注">{groups.map(group => <span key={group.key}>{group.label} <b>{group.choices.map(item => item.selectionLabel).join('、')}</b></span>)}</div>}
        {selectionOpen && <div className="full-bet-selection-list"><header><b>本次网投清单</b><span>合计 ¥ {total}</span></header>{groups.length ? groups.map(group => <article key={group.key}><div><b>{group.label}</b><div className="board-selection-chips">{group.choices.map(ticket => <button type="button" key={markSixTicketKey(ticket)} disabled={busy} aria-label={`移除${group.label}${ticket.selectionLabel}`} onClick={() => updateTickets(previous => previous.filter(item => markSixTicketKey(item) !== markSixTicketKey(ticket)))}>{ticket.selectionLabel}<span aria-hidden="true"> ×</span></button>)}</div></div></article>) : <p>暂未选择玩法或号码</p>}</div>}
        <div className="amount-pills mark-six-amount-pills" aria-label="单注金额">{[10, 20, 50, 100, 200].map(value => <button type="button" key={value} aria-pressed={cents === value * 100} className={cents === value * 100 ? 'active' : ''} disabled={busy} onClick={() => { if (!busy) setAmount(String(value)) }}>{value}</button>)}</div>
        <div className="board-submit-row"><label className="board-custom-amount">单注<input aria-label="自定义单注金额" aria-invalid={cents === null} inputMode="decimal" autoComplete="off" placeholder="金额" disabled={busy} value={amount} onChange={event => { if (!busy) setAmount(event.target.value) }} /></label><button type="button" className="full-bet-confirm" disabled={!canSubmit} onClick={() => void submit()}>{busy ? '提交中…' : !rulesReady ? '规则待配置' : !bettingTarget.timing.accepting ? bettingTarget.timing.statusLabel : batchError ? '请检查投注清单' : '立即投注'} <small>¥ {total}</small></button></div>
      </footer>
    </section>
  </div>
}
