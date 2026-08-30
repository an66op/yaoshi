import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { Game, Theme } from '../types'
import { ballTone } from '../data/games'
import { Avatar } from '../components/Avatar'
import { Icon } from '../components/Icon'
import { ActionDialog, RedPacketDialog } from '../components/Dialogs'
import { DrawResultCards } from '../components/DrawResultCards'
import { GameChatMessage } from '../components/GameChatMessage'
import { LotteryCountdown } from '../components/LotteryCountdown'
import { ScratchDrawDialog } from '../components/ScratchDrawDialog'
import { ScrollToLatestButton } from '../components/ScrollToLatestButton'
import { buildGameTimelineEntries, formatGameMessageTime as formatFeedTime, isRoomCommandContent, ticketsForGame, type AcceptedTicket } from '../utils/gameRoomMessages'
import { playNotificationSound } from '../utils/notificationAudio'
import { CheckIn } from './CheckIn'
import { parseBetInput, type ParsedBet } from '../utils/betParser'
import { createRequestId } from '../utils/requestId'
import { betsApi, type AssistantDrawStatus, type MemberBet } from '../api/bets'
import { useGameDraws } from '../hooks/useGameDraws'
import type { DrawResult } from '../api/lottery'
import { useMemberPreferences } from '../hooks/useMemberPreferences'
import { WS_EVENT, type WsEvent, useWebSocketConnected } from '../hooks/useWebSocket'
import { portalApi, type GameFeedItem, type GameOdds, type MemberNotification } from '../api/portal'
import { chatApi, type ChatMessage } from '../api/chat'
import { memberApi, type WalletSummary } from '../api/member'
import type { WalletActionSlug } from '../router'
import { chatScrollState } from '../utils/chatScroll'
import { isClaimableRoomRedPacket } from '../utils/roomRedPacket'
import {
  DEFAULT_ROOM_FEATURES,
  canSubmitPlayWithOddsResponse,
  oddsForPlayCode,
  oddsForSelection,
  oddsLabel,
  playOddsFromResponse,
  roomFeaturesFromSettings,
  type PlayOdds,
  type RoomFeatureSettings,
} from '../utils/gameRoomSafety'

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
type Dialog = 'scratch' | 'orders' | 'trend' | 'forecast' | 'assist' | 'required' | 'bet-error' | null
type BetMode = 'quick' | 'dual' | 'numbers'
type KeyboardShortcut = 'all-in' | 'cancel' | 'credit' | 'check' | 'debit' | 'repeat'
type WinningPopupData = { id: string; issue: string; amount: number }

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
const dualOptions = ['冠军大', '冠军小', '冠军单', '冠军双', '冠军龙', '冠军虎', '亚军大', '亚军小', '亚军单', '亚军双', '冠亚和大', '冠亚和小', '冠亚和单', '冠亚和双']
const modes: Array<{ id: BetMode; label: string }> = [{ id: 'quick', label: '快捷' }, { id: 'dual', label: '两面盘' }, { id: 'numbers', label: '号码 1~10' }]
const drawPositionNames = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十']

function shortIssue(issue: string) {
  return issue.match(/^(\d{8})-/)?.[1] ?? issue
}

function gameAcceptance(game: Game) {
  const { timing } = game
  return {
    label: timing.statusLabel,
    tone: timing.accepting ? 'open' : timing.phase === 'pending' || timing.phase === 'unavailable' ? 'syncing' : 'closed',
  }
}

function canAcceptBet(game: Game) {
  return game.timing.accepting
}

