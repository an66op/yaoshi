import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { Game, Theme } from '../types'
import { ballTone } from '../data/games'
import { Avatar } from '../components/Avatar'
import { Icon } from '../components/Icon'
import { ActionDialog } from '../components/Dialogs'
import { DrawResultCards } from '../components/DrawResultCards'
import { playNotificationSound } from '../utils/notificationAudio'
import { CheckIn } from './CheckIn'
import { parseBetInput, type ParsedBet } from '../utils/betParser'
import { betsApi, type AssistantDrawStatus, type MemberBet } from '../api/bets'
import { useGameDraws } from '../hooks/useGameDraws'
import type { DrawResult } from '../api/lottery'
import { useMemberPreferences } from '../hooks/useMemberPreferences'
import { WS_EVENT, type WsEvent, useWebSocketConnected } from '../hooks/useWebSocket'
import { portalApi, type GameFeedItem, type GameOdds, type MemberNotification } from '../api/portal'
import { chatApi, type ChatMessage } from '../api/chat'
import { memberApi, type WalletSummary } from '../api/member'
import type { WalletActionSlug } from '../router'

type Props = {
  game: Game
  games: Game[]
  theme: Theme
  nickname: string
  balance: number
  onBack: () => void
  onOpenGame: (gameId: string) => void
  onOpenService: () => void
  onOpenWallet: (action?: WalletActionSlug) => void
  onOpenResults: () => void
  startWithQuickMenu?: boolean
  onRefreshBalance: () => Promise<void>
}
type Dialog = 'mipai' | 'orders' | 'trend' | 'forecast' | 'assist' | 'required' | 'bet-error' | null
type BetMode = 'quick' | 'dual' | 'numbers'
type KeyboardShortcut = 'all-in' | 'cancel' | 'credit' | 'check' | 'debit' | 'repeat'
type AcceptedTicket = { gameId: string; content: string; lines: string[]; total: number; issue: string; acceptedAt: string }
type WinningPopupData = { id: string; issue: string; amount: number }
type RoomFeatureSettings = {
  showTurnover: boolean
  showProfit: boolean
  showRebate: boolean
  webKeyboard: boolean
  showMipai: boolean
  showOrders: boolean
  showStreak: boolean
  showPrediction: boolean
}
type TimelineEntry =
  | { kind: 'chat'; key: string; at: number; value: ChatMessage }
  | { kind: 'draw'; key: string; at: number; value: DrawResult }
  | { kind: 'settlement'; key: string; at: number; value: MemberNotification }
  | { kind: 'feed'; key: string; at: number; value: GameFeedItem; index: number }
  | { kind: 'ticket'; key: string; at: number; value: AcceptedTicket }
  | { kind: 'persisted'; key: string; at: number; value: MemberBet[] }

function timelineTime(value?: string) {
  const parsed = new Date(value ?? '').getTime()
  return Number.isFinite(parsed) ? parsed : 0
}

function formatHeaderAmount(value: number) {
  return value.toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
}

