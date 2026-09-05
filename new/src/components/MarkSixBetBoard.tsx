import { useEffect, useState } from 'react'
import type { AssistantBetResult, WebBetBatchItem } from '../api/bets'
import type { GameOdds, OddsItem } from '../api/portal'
import type { Game } from '../types'
import { formatBetAmount } from '../utils/betAmount'
import { controlSurfaceProps } from '../utils/controlSurface'
import { boardAmountCents } from '../utils/fullBetSelection'
import {
  markSixBatchError,
  markSixBatchItems,
  markSixCategories,
  markSixComboTicket,
  markSixMarket,
  markSixNumberFilters,
  markSixNumberFilterValues,
  markSixNumberSelectionValues,
  markSixOddsItem,
  markSixOptionPlayCode,
  markSixOptionPricingCode,
  markSixPosition,
  markSixPricingCodes,
  markSixSingleTicket,
  markSixTicketGroups,
  markSixTicketKey,
  toggleMarkSixDraft,
  toggleMarkSixFilterSelection,
  toggleMarkSixManualSelection,
  toggleMarkSixTicket,
  type MarkSixCategoryID,
  type MarkSixMarketSpec,
  type MarkSixNumberFilterID,
  type MarkSixNumberSelection,
  type MarkSixOption,
  type MarkSixTicket,
} from '../utils/markSixBetSelection'
import {
  markSixBoardMarkets,
  markSixBoardMarketFamily,
  markSixBoardOptionLabel,
  markSixBoardOptions,
  markSixBoardTabLabel,
  markSixBoardTabs,
  markSixBoardVariants,
  markSixOptionNumbers,
} from '../utils/markSixBoardPresentation'
import { markSixBallClass, markSixZodiac } from '../utils/lotteryRules'
import { roomBettingTarget } from '../utils/gameRoomBetting'
import { oddsLabel } from '../utils/gameRoomSafety'
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

type NumberCartEntry = { marketId: string; position: number; selection: MarkSixNumberSelection }
type ContextCart = { context: string; items: MarkSixTicket[]; numbers: Partial<Record<string, NumberCartEntry>> }
type ComboDraft = { marketId: string; values: string[] }
const hasNumberShortcuts = (marketId: string) => ['special_a_number', 'special_b_number', 'regular_number', 'regular_special_number'].includes(marketId)
const numberSelectionKey = (marketId: string, position: number) => `${marketId}:${position}`