function supportsRankedBetBoard(game: Game) {
  return /赛车|飞艇|幸运10/i.test(`${game.category} ${game.title}`)
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
  const timelineRef = useRef<HTMLDivElement>(null)
  const nearBottomRef = useRef(true)
  const forceBottomRef = useRef(false)
  const latestScrollFrameRef = useRef<number | null>(null)
  const latestSettleFrameRef = useRef<number | null>(null)
  const lastSentContentRef = useRef('')
  const betSubmitLockRef = useRef(false)
  const messageSubmitLockRef = useRef(false)
  const pendingBetRequestRef = useRef<{ key: string; id: string } | null>(null)
  const pendingCommandRequestRef = useRef<{ key: string; id: string } | null>(null)
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
  const [roomFeatures, setRoomFeatures] = useState<RoomFeatureSettings>({ ...DEFAULT_ROOM_FEATURES })
  const [roomRedPacket, setRoomRedPacket] = useState<ChatMessage | null>(null)
  const redPacketRequestRef = useRef(0)
  const claimedRedPacketIDsRef = useRef(new Set<number>())
  const [packetDialog, setPacketDialog] = useState<ChatMessage | null>(null)
  const [packetReward, setPacketReward] = useState<number | null>(null)
  const [packetOpening, setPacketOpening] = useState(false)
  const [packetError, setPacketError] = useState('')
  const seenWinningEventsRef = useRef(new Set<string>())

  const syncChatScroll = useCallback((node: HTMLElement) => {
    const state = chatScrollState(node)
    nearBottomRef.current = state.following
    setShowScrollLatest(state.showLatest)
    return state
  }, [])

  const placeAtLatest = useCallback(() => {
    const node = chatRef.current
    if (!node) return Number.POSITIVE_INFINITY
    node.scrollTop = node.scrollHeight
    nearBottomRef.current = true
    setShowScrollLatest(false)
    return chatScrollState(node).distance
  }, [])

  // A message can grow more than once: first the user's row, then the assistant
  // receipt, draw card or image. Two layout frames keep the viewport anchored
  // until the complete message has taken its final height.
  const followLatestAfterLayout = useCallback(() => {
    forceBottomRef.current = true
    if (latestScrollFrameRef.current !== null) window.cancelAnimationFrame(latestScrollFrameRef.current)
    if (latestSettleFrameRef.current !== null) window.cancelAnimationFrame(latestSettleFrameRef.current)
    latestScrollFrameRef.current = window.requestAnimationFrame(() => {
      latestScrollFrameRef.current = null
      placeAtLatest()
      latestSettleFrameRef.current = window.requestAnimationFrame(() => {
        latestSettleFrameRef.current = null
        const distance = placeAtLatest()
        if (distance <= 2) forceBottomRef.current = false
      })
    })
  }, [placeAtLatest])

  const scrollToLatest = useCallback(() => {
    followLatestAfterLayout()
  }, [followLatestAfterLayout])

  useEffect(() => () => {
    if (latestScrollFrameRef.current !== null) window.cancelAnimationFrame(latestScrollFrameRef.current)
    if (latestSettleFrameRef.current !== null) window.cancelAnimationFrame(latestSettleFrameRef.current)
  }, [])

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
      setRoomFeatures(roomFeaturesFromSettings(settings))
    }).catch(() => {
      if (active) setRoomFeatures({ ...DEFAULT_ROOM_FEATURES })
    })
    return () => { active = false }
  }, [])

  useEffect(() => {
    if (!roomFeatures.webKeyboard) setShowKeyboard(false)
  }, [roomFeatures.webKeyboard])

  const loadRoomRedPacket = useCallback(async () => {
    const requestID = ++redPacketRequestRef.current
    try {
      const packet = await chatApi.availableRedPacket()
      if (requestID !== redPacketRequestRef.current) return
      const claimable = packet
        && !claimedRedPacketIDsRef.current.has(packet.id)
        && isClaimableRoomRedPacket(packet)
      setRoomRedPacket(claimable ? packet : null)
    } catch {
      // A short reconnect must not make a valid room packet disappear. The
      // recovery request or its expiry timer will reconcile the prompt.
    }
  }, [])

  useEffect(() => {
    void loadRoomRedPacket()
    // WebSocket is the live path. A recovery timer only runs while disconnected;
    // reconnection changes realtimeConnected and immediately performs a reload.
    const timer = realtimeConnected ? 0 : window.setInterval(() => void loadRoomRedPacket(), 15_000)
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail?.type === 'chat_message'
        && detail.data.room_type === 'group'
        && detail.data.game_id === 'lobby'
        && detail.data.message_type === 'redpacket') void loadRoomRedPacket()
    }
    const onVisibility = () => {
      if (document.visibilityState === 'visible') void loadRoomRedPacket()
    }
    window.addEventListener(WS_EVENT, onWs)
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      if (timer) window.clearInterval(timer)
      window.removeEventListener(WS_EVENT, onWs)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [loadRoomRedPacket, realtimeConnected])

  useEffect(() => {
    if (!roomRedPacket?.red_packet_expires_at) return
    const expiresAt = new Date(roomRedPacket.red_packet_expires_at).getTime()
    if (!Number.isFinite(expiresAt)) return
    const remaining = expiresAt - Date.now()
    if (remaining <= 0) {
      setRoomRedPacket(null)
      return
    }
    const timer = window.setTimeout(() => setRoomRedPacket(null), Math.min(remaining, 2_147_000_000))
    return () => window.clearTimeout(timer)
  }, [roomRedPacket])

  const openRoomRedPacket = () => {
    if (!roomRedPacket) return
    setPacketDialog(roomRedPacket)
    setPacketReward(null)
    setPacketError('')
  }

  const claimRoomRedPacket = async () => {
    if (!packetDialog || packetOpening) return
    setPacketOpening(true)
    setPacketError('')
    try {
      const result = await chatApi.claimRedPacket(packetDialog.id)
      claimedRedPacketIDsRef.current.add(packetDialog.id)
      redPacketRequestRef.current += 1
      setPacketReward(result.reward)
      setRoomRedPacket(null)
      playNotificationSound('reward')
      await Promise.all([onRefreshBalance(), loadWalletSummary(), loadRoomRedPacket()])
    } catch (reason) {
      setPacketError(reason instanceof Error ? reason.message : '红包暂时无法领取，请稍后重试')
      void loadRoomRedPacket()
    } finally {
      setPacketOpening(false)
    }
  }

  const closeRoomRedPacket = () => {
    if (packetOpening) return
    setPacketDialog(null)
    setPacketReward(null)
    setPacketError('')
  }

  // 换期重置未提交输入和弹层，但已经确认的回执属于持久时间线。
  // 不能因历史接口瞬断把上一期成功回执清掉；仅切换彩种时隔离消息。
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
    setSubmittedBets((tickets) => ticketsForGame(tickets, game.id))
    setMemberBets([])
    if (gameChanged) setOddsInfo(null)
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
    // 开奖后服务端通常会立刻推进到下一期。中奖弹窗属于刚结算的上一期，
    // 不能跟着 period 会话重置一起清掉，否则会出现“一闪而过”。只有
    // 真正切换彩种时才关闭；留在当前彩种时由会员点击按钮主动收下。
    if (gameChanged) setWinningPopup(null)
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
      // A reconnect/poll failure must not make persisted chat appear deleted.
      // Game changes clear the previous timeline in the session-reset effect.
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
      if (detail?.type === 'chat_message' && detail.data.game_id === game.id) {
        const shouldFollow = nearBottomRef.current
        if (shouldFollow) forceBottomRef.current = true
        void loadGameMessages().finally(() => {
          if (shouldFollow) followLatestAfterLayout()
        })
      }
    }
    window.addEventListener(WS_EVENT, onWs)
    return () => {
      if (timer) window.clearInterval(timer)
      window.removeEventListener(WS_EVENT, onWs)
    }
  }, [followLatestAfterLayout, game.id, loadGameMessages, realtimeConnected])

  useEffect(() => {
    let active = true
    setOddsInfo(null)
    void portalApi.gameOdds(game.id).then((result) => {
      if (active) setOddsInfo(result)
    }).catch(() => {
      if (active) setOddsInfo(null)
    })
    return () => { active = false }
  }, [game.id])

  const loadAssistant = useCallback(async () => {
    const requestGameID = game.id
    try {
      const result = await betsApi.assistantStatus(requestGameID)
      if (gameSessionRef.current.startsWith(`${requestGameID}:`)) setAssistantStatus(result)
    } catch {
      // Preserve the last server status during a transient reconnect. The next
      // WebSocket event or recovery poll replaces it with authoritative data.
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

  const playOdds = useMemo(() => playOddsFromResponse(oddsInfo), [oddsInfo])
  const oddsHidden = oddsInfo?.show_odds === false
  const oddsResponseReady = oddsInfo !== null

  const loadBets = useCallback(async () => {
    const requestSession = `${game.id}:${game.period}`
    try {
      // Load recent bets for this game, not only the issue currently displayed.
      // The issue may advance while the member is outside the room; filtering
      // to the new issue would make their just-placed ticket appear to vanish.
      const result = await betsApi.list({ game_id: game.id, page_size: 50 })
      if (gameSessionRef.current === requestSession) setMemberBets(result.items)
    } catch {
      // Keep already loaded tickets visible. Clearing here made a temporary
      // network error look exactly like the member's bets had been deleted.
    } finally {
      if (gameSessionRef.current === requestSession) setBetsReady(true)
    }
  }, [game.id, game.period])

  const loadAssistantHistory = useCallback(async () => {
    const requestGameID = game.id
    try {
      // 文本指令的回执来自群消息表；该接口只返回详细面板产生的直接
      // 投注，因此可以安全恢复而不会把同一张票显示两遍。
      const history = await betsApi.assistantHistory(requestGameID, 20)
      if (!gameSessionRef.current.startsWith(`${requestGameID}:`)) return
      const restored = history.map<AcceptedTicket>((item) => ({
        gameId: item.game_id,
        content: item.content,
        lines: item.lines.map((line) => line.label),
        total: item.total,
        balance: item.balance,
        issue: item.issue,
        acceptedAt: item.accepted_at,
      }))
      setSubmittedBets((current) => mergeAcceptedTickets(restored, current))
    } catch {
      // 注单接口仍会恢复财务记录；下一次重连/换期继续补拉原始回执。
    } finally {
      if (gameSessionRef.current.startsWith(`${requestGameID}:`)) setAssistantHistoryReady(true)
    }
  }, [game.id])

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
      if (detail?.type === 'bet_feed' && detail.data.game_id === game.id) {
        const shouldFollow = nearBottomRef.current
        if (shouldFollow) forceBottomRef.current = true
        void loadGameFeed().finally(() => {
          if (shouldFollow) followLatestAfterLayout()
        })
      }
    }
    window.addEventListener(WS_EVENT, onWs)
    return () => {
      if (timer) window.clearInterval(timer)
      window.removeEventListener(WS_EVENT, onWs)
    }
  }, [followLatestAfterLayout, game.id, loadGameFeed, realtimeConnected])

  // 开奖提示属于当前正在观看的彩种，不应在大厅、钱包或消息页响起。
  useEffect(() => {
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      const eventGameID = String(detail?.game_id ?? detail?.data.game_id ?? '')
      if (detail?.type === 'draw_update' && eventGameID === game.id) {
        forceBottomRef.current = true
        playNotificationSound('lottery')
        void Promise.all([
          loadBets(),
          loadGameFeed(),
          loadSettlementNotices(),
          loadAssistant(),
          onRefreshBalance(),
          loadWalletSummary(),
        ]).finally(followLatestAfterLayout)
      }
      if (detail?.type === 'notification' && detail.data.category === 'winning' && eventGameID === game.id) {
        forceBottomRef.current = true
        void Promise.all([
          loadSettlementNotices(),
          loadBets(),
          onRefreshBalance(),
          loadWalletSummary(),
        ]).finally(followLatestAfterLayout)
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
  }, [followLatestAfterLayout, game.id, loadAssistant, loadBets, loadGameFeed, loadSettlementNotices, loadWalletSummary, onRefreshBalance])

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
      // “重复”由服务端读取上一笔已受理投注，不能复制最近任意聊天文字
      // （否则上一条若是上分/下分申请会被错误再次发送）。
      setBetInput('重复')
      setKeyboardNotice('发送后重复上一笔已受理投注')
      return
    }

    // 梭哈就是输入内容中的文字金额：大 + 梭哈 => 大梭哈，
    // 12345/ + 梭哈 => 12345/梭哈。具体可用积分与限额只由后端决定。
    setBetInput((current) => current.endsWith('梭哈') ? current : `${current}梭哈`)
    setKeyboardNotice('梭哈已作为金额文字填入，发送后按可用积分受理')
  }
  const submitBet = async (rawInput?: string, fallbackAmount?: number) => {
    if (betSubmitLockRef.current) return
    if (!canAcceptBet(game)) {
      setBetError(`${gameAcceptance(game).label}，请等待可投注状态后再试。`)
      setDialog('bet-error')
      return
    }
    let content = (rawInput ?? betInput).trim()
    if (!content) return setDialog('required')
    if (fallbackAmount && !content.includes('/')) content = `${content}/${fallbackAmount}`
    const validationContent = content.includes('梭哈')
      ? content.endsWith('/梭哈')
        ? `${content.slice(0, -2)}1`
        : /^[大小单双龙虎]梭哈$/.test(content)
          ? `${content.slice(0, -2)}/1`
          : content
      : content
    const requestedPlays = parseBetInput(validationContent).payloads
    if (!requestedPlays.length || requestedPlays.some((play) => !canSubmitPlayWithOddsResponse(play.play_code ?? '', oddsInfo))) {
      setBetError('当前玩法赔率待配置，暂时不能提交。')
      setDialog('bet-error')
      return
    }
    const requestKey = `${game.id}:${game.period}:${content}`
    if (pendingBetRequestRef.current?.key !== requestKey) {
      pendingBetRequestRef.current = { key: requestKey, id: `board-${createRequestId()}` }
    }
    const requestId = pendingBetRequestRef.current.id
    betSubmitLockRef.current = true
    setSubmitting(true)
    setBetError('')
    const requestSession = `${game.id}:${game.period}`
    try {
      // 始终绑定会员确认时看到的精确期号；封盘/换期由服务端原子拒绝，
      // 绝不能静默把 A 期的确认落到 B 期。
      const accepted = await betsApi.assistantPlace(game.id, { issue: game.period, content, request_id: requestId })
      pendingBetRequestRef.current = null
      // 真正切换彩种后不把旧彩种回执写进当前页面；仅仅期号推进时仍
      // 展示服务端返回的 accepted.issue，避免成功扣分却看不到回执。
      if (!gameSessionRef.current.startsWith(`${game.id}:`)) {
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
        balance: accepted.balance,
        issue: accepted.issue,
        acceptedAt: accepted.accepted_at,
      }]))
      if (gameSessionRef.current === requestSession) {
        setAssistantStatus((current) => current ? { ...current, issue: accepted.issue, accepting: true } : current)
      }
      await onRefreshBalance()
      await loadWalletSummary()
      await loadBets()
      void loadGameFeed()
      clearSelection()
      setShowKeyboard(false)
      setShowQuickBet(false)
      forceBottomRef.current = true
      followLatestAfterLayout()
    } catch (reason) {
      setBetError(reason instanceof Error ? reason.message : '投注失败')
      setDialog('bet-error')
    } finally {
      betSubmitLockRef.current = false
      setSubmitting(false)
    }
  }

  const submitInput = async () => {
    if (messageSubmitLockRef.current || betSubmitLockRef.current) return
    const content = betInput.trim()
    if (!content) return setDialog('required')
    const command = isRoomCommandContent(content)
    const requestKey = `${game.id}:${game.period}:${content}`
    if (command && pendingCommandRequestRef.current?.key !== requestKey) {
      pendingCommandRequestRef.current = { key: requestKey, id: `chat-command-${createRequestId()}` }
    }
    messageSubmitLockRef.current = true
    setSendingMessage(true)
    try {
      const message = command
        ? await chatApi.command(content, game.id, { issue: game.period, request_id: pendingCommandRequestRef.current!.id })
        : await chatApi.send(content, 'group', game.id)
      if (command) pendingCommandRequestRef.current = null
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
        forceBottomRef.current = true
        followLatestAfterLayout()
      }
    } catch (reason) {
      setBetError(reason instanceof Error ? reason.message : '消息发送失败')
      setDialog('bet-error')
    } finally {
      messageSubmitLockRef.current = false
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
    : assistantStatus?.issue === game.period
    ? assistantStatus.accepting ? '本期投注受理中，请核对玩法与金额后提交。' : '本期已封盘，请等待下一期开始受理。'
    : `本期${acceptance.label}。`
  const visibleSubmittedBets = useMemo(() => submittedBets.filter((ticket) => {
    if (ticket.gameId !== game.id) return false
    const ticketBets = memberBets.filter((bet) => bet.issue === ticket.issue)
    // Hide an assistant receipt only when the persisted rows prove the whole
    // ticket was cancelled. A temporary bet-list failure must not hide a newly
    // accepted receipt that already came back from the placement endpoint.
    return ticketBets.length === 0 || ticketBets.some((bet) => bet.status !== 'cancelled')
  }), [game.id, memberBets, submittedBets])
  const timelineReady = messagesReady && settlementsReady && betsReady && assistantHistoryReady && feedReady
  const timelineVersion = useMemo(() => [
    gameMessages.map((message) => `chat:${message.id}`).join(','),
    feedItems.map((item) => `feed:${item.created_at}:${item.nickname}:${item.detail}`).join(','),
    visibleSubmittedBets.map((ticket) => `ticket:${ticket.issue}:${ticket.acceptedAt}:${ticket.content}`).join(','),
    settlementNotices.map((notice) => `settlement:${notice.id}:${notice.created_at}`).join(','),
    draws[0] ? `draw:${draws[0].id}:${draws[0].draw_at}` : '',
  ].join('|'), [draws, feedItems, gameMessages, settlementNotices, visibleSubmittedBets])

  // First paint is positioned before the browser displays the timeline, so
  // users never see the top and then a visible jump to the newest message.
  useLayoutEffect(() => {
    if (!timelineReady || timelinePositioned) return
    if (!chatRef.current) return
    placeAtLatest()
    setTimelinePositioned(true)
  }, [game.id, placeAtLatest, timelinePositioned, timelineReady])

  // New data follows while the reader is at the bottom. Explicit actions such
  // as sending, placing a bet and receiving the current draw set forceBottom.
  useLayoutEffect(() => {
    if (!timelineReady || !timelinePositioned) return
    const forced = forceBottomRef.current
    if (forced || nearBottomRef.current) followLatestAfterLayout()
    else if (chatRef.current) syncChatScroll(chatRef.current)
  }, [followLatestAfterLayout, syncChatScroll, timelinePositioned, timelineReady, timelineVersion])

  // Images, assistant cards and the on-screen keyboard change height after the
  // message state is already committed. Observe both the timeline and its
  // viewport so those later layout changes cannot leave new content below view.
  useLayoutEffect(() => {
    if (!timelineReady || !timelinePositioned || typeof ResizeObserver === 'undefined') return
    const viewport = chatRef.current
    const timeline = timelineRef.current
    if (!viewport || !timeline) return
    const observer = new ResizeObserver(() => {
      if (forceBottomRef.current || nearBottomRef.current) followLatestAfterLayout()
      else syncChatScroll(viewport)
    })
    observer.observe(viewport)
    observer.observe(timeline)
    return () => observer.disconnect()
  }, [followLatestAfterLayout, syncChatScroll, timelinePositioned, timelineReady])

  const toggleKeyboard = () => {
    if (nearBottomRef.current) forceBottomRef.current = true
    setShowAddMenu(false)
    setShowKeyboard((visible) => !visible)
  }

  if (showCheckIn) {
    return (
      <div className={`check-in-shell theme-${theme}`}>
        <CheckIn onBack={() => setShowCheckIn(false)} onComplete={() => { void onRefreshBalance(); void loadWalletSummary() }} />
      </div>
    )
  }
  return <main className={`game-room theme-${theme} font-scale-${fontScale}${historyOpen ? ' history-expanded' : ''}`} onClick={() => { setShowKeyboard(false); setShowAddMenu(false) }}>
    <header className="game-header">
      <button aria-label="返回大厅" onClick={onBack}><Icon name="back" /></button>
      <b>{game.title}</b>
      <div className="game-header-right">
        <div className="game-header-meta" aria-label="账户今日统计"><span><em>积分</em><strong>{formatHeaderAmount(balance)}</strong></span>{roomFeatures.showTurnover && <span><em>流水</em><strong>{walletSummary ? formatHeaderAmount(walletSummary.today_turnover) : '—'}</strong></span>}{roomFeatures.showProfit && <span><em>输赢</em><strong>{walletSummary ? formatHeaderAmount(walletSummary.today_profit) : '—'}</strong></span>}{roomFeatures.showRebate && <span><em>回水</em><strong>{walletSummary ? formatHeaderAmount(walletSummary.today_rebate) : '—'}</strong></span>}</div>
      </div>
    </header>
    <section className="game-info">
      <div className="game-round-info"><span className="game-round-issue" aria-label={`当前期号 ${game.period}`}><strong>{shortIssue(game.period)}</strong></span><LotteryCountdown timing={game.timing} compact /></div>
      {(roomFeatures.showMipai || roomFeatures.showOrders || roomFeatures.showStreak || roomFeatures.showPrediction) && <nav className="game-tool-tabs" aria-label="游戏工具">{roomFeatures.showMipai && <button onClick={() => setDialog('scratch')}>咪牌</button>}{roomFeatures.showOrders && <button onClick={() => setDialog('orders')}>注单</button>}{roomFeatures.showStreak && <button onClick={() => setDialog('trend')}>长龙</button>}{roomFeatures.showPrediction && <button onClick={() => setDialog('forecast')}>预测</button>}</nav>}
    </section>
    <section className={`draw-history ${historyOpen ? 'open' : ''}${drawPositionLabels.length > 5 ? ' racing-draw-ui' : ''}`}><button className="last-draw" aria-expanded={historyOpen} onClick={() => setHistoryOpen((open) => !open)}><span>上期 {shortIssue(game.latestIssue)}</span><div>{game.balls.map((number, index) => <b className={ballTone(number)} key={index}>{number}</b>)}</div><small>{drawPositionLabels.length <= 5 && '冠亚 '}<b>{latestMeta.crownResult}</b></small></button><div className="recent-draws" aria-hidden={!historyOpen}><header><span>期数</span><b>{drawPositionLabels.map((label) => <i key={label}>{label}</i>)}</b><small><b>冠亚和</b><i aria-hidden="true">·</i><em>龙虎</em></small></header>{drawsLoading && <p className="recent-draws-loading">加载开奖…</p>}{recentDraws.slice(0, 5).map((draw) => <article key={draw.period}><span>{shortIssue(draw.period)}</span><div>{draw.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div><small><b>{draw.crownResult}</b><em>{draw.dragonTiger}</em></small></article>)}<button className="more-draws" onClick={onOpenResults}>查看更多开奖</button></div></section>
    <section className="bet-chat" ref={chatRef} onScroll={(event) => {
      const node = event.currentTarget
      if (forceBottomRef.current) {
        nearBottomRef.current = true
        setShowScrollLatest(false)
        return
      }
      syncChatScroll(node)
    }}>
      <p>以上全接，以下无效。</p>
      <div className="admin-message assistant-notice">
        <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
        <div><small>开奖助手 · 24小时在线</small><article><b>【{game.title} - {shortIssue(game.period)}】</b><hr /><span>{assistantAcceptance}</span><span className="assistant-help-example">多车道示例：1/12345/100#6/大/200#7/67890/100</span><span>每组用 # 分开，可一次提交多个车道。</span></article></div>
      </div>
      <div className={`game-timeline ${timelineReady ? (timelinePositioned ? 'ready' : 'positioning') : 'loading'}`} ref={timelineRef}>
        {timelineReady
          ? <GameTimeline gameId={game.id} gameTitle={game.title} currentIssue={game.period} accepting={game.timing.accepting} messages={gameMessages} draws={draws} notices={settlementNotices} feed={feedItems} tickets={visibleSubmittedBets} nickname={nickname} />
          : <div className="game-timeline-loading"><i /><span>正在载入最新消息…</span></div>}
      </div>
    </section>
    {showScrollLatest && <ScrollToLatestButton keyboardOpen={showKeyboard} onScrollToLatest={scrollToLatest} />}
    {showKeyboard && roomFeatures.webKeyboard && <BetKeyboard mode={betMode} odds={playOdds} oddsHidden={oddsHidden} oddsResponseReady={oddsResponseReady} selectedCount={betInput.length} submitting={submitting || sendingMessage} notice={keyboardNotice} onShortcut={handleKeyboardShortcut} onBackspace={removeNumber} onClear={clearSelection} onConfirm={() => void submitInput()} onModeChange={setBetMode} onSelectNumber={appendNumber} onSelectOption={appendOption} showModes={false} />}
    <QuickActions hasRedPacket={Boolean(roomRedPacket)} keyboardOpen={showKeyboard && roomFeatures.webKeyboard} onCheckIn={() => setShowCheckIn(true)} onCustomerService={onOpenService} onOpenRedPacket={openRoomRedPacket} onQuickBet={() => { setShowKeyboard(false); if (supportsRankedBetBoard(game)) setShowQuickBet(true); else { setBetError('该彩种暂未配置详细选号面板，请使用输入框发送开奖助手下单规则。'); setDialog('bet-error') } }} onSwitchGame={() => setShowGameSwitcher(true)} />
    <section className="ticket-strip" onClick={(event) => event.stopPropagation()}>{roomFeatures.webKeyboard && <button aria-label={showKeyboard ? '收起快捷键盘' : '打开快捷键盘'} className="ticket-ime" onClick={toggleKeyboard}><img alt="" src="/icons/lucide/keyboard.svg" /></button>}{roomFeatures.webKeyboard ? <button aria-label="打开投注键盘" className="ticket-selection" onClick={toggleKeyboard}>{betInput || '输入玩法/金额或聊天内容'}</button> : <input aria-label="输入玩法、金额或聊天内容" className="ticket-selection ticket-native-input" autoComplete="off" disabled={submitting || sendingMessage} enterKeyHint="send" placeholder="输入玩法/金额或聊天内容" value={betInput} onChange={(event) => setBetInput(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void submitInput() } }} />}{betInput ? <button aria-label="发送" className="ticket-add ticket-send" disabled={submitting || sendingMessage} onClick={() => void submitInput()}>{submitting || sendingMessage ? '…' : '发送'}</button> : <button aria-expanded={showAddMenu} aria-label="打开更多功能" className="ticket-add" onClick={() => { setShowKeyboard(false); setShowAddMenu((visible) => !visible) }}><Icon name="plus" /></button>}</section>
    {showAddMenu && <AddMenu onSelect={(action) => onOpenWallet(action)} />}
    {showQuickBet && <FullBetBoard game={game} mode={betMode} submitting={submitting} odds={playOdds} oddsHidden={oddsHidden} oddsResponseReady={oddsResponseReady} onClose={() => setShowQuickBet(false)} onConfirm={(content) => void submitBet(content)} onModeChange={setBetMode} />}
    {showGameSwitcher && <GameSwitcher currentGame={game.id} games={games} onClose={() => setShowGameSwitcher(false)} onSelect={onOpenGame} />}
    {dialog === 'scratch' && <ScratchDrawDialog game={game} draw={draws[0]} onClose={() => setDialog(null)} />}
    {dialog === 'orders' && <OrdersDialog bets={memberBets} onCancel={(id) => void cancelBet(id)} onClose={() => setDialog(null)} />}
    {dialog === 'trend' && <TrendDialog game={game} draws={draws} onClose={() => setDialog(null)} />}
    {dialog === 'forecast' && <ForecastDialog game={game} draws={draws} onClose={() => setDialog(null)} />}
    {dialog === 'assist' && <ActionDialog title="投注助手" description="选择快捷、两面盘或号码面板后可自由组合；确认格式为 玩法/金额，多条用 # 分隔。" onClose={() => setDialog(null)} />}
    {dialog === 'required' && <ActionDialog title="请先选择投注内容" description="点击输入框或左侧输入法按钮打开投注面板，再选择号码或玩法并加上金额。" onClose={() => setDialog(null)} />}
    {dialog === 'bet-error' && <ActionDialog title="投注未成功" description={betError || '请检查余额、格式或封盘状态后重试。'} onClose={() => setDialog(null)} />}
    {winningPopup && <WinningPopup game={game} data={winningPopup} onClose={() => setWinningPopup(null)} />}
    {packetDialog && <RedPacketDialog type="lucky" claimed={packetReward !== null} reward={packetReward} greeting={packetDialog.content || '恭喜发财'} cover={packetDialog.red_packet_cover || 'classic'} minTurnover={Number(packetDialog.red_packet_min_turnover || 0)} opening={packetOpening} error={packetError} onOpen={() => void claimRoomRedPacket()} onClose={closeRoomRedPacket} />}
  </main>
}

