import { Alert, Avatar, Box, Button, Card, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Divider, IconButton, InputAdornment, MenuItem, Paper, Skeleton, Stack, Tab, Tabs, TextField, Tooltip, Typography } from '@mui/material'
import SupportAgentRounded from '@mui/icons-material/SupportAgentRounded'
import ForumRounded from '@mui/icons-material/ForumRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import SendRounded from '@mui/icons-material/SendRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import VolumeOffRounded from '@mui/icons-material/VolumeOffRounded'
import VolumeUpRounded from '@mui/icons-material/VolumeUpRounded'
import ArrowUpwardRounded from '@mui/icons-material/ArrowUpwardRounded'
import PushPinRounded from '@mui/icons-material/PushPinRounded'
import CardGiftcardRounded from '@mui/icons-material/CardGiftcardRounded'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, type AdminChatConversation, type AdminChatMessage, type AdminGame, type AdminUser, type DrawResult, type SystemSettings } from '../api'
import { useFeedback } from '../components/feedback'
import { AdminRedPacketCard, RedPacketForm, type RedPacketCover } from '../components/RedPacketForm'
import { MANAGEMENT_WS_EVENT, useManagementWebSocketConnected } from '../hooks/useManagementWebSocket'
import type { ManagementWsEvent } from '../api'
import { gameLogo } from '../gameLogos'
import { mergeAdminChatMessages, sameConversation, selectConversation } from '../utils/chatState'
import { createRequestId } from '../utils/requestId'
import { responsiveSplitPanelBorderSx } from '../theme'
import {
  CHAT_OPEN_CONVERSATION_EVENT, chatPageForTarget, consumePendingChatConversation, reportChatUnreadChanged,
  setActiveChatConversation, type ChatConversationTarget,
} from '../utils/chatNotifications'

type ChatMode = 'service' | 'room' | 'lottery'
type ChatRoomItem = Pick<SystemSettings, 'chat_nickname' | 'room_notice' | 'announcements'> & { id: number; scope: string; kind: 'agent' | 'tenant'; room_code: string; room_name: string; room_logo: string; status: number }
type MemberActivitySummary = { betCount: number; pendingCount: number; recentStake: number; recentPayout: number; sampleSize: number }

const dateTime = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value)) : '—'
const conversationPreview = (item: AdminChatConversation) => {
  if (!item.latest_at || item.latest_text === '暂无聊天记录') return '暂无聊天记录'
  if (item.latest_message_type === 'redpacket') return `红包 · ${item.latest_text || '恭喜发财'}`
  return `${item.latest_is_staff ? '客服' : '会员'}：${item.latest_text}`
}