export function MarkSixBetBoard({ game, oddsInfo, rulesReady, rulesMessage, submitting, active = true, surfaceId, onConfirm }: Props) {
  const bettingTarget = roomBettingTarget(game)
  const context = `${game.id}:${bettingTarget.issue}:mark-six`
  const [cart, setCart] = useState<ContextCart>({ context, items: [], numbers: {} })
  const [category, setCategory] = useState<MarkSixCategoryID>('special_a')
  const [marketId, setMarketId] = useState('special_a_number')
  const [position, setPosition] = useState(1)
  const [comboDraft, setComboDraft] = useState<ComboDraft>({ marketId: '', values: [] })
  const [amount, setAmount] = useState('20')
  const [selectionOpen, setSelectionOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [accepted, setAccepted] = useState<AssistantBetResult | null>(null)

  useEffect(() => {
    setCart({ context, items: [], numbers: {} })
    setCategory('special_a')
    setMarketId('special_a_number')
    setPosition(1)
    setComboDraft({ marketId: '', values: [] })
    setSelectionOpen(false)
    setAccepted(null)
  }, [context])

  const numberSelections = cart.context === context ? cart.numbers : {}
  const tickets = [
    ...(cart.context === context ? cart.items : []),
    ...Object.values(numberSelections).flatMap(entry => {
      const market = entry && markSixMarket(entry.marketId)
      if (!entry || !market || !hasNumberShortcuts(market.id)) return []
      return markSixNumberSelectionValues(entry.selection, bettingTarget.timing.drawAtMs)
        .map(value => markSixSingleTicket(market, value, entry.position))
        .filter((ticket): ticket is MarkSixTicket => ticket !== null)
    }),
  ]
  const marketTabs = markSixBoardTabs(category)
  const visibleMarkets = markSixBoardMarkets(category, marketId)
  const activeMarket = visibleMarkets[0]
  const numberMarket = visibleMarkets.find(market => market.optionKind === 'number')
  const shortcutMarket = numberMarket && hasNumberShortcuts(numberMarket.id) ? numberMarket : undefined
  const shortcutPosition = shortcutMarket ? markSixPosition(shortcutMarket, position) : null
  const draftValues = comboDraft.marketId === activeMarket?.id ? comboDraft.values : []
  const busy = Boolean(submitting) || confirming
  const isLinkedMarket = (market: MarkSixMarketSpec) => market.id.startsWith('link_zodiac_') || market.id.startsWith('link_tail_')
  const isTieredMarket = (market: MarkSixMarketSpec) => market.id === 'combo_3_2' || market.id === 'combo_2_special'
  const marketConfigured = (market: MarkSixMarketSpec) => {
    if (market.blockedReason) return false
    if (isLinkedMarket(market)) {
      return market.options.filter(option => {
        const code = markSixOptionPricingCode(market, option.value)
        return code && markSixOddsItem(code, oddsInfo)
      }).length >= market.pickCount
    }
    if (isTieredMarket(market)) return markSixPricingCodes(market).every(code => Boolean(markSixOddsItem(code, oddsInfo)))
    if (market.playCode) return Boolean(markSixOddsItem(market.playCode, oddsInfo))
    return market.options.some(option => {
      const code = markSixOptionPricingCode(market, option.value)
      return code && markSixOddsItem(code, oddsInfo)
    })
  }
  const marketReady = (market: MarkSixMarketSpec) => rulesReady && !busy && marketConfigured(market)
  const hasConfiguredOption = visibleMarkets.some(marketConfigured)
  const activeVariants = markSixBoardVariants(activeMarket)
  const activePricingQuotes = activeMarket
    ? markSixPricingCodes(activeMarket, draftValues).map(code => markSixOddsItem(code, oddsInfo)).filter((item): item is OddsItem => Boolean(item))
    : []
  const groups = markSixTicketGroups(tickets)
  const cents = boardAmountCents(amount)
  const totalCents = cents === null ? null : cents * tickets.length
  const total = totalCents !== null && Number.isSafeInteger(totalCents) ? formatBetAmount(totalCents / 100) : '—'
  const batchItems = markSixBatchItems(tickets, amount)
  const batchError = markSixBatchError(tickets, amount, oddsInfo)
  const canSubmit = rulesReady && !busy && bettingTarget.timing.accepting && tickets.length > 0 && batchItems.length === tickets.length && !batchError
  const zodiacFiltersReady = markSixZodiac(1, bettingTarget.timing.drawAtMs) !== null
  // Only explicitly clicked shortcuts are highlighted. Their number sets are
  // merged once per market; manual inclusions/exclusions remain independent.
  const selectedFilters = shortcutMarket && shortcutPosition !== null
    ? numberSelections[numberSelectionKey(shortcutMarket.id, shortcutPosition)]?.selection.filters ?? []
    : []
  const hasSelection = tickets.length > 0 || draftValues.length > 0 || Object.values(numberSelections).some(entry =>
    entry && (entry.selection.filters.length > 0 || entry.selection.included.length > 0 || entry.selection.excluded.length > 0),
  )

  const chooseCategory = (next: MarkSixCategoryID) => {
    if (busy) return
    const nextMarket = markSixBoardMarkets(next, '')[0]
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
    setCart(previous => ({ context, items: update(previous.context === context ? previous.items : []), numbers: previous.context === context ? previous.numbers : {} }))
    setAccepted(null)
  }

  const updateNumberSelection = (market: MarkSixMarketSpec, requestedPosition: number, update: (previous: MarkSixNumberSelection | undefined) => MarkSixNumberSelection) => {
    const marketPosition = markSixPosition(market, requestedPosition)
    if (marketPosition === null) return
    const key = numberSelectionKey(market.id, marketPosition)
    setCart(previous => {
      const current: ContextCart = previous.context === context ? previous : { context, items: [], numbers: {} }
      return { ...current, numbers: { ...current.numbers, [key]: {
        marketId: market.id,
        position: marketPosition,
        selection: update(current.numbers[key]?.selection),
      } } }
    })
    setAccepted(null)
  }

  const chooseOption = (market: MarkSixMarketSpec, value: string) => {
    if (!marketReady(market)) return
    const playCode = markSixOptionPlayCode(market, value)
    if (!playCode) return
    const pricingCode = markSixOptionPricingCode(market, value)
    if (!isTieredMarket(market) && (!pricingCode || !markSixOddsItem(pricingCode, oddsInfo))) return
    if (markSixOptionNumbers(market.id, value, bettingTarget.timing.drawAtMs)?.length === 0) return
    if (market.pickCount > 1) {
      setComboDraft(previous => ({
        marketId: market.id,
        values: toggleMarkSixDraft(previous.marketId === market.id ? previous.values : [], value, market),
      }))
      return
    }
    if (hasNumberShortcuts(market.id)) {
      updateNumberSelection(market, position, previous => toggleMarkSixManualSelection(previous, value, bettingTarget.timing.drawAtMs))
      return
    }
    const ticket = markSixSingleTicket(market, value, position)
    if (ticket) updateTickets(previous => toggleMarkSixTicket(previous, ticket))
  }

  const addCombo = () => {
    if (!activeMarket || !marketReady(activeMarket)) return
    const requiredPrices = markSixPricingCodes(activeMarket, draftValues)
    if (!requiredPrices.length || requiredPrices.some(code => !markSixOddsItem(code, oddsInfo))) return
    const ticket = markSixComboTicket(activeMarket, draftValues, position)
    if (!ticket) return
    updateTickets(previous => previous.some(item => markSixTicketKey(item) === markSixTicketKey(ticket)) ? previous : [...previous, ticket])
    setComboDraft({ marketId: activeMarket.id, values: [] })
  }

  const applyNumberFilter = (filterId: MarkSixNumberFilterID) => {
    if (!shortcutMarket || !marketReady(shortcutMarket)) return
    if (!markSixNumberFilterValues(filterId, bettingTarget.timing.drawAtMs).length) return
    updateNumberSelection(shortcutMarket, position, previous => toggleMarkSixFilterSelection(previous, filterId))
  }

  const removeTicket = (ticket: MarkSixTicket) => {
    if (hasNumberShortcuts(ticket.marketId)) {
      const market = markSixMarket(ticket.marketId)
      if (!market) return
      updateNumberSelection(market, ticket.position, previous => ({
        filters: previous?.filters ?? [],
        included: (previous?.included ?? []).filter(value => value !== ticket.selection),
        excluded: [...new Set([...(previous?.excluded ?? []), ticket.selection])],
      }))
    } else {
      updateTickets(previous => previous.filter(item => markSixTicketKey(item) !== markSixTicketKey(ticket)))
    }
  }

  const clearSelection = () => {
    setCart({ context, items: [], numbers: {} })
    setComboDraft({ marketId: '', values: [] })
    setAccepted(null)
  }

  const submit = async () => {
    if (!canSubmit) return
    setConfirming(true)
    try {
      const result = await onConfirm(batchItems)
      if (!result) return
      setAccepted(result)
      setCart({ context, items: [], numbers: {} })
      setComboDraft({ marketId: '', values: [] })
      setSelectionOpen(false)
    } finally {
      setConfirming(false)
    }
  }

  const selectionLabel = activeMarket?.pickCount && activeMarket.pickCount > 1
      ? `请选择 ${activeMarket.pickCount} 项组成一注`
      : activeMarket?.positionMode === 'regular-position'
        ? `正${position} · 每个所选项一注`
        : '每个所选项均按单注金额计费'
  const groupedOptions = visibleMarkets.some(market => market.options.some(option =>
    markSixOptionNumbers(market.id, option.value, bettingTarget.timing.drawAtMs) !== null,
  ))
  const optionGridClass = numberMarket
    ? `full-bet-numbers mark-six-number-grid${numberMarket.pickCount > 1 ? ' mark-six-combo-grid' : ''}`
    : `full-bet-options mark-six-value-grid${groupedOptions ? ' mark-six-group-grid' : ''}`

  const renderOption = (market: MarkSixMarketSpec, option: MarkSixOption) => {
    const optionPricingCode = markSixOptionPricingCode(market, option.value)
    const optionQuote = optionPricingCode ? markSixOddsItem(optionPricingCode, oddsInfo) : null
    const single = market.pickCount === 1 ? markSixSingleTicket(market, option.value, position) : null
    const selected = market.pickCount > 1
      ? comboDraft.marketId === market.id && draftValues.includes(option.value)
      : Boolean(single && tickets.some(ticket => markSixTicketKey(ticket) === markSixTicketKey(single)))
    const maxed = market.pickCount > 1 && draftValues.length >= market.pickCount && !selected
    const numbers = markSixOptionNumbers(market.id, option.value, bettingTarget.timing.drawAtMs)
    const compactGroup = numbers !== null && !['color_wave', 'half_wave', 'half_half_wave', 'five_element'].includes(market.id)
    const missingZodiac = numbers !== null && numbers.length === 0
    const numeric = market.optionKind === 'number'
    const label = markSixBoardOptionLabel(market.id, option.value)
    const colorClass = label.startsWith('红') ? ' wave-red' : label.startsWith('蓝') ? ' wave-blue' : label.startsWith('绿') ? ' wave-green' : ''
    return <button type="button" key={`${market.id}:${option.value}`} data-market-id={market.id} data-choice={option.value}
      aria-label={`${market.label}${market.positionMode === 'regular-position' ? `正${position}` : ''}${option.label}`}
      aria-pressed={selected} className={`board-choice${numeric ? ' mark-six-number-choice' : ' mark-six-value-choice'}${numbers !== null ? ' mark-six-group-choice' : ''}${compactGroup ? ' mark-six-compact-group' : ''}${selected ? ' selected' : ''}`}
      disabled={!marketReady(market) || (!optionQuote && !isTieredMarket(market)) || maxed || missingZodiac} onClick={() => chooseOption(market, option.value)}>
      <span className="mark-six-choice-heading">
        <b className={numeric ? markSixBallClass(Number(option.value)) : `mark-six-option-label${colorClass}`}>{numeric ? option.label : label}</b>
        {(market.pickCount === 1 || isLinkedMarket(market)) && <small>{market.blockedReason ? '待核验' : !optionQuote ? '待配置' : oddsLabel(optionQuote.odds, 3, oddsInfo?.show_odds === false)}</small>}
      </span>
      {numbers !== null && <span className="mark-six-option-numbers" aria-hidden="true">{numbers.map(number =>
        <span key={number} className={markSixBallClass(number)}>{String(number).padStart(2, '0')}</span>,
      )}{missingZodiac && <span className="mark-six-number-hint">生肖年份暂时无法识别</span>}</span>}
    </button>
  }

  return <div aria-hidden={!active || undefined} className="full-bet-layer full-bet-drawer mark-six-bet-layer" hidden={!active} id={surfaceId}>
    <section className="full-bet-board embedded mark-six-bet-board" aria-label={`${game.title}网投面板`} {...controlSurfaceProps}>
      <div className="full-bet-workspace">
        <aside aria-label="六合彩投注玩法">{markSixCategories.map(item => <button type="button" key={item.id} aria-pressed={category === item.id} className={category === item.id ? 'active' : ''} disabled={busy} onClick={() => chooseCategory(item.id)}>{item.label}</button>)}</aside>
        <section className="full-bet-content" key={`${category}:${activeMarket?.id}`}>
          <header><div><b>{markSixCategories.find(item => item.id === category)?.label}</b><small>{selectionLabel}</small></div><span>第 <b>{bettingTarget.issue}</b> 期</span></header>
          {!rulesReady && <p className="mark-six-rule-notice" role="status">{rulesMessage}</p>}
          {marketTabs.length > 1 && <nav className="mark-six-market-tabs" data-tab-count={marketTabs.length} aria-label={`${markSixCategories.find(item => item.id === category)?.label}子玩法`}>{marketTabs.map(item => {
            const selected = markSixBoardMarketFamily(activeMarket?.id ?? '') === markSixBoardMarketFamily(item.id)
            return <button type="button" key={item.id} aria-label={markSixBoardTabLabel(item)} aria-pressed={selected} className={selected ? 'active' : ''} disabled={busy} onClick={() => chooseMarket(item)}>{markSixBoardTabLabel(item)}{item.blockedReason && <small>待核验</small>}</button>
          })}</nav>}
          {activeVariants.length > 1 && <nav className="mark-six-variant-tabs" aria-label={`${markSixBoardTabLabel(activeMarket!)}数量`}>{activeVariants.map(item => <button type="button" key={item.id} aria-label={item.label} aria-pressed={activeMarket?.id === item.id} className={activeMarket?.id === item.id ? 'active' : ''} disabled={busy} onClick={() => chooseMarket(item)}>{item.label}</button>)}</nav>}
          {activeMarket?.positionMode === 'regular-position' && <div className="rank-selector mark-six-position-selector" aria-label="切换正码位置（独立编辑）">{Array.from({ length: 6 }, (_, index) => index + 1).map(value => <button type="button" key={value} aria-label={`编辑正${value}`} aria-pressed={position === value} className={position === value ? 'active' : ''} disabled={busy} onClick={() => { if (!busy) { setPosition(value); setComboDraft({ marketId: '', values: [] }) } }}>{category === 'regular_special' ? `正${value}特` : `正码${value}`}</button>)}</div>}
          {shortcutMarket && <div className="mark-six-number-filters" aria-label="批量筛选号码">{markSixNumberFilters.map(filter => {
            const zodiacFilter = filter.id === 'domestic' || filter.id === 'wild'
            const unavailable = zodiacFilter && !zodiacFiltersReady
            const highlighted = selectedFilters.includes(filter.id)
            return <button type="button" key={filter.id} aria-label={filter.label} aria-pressed={highlighted} className={highlighted ? 'selected' : undefined} disabled={!marketReady(shortcutMarket) || unavailable} title={unavailable ? '生肖年份暂时无法识别' : undefined} onClick={() => applyNumberFilter(filter.id)}>{filter.label}</button>
          })}</div>}
          {activeMarket?.blockedReason && <p className="mark-six-market-notice" role="status">{activeMarket.blockedReason}，当前仅展示入口，不会生成注单。</p>}
          {!activeMarket?.blockedReason && !hasConfiguredOption && <p className="mark-six-market-notice" role="status">该玩法赔率待配置，当前不可选择或提交。</p>}
          {activeMarket?.id.startsWith('not_in_') && <p className="mark-six-reference-note">自选不中按所选数量独立计价；7个开奖号码均未命中所选号码即中奖。</p>}
          {activeMarket?.id.startsWith('combined_zodiac_') && <p className="mark-six-reference-note">特码属于任一所选生肖即中奖；特码49按和局返本。</p>}
          {activeMarket?.id === 'five_element' && <p className="mark-six-reference-note">号码分组按当前 {game.ruleVersion || '六合彩'} 结算规则显示。</p>}
          {activeMarket && activeMarket.pickCount > 1 && <p className="mark-six-combo-price">组合赔率：<b>{activeMarket.blockedReason ? '待核验' : isTieredMarket(activeMarket)
            ? activePricingQuotes.length === 2
              ? activeMarket.id === 'combo_3_2'
                ? `中二 ${oddsLabel(activePricingQuotes[0].odds, 3, oddsInfo?.show_odds === false)} / 中三 ${oddsLabel(activePricingQuotes[1].odds, 3, oddsInfo?.show_odds === false)}`
                : `中特 ${oddsLabel(activePricingQuotes[0].odds, 3, oddsInfo?.show_odds === false)} / 中二 ${oddsLabel(activePricingQuotes[1].odds, 3, oddsInfo?.show_odds === false)}`
              : '待配置'
            : isLinkedMarket(activeMarket)
              ? draftValues.length === activeMarket.pickCount && activePricingQuotes.length === activeMarket.pickCount
                ? oddsLabel(Math.min(...activePricingQuotes.map(item => item.odds)), 3, oddsInfo?.show_odds === false)
                : '选满后按所选最低赔率'
              : activePricingQuotes[0] ? oddsLabel(activePricingQuotes[0].odds, 3, oddsInfo?.show_odds === false) : '待配置'}</b></p>}
          <div className={optionGridClass}>{visibleMarkets.flatMap(market => markSixBoardOptions(market).map(option => renderOption(market, option)))}</div>
          {activeMarket && activeMarket.pickCount > 1 && !activeMarket.blockedReason && <div className="mark-six-combo-bar"><span>已选 {draftValues.length}/{activeMarket.pickCount}</span><button type="button" disabled={!marketReady(activeMarket) || draftValues.length !== activeMarket.pickCount || busy} onClick={addCombo}>加入{activeMarket.playName}清单</button></div>}
        </section>
      </div>
      <footer className="full-bet-footer">
        {accepted && <p className="mark-six-accepted" role="status">第 {accepted.issue} 期已受理 {accepted.bet_count} 注，合计 ¥ {formatBetAmount(accepted.total)}</p>}
        {(batchError || (!rulesReady && tickets.length > 0)) && <p className="board-command-error" role="alert">{!rulesReady ? rulesMessage : batchError}</p>}
        <div className="full-bet-summary"><button type="button" disabled={busy || !hasSelection} onClick={clearSelection}>清空选择</button><button type="button" className="full-bet-selection-toggle" aria-expanded={selectionOpen} onClick={() => setSelectionOpen(previous => !previous)}><span aria-live="polite">已选 <b>{groups.length}</b> 组 · <b>{tickets.length}</b> 注</span><i>{selectionOpen ? '收起' : '查看清单'}</i></button></div>
        {tickets.length > 0 && <div className="board-selected-preview" aria-label="已选投注">{groups.map(group => <span key={group.key}>{group.label} <b>{group.choices.map(item => item.selectionLabel).join('、')}</b></span>)}</div>}
        {selectionOpen && <div className="full-bet-selection-list"><header><b>本次网投清单</b><span>合计 ¥ {total}</span></header>{groups.length ? groups.map(group => <article key={group.key}><div><b>{group.label}</b><div className="board-selection-chips">{group.choices.map(ticket => <button type="button" key={markSixTicketKey(ticket)} disabled={busy} aria-label={`移除${group.label}${ticket.selectionLabel}`} onClick={() => removeTicket(ticket)}>{ticket.selectionLabel}<span aria-hidden="true"> ×</span></button>)}</div></div></article>) : <p>暂未选择玩法或号码</p>}</div>}
        <div className="amount-pills mark-six-amount-pills" aria-label="单注金额">{[10, 20, 50, 100, 200].map(value => <button type="button" key={value} aria-pressed={cents === value * 100} className={cents === value * 100 ? 'active' : ''} disabled={busy} onClick={() => { if (!busy) setAmount(String(value)) }}>{value}</button>)}</div>
        <div className="board-submit-row"><label className="board-custom-amount">单注<input aria-label="自定义单注金额" aria-invalid={cents === null} inputMode="decimal" autoComplete="off" placeholder="金额" disabled={busy} value={amount} onChange={event => { if (!busy) setAmount(event.target.value) }} /></label><button type="button" className="full-bet-confirm" disabled={!canSubmit} onClick={() => void submit()}>{busy ? '提交中…' : !rulesReady ? '规则待配置' : !bettingTarget.timing.accepting ? bettingTarget.timing.statusLabel : batchError ? '请检查投注清单' : '立即投注'} <small>¥ {total}</small></button></div>
      </footer>
    </section>
  </div>
}