// A clock tick replaces Game, but does not change historical chat content.
// Keep every field this subtree actually renders as an ordinary memo prop;
// accepting/issue changes still update the next-period announcement normally.
export const GameTimeline = memo(function GameTimeline({ gameId, gameTitle, currentIssue, accepting, messages, draws, notices, feed, tickets, nickname }: { gameId: string; gameTitle: string; currentIssue: string; accepting: boolean; messages: ChatMessage[]; draws: DrawResult[]; notices: MemberNotification[]; feed: GameFeedItem[]; tickets: AcceptedTicket[]; nickname: string }) {
  const draw = draws[0]
  const entries = useMemo(() => buildGameTimelineEntries({ gameId, messages, draw, notices, feed, tickets }), [draw, feed, gameId, messages, notices, tickets])

  return <div className="game-timeline-items">{entries.map((entry) => {
    if (entry.kind === 'chat') return <GameChatMessage key={entry.key} message={entry.value} nickname={nickname} />
    if (entry.kind === 'feed') return <GameFeedMessage key={entry.key} gameTitle={gameTitle} currentIssue={currentIssue} item={entry.value} index={entry.index} />
    if (entry.kind === 'ticket') return <SubmittedTicketMessage key={entry.key} gameTitle={gameTitle} ticket={entry.value} nickname={nickname} />
    if (entry.kind === 'draw') return <DrawAssistantMessage key={entry.key} gameTitle={gameTitle} currentIssue={currentIssue} accepting={accepting} draw={entry.value} draws={draws} />
    return <SettlementAssistantMessages key={entry.key} gameId={gameId} gameTitle={gameTitle} notices={[entry.value]} nickname={nickname} />
  })}</div>
})

