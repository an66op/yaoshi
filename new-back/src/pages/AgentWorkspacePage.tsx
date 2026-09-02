import {
  Alert, Avatar, Box, Button, Card, CardContent, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle,
  Divider, FormControlLabel, List, ListItemButton, ListItemText, MenuItem, Paper, Stack, Switch, Table, TableBody,
  TableCell, TableContainer, TableHead, TablePagination, TableRow, Tab, Tabs, TextField, Typography, Skeleton,
} from '@mui/material'
import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
import CardGiftcardRounded from '@mui/icons-material/CardGiftcardRounded'
import PhotoCameraRounded from '@mui/icons-material/PhotoCameraRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import ArrowUpwardRounded from '@mui/icons-material/ArrowUpwardRounded'
import CampaignRounded from '@mui/icons-material/CampaignRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import TuneRounded from '@mui/icons-material/TuneRounded'
import { agentApi, tenantApi, type AdminApplication, type AdminBet, type AdminChatConversation, type AdminChatMessage, type WorkspaceMember, type AgentDashboard, type ManagementWsEvent, type SystemSettings, type UserTradingConfig, type WorkspaceGame } from '../api'
import { useFeedback } from '../components/feedback'
import { OperatingReportPanel } from '../components/OperatingReportPanel'
import { GameOddsNavigation, OddsOverrideGrid } from '../components/OddsEditors'
import { AdminRedPacketCard, RedPacketForm, type RedPacketCover } from '../components/RedPacketForm'
import { UserPresenceChip } from '../components/UserPresenceChip'
import { gameLogo } from '../gameLogos'
import { MANAGEMENT_WS_EVENT, useManagementWebSocketConnected } from '../hooks/useManagementWebSocket'
import { prepareRoomLogo } from '../utils/roomLogo'
import { mergeAdminChatMessages, sameConversation, selectConversation } from '../utils/chatState'
import { createRequestId } from '../utils/requestId'
import { isBalanceApplication, reviewedBalance } from '../utils/workspaceReview'
import {
  CHAT_OPEN_CONVERSATION_EVENT, chatPageForTarget, consumePendingChatConversation, reportChatUnreadChanged,
  setActiveChatConversation, type ChatConversationTarget,
} from '../utils/chatNotifications'

