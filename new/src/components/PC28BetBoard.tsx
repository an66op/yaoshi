import { useEffect, useState } from 'react'
import type { AssistantBetResult, WebBetBatchItem } from '../api/bets'
import type { GameOdds } from '../api/portal'
import type { Game } from '../types'
import { formatBetAmount } from '../utils/betAmount'
import { controlSurfaceProps } from '../utils/controlSurface'
import { boardAmountCents } from '../utils/fullBetSelection'
import { oddsLabel } from '../utils/gameRoomSafety'
import { isPC28RuleVersion, pc28RuleVersionForGame } from '../utils/lotteryRules'
import {
  PC28_PRICED_PLAY_CODE_COUNT,
  pc28BatchError,
  pc28BatchItems,
  pc28Categories,
  pc28MarketsForCategory,
  pc28OddsItem,
  pc28OptionPlayCode,
  pc28PackageTicket,
  pc28PricedPlayCodes,
  pc28SingleTicket,
  pc28TicketGroups,
  pc28TicketAddError,
  pc28TicketKey,
  togglePC28Draft,
  togglePC28Ticket,
  type PC28CategoryID,
  type PC28MarketSpec,
  type PC28Ticket,
} from '../utils/pc28BetSelection'
import { roomBettingTarget } from '../utils/gameRoomBetting'
import { Icon } from './Icon'
import './pc28-bet-board.css'

type Props = {
  game: Game
  ruleVersion: string
  oddsInfo: GameOdds | null
  rulesReady: boolean
  rulesMessage: string
  submitting?: boolean
  active?: boolean
  surfaceId?: string
  onClose: () => void
  onConfirm: (items: WebBetBatchItem[]) => Promise<AssistantBetResult | null>
}

type ContextCart = { context: string; items: PC28Ticket[] }
type PackageDraft = { marketId: string; values: string[] }

const versionLabels = { 'pc28-v1': '玩法一', 'pc28-v2': '玩法二', 'pc28-v3': '玩法三' } as const
const dynamicSettlementDisclosures = {
  'pc28-v1': '13/14动态结算（玩法一）：会员在当前房间、当前彩种、当前期的全部总注严格大于1且开13/14时，和值大小/单双按1.5倍、和值组合按1倍；全部总注严格大于9999时，和值大小/单双按1倍并覆盖1.5倍。开13或14时，本期全部下注有效流水为0。',
  'pc28-v2': '13/14动态结算（玩法二）：会员在当前房间、当前彩种、当前期的全部总注严格大于1且开13/14时，和值大小/单双按1.5倍、和值组合庄家通吃；全部总注严格大于9999时，和值大小/单双按1倍并覆盖1.5倍。',
  'pc28-v3': '13/14动态结算（玩法三）：会员在当前房间、当前彩种、当前期的全部总注严格大于1且开13/14时，和值大小/单双按1.98倍，和值组合按3.65倍。',
} as const
const dynamicSettlementBaseOddsNote = '页面展示的是当前房间基础赔率；最终派彩、返还与有效流水按当前规则版本及本期总注由服务端结算。'
const positionLabels = ['第一球', '第二球', '第三球']