function QuickActions({ hasRedPacket, keyboardOpen, onSwitchGame, onCustomerService, onQuickBet, onCheckIn, onOpenRedPacket }: { hasRedPacket: boolean; keyboardOpen: boolean; onSwitchGame: () => void; onCustomerService: () => void; onQuickBet: () => void; onCheckIn: () => void; onOpenRedPacket: () => void }) {
  return <div className={`quick-actions${keyboardOpen ? ' keyboard-open' : ''}`}><button aria-label="切换游戏" onClick={onSwitchGame}>⇄</button><button aria-label="联系客服" onClick={onCustomerService}>🎧</button><button aria-label="快捷投注" onClick={onQuickBet}>☷</button><button aria-label="每日签到" className="quick-check-in" onClick={onCheckIn}><span>签</span></button>{hasRedPacket && <button aria-label="领取房间红包" className="quick-red-packet" onClick={onOpenRedPacket}><i aria-hidden="true" /><Icon name="gift" /><small>红包</small></button>}</div>
}

function GameFeedMessage({ gameTitle, currentIssue, item, index }: { gameTitle: string; currentIssue: string; item: GameFeedItem; index: number }) {
  return <article className="market-bet"><Avatar index={index} label={`${item.nickname}的头像`} /><div><small>{item.nickname}</small><p><b>【{gameTitle} · 第 {shortIssue(currentIssue)} 期】</b><br />{item.detail} · {item.amount} 元<em>已受理</em><time className="game-message-time">{formatFeedTime(item.created_at)}</time></p></div></article>
}

