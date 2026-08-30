import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { Game, Theme } from '../types'
import { ballTone } from '../data/games'
import { Avatar } from '../components/Avatar'
import { Icon } from '../components/Icon'
import { ActionDialog, RedPacketDialog } from '../components/Dialogs'
import { DrawResultCards } from '../components/DrawResultCards'
import { GameChatMessage } from '../components/GameChatMessage'
import { FullBetBoard } from '../components/FullBetBoard'
import { LotteryCountdown } from '../components/LotteryCountdown'
import { ScratchDrawDialog } from '../components/ScratchDrawDialog'
import { ScrollToLatestButton } from '../components/ScrollToLatestButton'
import { buildGameTimelineEntries, drawHistoryAtIssue, formatGameMessageTime as formatFeedTime, isRepeatableBetInput, isRoomCommandContent, keyboardShortcutInput, latestBetInput, ticketsForGame, type AcceptedTicket, type RoomKeyboardShortcut } from '../utils/gameRoomMessages'
import { roomBettingTarget } from '../utils/gameRoomBetting'
import { assistantReceiptLines } from '../utils/assistantReceipt'
import { formatBetAmount } from '../utils/betAmount'
import { playNotificationSound } from '../utils/notificationAudio'
import { CheckIn } from './CheckIn'
import { parseBetInput } from '../utils/betParser'
import { createRequestId } from '../utils/requestId'
import { betsApi, type AssistantDrawStatus, type MemberBet } from '../api/bets'
import { useGameDraws } from '../hooks/useGameDraws'
import { useGameTimelineWindow } from '../hooks/useGameTimelineWindow'
import type { DrawResult } from '../api/lottery'
import { useMemberPreferences } from '../hooks/useMemberPreferences'
import { WS_EVENT, type WsEvent, useWebSocketConnected } from '../hooks/useWebSocket'
import { portalApi, type GameFeedItem, type GameOdds } from '../api/portal'
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
type KeyboardShortcut = RoomKeyboardShortcut
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

function mergeGameFeed(previous: GameFeedItem[], incoming: GameFeedItem[]) {
  const rows = new Map<string, GameFeedItem>()
  for (const item of [...previous, ...incoming]) rows.set(`${item.issue}:${item.created_at}:${item.nickname}:${item.detail}:${item.amount}`, item)
  return [...rows.values()]
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
  const { timing } = roomBettingTarget(game)
  return {
    label: timing.statusLabel,
    tone: timing.accepting ? 'open' : timing.phase === 'pending' || timing.phase === 'unavailable' ? 'syncing' : 'closed',
  }
}

function canAcceptBet(game: Game) {
  return roomBettingTarget(game).timing.accepting
}