const issueStatusText: Record<string, string> = { accepting: '受理中', sealed: '已封盘', awaiting_draw: '待开奖', settling: '结算中', settled: '已结算', abnormal: '异常' }
const countdownText = (target: string | undefined, now: number) => {
  const seconds = Math.max(0, Math.ceil((new Date(target || 0).getTime() - now) / 1000))
  const minutes = Math.floor(seconds / 60)
  return `${String(minutes).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
}
const ballColor = (value: number) => ['#f97316', '#ef4444', '#0ea5e9', '#8b5cf6', '#16a34a'][Math.abs(value) % 5]
const identityGradient = 'linear-gradient(135deg,#0891b2 0%,#2563eb 54%,#7c3aed 100%)'
const memberAvatar = (userID: number, provided?: string) => provided?.trim() || `/images/avatars/avatar-${String(Math.abs(Math.trunc(userID || 0)) % 16).padStart(2, '0')}.png`

export function ChatPage({ view = 'support' }: { view?: 'support' | 'lottery' }) {
	const lotteryView = view === 'lottery'
  const websocketConnected = useManagementWebSocketConnected()
  const [mode, setMode] = useState<ChatMode>(view === 'lottery' ? 'lottery' : 'room')
  const [rooms, setRooms] = useState<ChatRoomItem[]>([])
  const [roomScope, setRoomScope] = useState('')
  const [query, setQuery] = useState('')
  const [appliedQuery, setAppliedQuery] = useState('')
  const [lotteryCategory, setLotteryCategory] = useState('')
  const [conversations, setConversations] = useState<AdminChatConversation[]>([])
  const [selected, setSelected] = useState<AdminChatConversation | null>(null)
  const [messages, setMessages] = useState<AdminChatMessage[]>([])
  const [nextBeforeID, setNextBeforeID] = useState<number | undefined>()
  const [hasMore, setHasMore] = useState(false)
  const [draft, setDraft] = useState('')
  const [loading, setLoading] = useState(true)
  const [transitioningMode, setTransitioningMode] = useState<ChatMode | null>(null)
  const [messageLoading, setMessageLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [redPacketOpen, setRedPacketOpen] = useState(false)
  const [redPacketCount, setRedPacketCount] = useState('10')
  const [redPacketTotal, setRedPacketTotal] = useState('100')
  const [redPacketGreeting, setRedPacketGreeting] = useState('恭喜发财')
  const [redPacketCover, setRedPacketCover] = useState<RedPacketCover>('classic')
  const [redPacketMinTurnover, setRedPacketMinTurnover] = useState('0')
  const [error, setError] = useState('')
	const [games, setGames] = useState<AdminGame[]>([])
	const [draws, setDraws] = useState<DrawResult[]>([])
	const [now, setNow] = useState(0)
	const [attentionRevision, setAttentionRevision] = useState(0)
	const [memberInfo, setMemberInfo] = useState<AdminUser | null>(null)
	const [memberLoading, setMemberLoading] = useState(false)
	const [memberActivity, setMemberActivity] = useState<MemberActivitySummary | null>(null)
  const { showMessage } = useFeedback()
  const selectedRef = useRef<AdminChatConversation | null>(null)
  const pendingTargetRef = useRef<ChatConversationTarget | null>(null)
  const markedThroughRef = useRef(new Map<string, number>())
  const redPacketRequestID = useRef(createRequestId())
  const connectionStatusInitialized = useRef(false)
	const conversationRequestRef = useRef(0)
	const messageRequestRef = useRef(0)
	useEffect(() => { selectedRef.current = selected }, [selected])
	const focusConversation = useCallback((target: ChatConversationTarget) => {
		if (chatPageForTarget(target) !== (lotteryView ? '/lottery-chat' : '/chat')) return
		pendingTargetRef.current = target
		setMode(target.room_type === 'service' ? 'service' : target.game_id === 'lobby' ? 'room' : 'lottery')
		setRoomScope(target.room_scope)
		setQuery('')
		setAppliedQuery('')
	}, [lotteryView])
	useEffect(() => {
		const pending = consumePendingChatConversation()
		const initial = pending ? window.setTimeout(() => focusConversation(pending), 0) : 0
		const onOpen = (event: Event) => {
			const stored = consumePendingChatConversation()
			focusConversation(stored ?? (event as CustomEvent<ChatConversationTarget>).detail)
		}
		window.addEventListener(CHAT_OPEN_CONVERSATION_EVENT, onOpen)
		return () => { window.clearTimeout(initial); window.removeEventListener(CHAT_OPEN_CONVERSATION_EVENT, onOpen) }
	}, [focusConversation])
	useEffect(() => {
		setActiveChatConversation(selected ?? null)
	}, [selected])
	useEffect(() => () => setActiveChatConversation(null), [])
	useEffect(() => {
		if (!lotteryView) return
		void adminApi.games().then(rows => setGames(Array.isArray(rows) ? rows : [])).catch(reason => setError(reason instanceof Error ? reason.message : '彩票信息暂时未加载'))
		const tick = () => setNow(new Date().getTime())
		const initial = window.setTimeout(tick, 0)
		const timer = window.setInterval(tick, 1000)
		return () => { window.clearTimeout(initial); window.clearInterval(timer) }
	}, [lotteryView])
	useEffect(() => {
		if (!lotteryView || !selected?.game_id || selected.game_id === 'lobby') {
			const timer = window.setTimeout(() => setDraws([]), 0)
			return () => window.clearTimeout(timer)
		}
		let active = true
		void adminApi.draws(selected.game_id).then(rows => { if (active) setDraws(Array.isArray(rows) ? rows : []) }).catch(reason => { if (active) setError(reason instanceof Error ? reason.message : '开奖记录暂时未加载') })
		return () => { active = false }
	}, [lotteryView, selected?.game_id])

  const loadConversations = useCallback(async (preserve = true) => {
    const requestID = ++conversationRequestRef.current
    if (!roomScope) {
      setConversations([])
      setSelected(null)
      setLoading(false)
      setTransitioningMode(null)
      return
    }
    // Background refreshes should never replace the open conversation with a
    // spinner. Only an initial load, room change, search, or tab change blocks
    // interaction while the new dataset is prepared.
    if (!preserve) setLoading(true)
    setError('')
    try {
      const result = await adminApi.chatConversations({ roomType: mode === 'service' ? 'service' : 'group', roomScope, channel: mode, query: appliedQuery, page: 1, pageSize: 60 })
      if (requestID !== conversationRequestRef.current) return
      const items = Array.isArray(result?.items) ? result.items : []
      const current = selectedRef.current
      const pending = pendingTargetRef.current
      const targetMatch = pending ? items.find(item => sameConversation(pending as AdminChatConversation, item)) : undefined
      const currentMatch = current ? items.find(item => sameConversation(current, item)) : undefined
      const nextSelected = targetMatch ?? (preserve && currentMatch && current ? current : items[0] ?? null)
      if (targetMatch) pendingTargetRef.current = null
      setConversations(items)
      if (!nextSelected || !sameConversation(current, nextSelected)) {
        messageRequestRef.current += 1
        setMessages([])
        setHasMore(false)
        setNextBeforeID(undefined)
        // Prevent a one-frame "no history" state before the message request
        // effect starts after selecting the first conversation in the new tab.
        setMessageLoading(Boolean(nextSelected))
      }
      setSelected(nextSelected)
    } catch (reason) {
      if (requestID === conversationRequestRef.current) {
        setError(reason instanceof Error ? reason.message : '读取会话失败')
        if (!preserve) {
          // A tab/room/search transition must never leave the previous
          // conversation actionable under the newly selected scope.
          messageRequestRef.current += 1
          setConversations([])
          setSelected(null)
          setMessages([])
          setHasMore(false)
          setNextBeforeID(undefined)
          setMessageLoading(false)
        }
      }
    } finally {
      if (requestID === conversationRequestRef.current) {
        setLoading(false)
        setTransitioningMode(null)
      }
    }
  }, [appliedQuery, mode, roomScope])

  const loadMessages = useCallback(async (conversation: AdminChatConversation, beforeId?: number, prepend = false, mergeLatest = false) => {
    const requestID = ++messageRequestRef.current
    setMessageLoading(true)
    try {
      const result = await adminApi.chatMessages({ scope: conversation.scope, roomScope: conversation.room_scope, gameId: conversation.game_id, roomType: conversation.room_type, beforeId, limit: 50 })
      if (requestID !== messageRequestRef.current || !sameConversation(selectedRef.current, conversation)) return
      const items = Array.isArray(result?.items) ? result.items : []
      setMessages(current => prepend || mergeLatest ? mergeAdminChatMessages(items, current) : mergeAdminChatMessages(items))
      setHasMore(Boolean(result?.has_more))
      setNextBeforeID(result?.next_before_id)
      setError('')
    } catch (reason) {
      if (requestID === messageRequestRef.current) setError(reason instanceof Error ? reason.message : '读取聊天记录失败')
    } finally {
      if (requestID === messageRequestRef.current) setMessageLoading(false)
    }
  }, [])

  useEffect(() => { const timer = window.setTimeout(() => void loadConversations(false), 0); return () => window.clearTimeout(timer) }, [loadConversations])
  useEffect(() => {
    let active = true
    void (async () => {
      const [firstAgents, firstTenants] = await Promise.all([adminApi.agents({ page: 1, pageSize: 100 }), adminApi.tenants({ page: 1, pageSize: 100 })])
      const agents = [...firstAgents.items]
      for (let page = 2; agents.length < firstAgents.total; page += 1) {
        const next = await adminApi.agents({ page, pageSize: 100 })
        agents.push(...next.items)
        if (next.items.length === 0) break
      }
      const tenants = [...firstTenants.items]
      for (let page = 2; tenants.length < firstTenants.total; page += 1) {
        const next = await adminApi.tenants({ page, pageSize: 100 })
        tenants.push(...next.items)
        if (next.items.length === 0) break
      }
      return [
        ...agents.map(item => ({ ...item, scope: `agent:${item.id}`, kind: 'agent' as const })),
        ...tenants.map(item => ({ ...item, scope: `tenant:${item.id}`, kind: 'tenant' as const })),
      ]
    })().then(async items => {
      if (!active) return
      const available = await Promise.all(items.filter(item => item.room_code.trim()).map(async item => {
        try {
          const settings = item.kind === 'tenant' ? await adminApi.tenantRoomSettings(item.id) : await adminApi.agentRoomSettings(item.id)
          return { id: item.id, scope: item.scope, kind: item.kind, status: item.status, room_code: item.room_code, room_name: settings.room_name, room_logo: settings.room_logo, chat_nickname: settings.chat_nickname, room_notice: settings.room_notice, announcements: settings.announcements }
        } catch {
          // Never expose the stale legacy agent_room_name/logo as the public
          // room identity when the workspace settings request is unavailable.
          return { id: item.id, scope: item.scope, kind: item.kind, status: item.status, room_code: item.room_code, room_name: '', room_logo: '', chat_nickname: '', room_notice: '', announcements: [] }
        }
      }))
      if (!active) return
      setRooms(available)
      setRoomScope(current => available.some(item => item.scope === current) ? current : (available[0]?.scope ?? ''))
    }).catch(reason => {
      if (active) setError(reason instanceof Error ? reason.message : '读取房间列表失败')
    })
    return () => { active = false }
  }, [])
  useEffect(() => {
    if (!selected) return
    const timer = window.setTimeout(() => void loadMessages(selected), 0)
    return () => window.clearTimeout(timer)
  }, [selected, loadMessages])
	useEffect(() => {
		if (selected?.room_type !== 'service' || document.visibilityState !== 'visible' || !document.hasFocus()) return
		const throughMessageID = messages.reduce((latest, item) => item.scope === selected.scope
			&& item.room_scope === selected.room_scope && item.game_id === selected.game_id && item.room_type === selected.room_type
			? Math.max(latest, item.id) : latest, 0)
		if (!throughMessageID) return
		const key = `${selected.scope}\u0000${selected.room_scope}\u0000${selected.game_id}\u0000${selected.room_type}`
		if (throughMessageID <= (markedThroughRef.current.get(key) ?? 0)) return
		markedThroughRef.current.set(key, throughMessageID)
		void adminApi.markChatRead({ scope: selected.scope, room_scope: selected.room_scope, game_id: selected.game_id, through_message_id: throughMessageID })
			.then(reportChatUnreadChanged)
			.catch(() => markedThroughRef.current.delete(key))
	}, [attentionRevision, messages, selected])

  useEffect(() => {
    const onRealtime = (event: Event) => {
      const detail = (event as CustomEvent<ManagementWsEvent>).detail
      if (detail?.type !== 'chat_message') return
      const current = selectedRef.current
      const data = detail.data ?? {}
      if (data.room_scope !== roomScope) return
      if (current
        && data.scope === current.scope
        && data.room_scope === current.room_scope
        && data.game_id === current.game_id
        && data.room_type === current.room_type) {
        void loadMessages(current, undefined, false, true)
      }
      // Also refresh the left list so a member's first message creates a new
      // customer-service conversation without any manual reload.
      void loadConversations()
    }
    window.addEventListener(MANAGEMENT_WS_EVENT, onRealtime)
    return () => window.removeEventListener(MANAGEMENT_WS_EVENT, onRealtime)
  }, [loadConversations, loadMessages, roomScope])

	useEffect(() => {
		if (lotteryView) return
		const onVisibility = () => {
			const current = selectedRef.current
			if (document.visibilityState === 'visible' && document.hasFocus() && current?.room_type === 'service') {
				setAttentionRevision(value => value + 1)
				void loadMessages(current, undefined, false, true)
			}
		}
		document.addEventListener('visibilitychange', onVisibility)
		window.addEventListener('focus', onVisibility)
		return () => { document.removeEventListener('visibilitychange', onVisibility); window.removeEventListener('focus', onVisibility) }
	}, [loadMessages, lotteryView])

  useEffect(() => {
    const current = selectedRef.current
    if (connectionStatusInitialized.current && current) {
      void loadMessages(current, undefined, false, true)
      void loadConversations()
    }
    connectionStatusInitialized.current = true
    if (websocketConnected) return
    const timer = window.setInterval(() => {
      const active = selectedRef.current
      if (active) void loadMessages(active, undefined, false, true)
      void loadConversations()
    }, 15_000)
    return () => window.clearInterval(timer)
  }, [loadConversations, loadMessages, websocketConnected])

	useEffect(() => {
		if (!lotteryView) return
		const onDraw = (event: Event) => {
			const detail = (event as CustomEvent<ManagementWsEvent>).detail
			if (detail?.type !== 'draw_update') return
			void adminApi.games().then(rows => setGames(Array.isArray(rows) ? rows : [])).catch(reason => setError(reason instanceof Error ? reason.message : '彩票信息暂时未更新'))
			const gameID = detail.game_id || String(detail.data?.game_id || '')
			if (gameID && gameID === selectedRef.current?.game_id) void adminApi.draws(gameID).then(rows => setDraws(Array.isArray(rows) ? rows : [])).catch(reason => setError(reason instanceof Error ? reason.message : '开奖记录暂时未更新'))
		}
		window.addEventListener(MANAGEMENT_WS_EVENT, onDraw)
		return () => window.removeEventListener(MANAGEMENT_WS_EVENT, onDraw)
	}, [lotteryView])

  const title = selected?.title ?? '选择一个会话'
  const subtitle = selected?.subtitle ?? '从左侧选择需要处理的会话'
  const conversationCount = useMemo(() => conversations.length, [conversations])
  const lotteryCategories = useMemo(() => Array.from(new Set(conversations.map(item => item.lobby_category?.trim()).filter((item): item is string => Boolean(item)))), [conversations])
  const activeLotteryCategory = lotteryCategories.includes(lotteryCategory) ? lotteryCategory : (lotteryCategories[0] ?? '')
  const visibleConversations = useMemo(() => view === 'lottery' && activeLotteryCategory
    ? conversations.filter(item => item.lobby_category?.trim() === activeLotteryCategory)
    : conversations, [activeLotteryCategory, conversations, view])
	const selectedGame = useMemo(() => games.find(game => game.id === selected?.game_id), [games, selected?.game_id])
	const activeRoom = useMemo(() => rooms.find(room => room.scope === roomScope), [roomScope, rooms])
	const activeRoomName = activeRoom?.room_name.trim() || (activeRoom?.room_code ? `房间 ${activeRoom.room_code}` : '当前房间')
	const activeRoomLogo = activeRoom?.room_logo.trim() || ''
	const staffTitle = activeRoom?.chat_nickname.trim() || '客服'
	const messageIdentity = (message: AdminChatMessage) => {
		if (!message.is_staff) return { name: message.nickname || message.username || '会员', badge: message.badge?.trim() || message.title?.trim() || '会员', avatar: memberAvatar(message.user_id, message.avatar) }
		const drawAssistant = message.username === 'draw_assistant'
		return { name: drawAssistant ? (message.nickname || '开奖助手') : staffTitle, badge: drawAssistant ? '开奖助手' : lotteryView ? '房间开奖员' : '房间客服', avatar: activeRoomLogo || undefined }
	}
	const latestDraw = draws[0]
	const openMember = async (message: AdminChatMessage) => {
		if (!message.user_id || message.is_staff) return
		setMemberLoading(true)
		setMemberInfo(null)
		setMemberActivity(null)
		try {
			const [profile, allBets, pendingBets] = await Promise.all([
				adminApi.user(message.user_id),
				adminApi.bets({ userId: message.user_id, page: 1, pageSize: 100 }),
				adminApi.bets({ userId: message.user_id, status: 'pending', page: 1, pageSize: 1 }),
			])
			const recent = Array.isArray(allBets?.items) ? allBets.items : []
			setMemberInfo(profile)
			setMemberActivity({ betCount: Number(allBets?.total) || 0, pendingCount: Number(pendingBets?.total) || 0, recentStake: recent.reduce((sum, item) => sum + Number(item.amount || 0), 0), recentPayout: recent.reduce((sum, item) => sum + Number(item.payout || 0), 0), sampleSize: recent.length })
		}
		catch (reason) { setError(reason instanceof Error ? reason.message : '读取会员资料失败') }
		finally { setMemberLoading(false) }
	}

  const reply = async () => {
    if (!selected || !draft.trim() || loading || transitioningMode) return
    setSaving(true)
    try {
      const message = await adminApi.replyChat({ scope: selected.scope, room_scope: selected.room_scope, game_id: selected.game_id, room_type: selected.room_type, content: draft.trim() })
      setMessages(current => mergeAdminChatMessages(current, [message]))
      setDraft('')
      await loadConversations()
      showMessage('回复已发送')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '发送失败') } finally { setSaving(false) }
  }

  const deleteMessage = async (id: number) => {
    setSaving(true)
    try {
      await adminApi.deleteChatMessage(id)
      setMessages(current => current.filter(item => item.id !== id))
      await loadConversations()
      showMessage('消息已撤回')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '撤回失败') } finally { setSaving(false) }
  }

  const setRoomGroupChat = async (enabled: boolean) => {
    if (view === 'lottery' || selected?.room_type !== 'group') return
    const agentID = Number(selected.room_scope.replace(/^agent:/, ''))
    if (!Number.isInteger(agentID) || agentID < 1) return
    setSaving(true)
    try {
      const next = await adminApi.setRoomGroupChat(agentID, enabled)
      setSelected(current => current ? { ...current, group_chat_enabled: next.group_chat_enabled } : current)
      setConversations(current => current.map(item => item.room_scope === selected.room_scope && item.game_id === 'lobby' && item.room_type === 'group' ? { ...item, group_chat_enabled: next.group_chat_enabled } : item))
      showMessage(enabled ? '已开放群聊' : '已开启群聊禁言')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '更新禁言失败') } finally { setSaving(false) }
  }

  const openRedPacket = () => {
    if (view === 'lottery' || !selected || selected.room_type !== 'group' || loading || transitioningMode) return
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
    if (view === 'lottery' || !selected || loading || transitioningMode || !Number.isInteger(count) || count < 1 || total < count * .01) return
    setSaving(true)
    try {
      const message = await adminApi.sendChatRedPacket({
        request_id: redPacketRequestID.current,
        scope: selected.scope,
        room_scope: selected.room_scope,
        game_id: selected.game_id,
        count,
        total_amount: total,
        min_daily_turnover: Math.max(0, Number(redPacketMinTurnover) || 0),
        greeting: redPacketGreeting.trim() || '恭喜发财',
        cover: redPacketCover,
      })
      setMessages(current => [...current, message])
      redPacketRequestID.current = createRequestId()
      setRedPacketOpen(false)
      showMessage('红包已发送到当前聊天室')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '发送红包失败') }
    finally { setSaving(false) }
  }

  return <Box p={{ xs: 2, lg: 2.5 }}>
    {error && <Alert severity="error" onClose={() => setError('')} action={<Button color="inherit" size="small" onClick={() => { void loadConversations(); const current = selectedRef.current; if (current) void loadMessages(current, undefined, false, true) }}>重试</Button>} sx={{ mt: 2 }}>{error}</Alert>}
    <Card sx={{
      mt: 1.25,
      width: '100%',
      maxWidth: lotteryView ? 1480 : 1280,
      mx: 'auto',
      height: { md: 'calc(100dvh - 170px)' },
      minHeight: { md: lotteryView ? 620 : 560 },
      maxHeight: { md: lotteryView ? 900 : 820 },
      overflow: 'hidden',
    }}>
      <Box sx={{
        display: 'grid',
        gridTemplateColumns: { xs: '1fr', md: lotteryView ? '340px minmax(0, 1fr)' : '310px minmax(0, 1fr)' },
        height: { md: '100%' },
        minHeight: 0,
        overflow: { md: 'hidden' },
      }}>
        <Box sx={{
          ...responsiveSplitPanelBorderSx,
          minHeight: { xs: 280, md: 0 },
          height: { md: '100%' },
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}>
          <Box px={1.3} py={1.1} borderBottom={1} borderColor="divider">
            <TextField fullWidth select size="small" label="房间号" value={roomScope} onChange={event => { setRedPacketOpen(false); setTransitioningMode(mode); setRoomScope(event.target.value) }} slotProps={{ select: { renderValue: value => {
              const room = rooms.find(item => item.scope === value)
              return room ? `${room.room_code} · ${room.room_name || `房间 ${room.room_code}`}` : ''
            } } }}>
              {rooms.length ? rooms.map(room => <MenuItem key={room.scope} value={room.scope}><Avatar src={room.room_logo || undefined} variant="rounded" sx={{ width: 26, height: 26, mr: .9, color: '#fff', background: room.room_logo ? undefined : identityGradient, fontSize: 10.5, fontWeight: 900 }}>{(room.room_name || '房').slice(0, 1)}</Avatar>{room.room_code} · {room.room_name || `房间 ${room.room_code}`}{room.kind === 'tenant' ? '（租户直属）' : ''}{room.status === 1 ? '' : '（停用）'}</MenuItem>) : <MenuItem value="" disabled>暂无可用房间</MenuItem>}
            </TextField>
          </Box>
          {lotteryView ? <><Stack direction="row" alignItems="center" gap={1.15} px={1.8} py={1.25}><Avatar src={activeRoomLogo || undefined} variant="rounded" sx={{ color: '#fff', background: activeRoomLogo ? undefined : identityGradient, width: 40, height: 40, fontWeight: 900 }}>{activeRoomName.slice(0, 1)}</Avatar><Box minWidth={0}><Typography fontWeight={850} fontSize={16} noWrap>{activeRoomName}</Typography><Typography fontSize={11} color="text.secondary" noWrap>房间号 {activeRoom?.room_code || '—'} · 彩票室</Typography></Box></Stack>{lotteryCategories.length > 0 && <Tabs value={activeLotteryCategory} onChange={(_, next: string) => {
            setLotteryCategory(next)
            setMessages([])
            setSelected(conversations.find(item => item.lobby_category?.trim() === next) ?? null)
          }} variant="scrollable" scrollButtons={false} sx={{ px: 1.15, minHeight: 38, borderTop: 1, borderBottom: 1, borderColor: 'divider', '& .MuiTab-root': { minWidth: 64, minHeight: 38, px: 1.35, py: .45, fontSize: 12, fontWeight: 850 } }}>{lotteryCategories.map(category => <Tab key={category} value={category} label={category} />)}</Tabs>}</> : <Tabs value={mode} onChange={(_, next: ChatMode) => { if (next === mode) return; setRedPacketOpen(false); setTransitioningMode(next); setMode(next) }} variant="fullWidth" sx={{ '& .MuiTab-root': { transition: 'color 160ms ease, background-color 160ms ease' }, '@media (prefers-reduced-motion: reduce)': { '& .MuiTab-root': { transition: 'none' } } }}>
            <Tab value="service" icon={<SupportAgentRounded />} iconPosition="start" label="在线客服" />
            <Tab value="room" icon={<ForumRounded />} iconPosition="start" label="房间群聊" />
          </Tabs>}
          <Box p={1.5}><TextField fullWidth size="small" value={query} placeholder={lotteryView ? '搜索彩种、房间号或消息内容' : '搜索会员、昵称或消息内容'} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') setAppliedQuery(query.trim()) }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /></Box>
          <Divider />
          <Box sx={{ position: 'relative', flex: 1, minHeight: 0, maxHeight: { xs: 300, md: 'none' }, overflow: 'hidden' }}>
            <Box aria-busy={loading} sx={{ height: '100%', overflowY: 'auto', overscrollBehavior: 'contain', pointerEvents: loading ? 'none' : 'auto', opacity: loading && conversations.length ? .66 : 1, transition: 'opacity 160ms ease', '@media (prefers-reduced-motion: reduce)': { transition: 'none' } }}>
              {visibleConversations.map(item => <Box key={`${item.room_type}:${item.scope}:${item.room_scope}:${item.game_id}`}>
              <Box component="div" role="button" tabIndex={0} onClick={() => {
              const next = selectConversation(selected, item, messages)
              setMessages(next.messages)
              setSelected(next.selected)
            }} onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); const next = selectConversation(selected, item, messages); setMessages(next.messages); setSelected(next.selected) } }} sx={{ display: 'block', width: '100%', border: 0, textAlign: 'left', cursor: 'pointer', px: 1.8, py: lotteryView ? 1.7 : 1.5, bgcolor: selected?.scope === item.scope && selected?.room_scope === item.room_scope && selected?.game_id === item.game_id && selected?.room_type === item.room_type ? 'action.selected' : 'transparent', color: 'inherit', '&:hover': { bgcolor: 'action.hover' } }}>
                <Stack direction="row" gap={1.1} alignItems="center"><Avatar src={lotteryView ? gameLogo(item.game_id) : undefined} sx={{ bgcolor: item.room_type === 'service' ? 'primary.main' : 'secondary.main', width: lotteryView ? 42 : 38, height: lotteryView ? 42 : 38 }}>{lotteryView ? item.title.slice(0, 1) : item.room_type === 'service' ? <SupportAgentRounded fontSize="small" /> : <ForumRounded fontSize="small" />}</Avatar><Box flex={1} minWidth={0}><Stack direction="row" gap={.5} alignItems="center"><Typography fontSize={lotteryView ? 14 : 12} fontWeight={800} sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{item.title}</Typography>{item.pinned && <Tooltip title="固定置顶"><PushPinRounded color="primary" sx={{ fontSize: 13, transform: 'rotate(-18deg)' }} /></Tooltip>}{!lotteryView && item.room_type === 'group' && !item.group_chat_enabled && <Tooltip title="群聊已禁言"><VolumeOffRounded color="warning" sx={{ fontSize: 14 }} /></Tooltip>}<Typography ml="auto" fontSize={lotteryView ? 10 : 9} color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>{dateTime(item.latest_at)}</Typography></Stack><Typography fontSize={lotteryView ? 11.5 : 10} color="text.secondary" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{item.subtitle}</Typography><Typography fontSize={lotteryView ? 12 : 11} mt={.35} color="text.secondary" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{conversationPreview(item)}</Typography></Box></Stack>
              </Box>
              </Box>)}
              {!conversations.length && !loading && <Box textAlign="center" py={7} color="text.secondary"><ForumRounded sx={{ opacity: .35, fontSize: 36 }} /><Typography fontSize={12}>暂无待处理会话</Typography></Box>}
              {!conversations.length && loading && <Stack aria-label="正在加载会话" gap={1.2} p={1.5}>{[0, 1, 2, 3].map(index => <Stack key={index} direction="row" gap={1.1} alignItems="center"><Skeleton variant="rounded" width={38} height={38} /><Box flex={1}><Skeleton width={index % 2 ? '48%' : '62%'} height={18} /><Skeleton width="86%" height={16} /></Box></Stack>)}</Stack>}
            </Box>
            {loading && conversations.length > 0 && <Stack role="status" aria-live="polite" direction="row" alignItems="center" justifyContent="center" gap={.8} sx={{ position: 'absolute', inset: 0, pointerEvents: 'none', color: 'text.secondary' }}><CircularProgress size={18} /><Typography fontSize={11} fontWeight={750}>{transitioningMode === 'service' ? '正在切换到在线客服' : transitioningMode === 'room' ? '正在切换到房间群聊' : '正在更新会话'}</Typography></Stack>}
          </Box>
          <Box px={1.8} py={1} borderTop={1} borderColor="divider"><Typography fontSize={10} color="text.secondary">{loading ? '正在读取当前分类…' : `当前 ${conversationCount} 个${lotteryView ? '彩票房间' : '会话'}${!lotteryView ? ' · 服务会话仅管理员可查看' : ''}`}</Typography></Box>
        </Box>
        <Box sx={{
          position: 'relative',
          minWidth: 0,
          minHeight: { xs: 420, md: 0 },
          height: { md: '100%' },
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}>
          <Stack direction="row" alignItems="center" gap={1} px={2} py={1.35} borderBottom={1} borderColor="divider"><Avatar src={lotteryView && selectedGame ? gameLogo(selectedGame.id) : undefined} sx={{ bgcolor: selected?.room_type === 'group' ? 'secondary.main' : 'primary.main', width: 36, height: 36 }}>{lotteryView && selectedGame ? selectedGame.name.slice(0, 1) : selected?.room_type === 'group' ? <ForumRounded fontSize="small" /> : <SupportAgentRounded fontSize="small" />}</Avatar><Box minWidth={0} flex={1}><Typography fontSize={13} fontWeight={850} noWrap>{title}</Typography><Typography fontSize={10} color="text.secondary" noWrap>{subtitle}</Typography></Box>{!lotteryView && selected?.room_type === 'group' && <Button size="small" color="secondary" startIcon={<CardGiftcardRounded />} onClick={openRedPacket}>发红包</Button>}{!lotteryView && mode === 'room' && selected?.room_type === 'group' && selected.room_scope.startsWith('agent:') && (selected.group_chat_enabled ? <Button size="small" color="warning" startIcon={<VolumeOffRounded />} disabled={saving} onClick={() => void setRoomGroupChat(false)}>开启禁言</Button> : <Button size="small" color="success" startIcon={<VolumeUpRounded />} disabled={saving} onClick={() => void setRoomGroupChat(true)}>关闭禁言</Button>)}</Stack>
			{lotteryView && selectedGame && <Box px={{ xs: 1.4, md: 2 }} py={1.15} borderBottom={1} borderColor="divider" sx={{ background: theme => theme.palette.mode === 'dark' ? 'linear-gradient(90deg,rgba(14,165,233,.14),rgba(139,92,246,.08))' : 'linear-gradient(90deg,#effbff,#f5f2ff)' }}>
				<Stack direction={{ xs: 'column', sm: 'row' }} gap={{ xs: 1, sm: 1.8 }} alignItems={{ sm: 'center' }}>
					<Stack direction="row" gap={1} alignItems="center" minWidth={{ sm: 170 }}><Avatar src={gameLogo(selectedGame.id)} sx={{ width: 44, height: 44, bgcolor: 'background.paper', border: 1, borderColor: 'divider' }}>{selectedGame.name.slice(0, 1)}</Avatar><Box minWidth={0}><Typography fontSize={14} fontWeight={900} noWrap>{selectedGame.name}</Typography><Typography fontSize={10} color="text.secondary" noWrap>第 {selectedGame.current_issue || selectedGame.issue || '—'} 期</Typography><Typography fontSize={9.5} color={selectedGame.source_healthy === false ? 'warning.main' : 'success.main'} noWrap>{selectedGame.source_name || (selectedGame.source_kind === 'official' ? '外部开奖源' : '平台开奖')} · {selectedGame.source_healthy === false ? '异常' : '正常'}</Typography></Box></Stack>
					<Box minWidth={88}><Typography fontSize={9.5} color="text.secondary">封盘倒计时</Typography><Typography fontSize={23} lineHeight={1.1} fontWeight={950} color={new Date(selectedGame.seal_at || selectedGame.next_draw_at).getTime() <= now ? 'warning.main' : 'primary.main'}>{countdownText(selectedGame.seal_at || selectedGame.next_draw_at, now)}</Typography></Box>
					<Box flex={1} minWidth={0}><Stack direction="row" alignItems="center" gap={.7} mb={.55}><Typography fontSize={9.5} color="text.secondary">上期 {latestDraw?.issue || '—'}</Typography><Chip size="small" label={issueStatusText[selectedGame.issue_status || ''] || '运行中'} color={selectedGame.issue_status === 'abnormal' || selectedGame.source_healthy === false ? 'warning' : 'success'} sx={{ height: 19, fontSize: 9 }} /></Stack><Stack direction="row" gap={.42} flexWrap="wrap" useFlexGap>{(latestDraw?.numbers || selectedGame.latest_numbers || []).map((number, index) => <Box key={`${index}-${number}`} sx={{ width: 25, height: 25, display: 'grid', placeItems: 'center', borderRadius: 1, bgcolor: ballColor(number), color: '#fff', fontSize: 11, fontWeight: 950, boxShadow: '0 2px 5px rgba(15,23,42,.18)' }}>{number}</Box>)}{!(latestDraw?.numbers || selectedGame.latest_numbers || []).length && <Typography fontSize={11} color="text.secondary">等待开奖数据</Typography>}</Stack></Box>
				</Stack>
			</Box>}
          <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overscrollBehavior: 'contain', p: { xs: 1.5, md: 2 }, bgcolor: 'background.default' }}>
            {selected && hasMore && <Box textAlign="center" mb={1.5}><Button size="small" startIcon={messageLoading ? <CircularProgress size={13} /> : <ArrowUpwardRounded />} disabled={messageLoading} onClick={() => void loadMessages(selected, nextBeforeID, true)}>加载更早消息</Button></Box>}
            {!selected ? <Box height="100%" display="grid" sx={{ placeItems: 'center' }} color="text.secondary"><Typography>请选择一个会话开始处理</Typography></Box> : <Stack gap={1.25} sx={{ width: '100%', maxWidth: 860, mx: 'auto' }}>
              {messages.map(item => {
				const identity = messageIdentity(item)
				return <Stack key={item.id} direction={item.is_staff ? 'row-reverse' : 'row'} gap={.8} alignItems="flex-start">
				<Tooltip title={item.is_staff ? identity.badge : '查看会员资料'}><Avatar src={identity.avatar} component={item.is_staff ? 'div' : 'button'} onClick={() => void openMember(item)} sx={{ width: 31, height: 31, fontSize: 11, color: '#fff', background: item.is_staff && !identity.avatar ? identityGradient : undefined, flexShrink: 0, border: item.is_staff ? 0 : 1, borderColor: 'divider', cursor: item.is_staff ? 'default' : 'pointer', '&:hover': item.is_staff ? undefined : { boxShadow: '0 0 0 3px rgba(14,165,233,.2)' } }}>{identity.name.slice(0, 1)}</Avatar></Tooltip>
                <Box sx={{ width: 'fit-content', minWidth: 0, maxWidth: { xs: '86%', md: lotteryView ? 680 : 560 } }}>
                  <Stack direction="row" gap={.7} justifyContent={item.is_staff ? 'flex-end' : 'flex-start'}>
                    <Typography fontSize={10} color="text.secondary" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: { xs: 170, md: 220 } }}>{identity.name}</Typography><Chip size="small" variant="outlined" color={item.is_staff ? 'primary' : 'default'} label={identity.badge} sx={{ height: 17, fontSize: 8, '& .MuiChip-label': { px: .55 } }} />
                  </Stack>
                  {item.message_type === 'redpacket' ? <Box mt={.3}>
                    <AdminRedPacketCard
                      count={item.red_packet_count || 1}
                      total={Number(item.red_packet_total || 0)}
                      minTurnover={Number(item.red_packet_min_turnover || 0)}
                      greeting={item.content}
                      cover={item.red_packet_cover}
                      status={item.red_packet_status}
                      claimedCount={Number(item.red_packet_claimed_count || 0)}
                      refunded={Number(item.red_packet_refunded || 0)}
                      closeReason={item.red_packet_close_reason}
                      time={dateTime(item.created_at)}
                      action={<Tooltip title="撤回"><IconButton size="small" disabled={saving} onClick={() => void deleteMessage(item.id)} sx={{ color: 'inherit', opacity: .8, p: .1 }}><DeleteOutlineRounded sx={{ fontSize: 14 }} /></IconButton></Tooltip>}
                    />
                  </Box> : <Paper sx={{ mt: .3, px: 1.25, pt: 1, pb: .65, bgcolor: item.is_staff ? 'primary.main' : 'background.paper', color: item.is_staff ? 'primary.contrastText' : 'text.primary', borderRadius: item.is_staff ? '14px 3px 14px 14px' : '3px 14px 14px 14px', boxShadow: 'none' }}>
                    <Typography fontSize={13} sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', wordBreak: 'break-word', lineHeight: 1.55 }}>{item.content}</Typography>
                    <Stack direction="row" justifyContent="flex-end" alignItems="center" gap={.35} mt={.2}><Typography fontSize={9} sx={{ opacity: .7 }}>{dateTime(item.created_at)}</Typography><Tooltip title="撤回"><IconButton size="small" disabled={saving} onClick={() => void deleteMessage(item.id)} sx={{ color: 'inherit', opacity: .68, p: .15 }}><DeleteOutlineRounded sx={{ fontSize: 14 }} /></IconButton></Tooltip></Stack>
                  </Paper>}
                </Box>
				</Stack>
			})}
            </Stack>}
            {selected && messageLoading && !messages.length && <Stack aria-label="正在加载聊天记录" gap={1.6} sx={{ width: '100%', maxWidth: 720, mx: 'auto', pt: 1 }}>
              <Stack direction="row" gap={1} alignItems="flex-start"><Skeleton variant="circular" width={31} height={31} /><Box width="42%"><Skeleton width="38%" height={16} /><Skeleton variant="rounded" width="100%" height={58} /></Box></Stack>
              <Stack direction="row-reverse" gap={1} alignItems="flex-start"><Skeleton variant="circular" width={31} height={31} /><Box width="54%"><Skeleton width="35%" height={16} sx={{ ml: 'auto' }} /><Skeleton variant="rounded" width="100%" height={72} /></Box></Stack>
              <Stack direction="row" gap={1} alignItems="flex-start"><Skeleton variant="circular" width={31} height={31} /><Box width="34%"><Skeleton width="42%" height={16} /><Skeleton variant="rounded" width="100%" height={46} /></Box></Stack>
            </Stack>}
            {selected && !messages.length && !messageLoading && <Box textAlign="center" py={8} color="text.secondary"><Typography fontSize={12}>暂无聊天记录</Typography></Box>}
          </Box>
          <Divider />
          <Box p={1.4}><Stack direction="row" gap={1} alignItems="flex-end"><TextField fullWidth multiline maxRows={4} placeholder={selected ? '输入回复内容，Enter 发送，Shift + Enter 换行' : '请先选择会话'} disabled={!selected || saving || Boolean(transitioningMode)} value={draft} onChange={event => setDraft(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void reply() } }} inputProps={{ maxLength: 500 }} /><Button size="small" variant="contained" startIcon={<SendRounded sx={{ fontSize: '17px !important' }} />} disabled={!selected || !draft.trim() || saving || Boolean(transitioningMode)} onClick={() => void reply()} sx={{ flex: '0 0 auto', minWidth: 78, height: 40, px: 1.3, whiteSpace: 'nowrap', fontSize: 12 }}>发送</Button></Stack></Box>
          {loading && transitioningMode && <Stack role="status" aria-live="polite" alignItems="center" justifyContent="center" sx={{ position: 'absolute', zIndex: 3, inset: 0, color: 'text.primary', bgcolor: theme => theme.palette.mode === 'dark' ? 'rgba(7,26,46,.34)' : 'rgba(247,251,252,.40)', transition: 'opacity 160ms ease', '@media (prefers-reduced-motion: reduce)': { transition: 'none' } }}><Box sx={{ display: 'flex', alignItems: 'center', gap: .8, px: 1.4, py: .9, border: 1, borderColor: 'divider', borderRadius: 1.2, bgcolor: 'background.paper', boxShadow: 3 }}><CircularProgress size={19} /><Typography fontSize={11.5} fontWeight={800}>{transitioningMode === 'service' ? '正在切换到在线客服' : '正在切换到房间群聊'}</Typography></Box></Stack>}
        </Box>
      </Box>
    </Card>
    <Dialog open={!lotteryView && redPacketOpen} onClose={() => !saving && setRedPacketOpen(false)} fullWidth maxWidth="sm" slotProps={{ paper: { sx: { width: 'min(560px, calc(100% - 24px))', maxHeight: 'calc(100dvh - 32px)', borderRadius: 2, overflow: 'hidden' } } }}><DialogTitle sx={{ color: '#fff', background: 'linear-gradient(135deg,#d94b45,#ed7954)' }}><Typography fontSize={18} fontWeight={900}>发送房间红包</Typography><Typography fontSize={10.5} sx={{ opacity: .82 }}>红包会实时发送到当前房间聊天室</Typography></DialogTitle><DialogContent sx={{ pt: '18px !important', bgcolor: 'background.default' }}><RedPacketForm count={redPacketCount} total={redPacketTotal} greeting={redPacketGreeting} cover={redPacketCover} minTurnover={redPacketMinTurnover} onCount={setRedPacketCount} onTotal={setRedPacketTotal} onGreeting={setRedPacketGreeting} onCover={setRedPacketCover} onMinTurnover={setRedPacketMinTurnover} /></DialogContent><DialogActions sx={{ px: 2.5, py: 1.25, bgcolor: 'background.paper' }}><Button size="small" onClick={() => setRedPacketOpen(false)}>取消</Button><Button size="small" variant="contained" color="error" disabled={saving || loading || Boolean(transitioningMode) || !Number.isInteger(Number(redPacketCount)) || Number(redPacketCount) < 1 || Number(redPacketTotal) < Number(redPacketCount) * .01 || Number(redPacketMinTurnover) < 0} onClick={() => void sendRedPacket()} sx={{ minWidth: 88, height: 34, px: 1.5 }}>{saving ? '发送中…' : '发送红包'}</Button></DialogActions></Dialog>
		<Dialog open={Boolean(memberInfo) || memberLoading} onClose={() => !memberLoading && setMemberInfo(null)} fullWidth maxWidth="sm"><DialogTitle>会员资料</DialogTitle><DialogContent dividers>{memberLoading && !memberInfo ? <Box py={5} textAlign="center"><CircularProgress size={26} /><Typography mt={1} fontSize={12} color="text.secondary">正在读取会员与注单资料…</Typography></Box> : memberInfo && <Stack gap={1.5}><Stack direction="row" alignItems="center" gap={1.2}><Avatar src={memberAvatar(memberInfo.id, memberInfo.robot_avatar || memberInfo.avatar)} sx={{ width: 56, height: 56, border: 1, borderColor: 'divider' }}>{(memberInfo.nickname || memberInfo.username).slice(0, 1)}</Avatar><Box minWidth={0} flex={1}><Stack direction="row" alignItems="center" gap={.65} flexWrap="wrap"><Typography fontSize={16} fontWeight={900} noWrap>{memberInfo.nickname || memberInfo.username}</Typography>{memberInfo.public_title?.trim() && <Chip size="small" color="primary" variant="outlined" label={memberInfo.public_title.trim()} />}{memberInfo.badge?.trim() && <Chip size="small" color="warning" variant="outlined" label={memberInfo.badge.trim()} />}{memberInfo.is_robot && <Chip size="small" color="secondary" label="房间机器人" />}</Stack><Typography fontSize={11} color="text.secondary">会员 ID {memberInfo.public_id} · @{memberInfo.username}</Typography></Box><Chip size="small" color={memberInfo.status === 1 ? 'success' : 'default'} label={memberInfo.status === 1 ? '账号正常' : '账号停用'} /></Stack><Box display="grid" gridTemplateColumns={{ xs: '1fr 1fr', sm: 'repeat(4,1fr)' }} gap={1}>{[['可用积分', memberInfo.balance.toLocaleString('zh-CN', { minimumFractionDigits: 2 })], ['总注单', `${memberActivity?.betCount ?? 0} 笔`], ['待结算', `${memberActivity?.pendingCount ?? 0} 笔`], ['登录次数', `${memberInfo.login_count} 次`]].map(([label, value]) => <Paper key={label} variant="outlined" sx={{ p: 1.05 }}><Typography fontSize={9.5} color="text.secondary">{label}</Typography><Typography fontSize={13} fontWeight={900}>{value}</Typography></Paper>)}</Box>{memberActivity && <Paper variant="outlined" sx={{ p: 1.15 }}><Typography fontSize={10} color="text.secondary" mb={.6}>最近 {memberActivity.sampleSize} 笔注单活动</Typography><Stack direction="row" justifyContent="space-between"><Box><Typography fontSize={10} color="text.secondary">投注额</Typography><Typography fontWeight={900}>¥ {memberActivity.recentStake.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</Typography></Box><Box textAlign="right"><Typography fontSize={10} color="text.secondary">派彩额</Typography><Typography fontWeight={900}>¥ {memberActivity.recentPayout.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</Typography></Box></Stack></Paper>}<Divider /><Box display="grid" gridTemplateColumns={{ xs: '1fr', sm: '1fr 1fr' }} columnGap={2.5} rowGap={1}>{[['账号角色', memberInfo.role === 'member' ? '会员' : memberInfo.role], ['风险等级', memberInfo.risk_level === 'normal' ? '正常' : memberInfo.risk_level === 'watch' ? '关注' : '受限'], ['所属房间', `${activeRoomName}${memberInfo.room_code || activeRoom?.room_code ? ` · ${memberInfo.room_code || activeRoom?.room_code}` : ''}`], ['最近登录', dateTime(memberInfo.last_login_at || undefined)], ['注册时间', dateTime(memberInfo.created_at)], ['联系电话', memberInfo.phone || '未填写']].map(([label, value]) => <Stack key={label} direction="row" justifyContent="space-between" gap={1}><Typography color="text.secondary" fontSize={11}>{label}</Typography><Typography fontSize={11} fontWeight={750} textAlign="right">{value}</Typography></Stack>)}</Box>{memberInfo.remark && <Alert severity="info" icon={false}><Typography fontSize={11}><Box component="span" fontWeight={900}>备注：</Box>{memberInfo.remark}</Typography></Alert>}</Stack>}</DialogContent><DialogActions><Button onClick={() => setMemberInfo(null)}>关闭</Button></DialogActions></Dialog>
  </Box>
}