function SubmittedTicketMessage({ gameTitle, ticket, nickname }: { gameTitle: string; ticket: AcceptedTicket; nickname: string }) {
  return <div className="submitted-ticket">
    <div className="player-bet"><div><small>{nickname}</small><article><span>{ticket.content}</span><time className="game-message-time mine">{formatFeedTime(ticket.acceptedAt)}</time></article></div><Avatar className="player-avatar" index={-1} label="我的头像" /></div>
    <div className="admin-message parsed-ticket"><span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span><div><small>开奖助手 · 24小时在线</small><article><span className="assistant-mention">@{nickname}</span><strong>【{gameTitle} - {shortIssue(ticket.issue)}】下单成功</strong><br />{ticket.lines.map((line) => <span className="parsed-line" key={line}>{line}</span>)}<footer><span>使用：{formatHeaderAmount(ticket.total)}</span><span>剩余：{formatHeaderAmount(ticket.balance)}</span></footer><time className="game-message-time">{formatFeedTime(ticket.acceptedAt)}</time></article></div></div>
  </div>
}

function DrawAssistantMessage({ gameTitle, currentIssue, accepting, draw, draws }: { gameTitle: string; currentIssue: string; accepting: boolean; draw: DrawResult; draws: DrawResult[] }) {
  const meta = crownMeta(draw.numbers)
  return <div className="admin-message draw-announcement">
    <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
    <div><small>开奖助手 · 24小时在线</small><article>
      <strong>【{gameTitle} - {shortIssue(draw.issue)}】已开奖</strong>
      <div className="draw-announcement-balls">{draw.numbers.map((number, index) => <b className={ballTone(number)} key={`${draw.id}-${index}`}>{number}</b>)}</div>
      <span className="draw-announcement-meta">冠亚和：{meta.crownResult}{meta.dragonTiger ? ` · 龙虎：${meta.dragonTiger}` : ''}</span>
      {accepting && currentIssue !== draw.issue && <p>下一期已开始受理。</p>}
      <DrawResultCards title={gameTitle} draw={draw} draws={draws} />
      <time className="game-message-time">{formatFeedTime(draw.draw_at)}</time>
    </article></div>
  </div>
}