function supportsRankedBetBoard(game: Game) {
  return /赛车|飞艇|幸运10/i.test(`${game.category} ${game.title}`)
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
  const chatReadRef = useRef<{ key: string; afterID: number; queued: boolean; pending?: Promise<void> } | null>(null)
  const { draws, loading: drawsLoading, error: drawsError } = useGameDraws(game.id, drawHistoryLimit)
  const timelineWindow = useGameTimelineWindow(game.id, draws, drawsLoading, drawsError)
  const [oddsInfo, setOddsInfo] = useState<GameOdds | null>(null)
  const [assistantStatus, setAssistantStatus] = useState<AssistantDrawStatus | null>(null)
  const [gameMessages, setGameMessages] = useState<ChatMessage[]>([])
  const [sendingMessage, setSendingMessage] = useState(false)
  const [walletSummary, setWalletSummary] = useState<WalletSummary | null>(null)
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

  // 换期只刷新当期状态，不丢弃会员正在编辑的草稿或卸载选号面板。
  // 特别是开奖中已受理下一期时，官方切期仍是同一张编辑中的草稿。
  // 真正切换彩种才隔离输入和消息；实际提交继续绑定服务端明确的期号。
  useEffect(() => {
    const previousGameID = gameSessionRef.current.split(':')[0]
    const gameChanged = previousGameID !== game.id
    gameSessionRef.current = `${game.id}:${game.period}`
    setAssistantStatus(null)
    if (gameChanged) {
      setFeedItems([])
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
      setOddsInfo(null)
      setGameMessages([])
      setMessagesReady(false)
      setBetsReady(false)
      setAssistantHistoryReady(false)
      setFeedReady(false)
      setTimelinePositioned(false)
      setShowScrollLatest(false)
      nearBottomRef.current = true
      forceBottomRef.current = false
      lastSentContentRef.current = ''
      setSendingMessage(false)
    }
    // 开奖后服务端通常会立刻推进到下一期。中奖弹窗属于刚结算的上一期，
    // 不能跟着 period 会话重置一起清掉，否则会出现“一闪而过”。只有
    // 真正切换彩种时才关闭；留在当前彩种时由会员点击按钮主动收下。
    if (gameChanged) setWinningPopup(null)
  }, [game.id, game.period])

  const loadGameMessages = useCallback(async () => {
    if (!timelineWindow.ready) return
    const requestGameID = game.id
    const since = timelineWindow.startAt === undefined ? undefined : new Date(timelineWindow.startAt).toISOString()
    const key = `${requestGameID}:${since ?? ''}`
    if (chatReadRef.current?.key !== key) chatReadRef.current = { key, afterID: 0, queued: false }
    const read = chatReadRef.current
    if (read.pending) {
      read.queued = true
      return read.pending
    }
    read.pending = (async () => {
      try {
        do {
          read.queued = false
          const page = await chatApi.messages('group', requestGameID, 50, { since, after_id: read.afterID || undefined })
          if (chatReadRef.current !== read || !gameSessionRef.current.startsWith(`${requestGameID}:`)) return
          setGameMessages(current => mergeChatMessages(current, page.items))
          const afterID = Math.max(read.afterID, ...page.items.map(item => item.id))
          // Finish a burst/reconnect backlog instead of losing anything beyond
          // the latest page. A malformed non-advancing cursor cannot loop.
          if (afterID > read.afterID) {
            read.afterID = afterID
            if (page.has_more) read.queued = true
          }
        } while (read.queued)
      } catch {
        // Preserve already received messages and resume from the last page.
      } finally {
        read.pending = undefined
        if (chatReadRef.current === read && gameSessionRef.current.startsWith(`${requestGameID}:`)) setMessagesReady(true)
      }
    })()
    return read.pending
  }, [game.id, timelineWindow.ready, timelineWindow.startAt])

  useEffect(() => () => { chatReadRef.current = null }, [game.id])

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
          lines: assistantReceiptLines(item.lines),
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
      if (gameSessionRef.current === requestSession) setFeedItems(current => mergeGameFeed(current, feed.map(item => ({ ...item, issue: game.period }))))
    } catch {
      // A transient feed failure must not reorder or remove messages that are
      // already visible. Only a new game clears the old feed in the session
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
        const shouldFollow = nearBottomRef.current
        if (shouldFollow) forceBottomRef.current = true
        playNotificationSound('lottery')
        void Promise.all([
          loadBets(),
          loadGameFeed(),
          loadGameMessages(),
          loadAssistant(),
          onRefreshBalance(),
          loadWalletSummary(),
        ]).finally(() => { if (shouldFollow) followLatestAfterLayout() })
      }
      if (detail?.type === 'notification' && detail.data.category === 'winning' && eventGameID === game.id) {
        forceBottomRef.current = true
        void Promise.all([
          loadGameMessages(),
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
  }, [followLatestAfterLayout, game.id, loadAssistant, loadBets, loadGameFeed, loadGameMessages, loadWalletSummary, onRefreshBalance])

  const appendNumber = (number: number) => setBetInput((current) => `${current}${number}`)
  const appendOption = (option: string) => setBetInput((current) => `${current}${option}`)
  const clearSelection = () => setBetInput('')
  const removeNumber = () => setBetInput((current) => current.slice(0, -1))
  const handleKeyboardShortcut = (action: KeyboardShortcut) => {
    const previous = lastSentContentRef.current || latestBetInput(gameMessages, submittedBets, game.id)
    setBetInput(current => keyboardShortcutInput(action, current, previous))
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
    const targetIssue = roomBettingTarget(game).issue
    const requestKey = `${game.id}:${targetIssue}:${content}`
    if (pendingBetRequestRef.current?.key !== requestKey) {
      pendingBetRequestRef.current = { key: requestKey, id: `board-${createRequestId()}` }
    }
    const requestId = pendingBetRequestRef.current.id
    betSubmitLockRef.current = true
    setSubmitting(true)
    setBetError('')
    const requestSession = `${game.id}:${game.period}`
    try {
      // Bind the exact server-confirmed issue shown by the betting panel.
      // While the previous result is pending this can be the NEXT issue, but
      // the browser never guesses it or silently increments an old issue.
      const accepted = await betsApi.assistantPlace(game.id, { issue: targetIssue, content, request_id: requestId })
      pendingBetRequestRef.current = null
      // 真正切换彩种后不把旧彩种回执写进当前页面；仅仅期号推进时仍
      // 展示服务端返回的 accepted.issue，避免成功扣分却看不到回执。
      if (!gameSessionRef.current.startsWith(`${game.id}:`)) {
        await onRefreshBalance()
        await loadWalletSummary()
        return
      }
      forceBottomRef.current = true
      lastSentContentRef.current = content
      setSubmittedBets((bets) => mergeAcceptedTickets(bets, [{
        gameId: game.id,
        content: accepted.content,
        lines: assistantReceiptLines(accepted.lines),
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
    const targetIssue = roomBettingTarget(game).issue
    const requestKey = `${game.id}:${targetIssue}:${content}`
    if (command && pendingCommandRequestRef.current?.key !== requestKey) {
      pendingCommandRequestRef.current = { key: requestKey, id: `chat-command-${createRequestId()}` }
    }
    messageSubmitLockRef.current = true
    setSendingMessage(true)
    try {
      const message = command
        ? await chatApi.command(content, game.id, { issue: targetIssue, request_id: pendingCommandRequestRef.current!.id })
        : await chatApi.send(content, 'group', game.id)
      if (command) pendingCommandRequestRef.current = null
      if (gameSessionRef.current.startsWith(`${game.id}:`)) {
        forceBottomRef.current = true
        if (isRepeatableBetInput(content)) lastSentContentRef.current = content
        setGameMessages((current) => mergeChatMessages(current, [message]))
        setBetInput('')
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
  const bettingTarget = roomBettingTarget(game)
  const assistantIssue = assistantStatus?.betting_window?.issue ?? assistantStatus?.issue
  const assistantAcceptance = !canAcceptBet(game)
    ? `${acceptance.label}，当前暂停接单。`
    : assistantIssue === bettingTarget.issue && assistantStatus?.accepting === false
    ? '本期已封盘，请等待下一期开始受理。'
    : bettingTarget.issue !== game.period ? `下期 ${shortIssue(bettingTarget.issue)} 正在受理。` : '本期正在受理。'
  const visibleSubmittedBets = useMemo(() => submittedBets.filter((ticket) => {
    if (ticket.gameId !== game.id) return false
    const ticketBets = memberBets.filter((bet) => bet.issue === ticket.issue)
    // Hide an assistant receipt only when the persisted rows prove the whole
    // ticket was cancelled. A temporary bet-list failure must not hide a newly
    // accepted receipt that already came back from the placement endpoint.
    return ticketBets.length === 0 || ticketBets.some((bet) => bet.status !== 'cancelled')
  }), [game.id, memberBets, submittedBets])
  const timelineReady = timelineWindow.ready && messagesReady && betsReady && assistantHistoryReady && feedReady
  const timelineVersion = useMemo(() => [
    gameMessages.map((message) => `chat:${message.id}`).join(','),
    feedItems.map((item) => `feed:${item.created_at}:${item.nickname}:${item.detail}`).join(','),
    visibleSubmittedBets.map((ticket) => `ticket:${ticket.issue}:${ticket.acceptedAt}:${ticket.content}`).join(','),
    timelineWindow.draws.map(draw => `draw:${draw.game_id}:${draw.issue}:${draw.draw_at}`).join(','),
  ].join('|'), [timelineWindow.draws, feedItems, gameMessages, visibleSubmittedBets])

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
      {timelineReady && !timelineWindow.draws.length && <div className="admin-message assistant-notice">
        <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
        <div><small>开奖助手 · 24小时在线</small><article><b>【{game.title} - {shortIssue(game.period)}】</b><hr /><span>{assistantAcceptance}</span></article></div>
      </div>}
      <div className={`game-timeline ${timelineReady ? (timelinePositioned ? 'ready' : 'positioning') : 'loading'}`} ref={timelineRef}>
        {timelineReady
          ? <GameTimeline gameId={game.id} gameTitle={game.title} currentIssue={game.period} messages={gameMessages} draws={timelineWindow.draws} drawHistory={draws} startAt={timelineWindow.startAt} anchorIssue={timelineWindow.anchorIssue} feed={feedItems} tickets={visibleSubmittedBets} nickname={nickname} />
          : <div className="game-timeline-loading"><i /><span>{drawsError ? `${drawsError}，等待重新同步…` : '正在载入最新消息…'}</span></div>}
      </div>
    </section>
    {showScrollLatest && <ScrollToLatestButton keyboardOpen={showKeyboard} onScrollToLatest={scrollToLatest} />}
    {showKeyboard && roomFeatures.webKeyboard && <BetKeyboard mode={betMode} odds={playOdds} oddsHidden={oddsHidden} oddsResponseReady={oddsResponseReady} selectedCount={betInput.length} submitting={submitting || sendingMessage} onShortcut={handleKeyboardShortcut} onBackspace={removeNumber} onClear={clearSelection} onConfirm={() => void submitInput()} onModeChange={setBetMode} onSelectNumber={appendNumber} onSelectOption={appendOption} showModes={false} />}
    <QuickActions hasRedPacket={Boolean(roomRedPacket)} keyboardOpen={showKeyboard && roomFeatures.webKeyboard} onCheckIn={() => setShowCheckIn(true)} onCustomerService={onOpenService} onOpenRedPacket={openRoomRedPacket} onQuickBet={() => { setShowKeyboard(false); if (supportsRankedBetBoard(game)) setShowQuickBet(true); else { setBetError('该彩种暂未配置详细选号面板，请使用输入框发送开奖助手下单规则。'); setDialog('bet-error') } }} onSwitchGame={() => setShowGameSwitcher(true)} />
    <section className="ticket-strip" onClick={(event) => event.stopPropagation()}>{roomFeatures.webKeyboard && <button aria-label={showKeyboard ? '收起快捷键盘' : '打开快捷键盘'} className="ticket-ime" onClick={toggleKeyboard}><img alt="" src="/icons/lucide/keyboard.svg" /></button>}{roomFeatures.webKeyboard ? <button aria-label="打开投注键盘" className="ticket-selection" onClick={toggleKeyboard}>{bettingTarget.issue !== game.period && <small className="ticket-betting-issue">下期 {shortIssue(bettingTarget.issue)}</small>}{betInput || '输入玩法/金额或聊天内容'}</button> : <input aria-label="输入玩法、金额或聊天内容" className="ticket-selection ticket-native-input" autoComplete="off" disabled={submitting || sendingMessage} enterKeyHint="send" placeholder={bettingTarget.issue !== game.period ? `下期 ${shortIssue(bettingTarget.issue)} · 输入玩法/金额` : '输入玩法/金额或聊天内容'} value={betInput} onChange={(event) => setBetInput(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void submitInput() } }} />}{betInput ? <button aria-label="发送" className="ticket-add ticket-send" disabled={submitting || sendingMessage} onClick={() => void submitInput()}>{submitting || sendingMessage ? '…' : '发送'}</button> : <button aria-expanded={showAddMenu} aria-label="打开更多功能" className="ticket-add" onClick={() => { setShowKeyboard(false); setShowAddMenu((visible) => !visible) }}><Icon name="plus" /></button>}</section>
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
// The live acceptance status belongs in the header, never in old draw cards.
export const GameTimeline = memo(function GameTimeline({ gameId, gameTitle, currentIssue, messages, draws, drawHistory = draws, startAt, anchorIssue, feed, tickets, nickname }: { gameId: string; gameTitle: string; currentIssue: string; messages: ChatMessage[]; draws: DrawResult[]; drawHistory?: DrawResult[]; startAt?: number; anchorIssue?: string; feed: GameFeedItem[]; tickets: AcceptedTicket[]; nickname: string }) {
  const entries = useMemo(() => buildGameTimelineEntries({ gameId, messages, draws, feed, tickets, startAt, anchorIssue }), [draws, feed, gameId, messages, tickets, startAt, anchorIssue])

  return <div className="game-timeline-items">{entries.map((entry) => {
    if (entry.kind === 'chat') return <GameChatMessage key={entry.key} message={entry.value} nickname={nickname} />
    if (entry.kind === 'feed') return <GameFeedMessage key={entry.key} gameTitle={gameTitle} currentIssue={currentIssue} item={entry.value} index={entry.index} />
    if (entry.kind === 'ticket') return <SubmittedTicketMessage key={entry.key} gameTitle={gameTitle} ticket={entry.value} nickname={nickname} />
    return <DrawAssistantMessage key={entry.key} gameTitle={gameTitle} draw={entry.value} draws={drawHistory} />
  })}</div>
})

function QuickActions({ hasRedPacket, keyboardOpen, onSwitchGame, onCustomerService, onQuickBet, onCheckIn, onOpenRedPacket }: { hasRedPacket: boolean; keyboardOpen: boolean; onSwitchGame: () => void; onCustomerService: () => void; onQuickBet: () => void; onCheckIn: () => void; onOpenRedPacket: () => void }) {
  return <div className={`quick-actions${keyboardOpen ? ' keyboard-open' : ''}`}><button aria-label="切换游戏" onClick={onSwitchGame}>⇄</button><button aria-label="联系客服" onClick={onCustomerService}>🎧</button><button aria-label="快捷投注" onClick={onQuickBet}>☷</button><button aria-label="每日签到" className="quick-check-in" onClick={onCheckIn}><span>签</span></button>{hasRedPacket && <button aria-label="领取房间红包" className="quick-red-packet" onClick={onOpenRedPacket}><i aria-hidden="true" /><Icon name="gift" /><small>红包</small></button>}</div>
}

function GameFeedMessage({ gameTitle, currentIssue, item, index }: { gameTitle: string; currentIssue: string; item: GameFeedItem; index: number }) {
  return <article className="market-bet"><Avatar index={index} label={`${item.nickname}的头像`} /><div><small>{item.nickname}</small><p><b>【{gameTitle} · 第 {shortIssue(item.issue ?? currentIssue)} 期】</b><br />{item.detail} · {item.amount} 元<em>已受理</em><time className="game-message-time">{formatFeedTime(item.created_at)}</time></p></div></article>
}

function SubmittedTicketMessage({ gameTitle, ticket, nickname }: { gameTitle: string; ticket: AcceptedTicket; nickname: string }) {
  return <div className="submitted-ticket">
    <div className="player-bet"><div><small>{nickname}</small><article><span>{ticket.content}</span><time className="game-message-time mine">{formatFeedTime(ticket.acceptedAt)}</time></article></div><Avatar className="player-avatar" index={-1} label="我的头像" /></div>
    <div className="admin-message parsed-ticket"><span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span><div><small>开奖助手 · 24小时在线</small><article><span className="assistant-mention">@{nickname}</span><strong>【{gameTitle} - {shortIssue(ticket.issue)}】下单成功</strong><br />{ticket.lines.map((line) => <span className="parsed-line" key={line}>{line}</span>)}<footer><span>使用：{formatBetAmount(ticket.total)}</span><span>剩余：{formatHeaderAmount(ticket.balance)}</span></footer><time className="game-message-time">{formatFeedTime(ticket.acceptedAt)}</time></article></div></div>
  </div>
}

export function DrawAssistantMessage({ gameTitle, draw, draws }: { gameTitle: string; draw: DrawResult; draws: DrawResult[] }) {
  // Freeze the range at the moment this announcement enters the timeline.
  // Later draws append their own message instead of repainting old images.
  // A verified correction of this same result can still update its numbers.
  const [initialHistory] = useState(() => drawHistoryAtIssue(draws, draw))
  const history = useMemo(() => initialHistory.map(row => row.issue === draw.issue ? draw : row), [draw, initialHistory])
  const meta = crownMeta(draw.numbers)
  return <div className="admin-message draw-announcement">
    <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
    <div><small>开奖助手 · 24小时在线</small><article>
      <strong>【{gameTitle} - {shortIssue(draw.issue)}】已开奖</strong>
      <div className="draw-announcement-balls" style={{ gridTemplateColumns: `repeat(${draw.numbers.length}, minmax(0, 1fr))`, maxWidth: draw.numbers.length * 31 - 3 }}>{draw.numbers.map((number, index) => <b className={ballTone(number)} key={`${draw.id}-${index}`}>{number}</b>)}</div>
      <span className="draw-announcement-meta">冠亚和：{meta.crownResult}{meta.dragonTiger ? ` · 龙虎：${meta.dragonTiger}` : ''}</span>
      <DrawResultCards title={gameTitle} draw={draw} draws={history} />
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

export function BetKeyboard({ mode, odds, oddsHidden, oddsResponseReady, selectedCount, submitting, onShortcut, onModeChange, onBackspace, onClear, onSelectNumber, onSelectOption, onConfirm, showModes }: { mode: BetMode; odds: PlayOdds; oddsHidden: boolean; oddsResponseReady: boolean; selectedCount: number; submitting?: boolean; onShortcut: (action: KeyboardShortcut) => void; onModeChange: (mode: BetMode) => void; onBackspace: () => void; onClear: () => void; onSelectNumber: (number: number) => void; onSelectOption: (option: string) => void; onConfirm: () => void; showModes: boolean }) {
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
  return <section className={`bet-keyboard ${showModes ? 'complex-bet-keyboard' : 'input-bet-keyboard'}`} onClick={(event) => event.stopPropagation()} onContextMenu={(event) => event.preventDefault()}>{showModes && <header><div className="bet-mode-tabs">{modes.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}</button>)}</div><span>已选 <b>{selectedCount}</b> 项</span>{selectedCount > 0 && <button className="clear-selection" onClick={onClear}>清空</button>}</header>}<nav className="keyboard-shortcuts" aria-label="快捷操作">{shortcuts.map((item) => <button className={`keyboard-shortcut ${item.id}`} key={item.id} type="button" onClick={() => onShortcut(item.id)}>{item.label}</button>)}</nav>{activeMode === 'quick' ? <div>{quickKeys.map((key) => <button className={keyClass(key)} disabled={submitting && key === '确认'} key={key} onClick={() => key === '←' ? deleteOne() : selectQuick(key)} onPointerCancel={key === '←' ? endDelete : undefined} onPointerDown={key === '←' ? startDelete : undefined} onPointerLeave={key === '←' ? endDelete : undefined} onPointerUp={key === '←' ? endDelete : undefined}>{key === '确认' ? (submitting ? '提交中' : '确认投注') : key}</button>)}</div> : activeMode === 'dual' ? <div className="dual-board">{dualOptions.map((option) => { const value = oddsForSelection(option, odds); return <button disabled={!oddsResponseReady || (!oddsHidden && value === null)} key={option} onClick={() => onSelectOption(option)}><b>{option}</b><small>{oddsLabel(value, 3, oddsHidden)}</small></button> })}</div> : <div className="number-board">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button disabled={!oddsResponseReady || (!oddsHidden && oddsForPlayCode('ball_1_5', odds) === null)} key={number} onClick={() => onSelectNumber(number)}>{number}</button>)}</div>}</section>
}

function OrdersDialog({ bets, onCancel, onClose }: { bets: MemberBet[]; onCancel: (id: number) => void; onClose: () => void }) {
  return <ActionDialog title="我的注单" description={bets.length ? `当前彩种最近 ${bets.length} 条个人注单` : '当前彩种暂无我的注单。'} onClose={onClose}>
    {bets.length > 0 && <div className="my-orders-list">{bets.map((bet) => <article key={bet.id}><header><b>{bet.play_name || bet.selection}</b><span className={`my-order-status ${bet.status}`}>{betStatusText(bet.status)}</span></header><p>第 {shortIssue(bet.issue)} 期 · 赔率 {oddsLabel(bet.odds, 3)}</p><footer><strong>¥ {formatBetAmount(bet.amount)}</strong>{bet.status === 'pending' && <button onClick={() => onCancel(bet.id)}>撤单</button>}</footer></article>)}</div>}
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