export function PC28BetBoard({ game, ruleVersion, oddsInfo, rulesReady, rulesMessage, submitting, active = true, surfaceId, onClose, onConfirm }: Props) {
  const bettingTarget = roomBettingTarget(game)
  const context = `${game.id}:${ruleVersion}:${bettingTarget.issue}:pc28`
  const [cart, setCart] = useState<ContextCart>({ context, items: [] })
  const [category, setCategory] = useState<PC28CategoryID>('sum')
  const [marketId, setMarketId] = useState('sum_exact')
  const [position, setPosition] = useState(1)
  const [packageDraft, setPackageDraft] = useState<PackageDraft>({ marketId: '', values: [] })
  const [amount, setAmount] = useState('20')
  const [selectionOpen, setSelectionOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [accepted, setAccepted] = useState<AssistantBetResult | null>(null)

  useEffect(() => {
    setCart({ context, items: [] })
    setCategory('sum')
    setMarketId('sum_exact')
    setPosition(1)
    setPackageDraft({ marketId: '', values: [] })
    setSelectionOpen(false)
    setAccepted(null)
  }, [context])

  const tickets = cart.context === context ? cart.items : []
  const categoryMarkets = pc28MarketsForCategory(category)
  const activeMarket = categoryMarkets.find(item => item.id === marketId) ?? categoryMarkets[0]
  const draftValues = packageDraft.marketId === activeMarket?.id ? packageDraft.values : []
  const busy = Boolean(submitting) || confirming
  const versionReady = isPC28RuleVersion(game.id, ruleVersion)
    && oddsInfo?.game_id === game.id
    && oddsInfo.rules_ready === true
    && isPC28RuleVersion(game.id, oddsInfo.rule_version)
  const boardContractReady = game.sourceHealthy && rulesReady && versionReady
  const boardReady = boardContractReady && !busy
  const groups = pc28TicketGroups(tickets)
  const cents = boardAmountCents(amount)
  const totalCents = cents === null ? null : cents * tickets.length
  const total = totalCents !== null && Number.isSafeInteger(totalCents) ? formatBetAmount(totalCents / 100) : '—'
  const batchItems = pc28BatchItems(tickets, amount)
  const batchError = pc28BatchError(game.id, tickets, amount, oddsInfo)
  const configuredOddsCount = pc28PricedPlayCodes.filter(playCode => pc28OddsItem(game.id, playCode, oddsInfo)).length
  const canSubmit = boardReady && bettingTarget.timing.accepting && tickets.length > 0 && batchItems.length === tickets.length && !batchError
  const configuredOptions = activeMarket?.options.filter(option => {
    const playCode = pc28OptionPlayCode(activeMarket, option.value)
    return playCode && pc28OddsItem(game.id, playCode, oddsInfo)
  }).length ?? 0

  const chooseCategory = (next: PC28CategoryID) => {
    if (busy) return
    const nextMarket = pc28MarketsForCategory(next)[0]
    setCategory(next)
    setMarketId(nextMarket?.id ?? '')
    setPackageDraft({ marketId: '', values: [] })
    setAccepted(null)
  }

  const chooseMarket = (next: PC28MarketSpec) => {
    if (busy) return
    setMarketId(next.id)
    setPackageDraft({ marketId: '', values: [] })
    setAccepted(null)
  }

  const updateTickets = (update: (previous: PC28Ticket[]) => PC28Ticket[]) => {
    setCart(previous => ({ context, items: update(previous.context === context ? previous.items : []) }))
    setAccepted(null)
  }

  const chooseOption = (value: string) => {
    if (!activeMarket || !boardReady) return
    const playCode = pc28OptionPlayCode(activeMarket, value)
    if (!playCode || !pc28OddsItem(game.id, playCode, oddsInfo)) return
    if (activeMarket.pickCount === 3) {
      setPackageDraft(previous => ({
        marketId: activeMarket.id,
        values: togglePC28Draft(previous.marketId === activeMarket.id ? previous.values : [], value),
      }))
      return
    }
    const ticket = pc28SingleTicket(activeMarket, value, position)
    if (ticket) updateTickets(previous => {
      const alreadySelected = previous.some(item => pc28TicketKey(item) === pc28TicketKey(ticket))
      return !alreadySelected && pc28TicketAddError(previous, ticket, ruleVersion) ? previous : togglePC28Ticket(previous, ticket)
    })
  }

  const addPackage = () => {
    if (!activeMarket || !boardReady || !activeMarket.playCode || !pc28OddsItem(game.id, activeMarket.playCode, oddsInfo)) return
    const ticket = pc28PackageTicket(activeMarket, draftValues)
    if (!ticket) return
    updateTickets(previous => previous.some(item => pc28TicketKey(item) === pc28TicketKey(ticket)) ? previous : [...previous, ticket])
    setPackageDraft({ marketId: activeMarket.id, values: [] })
  }

  const submit = async () => {
    if (!canSubmit) return
    setConfirming(true)
    try {
      const result = await onConfirm(batchItems)
      if (!result) return
      setAccepted(result)
      setCart({ context, items: [] })
      setPackageDraft({ marketId: '', values: [] })
      setSelectionOpen(false)
    } finally {
      setConfirming(false)
    }
  }

  const expectedVersion = pc28RuleVersionForGame(game.id)
  const versionLabel = expectedVersion ? versionLabels[expectedVersion] : '未绑定'
  const selectionHint = activeMarket?.pickCount === 3
    ? '选择3个不同点数组成1注'
    : activeMarket?.positionMode === 'ball'
      ? `${positionLabels[position - 1]} · 每个选项独立计注`
      : '每个所选项均按单注金额计费'

  return <div aria-hidden={!active || undefined} className="full-bet-layer full-bet-drawer pc28-bet-layer" hidden={!active} id={surfaceId}>
    <section className="full-bet-board embedded pc28-bet-board" aria-label="PC28详细网投面板" {...controlSurfaceProps}>
      <div className="full-bet-workspace">
        <aside aria-label="PC28投注玩法">{pc28Categories.map(item => <button type="button" key={item.id} aria-pressed={category === item.id} className={category === item.id ? 'active' : ''} disabled={busy} onClick={() => chooseCategory(item.id)}>{item.label}</button>)}</aside>
        <section className="full-bet-content">
          <header><div><b>{pc28Categories.find(item => item.id === category)?.label}</b><small>{versionLabel} · {selectionHint} · 赔率目录 {configuredOddsCount}/{PC28_PRICED_PLAY_CODE_COUNT}</small></div><button type="button" className="detail-panel-collapse" aria-label="收起详细投注，返回聊天" disabled={busy} onClick={onClose}><Icon name="arrow" /></button></header>
          {!boardContractReady && <p className="pc28-rule-notice" role="status">{!game.sourceHealthy ? (rulesMessage || '开奖同步暂时暂停，投注已暂停。') : !rulesReady ? rulesMessage : `赔率目录身份或规则版本不匹配：${game.title}必须绑定 ${expectedVersion ?? 'PC28专用版本'}。`}</p>}
          {categoryMarkets.length > 1 && <nav className="pc28-market-tabs" aria-label={`${pc28Categories.find(item => item.id === category)?.label}子玩法`}>{categoryMarkets.map(item => <button type="button" key={item.id} aria-pressed={activeMarket?.id === item.id} className={activeMarket?.id === item.id ? 'active' : ''} disabled={busy} onClick={() => chooseMarket(item)}>{item.label}</button>)}</nav>}
          {activeMarket?.positionMode === 'ball' && <div className="rank-selector pc28-position-selector" aria-label="切换球位（独立编辑）">{positionLabels.map((label, index) => {
            const value = index + 1
            const count = tickets.filter(ticket => ticket.position === value && ticket.marketId.startsWith('position_')).length
            return <button type="button" key={value} aria-label={`编辑${label}`} aria-pressed={position === value} className={position === value ? 'active' : ''} disabled={busy} onClick={() => { if (!busy) setPosition(value) }}>{label}{count > 0 && <small className="rank-count">{count}</small>}</button>
          })}</div>}
          {activeMarket?.id === 'shape' && <p className="pc28-market-note">按原版规则，890、901及同组不同排列均算顺子。</p>}
          {activeMarket?.id === 'color' && <p className="pc28-market-note">0、13、14、27的灰波返本由服务端房间配置执行。</p>}
          {activeMarket?.id === 'sum_exact' && <p className="pc28-market-note">每期最多投注10个不同单点；当前清单已选 {new Set(tickets.filter(ticket => ticket.marketId === 'sum_exact').map(ticket => ticket.selection)).size}/10。</p>}
          {(activeMarket?.id === 'sum_size' || activeMarket?.id === 'sum_parity') && (ruleVersion === 'pc28-v1' || ruleVersion === 'pc28-v2') && <p className="pc28-market-note">玩法一、二仅禁止同期在和值大小或和值单双市场反向下注；球位定位两面不在此限制内。</p>}
          {(activeMarket?.id === 'sum_size' || activeMarket?.id === 'sum_parity' || activeMarket?.id === 'sum_combo') && isPC28RuleVersion(game.id, ruleVersion) && <p className="pc28-market-note pc28-dynamic-settlement-note"><b>{dynamicSettlementDisclosures[ruleVersion as keyof typeof dynamicSettlementDisclosures]}</b><span>{dynamicSettlementBaseOddsNote}</span></p>}
          {activeMarket && configuredOptions === 0 && <p className="pc28-market-notice" role="status">该玩法赔率待配置，当前不可选择或提交。</p>}
          {activeMarket && <div className={activeMarket.optionKind === 'number' ? `full-bet-numbers pc28-number-grid${activeMarket.id === 'position_number' ? ' pc28-digit-grid' : ''}` : 'full-bet-options pc28-value-grid'}>{activeMarket.options.map(option => {
            const playCode = pc28OptionPlayCode(activeMarket, option.value)
            const quote = playCode ? pc28OddsItem(game.id, playCode, oddsInfo) : null
            const single = activeMarket.pickCount === 1 ? pc28SingleTicket(activeMarket, option.value, position) : null
            const selected = activeMarket.pickCount === 3
              ? draftValues.includes(option.value)
              : Boolean(single && tickets.some(ticket => pc28TicketKey(ticket) === pc28TicketKey(single)))
            const maxed = activeMarket.pickCount === 3 && draftValues.length >= 3 && !selected
            const constraintError = single && !selected ? pc28TicketAddError(tickets, single, ruleVersion) : ''
            return <button type="button" key={option.value} data-choice={option.value} aria-label={`${activeMarket.label}${activeMarket.positionMode === 'ball' ? positionLabels[position - 1] : ''}${option.label}`} aria-pressed={selected} title={constraintError || undefined} className={`board-choice${selected ? ' selected' : ''}`} disabled={!boardReady || !quote || maxed || Boolean(constraintError)} onClick={() => chooseOption(option.value)}>
              <b>{option.label}</b><small>{quote ? oddsLabel(quote.odds, 3, oddsInfo?.show_odds === false) : '待配置'}</small>
            </button>
          })}</div>}
          {activeMarket?.pickCount === 3 && <div className="pc28-package-bar"><span>已选 {draftValues.length}/3</span><button type="button" disabled={!boardReady || draftValues.length !== 3 || busy} onClick={addPackage}>加入包三清单</button></div>}
        </section>
      </div>
      <footer className="full-bet-footer">
        {accepted && <p className="pc28-accepted" role="status">第 {accepted.issue} 期已受理 {accepted.bet_count} 注，合计 ¥ {formatBetAmount(accepted.total)}</p>}
        {(batchError || (!boardContractReady && tickets.length > 0)) && <p className="board-command-error" role="alert">{!game.sourceHealthy ? (rulesMessage || '开奖同步暂时暂停，尚未下注。') : !rulesReady ? rulesMessage : !versionReady ? '赔率目录身份或规则版本不匹配，尚未下注。' : batchError}</p>}
        <div className="full-bet-summary"><button type="button" disabled={busy || !tickets.length} onClick={() => updateTickets(() => [])}>清空选择</button><button type="button" className="full-bet-selection-toggle" aria-expanded={selectionOpen} onClick={() => setSelectionOpen(previous => !previous)}><span aria-live="polite">已选 <b>{groups.length}</b> 组 · <b>{tickets.length}</b> 注</span><i>{selectionOpen ? '收起' : '查看清单'}</i></button></div>
        {tickets.length > 0 && <div className="board-selected-preview" aria-label="已选投注">{groups.map(group => <span key={group.key}>{group.label} <b>{group.choices.map(item => item.selectionLabel).join('、')}</b></span>)}</div>}
        {selectionOpen && <div className="full-bet-selection-list"><header><b>本次PC28网投清单</b><span>合计 ¥ {total}</span></header>{groups.length ? groups.map(group => <article key={group.key}><div><b>{group.label}</b><div className="board-selection-chips">{group.choices.map(ticket => <button type="button" key={pc28TicketKey(ticket)} disabled={busy} aria-label={`移除${group.label}${ticket.selectionLabel}`} onClick={() => updateTickets(previous => previous.filter(item => pc28TicketKey(item) !== pc28TicketKey(ticket)))}>{ticket.selectionLabel}<span aria-hidden="true"> ×</span></button>)}</div></div></article>) : <p>暂未选择玩法或号码</p>}</div>}
        <div className="amount-pills pc28-amount-pills" aria-label="单注金额">{[10, 20, 50, 100, 200].map(value => <button type="button" key={value} aria-pressed={cents === value * 100} className={cents === value * 100 ? 'active' : ''} disabled={busy} onClick={() => { if (!busy) setAmount(String(value)) }}>{value}</button>)}</div>
        <div className="board-submit-row"><label className="board-custom-amount">单注<input aria-label="自定义单注金额" aria-invalid={cents === null} inputMode="decimal" autoComplete="off" placeholder="金额" disabled={busy} value={amount} onChange={event => { if (!busy) setAmount(event.target.value) }} /></label><button type="button" className="full-bet-confirm" disabled={!canSubmit} onClick={() => void submit()}>{busy ? '提交中…' : !game.sourceHealthy ? '开奖暂停' : !rulesReady || !versionReady ? '规则待配置' : !bettingTarget.timing.accepting ? bettingTarget.timing.statusLabel : batchError ? '请检查投注清单' : '立即投注'} <small>¥ {total}</small></button></div>
      </footer>
    </section>
  </div>
}