function WinningPopup({ game, data, onClose }: { game: Game; data: WinningPopupData; onClose: () => void }) {
  return <div className="winning-popup-layer" role="presentation">
    <section className="winning-popup" role="dialog" aria-modal="true" aria-label="中奖提示">
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

function SettlementAssistantMessages({ gameId, gameTitle, notices, nickname }: { gameId: string; gameTitle: string; notices: MemberNotification[]; nickname: string }) {
  const visible = notices.filter((notice) => notice.game_id === gameId).slice(-8)
  if (!visible.length) return null
  return <div className="settlement-notice-list">{visible.map((notice) => {
    const numbers = notice.draw_numbers ?? []
    const details = notice.bet_details ?? []
    const won = (notice.won_count ?? 0) > 0
    return <div className={`admin-message draw-announcement personal-settlement ${won ? 'won' : 'lost'}`} key={notice.id}>
      <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
      <div><small>开奖助手 · 24小时在线</small><article>
        <span className="assistant-mention">@{nickname}</span>
        <strong>【{notice.game_name || gameTitle} - {shortIssue(notice.issue ?? '')}】结算完成</strong>
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

function BetKeyboard({ mode, odds, oddsHidden, oddsResponseReady, selectedCount, submitting, notice, onShortcut, onModeChange, onBackspace, onClear, onSelectNumber, onSelectOption, onConfirm, showModes }: { mode: BetMode; odds: PlayOdds; oddsHidden: boolean; oddsResponseReady: boolean; selectedCount: number; submitting?: boolean; notice?: string | null; onShortcut: (action: KeyboardShortcut) => void; onModeChange: (mode: BetMode) => void; onBackspace: () => void; onClear: () => void; onSelectNumber: (number: number) => void; onSelectOption: (option: string) => void; onConfirm: () => void; showModes: boolean }) {
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
  return <section className={`bet-keyboard ${showModes ? 'complex-bet-keyboard' : 'input-bet-keyboard'}`} onClick={(event) => event.stopPropagation()} onContextMenu={(event) => event.preventDefault()}>{showModes && <header><div className="bet-mode-tabs">{modes.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}</button>)}</div><span>已选 <b>{selectedCount}</b> 项</span>{selectedCount > 0 && <button className="clear-selection" onClick={onClear}>清空</button>}</header>}<nav className="keyboard-shortcuts" aria-label="快捷操作">{shortcuts.map((item) => <button className={`keyboard-shortcut ${item.id}`} key={item.id} type="button" onClick={() => onShortcut(item.id)}>{item.label}</button>)}</nav>{notice && <output className="keyboard-shortcut-notice" aria-live="polite">{notice}</output>}{activeMode === 'quick' ? <div>{quickKeys.map((key) => <button className={keyClass(key)} disabled={submitting && key === '确认'} key={key} onClick={() => key === '←' ? deleteOne() : selectQuick(key)} onPointerCancel={key === '←' ? endDelete : undefined} onPointerDown={key === '←' ? startDelete : undefined} onPointerLeave={key === '←' ? endDelete : undefined} onPointerUp={key === '←' ? endDelete : undefined}>{key === '确认' ? (submitting ? '提交中' : '确认投注') : key}</button>)}</div> : activeMode === 'dual' ? <div className="dual-board">{dualOptions.map((option) => { const value = oddsForSelection(option, odds); return <button disabled={!oddsResponseReady || (!oddsHidden && value === null)} key={option} onClick={() => onSelectOption(option)}><b>{option}</b><small>{oddsLabel(value, 3, oddsHidden)}</small></button> })}</div> : <div className="number-board">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button disabled={!oddsResponseReady || (!oddsHidden && oddsForPlayCode('ball_1_5', odds) === null)} key={number} onClick={() => onSelectNumber(number)}>{number}</button>)}</div>}</section>
}

type FullBetSelection = { label: string; play: string }

function previewFullBetSelections(draft: string): FullBetSelection[] {
  const segments = draft.replace(/^买/, '').split('#').map((part) => part.trim()).filter(Boolean)
  return segments.flatMap((segment) => {
    const parts = segment.split('/').map(part => part.trim()).filter(Boolean)
    const play = parts.length >= 3 ? parts.slice(0, -1).join('/') : parts[0] ?? ''
    if (!play) return []
    const positionedNumbers = play.match(/^(10|[1-9])\/(\d+)$/)
    if (positionedNumbers) return [{ label: `第${positionedNumbers[1]}名号码 · ${positionedNumbers[2].split('').map((number) => number === '0' ? '10' : number).join(' ')}`, play }]
    const positionedSide = play.match(/^(10|[1-9])\/([大小单双龙虎])$/)
    if (positionedSide) return [{ label: `第${positionedSide[1]}名 · ${positionedSide[2]}`, play }]
    if (/^\d+$/.test(play)) return [{ label: `冠军号码 · ${play.split('').map((number) => number === '0' ? '10' : number).join(' ')}`, play }]
    const matched = play.match(/冠亚和[大小单双]|冠军[大小单双龙虎]|亚军[大小单双龙虎]|第[三四五六七八九十]名[大小单双龙虎]/g)
    if (matched?.length) return matched.map((item) => ({ label: item, play: item }))
    return [{ label: play, play }]
  })
}

function FullBetBoard({ game, mode, submitting, odds, oddsHidden, oddsResponseReady, onModeChange, onConfirm, onClose }: { game: Game; mode: BetMode; submitting?: boolean; odds: PlayOdds; oddsHidden: boolean; oddsResponseReady: boolean; onModeChange: (mode: BetMode) => void; onConfirm: (content: string) => void; onClose: () => void }) {
  const [rank, setRank] = useState('冠军')
  const [amount, setAmount] = useState(20)
  const [selectionOpen, setSelectionOpen] = useState(false)
  // 详细面板使用独立结构化草稿，不能读写聊天室输入框。旧实现会把
  // `1/123/100` 打开面板后静默改成 `/20`，也会把聊天文字混入注单。
  const [selectionDraft, setSelectionDraft] = useState('')
  const ranks = ['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名']
  const modeItems: Array<{ id: BetMode; label: string; helper: string }> = [{ id: 'quick', label: '快捷', helper: '常用玩法' }, { id: 'dual', label: '两面盘', helper: '大小单双' }, { id: 'numbers', label: '号码', helper: '选择名次' }]
  const quickOptions = ['大', '小', '单', '双', '龙', '虎']
  const rankIndex = ranks.indexOf(rank)
  const rankQuickOptions = rankIndex >= 0 && rankIndex < 5 ? quickOptions : quickOptions.slice(0, 4)
  const selections = previewFullBetSelections(selectionDraft)
  const preparedContent = selections.map((selection) => `${selection.play}/${amount}`).join('#')
  const preparedBet = parseBetInput(preparedContent)
  const acceptance = gameAcceptance(game)
  const accepting = canAcceptBet(game)
  const configuredOdds = Object.values(odds).some((value) => typeof value === 'number')
  const selectionsHaveOdds = oddsResponseReady && selections.length > 0 && (oddsHidden || selections.every((selection) => oddsForSelection(selection.play, odds) !== null))
  const isPlaySelected = (play: string) => selections.some((selection) => selection.play === play)
  const currentPosition = rankIndex + 1
  const currentNumericSelection = selections.find((selection) => selection.play.match(new RegExp(`^${currentPosition}/\\d+$`)))
  const numericPlay = currentNumericSelection?.play.split('/')[1] ?? ''
  const numberToken = (number: number) => number === 10 ? '0' : String(number)
  const isNumberSelected = (number: number) => numericPlay.includes(numberToken(number))
  const togglePlay = (play: string) => {
    const next = isPlaySelected(play)
      ? selections.filter((selection) => selection.play !== play)
      : [...selections, { label: play, play }]
    setSelectionDraft(next.map((selection) => selection.play).join('#'))
  }
  const toggleNumber = (number: number) => {
    const token = numberToken(number)
    const otherPlays = selections.filter((selection) => selection.play !== currentNumericSelection?.play).map((selection) => selection.play)
    const nextNumbers = isNumberSelected(number) ? numericPlay.replace(token, '') : `${numericPlay}${token}`
    const nextPlay = nextNumbers ? `${currentPosition}/${nextNumbers}` : ''
    setSelectionDraft([...otherPlays, nextPlay].filter(Boolean).join('#'))
  }
  const removeSelection = (play: string) => setSelectionDraft(selections.filter((selection) => selection.play !== play).map((selection) => selection.play).join('#'))
  const optionButton = (play: string, label: string) => {
    const value = oddsForSelection(play, odds)
    return <button className={isPlaySelected(play) ? 'selected' : ''} disabled={!oddsResponseReady || (!oddsHidden && value === null)} key={play} onClick={() => togglePlay(play)}><b>{label}</b><small>{oddsLabel(value, 3, oddsHidden)}</small></button>
  }
  const ballOdds = oddsForPlayCode('ball_1_5', odds)
  return <div className="full-bet-layer" onClick={submitting ? undefined : onClose}><section className="full-bet-board" onClick={(event) => event.stopPropagation()}><header className="full-bet-header"><button aria-label="返回游戏聊天室" disabled={submitting} onClick={onClose}><Icon name="back" /></button><div><b>{game.title}</b><small>第 {shortIssue(game.period)} 期 · {acceptance.label}</small></div><button className="full-bet-close" aria-label="关闭投注面板" disabled={submitting} onClick={onClose}>×</button></header><div className="full-bet-current"><span>{game.timing.phaseLabel} {game.due}</span><i className={`full-bet-acceptance ${acceptance.tone}`}>{acceptance.label}</i><div>{game.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div></div><div className="full-bet-workspace"><aside>{modeItems.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}<small>{item.helper}</small></button>)}</aside><section className="full-bet-content"><header><div><b>{mode === 'quick' ? '快捷投注' : mode === 'dual' ? '两面盘' : '号码投注'}</b><small>选择后高亮；再次点击可取消。</small></div><span>{oddsHidden ? <b>赔率已隐藏</b> : <>赔率 <b>{configuredOdds ? '按玩法' : '待配置'}</b></>}</span></header>{mode === 'quick' && <><div className="rank-selector">{ranks.map((item) => <button className={rank === item ? 'active' : ''} key={item} onClick={() => setRank(item)}>{item}</button>)}</div><p className="board-section-title">{rank} · 选择玩法</p><div className="full-bet-options">{rankQuickOptions.map((item) => optionButton(`${rank}${item}`, item))}</div></>}{mode === 'dual' && <div className="full-bet-options">{dualOptions.map((item) => optionButton(item, item))}</div>}{mode === 'numbers' && <><div className="rank-selector">{ranks.map((item) => <button className={rank === item ? 'active' : ''} key={item} onClick={() => setRank(item)}>{item}</button>)}</div><p className="board-section-title">{rank} · 选择号码（末尾金额为每个号码的单注金额）</p><div className="full-bet-numbers">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button className={isNumberSelected(number) ? 'selected' : ''} disabled={!oddsResponseReady || (!oddsHidden && ballOdds === null)} key={number} onClick={() => toggleNumber(number)}><b>{number}</b><small>{oddsLabel(ballOdds, 3, oddsHidden)}</small></button>)}</div></>}</section></div><footer className="full-bet-footer"><div className="full-bet-summary"><button onClick={() => setSelectionDraft('')}>清空选择</button><button className="full-bet-selection-toggle" onClick={() => setSelectionOpen((open) => !open)}><span>已选 <b>{selections.length}</b> 组 · {preparedBet.payloads.length} 注</span><i>{selectionOpen ? '收起' : '查看清单'}</i></button>{selections.length > 0 && <button aria-label="删除最后一组选择" onClick={() => removeSelection(selections.at(-1)?.play ?? '')}>⌫</button>}</div>{selectionOpen && <div className="full-bet-selection-list"><header><b>本次投注清单</b><span>合计 ¥ {preparedBet.total.toFixed(2)}</span></header>{selections.length ? <div>{selections.map((selection, index) => { const selectionBet = parseBetInput(`${selection.play}/${amount}`); return <article key={`${selection.play}-${index}`}><div><b>{selection.label}</b><small>{selectionBet.payloads.map(payloadLabel).join('、')}</small></div><strong>¥ {selectionBet.total.toFixed(2)}</strong><button aria-label={`删除${selection.label}`} onClick={() => removeSelection(selection.play)}>×</button></article> })}</div> : <p>暂未选择玩法或号码</p>}</div>}<div className="amount-pills" aria-label="单注金额">{[20, 50, 100, 200].map((value) => <button className={amount === value ? 'active' : ''} key={value} onClick={() => setAmount(value)}>{value}</button>)}</div><button className="full-bet-confirm" disabled={submitting || !selections.length || !accepting || !selectionsHaveOdds} onClick={() => onConfirm(preparedContent)}>{submitting ? '提交中…' : !accepting ? acceptance.label : !selectionsHaveOdds && selections.length ? '赔率待配置' : '立即投注'} <small>¥ {preparedBet.total.toFixed(2)}</small></button></footer></section></div>
}