const money = (value: number) => value.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const time = (value: string) => new Date(value).toLocaleString('zh-CN', { hour12: false })
const compactTime = (value?: string | null) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value)) : '—'
const identityGradient = 'linear-gradient(135deg,#0891b2 0%,#2563eb 54%,#7c3aed 100%)'
const memberAvatar = (userID: number, provided?: string) => provided?.trim() || `/images/avatars/avatar-${String(Math.abs(Math.trunc(userID || 0)) % 16).padStart(2, '0')}.png`
const issueStatusText: Record<string, string> = { accepting: '受理中', sealed: '已封盘', awaiting_draw: '待开奖', settling: '结算中', settled: '已结算', abnormal: '异常' }
const countdownText = (target: string | undefined, now: number) => {
  const targetTime = new Date(target || '').getTime()
  if (!Number.isFinite(targetTime)) return '--:--'
  const seconds = Math.max(0, Math.ceil((targetTime - now) / 1000))
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
}
const ballColor = (value: number) => ['#f97316', '#ef4444', '#0ea5e9', '#8b5cf6', '#16a34a'][Math.abs(value) % 5]
const roleText: Record<string, string> = { member: '会员', agent: '代理', tenant: '租户', admin: '管理员' }
const canManageMember = (member?: WorkspaceMember | null) => member?.in_current_room === true && member?.can_manage === true
// Treat membership flags as the boundary even if a stale response still contains private fields.
const publicMember = (member: WorkspaceMember): WorkspaceMember => ({
  id: member.id, public_id: member.public_id, username: member.username, nickname: member.nickname,
  avatar: member.avatar, public_title: member.public_title, badge: member.badge, role: 'member',
  in_current_room: member.in_current_room === true, can_manage: false,
  balance: null, status: null, online: null,
})
const safeMember = (member: WorkspaceMember) => canManageMember(member) ? member : publicMember(member)
const riskText: Record<string, string> = { normal: '正常', watch: '关注', restricted: '受限' }
const oddsMultiplierPresets = [.8, .9, 1, 1.1, 1.2]
type MemberActivitySummary = { betCount: number; pendingCount: number; recentStake: number; recentPayout: number; sampleSize: number }
type ApplicationCategory = 'wallet' | 'join' | 'entertainment'
export function AgentWorkspacePage({ section, tenantDirect = false }: {
  section: 'dashboard' | 'users' | 'applications' | 'room-reviews' | 'bets' | 'chat' | 'lottery-chat' | 'reports'
	tenantDirect?: boolean
}) {
  const chatSection = section === 'chat' || section === 'lottery-chat'
  const lotteryChat = section === 'lottery-chat'
  const websocketConnected = useManagementWebSocketConnected()
  const { showMessage } = useFeedback()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [dashboard, setDashboard] = useState<AgentDashboard | null>(null)
  const [users, setUsers] = useState<WorkspaceMember[]>([])
  const [applications, setApplications] = useState<AdminApplication[]>([])
  const [applicationCategory, setApplicationCategory] = useState<ApplicationCategory>(section === 'room-reviews' ? 'join' : 'wallet')
  const [bets, setBets] = useState<AdminBet[]>([])
  const [conversations, setConversations] = useState<AdminChatConversation[]>([])
  const [selected, setSelected] = useState<AdminChatConversation | null>(null)
  const [messages, setMessages] = useState<AdminChatMessage[]>([])
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [balanceUser, setBalanceUser] = useState<WorkspaceMember | null>(null)
  const [amount, setAmount] = useState('')
  const [remark, setRemark] = useState('')
  const [reviewing, setReviewing] = useState<AdminApplication | null>(null)
  const [reviewDecision, setReviewDecision] = useState<'approved' | 'rejected'>('approved')
  const [reviewReceivedAmount, setReviewReceivedAmount] = useState('')
  const [reviewOddsMultiplier, setReviewOddsMultiplier] = useState('1')
  const [reviewRemark, setReviewRemark] = useState('')
  const [reviewSaving, setReviewSaving] = useState(false)
  const [reply, setReply] = useState('')
  const [robotRunning, setRobotRunning] = useState(false)
  const [chatMode, setChatMode] = useState<'service' | 'room'>('service')
  const [transitioningChatMode, setTransitioningChatMode] = useState<'service' | 'room' | null>(null)
  const [redPacketOpen, setRedPacketOpen] = useState(false)
  const [redPacketCount, setRedPacketCount] = useState('10')
  const [redPacketTotal, setRedPacketTotal] = useState('100')
  const [redPacketGreeting, setRedPacketGreeting] = useState('恭喜发财')
  const [redPacketCover, setRedPacketCover] = useState<RedPacketCover>('classic')
  const redPacketRequestID = useRef(createRequestId())
  const [redPacketMinTurnover, setRedPacketMinTurnover] = useState('0')
  const [roomName, setRoomName] = useState('')
  const [roomLogo, setRoomLogo] = useState('')
  const [roomSaving, setRoomSaving] = useState(false)
  const [lotteryCategory, setLotteryCategory] = useState('')
  const [chatHasMore, setChatHasMore] = useState(false)
  const [chatNextBeforeID, setChatNextBeforeID] = useState<number | undefined>()
  const [chatLoading, setChatLoading] = useState(false)
  const [roomSettings, setRoomSettings] = useState<SystemSettings | null>(null)
  const [games, setGames] = useState<WorkspaceGame[]>([])
  const [now, setNow] = useState(() => Date.now())
  const [attentionRevision, setAttentionRevision] = useState(0)
  const [memberOpen, setMemberOpen] = useState(false)
  const [memberInfo, setMemberInfo] = useState<WorkspaceMember | null>(null)
  const [memberActivity, setMemberActivity] = useState<MemberActivitySummary | null>(null)
  const [memberLoading, setMemberLoading] = useState(false)
  const [memberError, setMemberError] = useState('')
  const [tradingUser, setTradingUser] = useState<WorkspaceMember | null>(null)
  const [userTrading, setUserTrading] = useState<UserTradingConfig | null>(null)
  const [userTradingLoading, setUserTradingLoading] = useState(false)
  const [userTradingSaving, setUserTradingSaving] = useState(false)
  const [userTradingDirty, setUserTradingDirty] = useState(false)
  const roomApi = useMemo(() => ({
    dashboard: () => tenantDirect ? tenantApi.roomDashboard() : agentApi.dashboard(),
    users: (params?: Parameters<typeof agentApi.users>[0]) => tenantDirect ? tenantApi.users(params) : agentApi.users(params),
    setUserStatus: (id: number, status: 0 | 1) => tenantDirect ? tenantApi.setUserStatus(id, status) : agentApi.setUserStatus(id, status),
    adjustUserBalance: (id: number, value: number, note: string) => tenantDirect ? tenantApi.adjustUserBalance(id, value, note) : agentApi.adjustUserBalance(id, value, note),
    userTrading: (id: number, gameId?: string) => tenantDirect ? tenantApi.userTrading(id, gameId) : agentApi.userTrading(id, gameId),
    updateUserTrading: (id: number, payload: Parameters<typeof agentApi.updateUserTrading>[1]) => tenantDirect ? tenantApi.updateUserTrading(id, payload) : agentApi.updateUserTrading(id, payload),
    bets: (params?: Parameters<typeof agentApi.bets>[0]) => tenantDirect ? tenantApi.bets(params) : agentApi.bets(params),
    applications: (params?: Parameters<typeof agentApi.applications>[0]) => tenantDirect ? tenantApi.applications(params) : agentApi.applications(params),
    reviewApplication: (id: number, payload: Parameters<typeof agentApi.reviewApplication>[1]) => tenantDirect ? tenantApi.reviewApplication(id, payload) : agentApi.reviewApplication(id, payload),
    chatConversations: (params: Parameters<typeof agentApi.chatConversations>[0]) => tenantDirect ? tenantApi.chatConversations(params) : agentApi.chatConversations(params),
    chatMessages: (params: Parameters<typeof agentApi.chatMessages>[0]) => tenantDirect ? tenantApi.chatMessages(params) : agentApi.chatMessages(params),
    markChatRead: (payload: Parameters<typeof agentApi.markChatRead>[0]) => tenantDirect ? tenantApi.markChatRead(payload) : agentApi.markChatRead(payload),
    replyChat: (payload: Parameters<typeof agentApi.replyChat>[0]) => tenantDirect ? tenantApi.replyChat(payload) : agentApi.replyChat(payload),
    sendChatRedPacket: (payload: Parameters<typeof agentApi.sendChatRedPacket>[0]) => tenantDirect ? tenantApi.sendChatRedPacket(payload) : agentApi.sendChatRedPacket(payload),
    settings: () => tenantDirect ? tenantApi.settings() : agentApi.settings(),
    updateSettings: (payload: SystemSettings) => tenantDirect ? tenantApi.updateSettings(payload) : agentApi.updateSettings(payload),
    games: () => tenantDirect ? tenantApi.games() : agentApi.games(),
    runRobotOnce: () => tenantDirect ? tenantApi.runRobotOnce() : agentApi.runRobotOnce(),
  }), [tenantDirect])
  const selectedRef = useRef<AdminChatConversation | null>(null)
  const pendingTargetRef = useRef<ChatConversationTarget | null>(null)
  const markedThroughRef = useRef(new Map<string, number>())
  const loadedConversationRef = useRef<AdminChatConversation | null>(null)
  const connectionStatusInitialized = useRef(false)
  const loadRequestRef = useRef(0)
  const chatMessageRequestRef = useRef(0)
  const knownMembers = useRef(new Map<number, WorkspaceMember>())
  const memberVersions = useRef(new Map<number, number>())
  const membershipSequence = useRef(0)
  const memberRequestRef = useRef(0)
  const memberIDRef = useRef<number | null>(null)
  const tradingRequestRef = useRef(0)
  const tradingIDRef = useRef<number | null>(null)
  const balanceIDRef = useRef<number | null>(null)
  const pendingMemberActions = useRef(new Set<string>())
  const alive = useRef(true)
  const activeRoomApi = useRef(roomApi)
  const [balanceSaving, setBalanceSaving] = useState(false)
  useEffect(() => {
    alive.current = true
    activeRoomApi.current = roomApi
    return () => {
      alive.current = false
      loadRequestRef.current += 1
      memberRequestRef.current += 1
      tradingRequestRef.current += 1
      tradingIDRef.current = null
      balanceIDRef.current = null
    }
  }, [roomApi])
  const acceptMember = useCallback((member: WorkspaceMember, version: number) => {
    if ((memberVersions.current.get(member.id) ?? 0) > version) return knownMembers.current.get(member.id) ?? publicMember(member)
    const next = safeMember(member)
    memberVersions.current.set(member.id, version)
    knownMembers.current.set(member.id, next)
    if (!canManageMember(next)) {
      if (balanceIDRef.current === next.id) { balanceIDRef.current = null; setBalanceUser(null); setAmount(''); setRemark('') }
      if (tradingIDRef.current === next.id) {
        tradingRequestRef.current += 1
        tradingIDRef.current = null
        setTradingUser(null); setUserTrading(null); setUserTradingDirty(false); setUserTradingLoading(false)
      }
      if (memberIDRef.current === next.id) { setMemberInfo(next); setMemberActivity(null) }
    }
    return next
  }, [])
  const refreshMember = async (target: Pick<WorkspaceMember, 'id'>) => {
    const version = ++membershipSequence.current
    const result = await roomApi.users({ userId: target.id, page: 1, pageSize: 1 })
    if (!alive.current || activeRoomApi.current !== roomApi) return null
    const found = (Array.isArray(result?.items) ? result.items : []).find(row => row.id === target.id)
    if (!found) {
      const previous = knownMembers.current.get(target.id)
      if (previous) {
        const denied = acceptMember({ ...publicMember(previous), in_current_room: false }, version)
        setUsers(rows => rows.map(row => row.id === target.id ? denied : row))
      }
      return null
    }
    const next = acceptMember(found, version)
    setUsers(rows => rows.map(row => row.id === next.id ? next : row))
    return next
  }
  const requireCurrentMember = async (target: WorkspaceMember) => {
    if (!canManageMember(knownMembers.current.get(target.id) ?? target)) return null
    const current = await refreshMember(target)
    if (!canManageMember(current)) {
      if (alive.current) showMessage('该会员已不在本房间或无管理权限，请刷新会员列表', 'warning')
      return null
    }
    return current
  }
  const closeTrading = () => {
    if (userTradingSaving) return
    tradingRequestRef.current += 1
    tradingIDRef.current = null
    setTradingUser(null); setUserTrading(null); setUserTradingDirty(false); setUserTradingLoading(false)
  }
  const closeBalance = () => {
    if (balanceSaving) return
    balanceIDRef.current = null
    setBalanceUser(null); setAmount(''); setRemark('')
  }
  const closeMember = () => {
    memberRequestRef.current += 1
    memberIDRef.current = null
    setMemberOpen(false); setMemberInfo(null); setMemberActivity(null); setMemberLoading(false)
  }
  useEffect(() => { selectedRef.current = selected }, [selected])
  const focusConversation = useCallback((target: ChatConversationTarget) => {
    if (!chatSection || chatPageForTarget(target) !== (lotteryChat ? '/lottery-chat' : '/chat')) return
    pendingTargetRef.current = target
    setChatMode(target.room_type === 'service' ? 'service' : 'room')
    setQuery('')
    setPage(0)
  }, [chatSection, lotteryChat])
  useEffect(() => {
    if (!chatSection) return
    const pending = consumePendingChatConversation()
    const initial = pending ? window.setTimeout(() => focusConversation(pending), 0) : 0
    const onOpen = (event: Event) => {
      const stored = consumePendingChatConversation()
      focusConversation(stored ?? (event as CustomEvent<ChatConversationTarget>).detail)
    }
    window.addEventListener(CHAT_OPEN_CONVERSATION_EVENT, onOpen)
    return () => { window.clearTimeout(initial); window.removeEventListener(CHAT_OPEN_CONVERSATION_EVENT, onOpen) }
  }, [chatSection, focusConversation])
  useEffect(() => {
    setActiveChatConversation(chatSection ? selected : null)
  }, [chatSection, selected])
  useEffect(() => () => setActiveChatConversation(null), [])

  const load = useCallback(async (blocking = true) => {
    const requestID = ++loadRequestRef.current
    if (blocking) setLoading(true)
    setError('')
    try {
      const [head, settings, roomGames] = await Promise.all([
        roomApi.dashboard(),
        roomApi.settings(),
        chatSection || section === 'users' ? roomApi.games() : Promise.resolve<WorkspaceGame[]>([]),
      ])
      if (requestID !== loadRequestRef.current) return
      // System settings are backed by the workspace record and are the public
      // room identity. Do not fall back to the legacy account room_name/logo.
      const authoritativeHead = { ...head, room_name: settings.room_name, room_logo: settings.room_logo }
      setDashboard(authoritativeHead)
      setRoomSettings(settings)
      setRoomName(settings.room_name)
      setRoomLogo(settings.room_logo)
      if (chatSection || section === 'users') setGames(Array.isArray(roomGames) ? roomGames : [])
      if (section === 'users') {
        const version = ++membershipSequence.current
        const result = await roomApi.users({ query, page: page + 1, pageSize })
        if (requestID !== loadRequestRef.current) return
        setUsers((Array.isArray(result?.items) ? result.items : []).map(row => acceptMember(row, version))); setTotal(Number(result?.total) || 0)
      }
      if (section === 'applications' || section === 'room-reviews') {
        const result = await roomApi.applications({ query, type: applicationCategory, page: page + 1, pageSize })
        if (requestID !== loadRequestRef.current) return
        setApplications(Array.isArray(result?.items) ? result.items : []); setTotal(Number(result?.total) || 0)
      }
      if (section === 'bets') {
        const result = await roomApi.bets({ query, page: page + 1, pageSize })
        if (requestID !== loadRequestRef.current) return
        setBets(Array.isArray(result?.items) ? result.items : []); setTotal(Number(result?.total) || 0)
      }
      if (chatSection) {
        const channel = lotteryChat ? 'lottery' : chatMode
        const result = await roomApi.chatConversations({ query, channel, roomType: channel === 'service' ? 'service' : 'group' })
        if (requestID !== loadRequestRef.current) return
        const rows = Array.isArray(result?.items) ? result.items : []
        const current = selectedRef.current
        const pending = pendingTargetRef.current
        const targetMatch = pending ? rows.find(row => sameConversation(pending as AdminChatConversation, row)) : undefined
        const currentMatch = current ? rows.find(row => sameConversation(current, row)) : undefined
        const nextSelected = targetMatch ?? (currentMatch && current ? current : rows[0] ?? null)
        if (targetMatch) pendingTargetRef.current = null
        setConversations(rows)
        if (!nextSelected || !sameConversation(current, nextSelected)) {
          chatMessageRequestRef.current += 1
          setMessages([])
          setChatHasMore(false)
          setChatNextBeforeID(undefined)
          setChatLoading(Boolean(nextSelected))
        }
        setSelected(nextSelected)
      }
    } catch (reason) {
      if (requestID === loadRequestRef.current) {
        setError(reason instanceof Error ? reason.message : '读取房间数据失败')
        if (chatSection && blocking) {
          // Do not expose an old conversation beneath a newly selected tab.
          chatMessageRequestRef.current += 1
          setConversations([])
          setSelected(null)
          setMessages([])
          setChatHasMore(false)
          setChatNextBeforeID(undefined)
          setChatLoading(false)
        }
      }
    }
    finally {
      if (requestID === loadRequestRef.current) {
        setLoading(false)
        setTransitioningChatMode(null)
      }
    }
  }, [acceptMember, applicationCategory, chatMode, chatSection, lotteryChat, page, pageSize, query, roomApi, section])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])
  useEffect(() => {
    if (section !== 'users') return
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load(false)
    }, 25_000)
    return () => window.clearInterval(timer)
  }, [load, section])
  useEffect(() => {
    if (!lotteryChat) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [lotteryChat])
  const loadChatMessages = useCallback(async (conversation: AdminChatConversation, mode: 'initial' | 'latest' | 'older' = 'initial', beforeId?: number) => {
    const requestID = ++chatMessageRequestRef.current
    setChatLoading(true)
    try {
      const result = await roomApi.chatMessages({
        scope: conversation.scope,
        roomScope: conversation.room_scope,
        gameId: conversation.game_id,
        roomType: conversation.room_type,
        beforeId: mode === 'older' ? beforeId : undefined,
        limit: 50,
      })
      if (requestID !== chatMessageRequestRef.current || !sameConversation(selectedRef.current, conversation)) return
      const items = Array.isArray(result?.items) ? result.items : []
      setMessages(current => mode === 'initial' ? mergeAdminChatMessages(items) : mergeAdminChatMessages(current, items))
      if (mode !== 'latest') {
        setChatHasMore(Boolean(result?.has_more))
        setChatNextBeforeID(result?.next_before_id)
      }
      setError('')
    } catch (reason) { if (requestID === chatMessageRequestRef.current) setError(reason instanceof Error ? reason.message : '聊天记录暂时未加载') }
    finally { if (requestID === chatMessageRequestRef.current) setChatLoading(false) }
  }, [roomApi])

  useEffect(() => {
    if (!selected) {
      loadedConversationRef.current = null
      return
    }
    const switched = !sameConversation(loadedConversationRef.current, selected)
    loadedConversationRef.current = selected
    const initial = window.setTimeout(() => {
      if (switched) setMessages([])
      setChatHasMore(false)
      setChatNextBeforeID(undefined)
      void loadChatMessages(selected, 'initial')
    }, 0)
    return () => window.clearTimeout(initial)
  }, [loadChatMessages, selected])

  useEffect(() => {
    if (!chatSection || lotteryChat || selected?.room_type !== 'service' || document.visibilityState !== 'visible' || !document.hasFocus()) return
    const throughMessageID = messages.reduce((latest, item) => item.scope === selected.scope
      && item.room_scope === selected.room_scope && item.game_id === selected.game_id && item.room_type === selected.room_type
      ? Math.max(latest, item.id) : latest, 0)
    if (!throughMessageID) return
    const key = `${selected.scope}\u0000${selected.room_scope}\u0000${selected.game_id}\u0000${selected.room_type}`
    if (throughMessageID <= (markedThroughRef.current.get(key) ?? 0)) return
    markedThroughRef.current.set(key, throughMessageID)
    void roomApi.markChatRead({ scope: selected.scope, room_scope: selected.room_scope, game_id: selected.game_id, through_message_id: throughMessageID })
      .then(reportChatUnreadChanged)
      .catch(() => markedThroughRef.current.delete(key))
  }, [attentionRevision, chatSection, lotteryChat, messages, roomApi, selected])

  useEffect(() => {
    if (!chatSection) return
    const onRealtime = (event: Event) => {
      const detail = (event as CustomEvent<ManagementWsEvent>).detail
      if (detail?.type !== 'chat_message') return
      const current = selectedRef.current
      const data = detail.data ?? {}
      if (current
        && data.scope === current.scope
        && data.room_scope === current.room_scope
        && data.game_id === current.game_id
        && data.room_type === current.room_type) {
        void loadChatMessages(current, 'latest')
      }
      void load(false)
    }
    window.addEventListener(MANAGEMENT_WS_EVENT, onRealtime)
    return () => window.removeEventListener(MANAGEMENT_WS_EVENT, onRealtime)
  }, [chatSection, load, loadChatMessages])

  useEffect(() => {
    if (!chatSection) return
    const current = selectedRef.current
    if (connectionStatusInitialized.current && current) {
      void loadChatMessages(current, 'latest')
      void load(false)
    }
    connectionStatusInitialized.current = true
    if (websocketConnected) return
    const timer = window.setInterval(() => {
      const active = selectedRef.current
      if (active) void loadChatMessages(active, 'latest')
      void load(false)
    }, 15_000)
    return () => window.clearInterval(timer)
  }, [chatSection, load, loadChatMessages, websocketConnected])

  useEffect(() => {
    if (!chatSection || lotteryChat) return
    const onVisibility = () => {
      const current = selectedRef.current
      if (document.visibilityState === 'visible' && document.hasFocus() && current?.room_type === 'service') {
        setAttentionRevision(value => value + 1)
        void loadChatMessages(current, 'latest')
      }
    }
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('focus', onVisibility)
    return () => { document.removeEventListener('visibilitychange', onVisibility); window.removeEventListener('focus', onVisibility) }
  }, [chatSection, loadChatMessages, lotteryChat])

  const cards = useMemo(() => dashboard ? [
    ['在房启用 / 已加入', `${dashboard.active_member_count} / ${dashboard.member_count}`], ['成员余额', `¥ ${money(dashboard.member_balance)}`],
    ['今日投注', `¥ ${money(dashboard.today_stake)}`], ['今日派彩', `¥ ${money(dashboard.today_payout)}`],
    ['今日净额', `¥ ${money(dashboard.today_net)}`], ['待处理', `${dashboard.pending_applications} 申请 · ${dashboard.pending_bets} 注单`],
  ] : [], [dashboard])
  const lotteryCategories = useMemo(() => Array.from(new Set(conversations.map(item => item.lobby_category?.trim()).filter((item): item is string => Boolean(item)))), [conversations])
  const activeLotteryCategory = lotteryCategories.includes(lotteryCategory) ? lotteryCategory : (lotteryCategories[0] ?? '')
  const visibleConversations = useMemo(() => lotteryChat && activeLotteryCategory
    ? conversations.filter(item => item.lobby_category?.trim() === activeLotteryCategory)
    : conversations, [activeLotteryCategory, conversations, lotteryChat])
  const selectedGame = useMemo(() => games.find(game => game.id === selected?.game_id), [games, selected?.game_id])
  const roomDisplayName = roomSettings?.room_name.trim() || '当前房间'
  const roomDisplayLogo = roomSettings?.room_logo.trim() || ''
  const staffTitle = roomSettings?.chat_nickname.trim() || (lotteryChat ? '开奖员' : '客服')
  const roomAnnouncement = useMemo(() => {
    const enabled = (roomSettings?.announcements ?? [])
      .filter(item => item.enabled && item.content.trim())
      .sort((left, right) => left.sort_order - right.sort_order)[0]
    if (enabled) return { title: enabled.title.trim() || '房间公告', content: enabled.content.trim() }
    const content = roomSettings?.room_notice.trim()
    return content ? { title: '房间公告', content } : null
  }, [roomSettings])

  const messageIdentity = (message: AdminChatMessage) => {
    if (!message.is_staff) return {
      name: message.nickname || message.username || '会员',
      badge: message.badge?.trim() || message.title?.trim() || '会员',
      avatar: memberAvatar(message.user_id, message.avatar),
    }
    const drawAssistant = message.username === 'draw_assistant'
    return {
      name: drawAssistant ? (message.nickname || '开奖助手') : staffTitle,
      badge: drawAssistant ? '开奖助手' : lotteryChat ? '房间开奖员' : '房间客服',
      avatar: roomDisplayLogo || undefined,
    }
  }

  const openMember = async (message: AdminChatMessage) => {
    if (!message.user_id || message.is_staff) return
    const requestID = ++memberRequestRef.current
    memberIDRef.current = message.user_id
    setMemberOpen(true); setMemberLoading(true); setMemberError(''); setMemberInfo(null); setMemberActivity(null)
    try {
      const profile = await refreshMember({ id: message.user_id })
      if (requestID !== memberRequestRef.current || !alive.current) return
      if (!profile) throw new Error('未找到该会员的本房间加入记录')
      setMemberInfo(profile)
      if (!canManageMember(profile)) return
      const [allBets, pendingBets] = await Promise.all([
        roomApi.bets({ userId: message.user_id, page: 1, pageSize: 100 }),
        roomApi.bets({ userId: message.user_id, status: 'pending', page: 1, pageSize: 1 }),
      ])
      if (requestID !== memberRequestRef.current || !alive.current) return
      // Membership may have changed while private activity was loading.
      const latest = await refreshMember(profile)
      if (requestID !== memberRequestRef.current || !alive.current) return
      if (!latest) { setMemberInfo(publicMember({ ...profile, in_current_room: false })); return }
      setMemberInfo(latest)
      if (!canManageMember(latest)) return
      const recent = Array.isArray(allBets?.items) ? allBets.items : []
      setMemberActivity({
        betCount: Number(allBets?.total) || 0, pendingCount: Number(pendingBets?.total) || 0,
        recentStake: recent.reduce((sum, item) => sum + Number(item.amount || 0), 0),
        recentPayout: recent.reduce((sum, item) => sum + Number(item.payout || 0), 0), sampleSize: recent.length,
      })
    } catch (reason) {
      if (requestID === memberRequestRef.current && alive.current) {
        setMemberInfo(null); setMemberActivity(null)
        setMemberError(reason instanceof Error ? reason.message : '读取会员资料失败')
      }
    } finally {
      if (requestID === memberRequestRef.current && alive.current) setMemberLoading(false)
    }
  }

  const changeMemberStatus = async (target: WorkspaceMember) => {
    const key = `status:${target.id}`
    if (!canManageMember(target) || pendingMemberActions.current.has(key)) return
    pendingMemberActions.current.add(key)
    try {
      const current = await requireCurrentMember(target)
      if (!current || current.status === null) return
      await roomApi.setUserStatus(current.id, current.status === 1 ? 0 : 1)
      if (alive.current) await load()
    } catch (reason) { if (alive.current) showMessage(reason instanceof Error ? reason.message : '修改会员状态失败', 'error') }
    finally { pendingMemberActions.current.delete(key) }
  }
  const openBalance = async (target: WorkspaceMember) => {
    if (!canManageMember(target) || !canManageMember(knownMembers.current.get(target.id) ?? target) || balanceSaving) return
    balanceIDRef.current = target.id
    try {
      const current = await requireCurrentMember(target)
      if (current && balanceIDRef.current === target.id) { setBalanceUser(current); setAmount(''); setRemark('') }
    } catch (reason) { if (alive.current) showMessage(reason instanceof Error ? reason.message : '读取会员权限失败', 'error') }
  }
  const adjustBalance = async () => {
    const target = balanceUser
    const key = `balance:${target?.id}`
    if (!target || !canManageMember(target) || balanceIDRef.current !== target.id || !Number.isFinite(Number(amount)) || !Number(amount) || !remark.trim() || pendingMemberActions.current.has(key)) return
    pendingMemberActions.current.add(key)
    setBalanceSaving(true)
    try {
      const current = await requireCurrentMember(target)
      if (!current || balanceIDRef.current !== target.id) return
      await roomApi.adjustUserBalance(current.id, Number(amount), remark.trim())
      if (!alive.current) return
      showMessage('余额调整成功'); balanceIDRef.current = null; setBalanceUser(null); setAmount(''); setRemark(''); await load()
    } catch (reason) { if (alive.current) showMessage(reason instanceof Error ? reason.message : '余额调整失败', 'error') }
    finally { pendingMemberActions.current.delete(key); if (alive.current) setBalanceSaving(false) }
  }
  const saveRoomProfile = async () => {
    const next = roomName.trim()
    if (next.length < 2 || next.length > 30) {
      showMessage('房间名称长度需为 2–30 个字符', 'error')
      return
    }
    setRoomSaving(true)
    try {
      if (!roomSettings) throw new Error('房间配置尚未加载')
      const settings = await roomApi.updateSettings({ ...roomSettings, room_name: next, room_logo: roomLogo })
      setRoomSettings(settings)
      setDashboard(current => current ? { ...current, room_name: settings.room_name, room_logo: settings.room_logo } : current)
      setRoomName(settings.room_name)
      setRoomLogo(settings.room_logo ?? '')
      showMessage('房间名称和 Logo 已保存')
    } catch (reason) {
      showMessage(reason instanceof Error ? reason.message : '保存房间资料失败', 'error')
    } finally {
      setRoomSaving(false)
    }
  }
  const chooseRoomLogo = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    try { setRoomLogo(await prepareRoomLogo(file)) }
    catch (reason) { showMessage(reason instanceof Error ? reason.message : '处理房间 Logo 失败', 'error') }
  }
  const openReview = (application: AdminApplication) => {
    setReviewing(application)
    setReviewDecision('approved')
    setReviewReceivedAmount(application.requested_amount > 0 ? String(application.requested_amount) : '')
    setReviewOddsMultiplier(String(application.odds_multiplier || 1))
    setReviewRemark('')
  }
  const review = async () => {
    if (!reviewing) return
    const balanceApplication = isBalanceApplication(reviewing)
    const receivedAmount = reviewDecision === 'approved' && balanceApplication ? Number(reviewReceivedAmount) : 0
    if (reviewDecision === 'approved' && balanceApplication && (!Number.isFinite(receivedAmount) || receivedAmount <= 0)) {
      showMessage('请输入大于 0 的实际到账或出款金额', 'error')
      return
    }
    if (reviewDecision === 'rejected' && !reviewRemark.trim()) {
      showMessage('拒绝申请时请填写审核原因', 'error')
      return
    }
    const oddsMultiplier = Number(reviewOddsMultiplier)
    if (reviewDecision === 'approved' && reviewing.request_type === 'join' && (!Number.isFinite(oddsMultiplier) || oddsMultiplier < .5 || oddsMultiplier > 1.5)) {
      showMessage('会员赔率倍率需在 0.50–1.50 之间', 'error')
      return
    }
    setReviewSaving(true)
    try {
      await roomApi.reviewApplication(reviewing.id, {
        decision: reviewDecision,
        received_amount: receivedAmount,
        odds_multiplier: reviewDecision === 'approved' && reviewing.request_type === 'join' ? oddsMultiplier : undefined,
        remark: reviewRemark.trim(),
      })
      setReviewing(null)
      showMessage(reviewing.request_type === 'join' ? reviewDecision === 'approved' ? '入房申请已通过' : '入房申请已拒绝' : reviewDecision === 'approved' ? '申请已通过并完成账户处理' : '申请已拒绝')
      await load()
    }
    catch (reason) { showMessage(reason instanceof Error ? reason.message : '审核失败', 'error') }
    finally { setReviewSaving(false) }
  }
  const loadUserTrading = async (target: WorkspaceMember, gameId?: string) => {
    if (!canManageMember(target) || !canManageMember(knownMembers.current.get(target.id) ?? target) || userTradingSaving) return
    const requestID = ++tradingRequestRef.current
    tradingIDRef.current = target.id
    setTradingUser(target)
    setUserTrading(null)
    setUserTradingDirty(false)
    setUserTradingLoading(true)
    try {
      const current = await requireCurrentMember(target)
      if (!current || requestID !== tradingRequestRef.current || tradingIDRef.current !== target.id) return
      const next = await roomApi.userTrading(target.id, gameId)
      if (requestID !== tradingRequestRef.current || !alive.current) return
      const latest = await refreshMember(current)
      if (!canManageMember(latest) || requestID !== tradingRequestRef.current || tradingIDRef.current !== target.id) return
      setTradingUser(latest)
      setUserTrading(next)
      setUserTradingDirty(false)
    } catch (reason) {
      if (requestID !== tradingRequestRef.current || !alive.current) return
      showMessage(reason instanceof Error ? reason.message : '读取会员赔率失败', 'error')
      tradingIDRef.current = null
      setTradingUser(null)
      setUserTrading(null)
    } finally {
      if (requestID === tradingRequestRef.current && alive.current) setUserTradingLoading(false)
    }
  }
  const saveUserTrading = async () => {
    if (!tradingUser || !canManageMember(tradingUser) || tradingIDRef.current !== tradingUser.id || !userTrading) return
    const target = tradingUser
    const key = `trading:${target.id}`
    if (pendingMemberActions.current.has(key)) return
    const multiplier = Number(userTrading.odds_multiplier || 1)
    if (!Number.isFinite(multiplier) || multiplier < .5 || multiplier > 1.5) {
      showMessage('会员赔率倍率需在 0.50–1.50 之间', 'error')
      return
    }
    pendingMemberActions.current.add(key)
    setUserTradingSaving(true)
    try {
      const current = await requireCurrentMember(target)
      if (!current || tradingIDRef.current !== target.id) return
      const next = await roomApi.updateUserTrading(tradingUser.id, {
        odds_multiplier: multiplier,
        fly_mode: userTrading.fly.mode,
        fly_rate: userTrading.fly.rate,
        rebate_mode: userTrading.rebate.mode,
        rebate_rate: userTrading.rebate.rate,
        game_id: userTrading.game_id,
        odds: userTrading.odds.map(item => ({ play_code: item.play_code, override: item.has_override ? item.override : null })),
      })
      if (!alive.current || tradingIDRef.current !== target.id) return
      const latest = await refreshMember(current)
      if (!canManageMember(latest) || tradingIDRef.current !== target.id) return
      setUserTrading(next)
      setUserTradingDirty(false)
      showMessage(`${tradingUser.nickname || tradingUser.username} 的会员赔率已保存`)
    } catch (reason) {
      if (alive.current && tradingIDRef.current === target.id) showMessage(reason instanceof Error ? reason.message : '保存会员赔率失败', 'error')
    } finally {
      pendingMemberActions.current.delete(key)
      if (alive.current) setUserTradingSaving(false)
    }
  }
  const sendReply = async () => {
    if (!selected || !reply.trim() || loading || transitioningChatMode) return
    try { const row = await roomApi.replyChat({ scope: selected.scope, room_scope: selected.room_scope, game_id: selected.game_id, room_type: selected.room_type, content: reply.trim() }); setMessages(current => [...current, row]); setReply('') }
    catch (reason) { showMessage(reason instanceof Error ? reason.message : '发送失败', 'error') }
  }
  const openRedPacket = () => {
    if (lotteryChat || loading || transitioningChatMode || !selected || selected.room_type !== 'group') return
    setRedPacketCount('10')
    setRedPacketTotal('100')
    setRedPacketGreeting('恭喜发财')
    setRedPacketCover('classic')
    setRedPacketMinTurnover('0')
    redPacketRequestID.current = createRequestId()
    setRedPacketOpen(true)
  }
  const sendRedPacket = async () => {
    const count = Number(redPacketCount)
    const total = Number(redPacketTotal)
    if (lotteryChat || loading || transitioningChatMode || !selected || !Number.isInteger(count) || count < 1 || total < count * .01) return
    try { const row = await roomApi.sendChatRedPacket({ request_id: redPacketRequestID.current, game_id: selected.game_id, count, total_amount: total, min_daily_turnover: Math.max(0, Number(redPacketMinTurnover) || 0), greeting: redPacketGreeting.trim() || '恭喜发财', cover: redPacketCover }); setMessages(current => mergeAdminChatMessages(current, [row])); redPacketRequestID.current = createRequestId(); setRedPacketOpen(false); showMessage('红包已发送到当前聊天室') }
    catch (reason) { showMessage(reason instanceof Error ? reason.message : '发送红包失败', 'error') }
  }
  return <Box p={{ xs: 1.5, md: 2.5 }}>
    {error && <Alert severity="error" sx={{ mb: 2 }} action={<Button onClick={() => { void load(); const current = selectedRef.current; if (current) void loadChatMessages(current, 'latest') }}>重试</Button>}>{error}</Alert>}
    {loading && !chatSection && <Box py={1}><CircularProgress size={20} /></Box>}

    {section === 'dashboard' && <>
      <Card sx={{ mb: 2 }}><CardContent><Stack direction={{ xs: 'column', md: 'row' }} alignItems={{ md: 'center' }} gap={1.5}><Avatar src={roomLogo || undefined} variant="rounded" sx={{ width: 64, height: 64, color: '#fff', background: roomLogo ? undefined : identityGradient, fontWeight: 900, fontSize: 24, boxShadow: '0 4px 12px rgba(37,99,235,.22)' }}>{(roomName || '房').slice(0, 1)}</Avatar><Box flex={1}><Typography fontWeight={850}>房间资料</Typography><Typography variant="caption" color="text.secondary">房间号 {dashboard?.room_code || '—'} · 名称和 Logo 会显示给进入该房间的用户</Typography><Stack direction="row" gap={.5} mt={.7}><Button component="label" size="small" startIcon={<PhotoCameraRounded />}>{roomLogo ? '更换 Logo' : '选择 Logo'}<input hidden type="file" accept="image/png,image/jpeg,image/webp" onChange={chooseRoomLogo} /></Button>{roomLogo && <Button color="error" size="small" startIcon={<DeleteOutlineRounded />} onClick={() => setRoomLogo('')}>移除</Button>}</Stack></Box><TextField size="small" label="房间名称" value={roomName} onChange={event => setRoomName(event.target.value)} inputProps={{ maxLength: 30 }} sx={{ width: { xs: '100%', md: 260 } }} /><Button variant="contained" disabled={roomSaving || !dashboard} onClick={() => void saveRoomProfile()}>{roomSaving ? '保存中…' : '保存资料'}</Button></Stack></CardContent></Card>
      <Box display="grid" gridTemplateColumns={{ xs: '1fr 1fr', lg: 'repeat(3,1fr)' }} gap={1.5}>{cards.map(([label, value]) => <Card key={label}><CardContent><Typography variant="caption" color="text.secondary">{label}</Typography><Typography variant="h6" fontWeight={850}>{value}</Typography></CardContent></Card>)}</Box>
      <Card sx={{ mt: 2 }}><CardContent><Typography fontWeight={800}>房间机器人</Typography><Typography variant="body2" color="text.secondary" mb={1.5}>使用普通会员身份在本房间持久化下注和聊天，可立即执行一轮。</Typography><Button disabled={robotRunning} variant="contained" onClick={() => { setRobotRunning(true); void roomApi.runRobotOnce().then(() => showMessage('本房间已执行一轮')).catch(reason => showMessage(reason instanceof Error ? reason.message : '执行失败', 'error')).finally(() => setRobotRunning(false)) }}>{robotRunning ? '执行中…' : '立即执行'}</Button></CardContent></Card>
    </>}

    {section === 'reports' && <OperatingReportPanel agent />}

    {section !== 'dashboard' && !chatSection && section !== 'reports' && <Paper variant="outlined" sx={{ p: 1.3, mb: 1.5 }}><Stack direction="row" gap={1}><TextField size="small" fullWidth placeholder="搜索当前房间数据" value={query} onChange={event => { setQuery(event.target.value); setPage(0) }} onKeyDown={event => { if (event.key === 'Enter') void load() }} /><Button variant="contained" onClick={() => void load()}>查询</Button></Stack></Paper>}

    {section === 'users' && <Card><TableContainer><Table size="small">
      <TableHead><TableRow><TableCell>用户</TableCell><TableCell>房间状态</TableCell><TableCell>余额</TableCell><TableCell>在线状态</TableCell><TableCell>账号状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead>
      <TableBody>{users.map(row => <TableRow key={row.id}>
        <TableCell><Stack direction="row" gap={1} alignItems="center"><Avatar src={memberAvatar(row.id, row.avatar)} sx={{ width: 34, height: 34 }}>{(row.nickname || row.username).slice(0, 1)}</Avatar><Box><Typography fontWeight={750}>{row.nickname || row.username}</Typography><Typography variant="caption" color="text.secondary">@{row.username} · {row.public_id}</Typography></Box></Stack></TableCell>
        <TableCell><Chip size="small" color={row.in_current_room === true ? 'success' : 'default'} variant="outlined" label={row.in_current_room === true ? '在本房间' : '已切换'} /></TableCell>
        <TableCell>{canManageMember(row) && row.balance !== null ? `¥ ${money(row.balance)}` : '—'}</TableCell>
        <TableCell>{canManageMember(row) && row.online !== null ? <UserPresenceChip online={row.online === true} /> : '—'}</TableCell>
        <TableCell>{canManageMember(row) && row.status !== null ? <FormControlLabel control={<Switch size="small" checked={row.status === 1} onChange={() => void changeMemberStatus(row)} />} label={row.status === 1 ? '账号正常' : '账号停用'} /> : '—'}</TableCell>
        <TableCell align="right">{canManageMember(row) ? <Stack direction="row" justifyContent="flex-end" gap={.5}><Button size="small" startIcon={<TuneRounded />} onClick={() => void loadUserTrading(row)}>赔率设置</Button><Button size="small" onClick={() => void openBalance(row)}>调整余额</Button></Stack> : '—'}</TableCell>
      </TableRow>)}
      {!loading && users.length === 0 && <TableRow><TableCell colSpan={6} align="center" sx={{ py: 5, color: 'text.secondary' }}>{query.trim() ? '未找到匹配的房间会员' : '还没有会员加入过本房间'}</TableCell></TableRow>}
      </TableBody></Table></TableContainer><WorkspacePagination total={total} page={page} pageSize={pageSize} onPage={setPage} onPageSize={value => { setPageSize(value); setPage(0) }} /></Card>}

    {(section === 'applications' || section === 'room-reviews') && <Stack gap={1.25}>
      <Paper variant="outlined" sx={{ borderRadius: 2.5, overflow: 'hidden' }}><Tabs value={applicationCategory} onChange={(_, next: ApplicationCategory) => { setApplicationCategory(next); setQuery(''); setPage(0); setApplications([]) }} variant="fullWidth" sx={{ minHeight: 56, '& .MuiTab-root': { minHeight: 56, fontSize: { xs: 12, sm: 14 }, fontWeight: 800 } }}><Tab value="wallet" label="上下分申请" /><Tab value="join" label="入房申请" /><Tab value="entertainment" label="娱乐上下分" /></Tabs></Paper>
      <Card><TableContainer><Table size="small"><TableHead><TableRow><TableCell>用户</TableCell><TableCell>{applicationCategory === 'join' ? '目标房间' : applicationCategory === 'entertainment' ? '娱乐平台' : '类型'}</TableCell>{applicationCategory !== 'join' && <TableCell>金额</TableCell>}<TableCell>状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>{applications.map(row => <TableRow key={row.id}><TableCell>{row.username}<Typography variant="caption" display="block" color="text.secondary">{time(row.created_at)}</Typography></TableCell><TableCell>{applicationCategory === 'join' ? row.target_room_code || '当前房间' : applicationCategory === 'entertainment' ? row.game_id || '未标记平台' : row.request_type === 'credit' ? '上分' : row.request_type === 'debit' ? '下分' : '入房'}</TableCell>{applicationCategory !== 'join' && <TableCell>¥ {money(row.requested_amount)}</TableCell>}<TableCell><Chip size="small" color={row.status === 'pending' ? 'warning' : row.status === 'approved' ? 'success' : 'default'} label={row.status === 'pending' ? '待审核' : row.status === 'approved' ? '已通过' : '已拒绝'} /></TableCell><TableCell align="right">{row.status === 'pending' && <Button size="small" onClick={() => openReview(row)}>审核</Button>}</TableCell></TableRow>)}{!loading && applications.length === 0 && <TableRow><TableCell colSpan={5}><Box py={5} textAlign="center"><Typography color="text.secondary">暂无{applicationCategory === 'wallet' ? '上下分' : applicationCategory === 'join' ? '入房' : '娱乐上下分'}申请</Typography></Box></TableCell></TableRow>}</TableBody></Table></TableContainer><WorkspacePagination total={total} page={page} pageSize={pageSize} onPage={setPage} onPageSize={value => { setPageSize(value); setPage(0) }} /></Card>
    </Stack>}

    {section === 'bets' && <Card><TableContainer><Table size="small"><TableHead><TableRow><TableCell>注单</TableCell><TableCell>玩法</TableCell><TableCell>金额</TableCell><TableCell>赔率</TableCell><TableCell>状态</TableCell></TableRow></TableHead><TableBody>{bets.map(row => <TableRow key={row.id}><TableCell>#{row.id}<Typography variant="caption" display="block" color="text.secondary">{row.game_id} · {row.issue}</Typography></TableCell><TableCell>{row.play_name} · {row.selection}</TableCell><TableCell>¥ {money(row.amount)}</TableCell><TableCell>{row.odds}</TableCell><TableCell>{row.status}</TableCell></TableRow>)}</TableBody></Table></TableContainer><WorkspacePagination total={total} page={page} pageSize={pageSize} onPage={setPage} onPageSize={value => { setPageSize(value); setPage(0) }} /></Card>}

    {chatSection && <Paper variant="outlined" sx={{
      position: 'relative',
      width: '100%',
      maxWidth: 1480,
      mx: 'auto',
      height: { xs: 'auto', md: 'calc(100dvh - 140px)' },
      minHeight: { md: 560 },
      maxHeight: { md: 900 },
      alignSelf: 'flex-start',
      overflow: 'hidden',
      borderRadius: 1.25,
    }}><Box sx={{
      display: 'grid',
      gridTemplateColumns: lotteryChat
        ? { xs: '1fr', md: selected ? '320px minmax(0, 1fr)' : '360px' }
        : { xs: '1fr', md: selected ? '300px minmax(0, 1fr)' : '360px' },
      width: '100%',
      height: { md: '100%' },
      minHeight: 0,
      overflow: { md: 'hidden' },
    }}>
      <Box sx={{ height: { md: '100%' }, minHeight: 0, overflow: 'hidden', borderRight: { md: 1 }, borderColor: 'divider' }}>
        <CardContent sx={{ p: 0, height: { md: '100%' }, minHeight: 0, display: 'flex', flexDirection: 'column', '&:last-child': { pb: 0 } }}>
          <Box sx={{ px: 1.25, pt: 1.15, pb: .9 }}>
            <Stack direction="row" alignItems="center" gap={1.05} mb={1}>
              <Avatar src={roomDisplayLogo || undefined} variant="rounded" sx={{ width: 43, height: 43, color: '#fff', background: roomDisplayLogo ? undefined : identityGradient, fontWeight: 950, boxShadow: roomDisplayLogo ? '0 2px 8px rgba(15,23,42,.16)' : '0 4px 12px rgba(37,99,235,.25)' }}>{roomDisplayName.slice(0, 1)}</Avatar>
              <Box minWidth={0} flex={1}><Stack direction="row" alignItems="center" gap={.65}><Typography fontWeight={900} fontSize={15.5} noWrap>{roomDisplayName}</Typography><Chip size="small" color="primary" variant="outlined" label={lotteryChat ? '彩票室' : '房间会话'} sx={{ height: 19, fontSize: 9 }} /></Stack><Typography fontSize={10.5} color="text.secondary" noWrap>房间号 {dashboard?.room_code || '—'} · {staffTitle}</Typography></Box>
            </Stack>
            {lotteryChat && lotteryCategories.length > 0 && <Tabs value={activeLotteryCategory} onChange={(_, next: string) => {
              setLotteryCategory(next)
              setMessages([])
              setSelected(conversations.find(item => item.lobby_category?.trim() === next) ?? null)
            }} variant="scrollable" scrollButtons={false} sx={{ minHeight: 34, mb: .7, '& .MuiTab-root': { minWidth: 56, minHeight: 34, px: 1, py: .25, fontSize: 11.5, fontWeight: 850 } }}>{lotteryCategories.map(category => <Tab key={category} value={category} label={category} />)}</Tabs>}
            {!lotteryChat && <Tabs value={chatMode} onChange={(_, next: 'service' | 'room') => { if (next === chatMode) return; setRedPacketOpen(false); setTransitioningChatMode(next); setChatMode(next) }} variant="fullWidth" sx={{ minHeight: 32, mb: .7, '& .MuiTab-root': { minHeight: 32, py: .3, fontSize: 11, transition: 'color 160ms ease, background-color 160ms ease' }, '@media (prefers-reduced-motion: reduce)': { '& .MuiTab-root': { transition: 'none' } } }}>
              <Tab value="service" label="在线客服" />
              <Tab value="room" label="房间群聊" />
            </Tabs>}
            <TextField size="small" fullWidth placeholder={lotteryChat ? '搜索彩种或消息内容' : '搜索会话'} value={query} onChange={event => setQuery(event.target.value)} slotProps={{ input: { startAdornment: <SearchRounded sx={{ mr: .7, color: 'text.disabled', fontSize: 19 }} /> } }} />
          </Box>
          <Divider />
          <List disablePadding sx={{ flex: 1, minHeight: 0, overflowY: 'auto', scrollbarWidth: 'thin', p: .5 }}>
            {visibleConversations.map(row => <Box key={`${row.scope}:${row.room_scope}:${row.game_id}:${row.room_type}`}>
              <ListItemButton
              selected={selected?.scope === row.scope && selected?.room_scope === row.room_scope && selected?.game_id === row.game_id && selected?.room_type === row.room_type}
              onClick={() => {
                // Clicking the already-open conversation must be a no-op. The
                // previous handler cleared the messages and then assigned the
                // same object, so React skipped the selection effect and the
                // right panel stayed empty until a full reload.
                const next = selectConversation(selected, row, messages)
                setMessages(next.messages)
                setSelected(next.selected)
              }}
              sx={{ borderRadius: 1.4, py: lotteryChat ? .75 : .2, px: lotteryChat ? 1.1 : .8, mb: lotteryChat ? .35 : .05, minHeight: lotteryChat ? 56 : 40 }}
            >{lotteryChat && <Avatar src={gameLogo(row.game_id)} variant="rounded" sx={{ width: 42, height: 42, mr: 1.05, bgcolor: 'background.paper', border: 1, borderColor: 'divider', fontSize: 13, fontWeight: 900 }}>{row.title.slice(0, 1)}</Avatar>}<ListItemText
              primary={row.title}
              secondary={<><Typography component="span" display="block" noWrap fontSize={10.5} color="text.secondary">房间号 {dashboard?.room_code || '—'}{row.enabled === false ? ' · 已关闭' : ''}</Typography><Typography component="span" display="block" noWrap fontSize={11.5} color="text.secondary">{row.latest_is_staff ? `${staffTitle}：` : '会员：'}{row.latest_text || '暂无聊天记录'}</Typography></>}
              primaryTypographyProps={{ fontWeight: 750, fontSize: lotteryChat ? 14 : 12.5, noWrap: true }}
              secondaryTypographyProps={{ component: 'div' }}
            /></ListItemButton>
            </Box>)}
            {conversations.length === 0 && !loading && <Box sx={{ px: 1.2, py: 2.5, textAlign: 'center' }}>
              <Typography fontSize={13} fontWeight={750} color="text.secondary">暂无房间会话</Typography>
              <Typography fontSize={11} color="text.disabled" mt={.4}>新消息到达后会显示在这里</Typography>
            </Box>}
            {conversations.length === 0 && loading && <Stack aria-label="正在加载会话" gap={1} p={.7}>{[0, 1, 2, 3].map(index => <Stack key={index} direction="row" gap={.8} alignItems="center"><Skeleton variant="rounded" width={34} height={34} /><Box flex={1}><Skeleton width={index % 2 ? '48%' : '64%'} height={17} /><Skeleton width="88%" height={15} /></Box></Stack>)}</Stack>}
          </List>
        </CardContent>
      </Box>

      {selected && <Box sx={{ height: { md: '100%' }, minHeight: { xs: 300, md: 0 }, overflow: 'hidden' }}>
        <CardContent sx={{ p: 0, display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0, minWidth: 0, '&:last-child': { pb: 0 } }}>
          <Box sx={{ px: 1.5, py: .8, minHeight: 54, display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
            <Stack direction="row" alignItems="center" gap={1} minWidth={0}>
              <Avatar src={lotteryChat ? gameLogo(selected.game_id) : roomDisplayLogo || undefined} variant="rounded" sx={{ width: 39, height: 39, flexShrink: 0, bgcolor: 'background.paper', background: !lotteryChat && !roomDisplayLogo ? identityGradient : undefined, border: lotteryChat ? 1 : 0, borderColor: 'divider', fontWeight: 900 }}>{selected.title.slice(0, 1)}</Avatar>
              <Box minWidth={0}><Typography fontWeight={850} noWrap>{selected.title}</Typography><Typography variant="caption" color="text.secondary" noWrap display="block">{roomDisplayName} · 房间号 {dashboard?.room_code || '—'}</Typography></Box>
            </Stack>
            {!lotteryChat && <Stack direction="row" gap={.4}>{selected.room_type === 'group' && <Button size="small" color="error" startIcon={<CardGiftcardRounded />} disabled={loading || Boolean(transitioningChatMode)} onClick={openRedPacket} sx={{ minWidth: 64 }}>红包</Button>}<Button size="small" color="inherit" onClick={() => { setSelected(null); setMessages([]) }} sx={{ minWidth: 42 }}>收起</Button></Stack>}
          </Box>
          <Divider />
          {lotteryChat && selectedGame && <Box px={{ xs: 1.25, md: 1.65 }} py={1.05} borderBottom={1} borderColor="divider" sx={{ background: theme => theme.palette.mode === 'dark' ? 'linear-gradient(90deg,rgba(14,165,233,.14),rgba(139,92,246,.08))' : 'linear-gradient(90deg,#effbff,#f5f2ff)' }}>
            <Stack direction={{ xs: 'column', sm: 'row' }} gap={{ xs: 1, sm: 1.7 }} alignItems={{ sm: 'center' }}>
              <Stack direction="row" gap={.9} alignItems="center" minWidth={{ sm: 178 }}><Avatar src={gameLogo(selectedGame.id)} variant="rounded" sx={{ width: 45, height: 45, bgcolor: 'background.paper', border: 1, borderColor: 'divider', fontWeight: 900 }}>{selectedGame.name.slice(0, 1)}</Avatar><Box minWidth={0}><Typography fontSize={14} fontWeight={900} noWrap>{selectedGame.name}</Typography><Typography fontSize={10} color="text.secondary" noWrap>第 {selectedGame.current_issue || selectedGame.issue || '—'} 期</Typography><Typography fontSize={9.5} color={selectedGame.source_healthy === false ? 'warning.main' : 'success.main'} noWrap>{selectedGame.source_name || (selectedGame.source_kind === 'official' ? '外部开奖源' : '平台开奖')} · {selectedGame.source_healthy === false ? '异常' : '正常'}</Typography></Box></Stack>
              <Box minWidth={92}><Typography fontSize={9.5} color="text.secondary">封盘倒计时</Typography><Typography fontSize={24} lineHeight={1.1} fontWeight={950} color={new Date(selectedGame.seal_at || selectedGame.next_draw_at).getTime() <= now ? 'warning.main' : 'primary.main'}>{countdownText(selectedGame.seal_at || selectedGame.next_draw_at, now)}</Typography></Box>
              <Box flex={1} minWidth={0}><Stack direction="row" alignItems="center" gap={.7} mb={.55}><Typography fontSize={9.5} color="text.secondary">上期 {selectedGame.issue || '—'}</Typography><Chip size="small" label={issueStatusText[selectedGame.issue_status || ''] || '运行中'} color={selectedGame.issue_status === 'abnormal' || selectedGame.source_healthy === false ? 'warning' : 'success'} sx={{ height: 19, fontSize: 9 }} /></Stack><Stack direction="row" gap={.42} flexWrap="wrap" useFlexGap>{(selectedGame.latest_numbers || []).map((number, index) => <Box key={`${index}-${number}`} sx={{ width: 26, height: 26, display: 'grid', placeItems: 'center', borderRadius: '50%', bgcolor: ballColor(number), color: '#fff', fontSize: 11, fontWeight: 950, boxShadow: '0 2px 5px rgba(15,23,42,.18)' }}>{number}</Box>)}{!selectedGame.latest_numbers?.length && <Typography fontSize={11} color="text.secondary">等待开奖数据</Typography>}</Stack></Box>
            </Stack>
          </Box>}
          {lotteryChat && roomAnnouncement && <Stack direction="row" alignItems="center" gap={.8} sx={{ px: 1.6, py: .65, borderBottom: 1, borderColor: 'divider', bgcolor: 'warning.50', color: 'text.primary' }}><CampaignRounded color="warning" sx={{ fontSize: 18, flexShrink: 0 }} /><Typography fontSize={11.5} noWrap><Box component="span" fontWeight={900}>{roomAnnouncement.title}</Box> · {roomAnnouncement.content}</Typography></Stack>}
          <Stack flex={1} minHeight={0} gap={.8} sx={{ overflowY: 'auto', p: 1.15, width: '100%' }}>
            {chatHasMore && <Box textAlign="center"><Button size="small" startIcon={chatLoading ? <CircularProgress size={13} /> : <ArrowUpwardRounded />} disabled={chatLoading || !chatNextBeforeID} onClick={() => void loadChatMessages(selected, 'older', chatNextBeforeID)}>加载更早消息</Button></Box>}
            {selected && messages.length === 0 && !loading && !chatLoading && <Box sx={{ mt: 2.5, textAlign: 'center', color: 'text.secondary' }}><Typography fontWeight={750}>暂无消息</Typography><Typography variant="body2">该聊天室暂时没有历史消息</Typography></Box>}
            {selected && messages.length === 0 && chatLoading && <Stack aria-label="正在加载聊天记录" gap={1.25} sx={{ width: '100%', pt: .8 }}><Stack direction="row" gap={.8}><Skeleton variant="circular" width={34} height={34} /><Skeleton variant="rounded" width="42%" height={54} /></Stack><Stack direction="row-reverse" gap={.8}><Skeleton variant="circular" width={34} height={34} /><Skeleton variant="rounded" width="56%" height={66} /></Stack><Stack direction="row" gap={.8}><Skeleton variant="circular" width={34} height={34} /><Skeleton variant="rounded" width="34%" height={44} /></Stack></Stack>}
            {messages.map(row => {
              const identity = messageIdentity(row)
              return <Stack key={row.id} direction={row.is_staff ? 'row-reverse' : 'row'} alignSelf={row.is_staff ? 'flex-end' : 'flex-start'} gap={.75} alignItems="flex-start" sx={{ width: 'fit-content', maxWidth: lotteryChat ? { xs: '94%', md: 700 } : { xs: '94%', md: 420 } }}>
                <Avatar src={identity.avatar} component={row.is_staff ? 'div' : 'button'} title={row.is_staff ? identity.badge : '查看会员资料'} aria-label={row.is_staff ? identity.badge : `查看${identity.name}的会员资料`} onClick={() => void openMember(row)} sx={{ width: 34, height: 34, flexShrink: 0, border: row.is_staff ? 0 : 1, borderColor: 'divider', color: '#fff', background: row.is_staff && !identity.avatar ? identityGradient : undefined, fontWeight: 900, fontSize: 12, cursor: row.is_staff ? 'default' : 'pointer', '&:hover': row.is_staff ? undefined : { boxShadow: '0 0 0 3px rgba(14,165,233,.2)' } }}>{identity.name.slice(0, 1)}</Avatar>
                <Box sx={{ minWidth: 0, maxWidth: 'calc(100% - 42px)' }}><Stack direction="row" gap={.55} alignItems="center" justifyContent={row.is_staff ? 'flex-end' : 'flex-start'} mb={.28}><Typography fontSize={10.5} color="text.secondary" noWrap>{identity.name}</Typography><Chip size="small" variant="outlined" color={row.is_staff ? 'primary' : 'default'} label={identity.badge} sx={{ height: 18, fontSize: 8.5, '& .MuiChip-label': { px: .65 } }} /></Stack>{row.message_type === 'redpacket' ? <AdminRedPacketCard count={row.red_packet_count || 1} total={Number(row.red_packet_total || 0)} minTurnover={Number(row.red_packet_min_turnover || 0)} greeting={row.content} cover={row.red_packet_cover} time={time(row.created_at)} /> : <Paper variant="outlined" sx={{ px: 1.15, pt: .75, pb: .45, bgcolor: row.is_staff ? 'primary.main' : 'background.paper', color: row.is_staff ? 'primary.contrastText' : 'text.primary', borderRadius: row.is_staff ? '13px 3px 13px 13px' : '3px 13px 13px 13px' }}><Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', lineHeight: 1.45 }}>{row.content}</Typography><Typography fontSize={9} sx={{ opacity: .7, textAlign: 'right', mt: .1 }}>{time(row.created_at)}</Typography></Paper>}</Box>
              </Stack>
            })}
          </Stack>
          {selected && <Stack direction="row" alignItems="center" gap={.75} sx={{ p: .85, borderTop: 1, borderColor: 'divider' }}><TextField size="small" fullWidth value={reply} disabled={loading || Boolean(transitioningChatMode)} onChange={event => setReply(event.target.value)} placeholder="回复当前会话" onKeyDown={event => { if (event.key === 'Enter') void sendReply() }} /><Button size="small" variant="contained" disabled={loading || !reply.trim() || Boolean(transitioningChatMode)} onClick={() => void sendReply()} sx={{ flex: '0 0 auto', minWidth: 72, height: 36, px: 1.25, whiteSpace: 'nowrap' }}>发送</Button></Stack>}
        </CardContent>
      </Box>}
    </Box>{loading && transitioningChatMode && <Stack role="status" aria-live="polite" alignItems="center" justifyContent="center" gap={.8} sx={{ position: 'absolute', zIndex: 3, inset: 0, color: 'text.primary', bgcolor: theme => theme.palette.mode === 'dark' ? 'rgba(7,26,46,.34)' : 'rgba(247,251,252,.40)', transition: 'opacity 160ms ease', '@media (prefers-reduced-motion: reduce)': { transition: 'none' } }}><Box sx={{ display: 'flex', alignItems: 'center', gap: .8, px: 1.3, py: .8, border: 1, borderColor: 'divider', borderRadius: 1.2, bgcolor: 'background.paper', boxShadow: 3 }}><CircularProgress size={18} /><Typography fontSize={11.5} fontWeight={800}>{transitioningChatMode === 'service' ? '正在切换到在线客服' : '正在切换到房间群聊'}</Typography></Box></Stack>}</Paper>}

    <Dialog open={!lotteryChat && redPacketOpen} onClose={() => setRedPacketOpen(false)} fullWidth maxWidth="sm" slotProps={{ paper: { sx: { width: 'min(560px, calc(100% - 24px))', maxHeight: 'calc(100dvh - 32px)', borderRadius: 2, overflow: 'hidden' } } }}><DialogTitle sx={{ color: '#fff', background: 'linear-gradient(135deg,#d94b45,#ed7954)' }}><Typography fontSize={18} fontWeight={900}>发送房间红包</Typography><Typography fontSize={10.5} sx={{ opacity: .82 }}>红包会实时发送到当前房间聊天室</Typography></DialogTitle><DialogContent sx={{ pt: '18px !important', bgcolor: 'background.default' }}><RedPacketForm count={redPacketCount} total={redPacketTotal} greeting={redPacketGreeting} cover={redPacketCover} minTurnover={redPacketMinTurnover} onCount={setRedPacketCount} onTotal={setRedPacketTotal} onGreeting={setRedPacketGreeting} onCover={setRedPacketCover} onMinTurnover={setRedPacketMinTurnover} /></DialogContent><DialogActions sx={{ px: 2.5, py: 1.25, bgcolor: 'background.paper' }}><Button size="small" onClick={() => setRedPacketOpen(false)}>取消</Button><Button size="small" variant="contained" color="error" disabled={loading || Boolean(transitioningChatMode) || !Number.isInteger(Number(redPacketCount)) || Number(redPacketCount) < 1 || Number(redPacketTotal) < Number(redPacketCount) * .01 || Number(redPacketMinTurnover) < 0} onClick={() => void sendRedPacket()} sx={{ minWidth: 88, height: 34, px: 1.5 }}>发送红包</Button></DialogActions></Dialog>
    <Dialog open={memberOpen} onClose={closeMember} fullWidth maxWidth="sm">
      <DialogTitle>会员资料</DialogTitle>
      <DialogContent dividers>
        {memberLoading && <Box py={5} textAlign="center"><CircularProgress size={28} /><Typography mt={1} fontSize={12} color="text.secondary">正在读取会员资料…</Typography></Box>}
        {!memberLoading && memberError && <Alert severity="warning">{memberError}</Alert>}
        {!memberLoading && memberInfo && <Stack gap={1.5}>
          <Stack direction="row" alignItems="center" gap={1.2}>
            <Avatar src={memberAvatar(memberInfo.id, memberInfo.avatar)} sx={{ width: 58, height: 58, border: 1, borderColor: 'divider' }}>{(memberInfo.nickname || memberInfo.username).slice(0, 1)}</Avatar>
            <Box minWidth={0} flex={1}><Stack direction="row" alignItems="center" gap={.65} flexWrap="wrap"><Typography fontSize={17} fontWeight={900} noWrap>{memberInfo.nickname || memberInfo.username}</Typography>{memberInfo.public_title?.trim() && <Chip size="small" color="primary" variant="outlined" label={memberInfo.public_title.trim()} />}{memberInfo.badge?.trim() && <Chip size="small" color="warning" variant="outlined" label={memberInfo.badge.trim()} />}</Stack><Typography fontSize={11} color="text.secondary">会员 ID {memberInfo.public_id} · @{memberInfo.username}</Typography></Box>
            <Chip size="small" color={memberInfo.in_current_room === true ? 'success' : 'default'} label={memberInfo.in_current_room === true ? '在本房间' : '已切换'} />
          </Stack>
          {!canManageMember(memberInfo) ? <Alert severity="info">该会员已切换房间或无当前管理权限，仅显示曾加入本房间的公开资料。</Alert> : <>
            <Chip size="small" sx={{ alignSelf: 'flex-start' }} color={memberInfo.status === 1 ? 'success' : 'default'} label={memberInfo.status === null ? '—' : memberInfo.status === 1 ? '账号正常' : '账号停用'} />
            <Box display="grid" gridTemplateColumns={{ xs: '1fr 1fr', sm: 'repeat(4,1fr)' }} gap={1}>{[
              ['可用积分', memberInfo.balance === null ? '—' : money(memberInfo.balance)],
              ['总注单', memberActivity ? `${memberActivity.betCount} 笔` : '—'],
              ['待结算', memberActivity ? `${memberActivity.pendingCount} 笔` : '—'],
              ['登录次数', memberInfo.login_count === undefined ? '—' : `${memberInfo.login_count} 次`],
            ].map(([label, value]) => <Paper key={label} variant="outlined" sx={{ p: 1.05, borderRadius: 1.6 }}><Typography fontSize={9.5} color="text.secondary">{label}</Typography><Typography mt={.25} fontSize={13} fontWeight={900}>{value}</Typography></Paper>)}</Box>
            {memberActivity && <Paper variant="outlined" sx={{ p: 1.2, borderRadius: 1.8 }}><Typography fontSize={10} color="text.secondary" mb={.7}>最近 {memberActivity.sampleSize} 笔注单活动</Typography><Stack direction="row" justifyContent="space-between" gap={2}><Box><Typography fontSize={10} color="text.secondary">投注额</Typography><Typography fontWeight={900}>¥ {money(memberActivity.recentStake)}</Typography></Box><Box textAlign="right"><Typography fontSize={10} color="text.secondary">派彩额</Typography><Typography fontWeight={900}>¥ {money(memberActivity.recentPayout)}</Typography></Box></Stack></Paper>}
            <Divider />
            <Box display="grid" gridTemplateColumns={{ xs: '1fr', sm: '1fr 1fr' }} columnGap={2.5} rowGap={1}>{[
              ['账号角色', roleText[memberInfo.role] || memberInfo.role],
              ['风险等级', memberInfo.risk_level ? riskText[memberInfo.risk_level] || memberInfo.risk_level : '—'],
              ['所属房间', `${roomDisplayName} · ${dashboard?.room_code || '—'}`],
              ['最近登录', compactTime(memberInfo.last_login_at)],
              ['注册时间', compactTime(memberInfo.created_at)],
              ['联系电话', memberInfo.phone || '—'],
            ].map(([label, value]) => <Stack key={label} direction="row" justifyContent="space-between" gap={1}><Typography fontSize={11} color="text.secondary">{label}</Typography><Typography fontSize={11} fontWeight={750} textAlign="right">{value}</Typography></Stack>)}</Box>
            {memberInfo.remark && <Alert severity="info" icon={false}><Typography fontSize={11}><Box component="span" fontWeight={900}>备注：</Box>{memberInfo.remark}</Typography></Alert>}
          </>}
        </Stack>}
      </DialogContent>
      <DialogActions><Button onClick={closeMember}>关闭</Button></DialogActions>
    </Dialog>
    <Dialog open={Boolean(tradingUser)} onClose={closeTrading} fullWidth maxWidth="lg" slotProps={{ paper: { sx: { borderRadius: 1.25, maxHeight: 'calc(100dvh - 24px)' } } }}>
      <DialogTitle sx={{ px: 1.5, py: 1.1 }}>
        <Stack direction="row" gap={1} alignItems="center" justifyContent="space-between">
          <Box minWidth={0}><Typography fontSize={16.5} fontWeight={900} noWrap>会员赔率 · {tradingUser?.nickname || tradingUser?.username}</Typography><Typography fontSize={9.8} color="text.secondary">仅当前房间生效，会员换房后不继承</Typography></Box>
          <Chip size="small" color="primary" variant="outlined" label={`房间 ${dashboard?.room_code || '—'}`} sx={{ flexShrink: 0, height: 22 }} />
        </Stack>
      </DialogTitle>
      <DialogContent dividers sx={{ bgcolor: 'background.default', p: '12px !important' }}>
        {userTradingLoading && <Box py={8} textAlign="center"><CircularProgress size={28} /><Typography mt={1} fontSize={12} color="text.secondary">正在读取当前房间赔率…</Typography></Box>}
        {!userTradingLoading && canManageMember(tradingUser) && userTrading && <Stack gap={.9}>
          <Paper variant="outlined" sx={{ p: 1, borderRadius: 1.1 }}>
            <Stack direction={{ xs: 'column', md: 'row' }} gap={.8} alignItems={{ md: 'center' }}>
              <Box flex={1}>
                <Typography fontSize={13} fontWeight={900}>会员赔率倍率</Typography>
                <Typography fontSize={9.6} color="text.secondary">整体调整继承赔率，单独玩法设置优先。</Typography>
              </Box>
              <Stack direction="row" gap={.4} flexWrap="wrap" useFlexGap>
                {oddsMultiplierPresets.map(value => <Button key={value} size="small" variant={Number(userTrading.odds_multiplier || 1) === value ? 'contained' : 'outlined'} onClick={() => { setUserTrading(current => current ? { ...current, odds_multiplier: value } : current); setUserTradingDirty(true) }} sx={{ minWidth: 53, borderRadius: 1 }}>{value.toFixed(2)}×</Button>)}
              </Stack>
              <TextField size="small" type="number" label="自定义倍率" value={userTrading.odds_multiplier || 1} onChange={event => { setUserTrading(current => current ? { ...current, odds_multiplier: Number(event.target.value) } : current); setUserTradingDirty(true) }} inputProps={{ min: .5, max: 1.5, step: .01 }} sx={{ width: { xs: '100%', md: 140 }, '& .MuiOutlinedInput-root': { borderRadius: 1 } }} />
            </Stack>
            <Divider sx={{ my: .85 }} />
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,minmax(0,1fr))', lg: 'repeat(4,minmax(0,1fr))' }, gap: .7, '& .MuiOutlinedInput-root': { borderRadius: 1 } }}>
              <TextField select size="small" label="飞单模式" value={userTrading.fly.mode} onChange={event => { setUserTrading(current => current ? { ...current, fly: { ...current.fly, mode: event.target.value } } : current); setUserTradingDirty(true) }}><MenuItem value="inherit">跟随房间</MenuItem><MenuItem value="custom">单独比例</MenuItem><MenuItem value="off">关闭飞单</MenuItem></TextField>
              <TextField size="small" type="number" label="飞单比例 %" disabled={userTrading.fly.mode !== 'custom'} value={userTrading.fly.rate} onChange={event => { setUserTrading(current => current ? { ...current, fly: { ...current.fly, rate: Number(event.target.value) } } : current); setUserTradingDirty(true) }} inputProps={{ min: 0, max: 100, step: .01 }} />
              <TextField select size="small" label="返水模式" value={userTrading.rebate.mode} onChange={event => { setUserTrading(current => current ? { ...current, rebate: { ...current.rebate, mode: event.target.value } } : current); setUserTradingDirty(true) }}><MenuItem value="inherit">跟随房间</MenuItem><MenuItem value="custom">会员单独返水</MenuItem><MenuItem value="off">关闭返水</MenuItem></TextField>
              <TextField size="small" type="number" label="返水比例 %" disabled={userTrading.rebate.mode !== 'custom'} value={userTrading.rebate.rate} onChange={event => { setUserTrading(current => current ? { ...current, rebate: { ...current.rebate, rate: Number(event.target.value) } } : current); setUserTradingDirty(true) }} inputProps={{ min: 0, max: 100, step: .01 }} />
            </Box>
          </Paper>
          <Paper variant="outlined" sx={{ px: 1, py: .6, borderRadius: 1, bgcolor: 'action.hover' }}><Stack direction={{ xs: 'column', sm: 'row' }} gap={.6} alignItems={{ sm: 'center' }}><Chip size="small" color="primary" variant="outlined" label="生效顺序" sx={{ width: 'fit-content', height: 20 }} /><Typography fontSize={10} color="text.secondary">会员单独玩法赔率 ＞ 房间赔率 × 会员倍率 ＞ 平台赔率 × 会员倍率</Typography></Stack></Paper>
          <GameOddsNavigation games={games.map(game => ({ ...game, enabled: game.platform_enabled && game.room_enabled }))} gameId={userTrading.game_id} onSelect={gameId => { if (!tradingUser || gameId === userTrading.game_id) return; if (userTradingDirty) { showMessage('请先保存当前彩种的会员赔率，再切换彩种', 'warning'); return }; void loadUserTrading(tradingUser, gameId) }} />
          <OddsOverrideGrid items={userTrading.odds} level="member" onChange={odds => { setUserTrading(current => current ? { ...current, odds: odds.map(item => ({ ...item, room_odds: item.room_odds ?? item.base_odds })) } : current); setUserTradingDirty(true) }} />
        </Stack>}
      </DialogContent>
      <DialogActions sx={{ px: 1.5, py: .75 }}><Button onClick={closeTrading} disabled={userTradingSaving}>取消</Button><Button variant="contained" disabled={userTradingSaving || userTradingLoading || !canManageMember(tradingUser) || !userTrading || !userTradingDirty} onClick={() => void saveUserTrading()}>{userTradingSaving ? '保存中…' : userTradingDirty ? '保存会员赔率' : '已保存'}</Button></DialogActions>
    </Dialog>
    <Dialog open={Boolean(balanceUser)} onClose={closeBalance} fullWidth maxWidth="xs"><DialogTitle>调整余额 · {balanceUser?.nickname || balanceUser?.username}</DialogTitle><DialogContent><Stack gap={2} pt={1}><TextField type="number" label="调整金额" helperText="正数为上分，负数为下分" value={amount} disabled={balanceSaving} onChange={event => setAmount(event.target.value)} /><TextField label="原因" value={remark} disabled={balanceSaving} onChange={event => setRemark(event.target.value)} /></Stack></DialogContent><DialogActions><Button disabled={balanceSaving} onClick={closeBalance}>取消</Button><Button variant="contained" disabled={balanceSaving || !canManageMember(balanceUser)} onClick={() => void adjustBalance()}>{balanceSaving ? '处理中…' : '确认'}</Button></DialogActions></Dialog>
    <Dialog open={Boolean(reviewing)} onClose={() => !reviewSaving && setReviewing(null)} fullWidth maxWidth="sm"><DialogTitle>{reviewing?.request_type === 'join' ? '审核入房申请' : '审核上下分申请'} #{reviewing?.id}</DialogTitle><DialogContent>{reviewing && <Stack gap={1.5} pt={1}><Paper variant="outlined" sx={{ p: 1.4 }}><Typography fontWeight={850}>{reviewing.username}</Typography><Typography variant="caption" color="text.secondary">{reviewing.request_type === 'join' ? `目标房间 ${reviewing.target_room_code || '当前房间'}` : `申请金额 ¥ ${money(reviewing.requested_amount)}`}</Typography>{reviewing.remark && <Typography fontSize={12} mt={1}>{reviewing.remark}</Typography>}</Paper><Tabs value={reviewDecision} onChange={(_, next: 'approved' | 'rejected') => setReviewDecision(next)} variant="fullWidth"><Tab value="approved" label="通过申请" /><Tab value="rejected" label="拒绝申请" /></Tabs>{reviewDecision === 'approved' && reviewing.request_type === 'join' && <Paper variant="outlined" sx={{ p: 1.4, borderColor: 'primary.light', bgcolor: 'action.hover' }}><Typography fontSize={13} fontWeight={900}>入房赔率倍率</Typography><Typography fontSize={10} color="text.secondary">只写入本房间会员关系；换房后自动采用目标房间对应配置。</Typography><Stack direction="row" gap={.6} flexWrap="wrap" useFlexGap mt={1.2}>{oddsMultiplierPresets.map(value => <Button key={value} size="small" variant={Number(reviewOddsMultiplier) === value ? 'contained' : 'outlined'} onClick={() => setReviewOddsMultiplier(String(value))}>{value.toFixed(2)}×</Button>)}</Stack><TextField fullWidth size="small" type="number" label="自定义倍率" value={reviewOddsMultiplier} onChange={event => setReviewOddsMultiplier(event.target.value)} inputProps={{ min: .5, max: 1.5, step: .01 }} helperText="0.50–1.50；1.00 为正常房间赔率" sx={{ mt: 1.2 }} /></Paper>}{reviewDecision === 'approved' && isBalanceApplication(reviewing) && <><TextField type="number" fullWidth label={reviewing.request_type === 'credit' ? '实际到账金额' : '实际出款金额'} value={reviewReceivedAmount} onChange={event => setReviewReceivedAmount(event.target.value)} helperText={reviewing.request_type === 'credit' ? '余额按实际到账金额增加' : '余额按申请金额扣减，实际出款金额进入审核记录'} inputProps={{ min: .01, step: .01 }} /><Paper variant="outlined" sx={{ p: 1.4 }}><Stack direction="row" justifyContent="space-between"><Typography color="text.secondary" fontSize={12}>变动前余额</Typography><Typography fontWeight={850}>¥ {money(reviewing.user_balance)}</Typography></Stack><Stack direction="row" justifyContent="space-between" mt={.8}><Typography color="text.secondary" fontSize={12}>预计变动后余额</Typography><Typography fontWeight={900} color="primary.main">¥ {money(reviewedBalance(reviewing, Number(reviewReceivedAmount) || 0))}</Typography></Stack></Paper></>}<TextField fullWidth multiline minRows={3} label={reviewDecision === 'rejected' ? '拒绝原因' : '审核备注'} value={reviewRemark} onChange={event => setReviewRemark(event.target.value)} required={reviewDecision === 'rejected'} inputProps={{ maxLength: 500 }} /><Alert severity={reviewDecision === 'approved' ? 'info' : 'warning'}>{reviewDecision === 'approved' ? reviewing.request_type === 'join' ? `通过后会员将进入房间 ${reviewing.target_room_code || '当前房间'}，倍率为 ${(Number(reviewOddsMultiplier) || 1).toFixed(2)}×。` : '通过后立即写入余额与资金流水，不能重复审核。' : reviewing.request_type === 'join' ? '拒绝不会改变会员的房间归属。' : '拒绝不会改变余额或房间归属。'}</Alert></Stack>}</DialogContent><DialogActions><Button onClick={() => setReviewing(null)} disabled={reviewSaving}>取消</Button><Button variant="contained" color={reviewDecision === 'approved' ? 'success' : 'error'} disabled={reviewSaving} onClick={() => void review()}>{reviewSaving ? '处理中…' : reviewDecision === 'approved' ? '确认通过' : '确认拒绝'}</Button></DialogActions></Dialog>
  </Box>
}

function WorkspacePagination({ total, page, pageSize, onPage, onPageSize }: { total: number; page: number; pageSize: number; onPage: (page: number) => void; onPageSize: (size: number) => void }) {
  return <TablePagination component="div" count={total} page={page} onPageChange={(_, next) => onPage(next)} rowsPerPage={pageSize} onRowsPerPageChange={event => onPageSize(Number(event.target.value))} rowsPerPageOptions={[10, 20, 50]} labelRowsPerPage="每页" />
}