function mergeAcceptedTickets(...groups: AcceptedTicket[][]) {
  const seen = new Set<string>()
  return groups.flat().filter((ticket) => {
    const key = `${ticket.gameId}:${ticket.issue}:${ticket.acceptedAt}:${ticket.content}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }).sort((left, right) => {
    const leftTime = new Date(left.acceptedAt).getTime()
    const rightTime = new Date(right.acceptedAt).getTime()
    if (!Number.isFinite(leftTime) || !Number.isFinite(rightTime)) return 0
    return leftTime - rightTime
  })
}

function mergeChatMessages(...groups: ChatMessage[][]) {
  const byID = new Map<number, ChatMessage>()
  groups.flat().forEach((message) => byID.set(message.id, message))
  return [...byID.values()].sort((left, right) => left.id - right.id)
}

function mergeSettlementNotices(...groups: MemberNotification[][]) {
  const byID = new Map<number, MemberNotification>()
  groups.flat().forEach((notice) => byID.set(notice.id, notice))
  return [...byID.values()].sort((left, right) => {
    const time = new Date(left.created_at).getTime() - new Date(right.created_at).getTime()
    return time || left.id - right.id
  })
}

const quickKeys = ['大', '1', '2', '3', '←', '小', '4', '5', '6', '龙', '单', '7', '8', '9', '冠亚', '双', '#', '0', '/', '虎']
const quickOptions = new Set(['大', '小', '单', '双', '龙', '虎', '冠亚'])
const dualOptions = ['冠军大', '冠军小', '冠军单', '冠军双', '冠军龙', '冠军虎', '亚军大', '亚军小', '亚军单', '亚军双', '冠亚和大', '冠亚和小']
const modes: Array<{ id: BetMode; label: string }> = [{ id: 'quick', label: '快捷' }, { id: 'dual', label: '两面盘' }, { id: 'numbers', label: '号码 1~10' }]
const drawPositionNames = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十']

function shortIssue(issue: string) {
  return issue.match(/^(\d{8})-/)?.[1] ?? issue
}

function gameAcceptance(game: Game) {
  if (!game.sourceHealthy || game.issueStatus === 'error') {
    return { label: '开奖源异常 · 已停盘', tone: 'closed' }
  }
  if (game.issueStatus === 'sealed') return { label: '已封盘', tone: 'closed' }
  if (game.issueStatus === 'awaiting_draw') return { label: '等待开奖', tone: 'closed' }
  if (game.issueStatus === 'settling') return { label: '正在结算', tone: 'closed' }
  if (game.issueStatus === 'settled') return { label: '正在切换下一期', tone: 'closed' }
  if (game.issueStatus === 'pending') return { label: '即将开始受理', tone: 'syncing' }
  const units = game.due.split(':').map(Number)
  const seconds = units.length === 3
    ? units[0] * 3600 + units[1] * 60 + units[2]
    : units.length === 2
      ? units[0] * 60 + units[1]
      : Number.NaN
  if (!Number.isFinite(seconds)) return { label: '状态同步中', tone: 'syncing' }
  if (seconds <= 0) return { label: '封盘中', tone: 'closed' }
  if (seconds <= 30) return { label: `${seconds} 秒后封盘`, tone: 'closing' }
  return { label: '正在受理', tone: 'open' }
}

function canAcceptBet(game: Game) {
  const tone = gameAcceptance(game).tone
  return tone === 'open' || tone === 'closing'
}

function payloadLabel(payload: ParsedBet['payloads'][number]) {
  return payload.play_name || `第${drawPositionNames[payload.position - 1] ?? payload.position}名${payload.selection}`
}

function crownMeta(balls: number[]) {
  if (balls.length < 2) return { crownResult: '—', dragonTiger: '—' }
  const crownSum = balls[0] + balls[1]
  const crownResult = `${crownSum}${crownSum >= 12 ? '大' : '小'}${crownSum % 2 ? '单' : '双'}`
  const dragonTiger = balls.slice(0, 5).map((ball, index) => (balls[9 - index] !== undefined && ball > balls[9 - index] ? '龙' : '虎')).join('')
  return { crownResult, dragonTiger }
}

/** 彩种会话：快捷输入、两面盘和注单提交接后端 API。 */
export function GameRoom({ game, games, theme, nickname, balance, onBack, onOpenGame, onOpenService, onOpenWallet, onOpenResults, startWithQuickMenu = false, onRefreshBalance }: Props) {
  const realtimeConnected = useWebSocketConnected()
  const [betInput, setBetInput] = useState('')
  const [showKeyboard, setShowKeyboard] = useState(false)
  const [showQuickBet, setShowQuickBet] = useState(false)
  const [betMode, setBetMode] = useState<BetMode>('quick')
  const { drawHistoryLimit, defaultBetMode, fontScale } = useMemberPreferences()
  const [dialog, setDialog] = useState<Dialog>(null)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [submittedBets, setSubmittedBets] = useState<AcceptedTicket[]>([])
  const [memberBets, setMemberBets] = useState<MemberBet[]>([])
  const [betError, setBetError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [showAddMenu, setShowAddMenu] = useState(startWithQuickMenu)
  const [showGameSwitcher, setShowGameSwitcher] = useState(false)
  const [showCheckIn, setShowCheckIn] = useState(false)
  const [feedItems, setFeedItems] = useState<GameFeedItem[]>([])
  const [feedReady, setFeedReady] = useState(false)
  const [messagesReady, setMessagesReady] = useState(false)
  const [settlementsReady, setSettlementsReady] = useState(false)
  const [betsReady, setBetsReady] = useState(false)
  const [assistantHistoryReady, setAssistantHistoryReady] = useState(false)
  const [timelinePositioned, setTimelinePositioned] = useState(false)
  const [showScrollLatest, setShowScrollLatest] = useState(false)
  const chatRef = useRef<HTMLElement>(null)
  const nearBottomRef = useRef(true)
  const forceBottomRef = useRef(false)
  const lastSentContentRef = useRef('')
  const gameSessionRef = useRef(`${game.id}:${game.period}`)
  const { draws, loading: drawsLoading } = useGameDraws(game.id, drawHistoryLimit)
  const [oddsInfo, setOddsInfo] = useState<GameOdds | null>(null)
  const [assistantStatus, setAssistantStatus] = useState<AssistantDrawStatus | null>(null)
  const [gameMessages, setGameMessages] = useState<ChatMessage[]>([])
  const [settlementNotices, setSettlementNotices] = useState<MemberNotification[]>([])
  const [sendingMessage, setSendingMessage] = useState(false)
  const [walletSummary, setWalletSummary] = useState<WalletSummary | null>(null)
  const [keyboardNotice, setKeyboardNotice] = useState<string | null>(null)
  const [winningPopup, setWinningPopup] = useState<WinningPopupData | null>(null)
  const [roomFeatures, setRoomFeatures] = useState<RoomFeatureSettings>({
    showTurnover: true,
    showProfit: true,
    showRebate: true,
    webKeyboard: true,
    showMipai: true,
    showOrders: true,
    showStreak: true,
    showPrediction: true,
  })
  const seenWinningEventsRef = useRef(new Set<string>())

  const loadWalletSummary = useCallback(async () => {
    try {
      setWalletSummary(await memberApi.walletSummary())
    } catch {
      // 余额仍由账户接口展示；统计读取失败时只隐藏统计值，绝不伪造数据。
      setWalletSummary(null)
    }
  }, [])

  useEffect(() => {
    void loadWalletSummary()
  }, [balance, loadWalletSummary])

  useEffect(() => {
    let active = true
    void portalApi.roomSettings().then((settings) => {
      if (!active) return
      const features = settings.game ?? {}
      setRoomFeatures({
        showTurnover: features.show_member_turnover !== false,
        showProfit: features.show_member_profit !== false,
        showRebate: features.show_member_rebate !== false,
        webKeyboard: features.web_keyboard_enabled !== false,
        showMipai: features.show_mipai_tool !== false,
        showOrders: features.show_orders_tool !== false,
        showStreak: features.show_streak_tool !== false,
        showPrediction: settings.prediction_enabled !== false && features.show_prediction_tool !== false,
      })
    }).catch(() => undefined)
    return () => { active = false }
  }, [])

  useEffect(() => {
    if (!roomFeatures.webKeyboard) setShowKeyboard(false)
  }, [roomFeatures.webKeyboard])

  // 游戏和期号共同构成一段独立会话。即使组件未来被其他入口复用，
  // 也不能把上一局的输入、订单回执或弹层带到下一局。
  useEffect(() => {
    const previousGameID = gameSessionRef.current.split(':')[0]
    const gameChanged = previousGameID !== game.id
    gameSessionRef.current = `${game.id}:${game.period}`
    setBetInput('')
    setShowKeyboard(false)
    setShowQuickBet(false)
    setShowAddMenu(false)
    setShowGameSwitcher(false)
    setHistoryOpen(false)
    setDialog(null)
    setBetError('')
    setSubmittedBets([])
    setMemberBets([])
    setOddsInfo(null)
    setAssistantStatus(null)
    setFeedItems([])
    setSettlementsReady(false)
    setBetsReady(false)
    setAssistantHistoryReady(false)
    if (gameChanged) {
      setGameMessages([])
      setSettlementNotices([])
      setMessagesReady(false)
      setTimelinePositioned(false)
      setShowScrollLatest(false)
      nearBottomRef.current = true
      forceBottomRef.current = false
      lastSentContentRef.current = ''
    }
    setSendingMessage(false)
    setKeyboardNotice(null)
    setWinningPopup(null)
    setFeedReady(false)
  }, [game.id, game.period])

  const loadGameMessages = useCallback(async () => {
    const requestGameID = game.id
    try {
      const page = await chatApi.messages('group', requestGameID, 30)
      if (gameSessionRef.current.startsWith(`${requestGameID}:`)) {
        setGameMessages(page.items)
        for (let index = page.items.length - 1; index >= 0; index -= 1) {
          const message = page.items[index]
          if (message.mine && message.content.trim() && message.content.trim() !== '重复') {
            lastSentContentRef.current = message.content
            break
          }
        }
      }
    } catch {
      if (gameSessionRef.current.startsWith(`${requestGameID}:`)) setGameMessages([])
    } finally {
      if (gameSessionRef.current.startsWith(`${requestGameID}:`)) setMessagesReady(true)
    }
  }, [game.id])

  const loadSettlementNotices = useCallback(async () => {
    const requestSession = `${game.id}:${game.period}`
    try {
      const page = await portalApi.notifications(20, { category: 'winning', game_id: game.id })
      if (gameSessionRef.current === requestSession) {
        setSettlementNotices(mergeSettlementNotices(page.items))
      }
    } catch {
      // Keep an already rendered settlement receipt during a transient retry.
      // The periodic recovery request and WebSocket event will try again.
    } finally {
      if (gameSessionRef.current === requestSession) setSettlementsReady(true)
    }
  }, [game.id, game.period])

  useEffect(() => {
    void loadSettlementNotices()
    const timer = realtimeConnected ? 0 : window.setInterval(() => void loadSettlementNotices(), 10_000)
    return () => { if (timer) window.clearInterval(timer) }
  }, [loadSettlementNotices, realtimeConnected])

  useEffect(() => {
    void loadGameMessages()
    const timer = realtimeConnected ? 0 : window.setInterval(() => void loadGameMessages(), 10_000)
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail?.type === 'chat_message' && detail.data.game_id === game.id) void loadGameMessages()
    }
    window.addEventListener(WS_EVENT, onWs)
    return () => {
      if (timer) window.clearInterval(timer)
      window.removeEventListener(WS_EVENT, onWs)
    }
  }, [game.id, loadGameMessages, realtimeConnected])

  useEffect(() => {
    void portalApi.gameOdds(game.id).then(setOddsInfo).catch(() => setOddsInfo(null))
  }, [game.id])

  const loadAssistant = useCallback(async () => {
    const requestGameID = game.id
    try {
      const result = await betsApi.assistantStatus(requestGameID)
      if (gameSessionRef.current.startsWith(`${requestGameID}:`)) setAssistantStatus(result)
    } catch {
      if (gameSessionRef.current.startsWith(`${requestGameID}:`)) setAssistantStatus(null)
    }
  }, [game.id])

  useEffect(() => {
    void loadAssistant()
    const timer = realtimeConnected ? 0 : window.setInterval(() => void loadAssistant(), 10_000)
    return () => { if (timer) window.clearInterval(timer) }
  }, [loadAssistant, realtimeConnected])

  useEffect(() => {
    if (startWithQuickMenu) setShowAddMenu(true)
  }, [startWithQuickMenu])

  const defaultOdds = oddsInfo?.items.find((item) => item.play_code === 'ball_1_5')?.odds
    ?? oddsInfo?.items[0]?.odds
    ?? 1.92

  const loadBets = useCallback(async () => {
    const requestSession = `${game.id}:${game.period}`
    try {
      // Load recent bets for this game, not only the issue currently displayed.
      // The issue may advance while the member is outside the room; filtering
      // to the new issue would make their just-placed ticket appear to vanish.
      const result = await betsApi.list({ game_id: game.id, page_size: 50 })
      if (gameSessionRef.current === requestSession) setMemberBets(result.items)
    } catch {
      if (gameSessionRef.current === requestSession) setMemberBets([])
    } finally {
      if (gameSessionRef.current === requestSession) setBetsReady(true)
    }
  }, [game.id, game.period])

  const loadAssistantHistory = useCallback(async () => {
    // 新版文本投注的原文和助手回执都在群消息表中持久化；不再把旧的
    // AssistantRequest 历史二次拼进时间线，否则同一注会出现两个回执。
    setAssistantHistoryReady(true)
  }, [])

  const loadGameFeed = useCallback(async () => {
    const requestSession = `${game.id}:${game.period}`
    try {
      const feed = await portalApi.gameFeed(game.id, game.period)
      if (gameSessionRef.current === requestSession) setFeedItems(feed)
    } catch {
      // A transient feed failure must not reorder or remove messages that are
      // already visible. A new game/issue clears the old feed in the session
      // reset above before this request starts.
    } finally {
      if (gameSessionRef.current === requestSession) setFeedReady(true)
    }
  }, [game.id, game.period])

  useEffect(() => {
    setBetMode(defaultBetMode)
  }, [game.id, defaultBetMode])

  useEffect(() => {
    void loadBets()
    void loadAssistantHistory()
  }, [loadAssistantHistory, loadBets])

  useEffect(() => {
    const timer = realtimeConnected ? 0 : window.setInterval(() => {
      void loadBets()
      void onRefreshBalance()
      void loadWalletSummary()
    }, 10_000)
    return () => { if (timer) window.clearInterval(timer) }
  }, [loadBets, loadWalletSummary, onRefreshBalance, realtimeConnected])

  useEffect(() => {
    void loadGameFeed()
    const timer = realtimeConnected ? 0 : window.setInterval(() => void loadGameFeed(), 10_000)
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail?.type === 'bet_feed' && detail.data.game_id === game.id) void loadGameFeed()
    }
    window.addEventListener(WS_EVENT, onWs)
    return () => {
      if (timer) window.clearInterval(timer)
      window.removeEventListener(WS_EVENT, onWs)
    }
  }, [game.id, loadGameFeed, realtimeConnected])

  // 开奖提示属于当前正在观看的彩种，不应在大厅、钱包或消息页响起。
  useEffect(() => {
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      const eventGameID = String(detail?.game_id ?? detail?.data.game_id ?? '')
      if (detail?.type === 'draw_update' && eventGameID === game.id) {
        playNotificationSound('lottery')
        void loadBets()
        void loadGameFeed()
        void loadSettlementNotices()
        void loadAssistant()
        void onRefreshBalance()
        void loadWalletSummary()
      }
      if (detail?.type === 'notification' && detail.data.category === 'winning' && eventGameID === game.id) {
        void loadSettlementNotices()
        void loadBets()
        void onRefreshBalance()
        void loadWalletSummary()
        const wonCount = Number(detail.data.won_count ?? 0)
        const amount = Number(detail.data.payout_amount ?? 0)
        const eventID = String(detail.event_id ?? detail.data.id ?? `${eventGameID}:${detail.data.issue ?? ''}:${amount}`)
        if (wonCount > 0 && amount > 0 && !seenWinningEventsRef.current.has(eventID)) {
          seenWinningEventsRef.current.add(eventID)
          setWinningPopup({ id: eventID, issue: String(detail.data.issue ?? ''), amount })
          playNotificationSound('reward')
        }
      }
    }
    window.addEventListener(WS_EVENT, onWs)
    return () => window.removeEventListener(WS_EVENT, onWs)
  }, [game.id, loadAssistant, loadBets, loadGameFeed, loadSettlementNotices, loadWalletSummary, onRefreshBalance])

  useEffect(() => {
    if (!winningPopup) return
    const timer = window.setTimeout(() => setWinningPopup(null), 6500)
    return () => window.clearTimeout(timer)
  }, [winningPopup])

  const appendNumber = (number: number) => setBetInput((current) => `${current}${number}`)
  const appendOption = (option: string) => setBetInput((current) => `${current}${option}`)
  const clearSelection = () => setBetInput('')
  const removeNumber = () => setBetInput((current) => current.slice(0, -1))
  const handleKeyboardShortcut = (action: KeyboardShortcut) => {
    if (action === 'cancel') {
      setBetInput('取消')
      setKeyboardNotice('发送后撤回当前期仍可撤销的注单')
      return
    }
    if (action === 'credit' || action === 'debit') {
      const label = action === 'credit' ? '上分' : '下分'
      setBetInput(`${label} `)
      setKeyboardNotice(`已填写${label}申请，输入金额或说明后发送到当前群聊`)
      return
    }
    if (action === 'check') {
      setBetInput('查')
      setKeyboardNotice(null)
      return
    }
    if (action === 'repeat') {
      const previous = lastSentContentRef.current.trim()
      if (!previous) {
        setKeyboardNotice('暂无可以重复的上一条内容')
        return
      }
      setBetInput(previous)
      setKeyboardNotice(null)
      return
    }

    // 梭哈就是输入内容中的文字金额：大 + 梭哈 => 大梭哈，
    // 12345/ + 梭哈 => 12345/梭哈。具体可用积分与限额只由后端决定。
    setBetInput((current) => current.endsWith('梭哈') ? current : `${current}梭哈`)
    setKeyboardNotice('梭哈已作为金额文字填入，发送后按可用积分受理')
  }
  const submitBet = async (rawInput?: string, fallbackAmount?: number) => {
    if (!canAcceptBet(game)) {
      setBetError(`${gameAcceptance(game).label}，请等待可投注状态后再试。`)
      setDialog('bet-error')
      return
    }
    let content = (rawInput ?? betInput).trim()
    if (!content) return setDialog('required')
    if (fallbackAmount && !content.includes('/')) content = `${content}/${fallbackAmount}`
    setSubmitting(true)
    setBetError('')
    const requestSession = `${game.id}:${game.period}`
    try {
      // The assistant resolves the live issue on the server, preventing a
      // countdown refresh from submitting a stale period captured by the UI.
      const requestId = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
      const accepted = await betsApi.assistantPlace(game.id, { content, request_id: requestId })
      // 请求返回时彩种或期号可能已切换；余额可以刷新，但旧会话不能继续写入 UI。
      if (gameSessionRef.current !== requestSession) {
        await onRefreshBalance()
        await loadWalletSummary()
        return
      }
      forceBottomRef.current = true
      setSubmittedBets((bets) => mergeAcceptedTickets(bets, [{
        gameId: game.id,
        content: accepted.content,
        lines: accepted.lines.map((line) => line.label),
        total: accepted.total,
        issue: accepted.issue,
        acceptedAt: accepted.accepted_at,
      }]))
      setAssistantStatus((current) => current ? { ...current, issue: accepted.issue, accepting: true } : current)
      await onRefreshBalance()
      await loadWalletSummary()
      await loadBets()
      void loadGameFeed()
      clearSelection()
      setShowKeyboard(false)
      setShowQuickBet(false)
    } catch (reason) {
      setBetError(reason instanceof Error ? reason.message : '投注失败')
      setDialog('bet-error')
    } finally {
      setSubmitting(false)
    }
  }

  const submitInput = async () => {
    const content = betInput.trim()
    if (!content) return setDialog('required')
    setSendingMessage(true)
    try {
      const message = await chatApi.send(content, 'group', game.id)
      if (gameSessionRef.current.startsWith(`${game.id}:`)) {
        forceBottomRef.current = true
        lastSentContentRef.current = content
        setGameMessages((current) => mergeChatMessages(current, [message]))
        setBetInput('')
        setKeyboardNotice(null)
        // 群聊接口会在同一请求内完成命令解析并持久化助手回执。
        // 重新拉取一次即可按消息 ID 顺序看到“我发送 → 助手回复”。
        await Promise.all([loadGameMessages(), loadBets(), onRefreshBalance(), loadWalletSummary()])
        void loadGameFeed()
      }
    } catch (reason) {
      setBetError(reason instanceof Error ? reason.message : '消息发送失败')
      setDialog('bet-error')
    } finally {
      setSendingMessage(false)
    }
  }

  const cancelBet = async (id: number) => {
    try {
      await betsApi.cancel(id)
      await onRefreshBalance()
      await loadWalletSummary()
      await loadBets()
    } catch (reason) {
      setBetError(reason instanceof Error ? reason.message : '撤单失败')
      setDialog('bet-error')
    }
  }

  const drawPositionLabels = drawPositionNames.slice(0, Math.max(game.balls.length, 5))
  const recentDraws = draws.slice(0, 8).map((draw) => {
    const balls = draw.numbers.length ? draw.numbers : game.balls
    const meta = crownMeta(balls)
    return { period: draw.issue, balls, ...meta }
  })
  const latestMeta = crownMeta(game.balls)
  const acceptance = gameAcceptance(game)
  const assistantAcceptance = !canAcceptBet(game)
    ? `${acceptance.label}，当前暂停接单。`
    : assistantStatus
    ? assistantStatus.accepting ? '本期投注受理中，请核对玩法与金额后提交。' : '本期已封盘，请等待下一期开始受理。'
    : `本期${acceptance.label}。`
  const visibleSubmittedBets = submittedBets.filter((ticket) => {
    if (ticket.gameId !== game.id) return false
    const ticketBets = memberBets.filter((bet) => bet.issue === ticket.issue)
    // Hide an assistant receipt only when the persisted rows prove the whole
    // ticket was cancelled. A temporary bet-list failure must not hide a newly
    // accepted receipt that already came back from the placement endpoint.
    return ticketBets.length === 0 || ticketBets.some((bet) => bet.status !== 'cancelled')
  })
  const timelineReady = messagesReady && settlementsReady && betsReady && assistantHistoryReady && feedReady
  const timelineVersion = useMemo(() => [
    gameMessages.map((message) => `chat:${message.id}`).join(','),
    feedItems.map((item) => `feed:${item.created_at}:${item.nickname}:${item.detail}`).join(','),
    visibleSubmittedBets.map((ticket) => `ticket:${ticket.issue}:${ticket.acceptedAt}:${ticket.content}`).join(','),
    settlementNotices.map((notice) => `settlement:${notice.id}:${notice.created_at}`).join(','),
    draws[0] ? `draw:${draws[0].id}:${draws[0].draw_at}` : '',
    visibleSubmittedBets.length === 0 ? memberBets.map((bet) => `bet:${bet.id}:${bet.status}`).join(',') : '',
  ].join('|'), [draws, feedItems, gameMessages, memberBets, settlementNotices, visibleSubmittedBets])

  const scrollToLatest = useCallback((behavior: ScrollBehavior = 'smooth') => {
    const node = chatRef.current
    if (!node) return
    node.scrollTo({ top: node.scrollHeight, behavior })
    nearBottomRef.current = true
    setShowScrollLatest(false)
  }, [])

  // First paint is positioned before the browser displays the timeline, so
  // users never see the top and then a visible jump to the newest message.
  useLayoutEffect(() => {
    if (!timelineReady || timelinePositioned) return
    const node = chatRef.current
    if (!node) return
    node.scrollTop = node.scrollHeight
    nearBottomRef.current = true
    setShowScrollLatest(false)
    setTimelinePositioned(true)
  }, [game.id, timelinePositioned, timelineReady])

  // New data follows the bottom only while the user is already reading the
  // latest messages. Polling must never pull someone away from older history.
  useEffect(() => {
    if (!timelineReady || !timelinePositioned) return
    const forced = forceBottomRef.current
    if (!forced && !nearBottomRef.current) return
    const frame = window.requestAnimationFrame(() => {
      scrollToLatest(forced ? 'smooth' : 'auto')
      forceBottomRef.current = false
    })
    return () => window.cancelAnimationFrame(frame)
  }, [scrollToLatest, timelinePositioned, timelineReady, timelineVersion])

  if (showCheckIn) {
    return (
      <div className={`check-in-shell theme-${theme}`}>
        <CheckIn onBack={() => setShowCheckIn(false)} onComplete={() => { void onRefreshBalance(); void loadWalletSummary() }} />
      </div>
    )
  }
  return <main className={`game-room theme-${theme} font-scale-${fontScale}${historyOpen ? ' history-expanded' : ''}`} onClick={() => { setShowKeyboard(false); setShowAddMenu(false) }}>
    <header className="game-header"><button aria-label="返回大厅" onClick={onBack}><Icon name="back" /></button><b>{game.title}</b><div className="game-header-meta" aria-label="账户今日统计"><span><em>积分</em><strong>{formatHeaderAmount(balance)}</strong></span>{roomFeatures.showTurnover && <span><em>流水</em><strong>{walletSummary ? formatHeaderAmount(walletSummary.today_turnover) : '—'}</strong></span>}{roomFeatures.showProfit && <span><em>输赢</em><strong>{walletSummary ? formatHeaderAmount(walletSummary.today_profit) : '—'}</strong></span>}{roomFeatures.showRebate && <span><em>回水</em><strong>{walletSummary ? formatHeaderAmount(walletSummary.today_rebate) : '—'}</strong></span>}</div></header>
    <section className="game-info"><div><span aria-label={`当前期号 ${assistantStatus?.issue ?? game.period}`}>{shortIssue(assistantStatus?.issue ?? game.period)}</span><b>{game.due.split('').map((number, index) => <i key={index}>{number}</i>)}</b><small className={`game-acceptance ${acceptance.tone}`}>{acceptance.label}</small></div>{(roomFeatures.showMipai || roomFeatures.showOrders || roomFeatures.showStreak || roomFeatures.showPrediction) && <nav className="game-tool-tabs" aria-label="游戏工具">{roomFeatures.showMipai && <button onClick={() => setDialog('mipai')}>咪牌</button>}{roomFeatures.showOrders && <button onClick={() => setDialog('orders')}>注单</button>}{roomFeatures.showStreak && <button onClick={() => setDialog('trend')}>长龙</button>}{roomFeatures.showPrediction && <button onClick={() => setDialog('forecast')}>预测</button>}</nav>}</section>
    <section className={`draw-history ${historyOpen ? 'open' : ''}${drawPositionLabels.length > 5 ? ' racing-draw-ui' : ''}`}><button className="last-draw" aria-expanded={historyOpen} onClick={() => setHistoryOpen((open) => !open)}><span>上期 {shortIssue(recentDraws[0]?.period ?? game.period)}</span><div>{game.balls.map((number, index) => <b className={ballTone(number)} key={index}>{number}</b>)}</div><small>{drawPositionLabels.length <= 5 && '冠亚 '}<b>{latestMeta.crownResult}</b></small></button><div className="recent-draws" aria-hidden={!historyOpen}><header><span>期数</span><b>{drawPositionLabels.map((label) => <i key={label}>{label}</i>)}</b><small><b>冠亚和</b><i aria-hidden="true">·</i><em>龙虎</em></small></header>{drawsLoading && <p className="recent-draws-loading">加载开奖…</p>}{recentDraws.slice(0, 5).map((draw) => <article key={draw.period}><span>{shortIssue(draw.period)}</span><div>{draw.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div><small><b>{draw.crownResult}</b><em>{draw.dragonTiger}</em></small></article>)}<button className="more-draws" onClick={onOpenResults}>查看更多开奖</button></div></section>
    <section className="bet-chat" ref={chatRef} onScroll={(event) => {
      const node = event.currentTarget
      const nearBottom = node.scrollHeight - node.scrollTop - node.clientHeight < 48
      nearBottomRef.current = nearBottom
      setShowScrollLatest(!nearBottom)
    }}>
      <p>以上全接，以下无效。</p>
      <div className="admin-message assistant-notice">
        <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
        <div><small>开奖助手 · 24小时在线</small><article><b>【{game.title} - {shortIssue(assistantStatus?.issue ?? game.period)}】</b><hr /><span>{assistantAcceptance}</span><span className="assistant-help-example">多车道示例：1/12345/100#6/大/200#7/67890/100</span><span>每组用 # 分开，可一次提交多个车道。</span></article></div>
      </div>
      <div className={`game-timeline ${timelineReady ? (timelinePositioned ? 'ready' : 'positioning') : 'loading'}`}>
        {timelineReady
          ? <GameTimeline game={game} messages={gameMessages} draws={draws} notices={settlementNotices} feed={feedItems} tickets={visibleSubmittedBets} memberBets={memberBets} nickname={nickname} onOpenOrders={() => setDialog('orders')} />
          : <div className="game-timeline-loading"><i /><span>正在载入最新消息…</span></div>}
      </div>
    </section>
    {showScrollLatest && <button className={`scroll-latest-button${showKeyboard ? ' keyboard-open' : ''}`} type="button" aria-label="回到最新消息" onClick={(event) => { event.stopPropagation(); scrollToLatest() }}><span>↓</span><small>最新</small></button>}
    {showKeyboard && roomFeatures.webKeyboard ? <BetKeyboard mode={betMode} selectedCount={betInput.length} submitting={submitting || sendingMessage} notice={keyboardNotice} onShortcut={handleKeyboardShortcut} onBackspace={removeNumber} onClear={clearSelection} onConfirm={() => void submitInput()} onModeChange={setBetMode} onSelectNumber={appendNumber} onSelectOption={appendOption} showModes={false} /> : <QuickActions onCheckIn={() => setShowCheckIn(true)} onCustomerService={onOpenService} onQuickBet={() => { setShowKeyboard(false); setShowQuickBet(true) }} onSwitchGame={() => setShowGameSwitcher(true)} />}
    <section className="ticket-strip" onClick={(event) => event.stopPropagation()}>{roomFeatures.webKeyboard && <button aria-label={showKeyboard ? '收起快捷键盘' : '打开快捷键盘'} className="ticket-ime" onClick={() => { setShowAddMenu(false); setShowKeyboard((visible) => !visible) }}><img alt="" src="/icons/lucide/keyboard.svg" /></button>}{roomFeatures.webKeyboard ? <button aria-label="打开投注键盘" className="ticket-selection" onClick={() => { setShowAddMenu(false); setShowKeyboard((visible) => !visible) }}>{betInput || '输入玩法/金额或聊天内容'}</button> : <input aria-label="输入玩法、金额或聊天内容" className="ticket-selection ticket-native-input" autoComplete="off" enterKeyHint="send" placeholder="输入玩法/金额或聊天内容" value={betInput} onChange={(event) => setBetInput(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void submitInput() }} />}{betInput ? <button aria-label="发送" className="ticket-add ticket-send" disabled={submitting || sendingMessage} onClick={() => void submitInput()}>{submitting || sendingMessage ? '…' : '发送'}</button> : <button aria-expanded={showAddMenu} aria-label="打开更多功能" className="ticket-add" onClick={() => { setShowKeyboard(false); setShowAddMenu((visible) => !visible) }}><Icon name="plus" /></button>}</section>
    {showAddMenu && <AddMenu onSelect={(action) => onOpenWallet(action)} />}
    {showQuickBet && <FullBetBoard game={game} mode={betMode} draft={betInput} submitting={submitting} defaultOdds={defaultOdds} onClear={clearSelection} onClose={() => setShowQuickBet(false)} onConfirm={(content) => void submitBet(content)} onModeChange={setBetMode} onSetDraft={setBetInput} />}
    {showGameSwitcher && <GameSwitcher currentGame={game.id} games={games} onClose={() => setShowGameSwitcher(false)} onSelect={onOpenGame} />}
    {dialog === 'mipai' && <MipaiDialog game={game} draw={draws[0]} onClose={() => setDialog(null)} />}
    {dialog === 'orders' && <OrdersDialog bets={memberBets} onCancel={(id) => void cancelBet(id)} onClose={() => setDialog(null)} />}
    {dialog === 'trend' && <TrendDialog game={game} draws={draws} onClose={() => setDialog(null)} />}
    {dialog === 'forecast' && <ForecastDialog game={game} draws={draws} onClose={() => setDialog(null)} />}
    {dialog === 'assist' && <ActionDialog title="投注助手" description="选择快捷、两面盘或号码面板后可自由组合；确认格式为 玩法/金额，多条用 # 分隔。" onClose={() => setDialog(null)} />}
    {dialog === 'required' && <ActionDialog title="请先选择投注内容" description="点击输入框或左侧输入法按钮打开投注面板，再选择号码或玩法并加上金额。" onClose={() => setDialog(null)} />}
    {dialog === 'bet-error' && <ActionDialog title="投注未成功" description={betError || '请检查余额、格式或封盘状态后重试。'} onClose={() => setDialog(null)} />}
    {winningPopup && <WinningPopup game={game} data={winningPopup} onClose={() => setWinningPopup(null)} />}
  </main>
}

function GameTimeline({ game, messages, draws, notices, feed, tickets, memberBets, nickname, onOpenOrders }: { game: Game; messages: ChatMessage[]; draws: DrawResult[]; notices: MemberNotification[]; feed: GameFeedItem[]; tickets: AcceptedTicket[]; memberBets: MemberBet[]; nickname: string; onOpenOrders: () => void }) {
  const draw = draws[0]
  const entries = useMemo(() => {
    const timeline: TimelineEntry[] = []
    messages.forEach((message) => timeline.push({ kind: 'chat', key: `chat:${message.id}`, at: timelineTime(message.created_at), value: message }))
    feed.forEach((item, index) => timeline.push({ kind: 'feed', key: `feed:${item.created_at}:${item.nickname}:${item.detail}`, at: timelineTime(item.created_at), value: item, index }))
    tickets.forEach((ticket) => timeline.push({ kind: 'ticket', key: `ticket:${ticket.issue}:${ticket.acceptedAt}:${ticket.content}`, at: timelineTime(ticket.acceptedAt), value: ticket }))
    notices.filter((notice) => notice.game_id === game.id).slice(-8).forEach((notice) => timeline.push({ kind: 'settlement', key: `settlement:${notice.id}`, at: timelineTime(notice.created_at), value: notice }))
    if (draw) timeline.push({ kind: 'draw', key: `draw:${draw.id}`, at: timelineTime(draw.draw_at), value: draw })
    if (!tickets.length && memberBets.length) {
      const latestBetAt = memberBets.reduce((latest, bet) => Math.max(latest, timelineTime(bet.created_at)), 0)
      timeline.push({ kind: 'persisted', key: `persisted:${memberBets[0]?.id ?? game.id}`, at: latestBetAt, value: memberBets })
    }
    const priority: Record<TimelineEntry['kind'], number> = { chat: 0, feed: 1, ticket: 2, persisted: 2, draw: 3, settlement: 4 }
    return timeline.sort((left, right) => left.at - right.at || priority[left.kind] - priority[right.kind] || left.key.localeCompare(right.key))
  }, [draw, feed, game.id, memberBets, messages, notices, tickets])

  return <div className="game-timeline-items">{entries.map((entry) => {
    if (entry.kind === 'chat') return <GameChatMessage key={entry.key} message={entry.value} nickname={nickname} />
    if (entry.kind === 'feed') return <GameFeedMessage key={entry.key} game={game} item={entry.value} index={entry.index} />
    if (entry.kind === 'ticket') return <SubmittedTicketMessage key={entry.key} game={game} ticket={entry.value} nickname={nickname} />
    if (entry.kind === 'draw') return <DrawAssistantMessage key={entry.key} game={game} draw={entry.value} draws={draws} />
    if (entry.kind === 'settlement') return <SettlementAssistantMessages key={entry.key} game={game} notices={[entry.value]} nickname={nickname} />
    return <PersistedBetSummary key={entry.key} game={game} bets={entry.value} nickname={nickname} onOpenOrders={onOpenOrders} />
  })}</div>
}

function GameChatMessage({ message, nickname }: { message: ChatMessage; nickname: string }) {
  if (message.mine) return <div className="player-bet game-chat-message mine"><div><small>{nickname}</small><article><span>{message.content}</span><time className="game-message-time mine">{formatFeedTime(message.created_at)}</time></article></div><Avatar className="player-avatar" index={-1} label="我的头像" /></div>
  if (['application', 'settlement', 'scoreboard'].includes(message.message_type)) {
    const [mention, ...content] = message.content.split('\n')
    const lines = mention.startsWith('@') ? content : [mention, ...content]
    const tone = message.message_type === 'settlement' ? ' room-settlement-message' : message.message_type === 'scoreboard' ? ' room-scoreboard-message' : ''
    return <div className={`admin-message application-assistant-message${tone}`}><span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span><div><small>开奖助手 · 24小时在线</small><article>{mention.startsWith('@') && <span className="assistant-mention">{mention}</span>}{lines.map((line, index) => index === 0 ? <strong key={`${message.id}-${index}`}>{line}</strong> : line ? <span className={`assistant-response-line${line.startsWith('得分：+') ? ' positive' : line.startsWith('得分：-') ? ' negative' : line.startsWith('[') ? ' player-row' : ''}`} key={`${message.id}-${index}`}>{line}</span> : <i className="assistant-response-gap" key={`${message.id}-${index}`} />)}<time className="game-message-time">{formatFeedTime(message.created_at)}</time></article></div></div>
  }
  return <article className="market-bet game-chat-message"><Avatar index={Number(message.public_id ?? message.user_id ?? 0)} label={`${message.nickname}的头像`} /><div><small>{message.nickname}</small><p>{message.content}<time className="game-message-time">{formatFeedTime(message.created_at)}</time></p></div></article>
}

function QuickActions({ onSwitchGame, onCustomerService, onQuickBet, onCheckIn }: { onSwitchGame: () => void; onCustomerService: () => void; onQuickBet: () => void; onCheckIn: () => void }) {
  return <div className="quick-actions"><button aria-label="切换游戏" onClick={onSwitchGame}>⇄</button><button aria-label="联系客服" onClick={onCustomerService}>🎧</button><button aria-label="快捷投注" onClick={onQuickBet}>☷</button><button aria-label="每日签到" className="quick-check-in" onClick={onCheckIn}>签</button></div>
}

function GameFeedMessage({ game, item, index }: { game: Game; item: GameFeedItem; index: number }) {
  return <article className="market-bet"><Avatar index={index} label={`${item.nickname}的头像`} /><div><small>{item.nickname}</small><p><b>【{game.title} · 第 {shortIssue(game.period)} 期】</b><br />{item.detail} · {item.amount} 元<em>已受理</em><time className="game-message-time">{formatFeedTime(item.created_at)}</time></p></div></article>
}

function SubmittedTicketMessage({ game, ticket, nickname }: { game: Game; ticket: AcceptedTicket; nickname: string }) {
  return <div className="submitted-ticket">
    <div className="player-bet"><div><small>{nickname}</small><article><span>{ticket.content}</span><time className="game-message-time mine">{formatFeedTime(ticket.acceptedAt)}</time></article></div><Avatar className="player-avatar" index={-1} label="我的头像" /></div>
    <div className="admin-message parsed-ticket"><span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span><div><small>开奖助手 · 24小时在线</small><article><span className="assistant-mention">@{nickname}</span><strong>【{game.title} - {shortIssue(ticket.issue)}】下单成功</strong><br />{ticket.lines.map((line) => <span className="parsed-line" key={line}>{line}</span>)}<footer>使用：{ticket.total.toLocaleString('zh-CN')}</footer><time className="game-message-time">{formatFeedTime(ticket.acceptedAt)}</time></article></div></div>
  </div>
}

function PersistedBetSummary({ game, bets, nickname, onOpenOrders }: { game: Game; bets: MemberBet[]; nickname: string; onOpenOrders: () => void }) {
  const latestIssue = bets[0]?.issue ?? game.period
  const issueBets = bets.filter((bet) => bet.issue === latestIssue)
  const visible = issueBets.slice(0, 8)
  const total = issueBets.filter((bet) => bet.status !== 'cancelled').reduce((sum, bet) => sum + bet.amount, 0)
  const isCurrent = latestIssue === game.period
  return <div className="admin-message parsed-ticket persisted-ticket"><span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span><div><small>开奖助手 · 24小时在线</small><article><span className="assistant-mention">@{nickname}</span><strong>【{game.title} - {shortIssue(latestIssue)}】{isCurrent ? '我的本期注单' : '我的最近注单'}</strong><i className="persisted-badge">历史记录</i>{visible.map((bet) => <span className="parsed-line persisted-line" key={bet.id}><span>{bet.play_name || `第${bet.position}球`} [{bet.selection}/{bet.amount.toFixed(2)}]</span><em className={bet.status}>{betStatusText(bet.status)}</em></span>)}{(issueBets.length > visible.length || bets.length > issueBets.length) && <button className="persisted-more" onClick={onOpenOrders}>查看该彩种全部注单</button>}<footer>共 {issueBets.length} 注 · 使用：{total.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</footer><time className="game-message-time">{formatFeedTime(issueBets[0]?.created_at ?? '')}</time></article></div></div>
}

function DrawAssistantMessage({ game, draw, draws }: { game: Game; draw: DrawResult; draws: DrawResult[] }) {
  const meta = crownMeta(draw.numbers)
  return <div className="admin-message draw-announcement">
    <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
    <div><small>开奖助手 · 24小时在线</small><article>
      <strong>【{game.title} - {shortIssue(draw.issue)}】已开奖</strong>
      <div className="draw-announcement-balls">{draw.numbers.map((number, index) => <b className={ballTone(number)} key={`${draw.id}-${index}`}>{number}</b>)}</div>
      <span className="draw-announcement-meta">冠亚和：{meta.crownResult}{meta.dragonTiger ? ` · 龙虎：${meta.dragonTiger}` : ''}</span>
      <p>本期开奖完成，下一期已经开始受理。</p>
      <DrawResultCards game={game} draw={draw} draws={draws} />
      <time className="game-message-time">{formatFeedTime(draw.draw_at)}</time>
    </article></div>
  </div>
}

function WinningPopup({ game, data, onClose }: { game: Game; data: WinningPopupData; onClose: () => void }) {
  return <div className="winning-popup-layer" role="presentation" onClick={onClose}>
    <section className="winning-popup" role="dialog" aria-modal="true" aria-label="中奖提示" onClick={(event) => event.stopPropagation()}>
      <div className="winning-coins" aria-hidden="true">{Array.from({ length: 18 }, (_, index) => <i key={index} style={{ left: `${4 + index * 5.35}%`, animationDelay: `${index * .08}s` }}>¥</i>)}</div>
      <span className="winning-crown">♛</span>
      <small>{game.title} · 第 {shortIssue(data.issue)} 期</small>
      <h2>中奖啦</h2>
      <strong>+{data.amount.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</strong>
      <p>中奖积分已经自动存入你的账户</p>
      <button onClick={onClose}>收下好运</button>
    </section>
  </div>
}

function SettlementAssistantMessages({ game, notices, nickname }: { game: Game; notices: MemberNotification[]; nickname: string }) {
  const visible = notices.filter((notice) => notice.game_id === game.id).slice(-8)
  if (!visible.length) return null
  return <div className="settlement-notice-list">{visible.map((notice) => {
    const numbers = notice.draw_numbers ?? []
    const details = notice.bet_details ?? []
    const won = (notice.won_count ?? 0) > 0
    return <div className={`admin-message draw-announcement personal-settlement ${won ? 'won' : 'lost'}`} key={notice.id}>
      <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
      <div><small>开奖助手 · 24小时在线</small><article>
        <span className="assistant-mention">@{nickname}</span>
        <strong>【{notice.game_name || game.title} - {shortIssue(notice.issue ?? '')}】结算完成</strong>
        {numbers.length > 0 && <div className="draw-announcement-balls">{numbers.map((number, index) => <b className={ballTone(number)} key={`${notice.id}-${index}`}>{number}</b>)}</div>}
        {details.length > 0 && <div className="settlement-bet-details">{details.map((detail, index) => <span className="parsed-line settlement-line" key={`${notice.id}-${index}`}>
          <span>{detail.play_name}{detail.selection ? ` · ${detail.selection}` : ''} · {detail.amount.toFixed(2)} 元</span>
          <em className={detail.result}>{detail.result === 'won' ? `中奖 ${detail.payout.toFixed(2)}` : '未中奖'}</em>
        </span>)}</div>}
        <footer>投注：{(notice.stake_amount ?? 0).toFixed(2)} · 中奖：{(notice.payout_amount ?? 0).toFixed(2)}</footer>
        <time className="game-message-time">{formatFeedTime(notice.draw_at ?? notice.created_at)}</time>
      </article></div>
    </div>
  })}</div>
}

function formatFeedTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '刚刚'
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

function BetKeyboard({ mode, selectedCount, submitting, notice, onShortcut, onModeChange, onBackspace, onClear, onSelectNumber, onSelectOption, onConfirm, showModes }: { mode: BetMode; selectedCount: number; submitting?: boolean; notice?: string | null; onShortcut: (action: KeyboardShortcut) => void; onModeChange: (mode: BetMode) => void; onBackspace: () => void; onClear: () => void; onSelectNumber: (number: number) => void; onSelectOption: (option: string) => void; onConfirm: () => void; showModes: boolean }) {
  const deleteTimerRef = useRef<number | null>(null)
  const didClearRef = useRef(false)
  const selectQuick = (key: string) => {
    if (key === '确认') return onConfirm()
    if (/^\d+$/.test(key)) return onSelectNumber(Number(key))
    if (quickOptions.has(key) || key === '/' || key === '#') onSelectOption(key)
  }
  const startDelete = () => {
    didClearRef.current = false
    deleteTimerRef.current = window.setTimeout(() => { onClear(); didClearRef.current = true }, 480)
  }
  const endDelete = () => {
    if (deleteTimerRef.current !== null) window.clearTimeout(deleteTimerRef.current)
    deleteTimerRef.current = null
  }
  const deleteOne = () => {
    if (didClearRef.current) { didClearRef.current = false; return }
    onBackspace()
  }
  const activeMode = showModes ? mode : 'quick'
  const keyClass = (key: string) => `bet-key ${key === '确认' ? 'confirm' : key === '←' ? 'command' : quickOptions.has(key) ? 'option' : 'number'}`
  const shortcuts: Array<{ id: KeyboardShortcut; label: string }> = [{ id: 'all-in', label: '梭哈' }, { id: 'cancel', label: '取消' }, { id: 'credit', label: '上分' }, { id: 'check', label: '查' }, { id: 'debit', label: '下分' }, { id: 'repeat', label: '重复' }]
  return <section className={`bet-keyboard ${showModes ? 'complex-bet-keyboard' : 'input-bet-keyboard'}`} onClick={(event) => event.stopPropagation()}>{showModes && <header><div className="bet-mode-tabs">{modes.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}</button>)}</div><span>已选 <b>{selectedCount}</b> 项</span>{selectedCount > 0 && <button className="clear-selection" onClick={onClear}>清空</button>}</header>}<nav className="keyboard-shortcuts" aria-label="快捷操作">{shortcuts.map((item) => <button className={`keyboard-shortcut ${item.id}`} key={item.id} type="button" onClick={() => onShortcut(item.id)}>{item.label}</button>)}</nav>{notice && <output className="keyboard-shortcut-notice" aria-live="polite">{notice}</output>}{activeMode === 'quick' ? <div>{quickKeys.map((key) => <button className={keyClass(key)} disabled={submitting && key === '确认'} key={key} onClick={() => key === '←' ? deleteOne() : selectQuick(key)} onPointerDown={key === '←' ? startDelete : undefined} onPointerLeave={key === '←' ? endDelete : undefined} onPointerUp={key === '←' ? endDelete : undefined}>{key === '确认' ? (submitting ? '提交中' : '确认投注') : key}</button>)}</div> : activeMode === 'dual' ? <div className="dual-board">{dualOptions.map((option) => <button key={option} onClick={() => onSelectOption(option)}><b>{option}</b><small>1.92</small></button>)}</div> : <div className="number-board">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button key={number} onClick={() => onSelectNumber(number)}>{number}</button>)}</div>}</section>
}

type FullBetSelection = { label: string; play: string }

function previewFullBetSelections(draft: string): FullBetSelection[] {
  const segments = draft.replace(/^买/, '').split('#').map((part) => part.trim()).filter(Boolean)
  return segments.flatMap((segment) => {
    const parts = segment.split('/').map(part => part.trim()).filter(Boolean)
    const play = parts.length >= 3 ? parts.slice(0, -1).join('/') : parts[0] ?? ''
    if (!play) return []
    const positionedNumbers = play.match(/^(10|[1-9])\/(\d+)$/)
    if (positionedNumbers) return [{ label: `第${positionedNumbers[1]}名号码 · ${positionedNumbers[2].split('').join(' ')}`, play }]
    const positionedSide = play.match(/^(10|[1-9])\/([大小单双龙虎])$/)
    if (positionedSide) return [{ label: `第${positionedSide[1]}名 · ${positionedSide[2]}`, play }]
    if (/^\d+$/.test(play)) return [{ label: `号码组合 · ${play.split('').join(' ')}`, play }]
    const matched = play.match(/冠亚和[大小单双]|冠军[大小单双龙虎]|亚军[大小单双龙虎]|第[三四五六七八九十]名[大小单双龙虎]/g)
    if (matched?.length) return matched.map((item) => ({ label: item, play: item }))
    return [{ label: play, play }]
  })
}

function FullBetBoard({ game, mode, draft, submitting, defaultOdds, onModeChange, onClear, onSetDraft, onConfirm, onClose }: { game: Game; mode: BetMode; draft: string; submitting?: boolean; defaultOdds: number; onModeChange: (mode: BetMode) => void; onClear: () => void; onSetDraft: (value: string) => void; onConfirm: (content: string) => void; onClose: () => void }) {
  const [rank, setRank] = useState('冠军')
  const [amount, setAmount] = useState(20)
  const [selectionOpen, setSelectionOpen] = useState(false)
  const ranks = ['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名']
  const modeItems: Array<{ id: BetMode; label: string; helper: string }> = [{ id: 'quick', label: '快捷', helper: '常用玩法' }, { id: 'dual', label: '两面盘', helper: '大小单双' }, { id: 'numbers', label: '号码', helper: '1 ~ 10' }]
  const quickOptions = ['大', '小', '单', '双', '龙', '虎']
  const selections = previewFullBetSelections(draft)
  const preparedContent = selections.map((selection) => `${selection.play}/${amount}`).join('#')
  const preparedBet = parseBetInput(preparedContent)
  const acceptance = gameAcceptance(game)
  const accepting = canAcceptBet(game)
  const isPlaySelected = (play: string) => selections.some((selection) => selection.play === play)
  const numericPlay = selections.find((selection) => /^\d+$/.test(selection.play))?.play ?? ''
  const isNumberSelected = (number: number) => numericPlay.includes(String(number))
  const togglePlay = (play: string) => {
    const next = isPlaySelected(play)
      ? selections.filter((selection) => selection.play !== play)
      : [...selections, { label: play, play }]
    onSetDraft(next.map((selection) => selection.play).join('#'))
  }
  const toggleNumber = (number: number) => {
    const token = String(number)
    const otherPlays = selections.filter((selection) => !/^\d+$/.test(selection.play)).map((selection) => selection.play)
    const nextNumbers = isNumberSelected(number) ? numericPlay.replace(token, '') : `${numericPlay}${token}`
    onSetDraft([...otherPlays, nextNumbers].filter(Boolean).join('#'))
  }
  const removeSelection = (play: string) => onSetDraft(selections.filter((selection) => selection.play !== play).map((selection) => selection.play).join('#'))
  return <div className="full-bet-layer" onClick={onClose}><section className="full-bet-board" onClick={(event) => event.stopPropagation()}><header className="full-bet-header"><button aria-label="返回游戏聊天室" onClick={onClose}><Icon name="back" /></button><div><b>{game.title}</b><small>第 {shortIssue(game.period)} 期 · {acceptance.label}</small></div><button className="full-bet-close" aria-label="关闭投注面板" onClick={onClose}>×</button></header><div className="full-bet-current"><span>距离截止 {game.due}</span><i className={`full-bet-acceptance ${acceptance.tone}`}>{acceptance.label}</i><div>{game.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div></div><div className="full-bet-workspace"><aside>{modeItems.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}<small>{item.helper}</small></button>)}</aside><section className="full-bet-content"><header><div><b>{mode === 'quick' ? '快捷投注' : mode === 'dual' ? '两面盘' : '号码投注'}</b><small>选择后高亮；再次点击可取消。</small></div><span>赔率 <b>{defaultOdds.toFixed(3)}</b></span></header>{mode === 'quick' && <><div className="rank-selector">{ranks.map((item) => <button className={rank === item ? 'active' : ''} key={item} onClick={() => setRank(item)}>{item}</button>)}</div><p className="board-section-title">{rank} · 选择玩法</p><div className="full-bet-options">{quickOptions.map((item) => { const play = `${rank}${item}`; return <button className={isPlaySelected(play) ? 'selected' : ''} key={item} onClick={() => togglePlay(play)}><b>{item}</b><small>{defaultOdds.toFixed(2)}</small></button> })}</div></>}{mode === 'dual' && <div className="full-bet-options">{dualOptions.map((item) => <button className={isPlaySelected(item) ? 'selected' : ''} key={item} onClick={() => togglePlay(item)}><b>{item}</b><small>{defaultOdds.toFixed(2)}</small></button>)}</div>}{mode === 'numbers' && <><p className="board-section-title">选择号码 · 已选会同步显示在投注清单，可再次点击取消</p><div className="full-bet-numbers">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button className={isNumberSelected(number) ? 'selected' : ''} key={number} onClick={() => toggleNumber(number)}><b>{number}</b><small>{(defaultOdds * 5).toFixed(2)}</small></button>)}</div></>}</section></div><footer className="full-bet-footer"><div className="full-bet-summary"><button onClick={onClear}>清空选择</button><button className="full-bet-selection-toggle" onClick={() => setSelectionOpen((open) => !open)}><span>已选 <b>{selections.length}</b> 组 · {preparedBet.payloads.length} 注</span><i>{selectionOpen ? '收起' : '查看清单'}</i></button>{selections.length > 0 && <button aria-label="删除最后一组选择" onClick={() => removeSelection(selections.at(-1)?.play ?? '')}>⌫</button>}</div>{selectionOpen && <div className="full-bet-selection-list"><header><b>本次投注清单</b><span>合计 ¥ {preparedBet.total.toFixed(2)}</span></header>{selections.length ? <div>{selections.map((selection, index) => { const selectionBet = parseBetInput(`${selection.play}/${amount}`); return <article key={`${selection.play}-${index}`}><div><b>{selection.label}</b><small>{selectionBet.payloads.map(payloadLabel).join('、')}</small></div><strong>¥ {selectionBet.total.toFixed(2)}</strong><button aria-label={`删除${selection.label}`} onClick={() => removeSelection(selection.play)}>×</button></article> })}</div> : <p>暂未选择玩法或号码</p>}</div>}<div className="amount-pills">{[20, 50, 100, 200].map((value) => <button className={amount === value ? 'active' : ''} key={value} onClick={() => setAmount(value)}>{value}</button>)}</div><button className="full-bet-confirm" disabled={submitting || !selections.length || !accepting} onClick={() => onConfirm(preparedContent)}>{submitting ? '提交中…' : !accepting ? acceptance.label : '立即投注'} <small>¥ {preparedBet.total.toFixed(2)}</small></button></footer></section></div>
}

function OrdersDialog({ bets, onCancel, onClose }: { bets: MemberBet[]; onCancel: (id: number) => void; onClose: () => void }) {
  return <ActionDialog title="我的注单" description={bets.length ? `当前彩种最近 ${bets.length} 条个人注单` : '当前彩种暂无我的注单。'} onClose={onClose}>
    {bets.length > 0 && <div className="my-orders-list">{bets.map((bet) => <article key={bet.id}><header><b>{bet.play_name || bet.selection}</b><span className={`my-order-status ${bet.status}`}>{betStatusText(bet.status)}</span></header><p>第 {shortIssue(bet.issue)} 期 · 赔率 {bet.odds.toFixed(2)}</p><footer><strong>¥ {bet.amount.toFixed(2)}</strong>{bet.status === 'pending' && <button onClick={() => onCancel(bet.id)}>撤单</button>}</footer></article>)}</div>}
  </ActionDialog>
}

function MipaiDialog({ game, draw, onClose }: { game: Game; draw?: DrawResult; onClose: () => void }) {
  const balls = draw?.numbers?.length ? draw.numbers : game.balls
  const [revealed, setRevealed] = useState(0)
  const [round, setRound] = useState(0)

  useEffect(() => {
    setRevealed(0)
    if (!balls.length) return
    const timer = window.setInterval(() => {
      setRevealed((current) => {
        if (current >= balls.length) {
          window.clearInterval(timer)
          return current
        }
        return current + 1
      })
    }, 260)
    return () => window.clearInterval(timer)
  }, [balls.length, draw?.id, round])

  const complete = revealed >= balls.length
  const total = balls.reduce((sum, ball) => sum + ball, 0)
  return <ActionDialog title="咪牌" description={`${game.title} · 第 ${shortIssue(draw?.issue ?? game.period)} 期`} confirmLabel="关闭" onClose={onClose}>
    <section className="mipai-board">
      <header><span>{complete ? '本期号码已全部揭晓' : `正在揭晓第 ${Math.min(revealed + 1, balls.length)} 个号码`}</span><b>{revealed}/{balls.length}</b></header>
      <div className="mipai-balls">{balls.map((ball, index) => <i className={`${index < revealed ? `${ballTone(ball)} revealed` : ''}`} key={`${ball}-${index}`}>{index < revealed ? ball : '?'}</i>)}</div>
      {complete && balls.length === 3 && <p className="mipai-total">{balls.join(' + ')} = <b>{total}</b><span>{total >= 14 ? '大' : '小'} · {total % 2 ? '单' : '双'}</span></p>}
      <button className="mipai-replay" onClick={() => { setRevealed(0); setRound((current) => current + 1) }}>重新咪牌</button>
    </section>
  </ActionDialog>
}

type TrendItem = { label: string; value: string; count: number; tone: 'blue' | 'orange' }

function buildTrendItems(draws: DrawResult[]): TrendItem[] {
  if (!draws.length) return []
  const latest = draws[0].numbers
  const threshold = Math.max(...draws.flatMap((draw) => draw.numbers), 0) >= 10 ? 5 : 4
  const positions = latest.slice(0, 10).flatMap((ball, index) => {
    const size = ball > threshold ? '大' : '小'
    const parity = ball % 2 ? '单' : '双'
    const count = (matcher: (value: number) => boolean) => {
      let streak = 0
      for (const draw of draws) {
        const value = draw.numbers[index]
        if (value === undefined || !matcher(value)) break
        streak += 1
      }
      return streak
    }
    return [
      { label: `第${drawPositionNames[index] ?? index + 1}名`, value: size, count: count((value) => (value > threshold ? '大' : '小') === size), tone: size === '大' ? 'blue' as const : 'orange' as const },
      { label: `第${drawPositionNames[index] ?? index + 1}名`, value: parity, count: count((value) => (value % 2 ? '单' : '双') === parity), tone: parity === '双' ? 'blue' as const : 'orange' as const },
    ]
  })
  const dragonTiger = latest.length >= 10 ? latest.slice(0, 5).map((ball, index) => {
    const value = ball > latest[latest.length - 1 - index] ? '龙' : '虎'
    let count = 0
    for (const draw of draws) {
      const opposite = draw.numbers[draw.numbers.length - 1 - index]
      if (opposite === undefined || (draw.numbers[index] > opposite ? '龙' : '虎') !== value) break
      count += 1
    }
    return { label: `第${drawPositionNames[index] ?? index + 1}名`, value, count, tone: value === '龙' ? 'blue' as const : 'orange' as const }
  }) : []
  return [...positions, ...dragonTiger].sort((left, right) => right.count - left.count).slice(0, 10)
}

function TrendDialog({ game, draws, onClose }: { game: Game; draws: DrawResult[]; onClose: () => void }) {
  const items = useMemo(() => buildTrendItems(draws), [draws])
  return <ActionDialog title="长龙走势" description={`${game.title} · 根据最近 ${draws.length} 期连续结果统计`} onClose={onClose}>
    {items.length ? <section className="trend-board">{items.map((item, index) => <article key={`${item.label}-${item.value}-${index}`}><span>{item.label}</span><b className={item.tone}>{item.value}</b><em>连续 {item.count} 期</em></article>)}</section> : <p className="game-tool-empty">暂无足够的开奖记录</p>}
  </ActionDialog>
}

function ForecastDialog({ game, draws, onClose }: { game: Game; draws: DrawResult[]; onClose: () => void }) {
  const forecast = useMemo(() => {
    if (!draws.length) return null
    const counts = new Map<number, number>()
    draws.slice(0, 20).forEach((draw) => draw.numbers.forEach((ball) => counts.set(ball, (counts.get(ball) ?? 0) + 1)))
    const ranked = [...counts.entries()].sort((left, right) => right[1] - left[1] || left[0] - right[0])
    const hot = ranked.slice(0, Math.min(5, ranked.length)).map(([number]) => number)
    const cold = ranked.slice(-Math.min(5, ranked.length)).reverse().map(([number]) => number)
    const latest = draws[0].numbers
    const threshold = Math.max(...ranked.map(([number]) => number), 0) >= 10 ? 5 : 4
    const bigCount = draws.slice(0, 10).reduce((count, draw) => count + draw.numbers.filter((number) => number > threshold).length, 0)
    const totalCount = draws.slice(0, 10).reduce((count, draw) => count + draw.numbers.length, 0)
    return { hot, cold, latest, bias: bigCount >= totalCount / 2 ? '大势偏热' : '小势偏热' }
  }, [draws])
  return <ActionDialog title="走势预测" description={`${game.title} · 第 ${shortIssue(game.period)} 期参考`} onClose={onClose}>
    {forecast ? <section className="forecast-board"><article><small>近期热号</small><div>{forecast.hot.map((ball) => <b className={ballTone(ball)} key={ball}>{ball}</b>)}</div></article><article><small>近期冷号</small><div>{forecast.cold.map((ball) => <b className={ballTone(ball)} key={ball}>{ball}</b>)}</div></article><article className="forecast-summary"><small>走势观察</small><strong>{forecast.bias}</strong><span>最近一期：{forecast.latest.join(' · ')}</span></article><p>根据最近开奖记录生成，仅供走势参考，不代表开奖结果。</p></section> : <p className="game-tool-empty">暂无足够的开奖记录</p>}
  </ActionDialog>
}

function betStatusText(status: string) {
  return ({ pending: '待开奖', won: '已中奖', lost: '未中奖', cancelled: '已撤销' } as Record<string, string>)[status] ?? status
}

function AddMenu({ onSelect }: { onSelect: (action?: WalletActionSlug) => void }) {
  const items: Array<{ icon: string; label: string; color: string; action?: WalletActionSlug }> = [
    { icon: '/icons/duo/coin-stack.svg', label: '上下分', color: '#4c8bf5', action: undefined }, { icon: '/icons/duo/clipboard.svg', label: '申请记录', color: '#f39a4b', action: 'applications' },
    { icon: '/icons/duo/clapperboard.svg', label: '游戏记录', color: '#42b99a', action: 'bets' }, { icon: '/icons/duo/chart-pie.svg', label: '竞猜报表', color: '#7b83ef', action: 'pending-bets' },
    { icon: '/icons/duo/credit-card.svg', label: '积分账变', color: '#e79b4b', action: 'ledger' }, { icon: '/icons/duo/clock.svg', label: '自助回水', color: '#42a8c2', action: 'rebate' },
    { icon: '/icons/duo/confetti.svg', label: '福利报表', color: '#e8799a', action: 'welfare' }, { icon: '/icons/duo/discount.svg', label: '红包报表', color: '#ef6b62', action: 'redpacket' },
  ]
  return <section className="add-menu add-menu-inline" onClick={(event) => event.stopPropagation()}><i className="add-menu-handle" /><div>{items.map((item) => <button key={item.label} onClick={() => onSelect(item.action)}><span className="duo-menu-icon" style={{ backgroundColor: item.color, maskImage: `url(${item.icon})`, WebkitMaskImage: `url(${item.icon})` }} /><b>{item.label}</b></button>)}</div></section>
}

function GameSwitcher({ currentGame, games, onClose, onSelect }: { currentGame: string; games: Game[]; onClose: () => void; onSelect: (id: string) => void }) {
  return <div className="game-menu-layer game-switch-layer" onClick={onClose}><aside className="game-switch-sheet" onClick={(event) => event.stopPropagation()}><header><b>⇄ 切换游戏</b><button onClick={onClose}>×</button></header>{games.map((item) => <button className={item.id === currentGame ? 'current' : ''} key={item.id} onClick={() => { onClose(); if (item.id !== currentGame) onSelect(item.id) }}><span className={`${item.logo ? 'has-image' : ''}${item.id === 'fly-racing' ? ' compact-source-logo' : ''}`} style={{ background: item.logo ? 'transparent' : item.color }}>{item.logo ? <img alt={`${item.title} Logo`} src={item.logo} /> : item.tag.slice(0, 2)}</span><div><b>{item.title}</b><small>第 {item.period} 期</small></div><em>{item.id === currentGame ? '当前游戏' : `剩余 ${item.due}`}</em></button>)}</aside></div>
}