function OrdersDialog({ bets, onCancel, onClose }: { bets: MemberBet[]; onCancel: (id: number) => void; onClose: () => void }) {
  return <ActionDialog title="我的注单" description={bets.length ? `当前彩种最近 ${bets.length} 条个人注单` : '当前彩种暂无我的注单。'} onClose={onClose}>
    {bets.length > 0 && <div className="my-orders-list">{bets.map((bet) => <article key={bet.id}><header><b>{bet.play_name || bet.selection}</b><span className={`my-order-status ${bet.status}`}>{betStatusText(bet.status)}</span></header><p>第 {shortIssue(bet.issue)} 期 · 赔率 {oddsLabel(bet.odds, 3)}</p><footer><strong>¥ {bet.amount.toFixed(2)}</strong>{bet.status === 'pending' && <button onClick={() => onCancel(bet.id)}>撤单</button>}</footer></article>)}</div>}
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
  return <div className="game-menu-layer game-switch-layer" onClick={onClose}><aside className="game-switch-sheet" onClick={(event) => event.stopPropagation()}><header><b>⇄ 切换游戏</b><button onClick={onClose}>×</button></header>{games.map((item) => <button className={item.id === currentGame ? 'current' : ''} key={item.id} onClick={() => { onClose(); if (item.id !== currentGame) onSelect(item.id) }}><span className={item.logo ? 'has-image' : ''} style={{ background: item.logo ? 'transparent' : item.color }}>{item.logo ? <img alt={`${item.title} Logo`} src={item.logo} /> : item.tag.slice(0, 2)}</span><div><b>{item.title}</b><small>第 {item.period} 期</small></div><em>{item.id === currentGame ? '当前游戏' : `剩余 ${item.due}`}</em></button>)}</aside></div>
}
