import {
  Alert, Avatar, Box, Button, Card, CardContent, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle,
  Divider, FormControlLabel, List, ListItemButton, ListItemText, MenuItem, Paper, Stack, Switch, Table, TableBody,
  TableCell, TableContainer, TableHead, TableRow, Tab, Tabs, TextField, Typography,
} from '@mui/material'
import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
import CardGiftcardRounded from '@mui/icons-material/CardGiftcardRounded'
import PhotoCameraRounded from '@mui/icons-material/PhotoCameraRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import { agentApi, tenantApi, type AdminApplication, type AdminBet, type AdminChatConversation, type AdminChatMessage, type AdminUser, type AgentDashboard, type ManagementWsEvent } from '../api'
import { useFeedback } from '../components/feedback'
import { OperatingReportPanel } from '../components/OperatingReportPanel'
import { AdminRedPacketCard, RedPacketForm, type RedPacketCover } from '../components/RedPacketForm'
import { MANAGEMENT_WS_EVENT } from '../hooks/useManagementWebSocket'
import { prepareRoomLogo } from '../utils/roomLogo'

const money = (value: number) => value.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const time = (value: string) => new Date(value).toLocaleString('zh-CN', { hour12: false })
type ApplicationCategory = 'wallet' | 'join' | 'entertainment'

export function AgentWorkspacePage({ section, tenantAgentId, embedded = false }: {
  section: 'dashboard' | 'users' | 'applications' | 'room-reviews' | 'bets' | 'chat' | 'lottery-chat' | 'reports'
  tenantAgentId?: number
  embedded?: boolean
}) {
  const chatSection = section === 'chat' || section === 'lottery-chat'
  const lotteryChat = section === 'lottery-chat'
  const { showMessage } = useFeedback()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [dashboard, setDashboard] = useState<AgentDashboard | null>(null)
  const [users, setUsers] = useState<AdminUser[]>([])
  const [applications, setApplications] = useState<AdminApplication[]>([])
  const [applicationCategory, setApplicationCategory] = useState<ApplicationCategory>(section === 'room-reviews' ? 'join' : 'wallet')
  const [bets, setBets] = useState<AdminBet[]>([])
  const [conversations, setConversations] = useState<AdminChatConversation[]>([])
  const [selected, setSelected] = useState<AdminChatConversation | null>(null)
  const [messages, setMessages] = useState<AdminChatMessage[]>([])
  const [query, setQuery] = useState('')
  const [balanceUser, setBalanceUser] = useState<AdminUser | null>(null)
  const [amount, setAmount] = useState('')
  const [remark, setRemark] = useState('')
  const [reviewing, setReviewing] = useState<AdminApplication | null>(null)
  const [reply, setReply] = useState('')
  const [robotRunning, setRobotRunning] = useState(false)
  const [chatMode, setChatMode] = useState<'service' | 'room'>('service')
  const [redPacketOpen, setRedPacketOpen] = useState(false)
  const [redPacketCount, setRedPacketCount] = useState('10')
  const [redPacketTotal, setRedPacketTotal] = useState('100')
  const [redPacketGreeting, setRedPacketGreeting] = useState('恭喜发财')
  const [redPacketCover, setRedPacketCover] = useState<RedPacketCover>('classic')
  const [redPacketMinTurnover, setRedPacketMinTurnover] = useState('0')
  const [roomName, setRoomName] = useState('')
  const [roomLogo, setRoomLogo] = useState('')
  const [roomSaving, setRoomSaving] = useState(false)
  const [switchingLotteryGame, setSwitchingLotteryGame] = useState('')
  const roomApi = useMemo(() => ({
    dashboard: () => tenantAgentId ? tenantApi.roomDashboard(tenantAgentId) : agentApi.dashboard(),
    updateRoomSettings: (name: string, logo: string) => tenantAgentId ? tenantApi.updateRoomSettings(tenantAgentId, name, logo) : agentApi.updateRoomSettings(name, logo),
    users: (params?: Parameters<typeof agentApi.users>[0]) => tenantAgentId ? tenantApi.roomUsers(tenantAgentId, params) : agentApi.users(params),
    setUserStatus: (id: number, status: 0 | 1) => tenantAgentId ? tenantApi.setRoomUserStatus(tenantAgentId, id, status) : agentApi.setUserStatus(id, status),
    adjustUserBalance: (id: number, value: number, note: string) => tenantAgentId ? tenantApi.adjustRoomUserBalance(tenantAgentId, id, value, note) : agentApi.adjustUserBalance(id, value, note),
    bets: (params?: Parameters<typeof agentApi.bets>[0]) => tenantAgentId ? tenantApi.roomBets(tenantAgentId, params) : agentApi.bets(params),
    applications: (params?: Parameters<typeof agentApi.applications>[0]) => tenantAgentId ? tenantApi.roomApplications(tenantAgentId, params) : agentApi.applications(params),
    reviewApplication: (id: number, payload: Parameters<typeof agentApi.reviewApplication>[1]) => tenantAgentId ? tenantApi.reviewRoomApplication(tenantAgentId, id, payload) : agentApi.reviewApplication(id, payload),
    chatConversations: (params: Parameters<typeof agentApi.chatConversations>[0]) => tenantAgentId ? tenantApi.roomChatConversations(tenantAgentId, params) : agentApi.chatConversations(params),
    chatMessages: (params: Parameters<typeof agentApi.chatMessages>[0]) => tenantAgentId ? tenantApi.roomChatMessages(tenantAgentId, params) : agentApi.chatMessages(params),
    replyChat: (payload: Parameters<typeof agentApi.replyChat>[0]) => tenantAgentId ? tenantApi.replyRoomChat(tenantAgentId, payload) : agentApi.replyChat(payload),
    sendChatRedPacket: (payload: Parameters<typeof agentApi.sendChatRedPacket>[0]) => tenantAgentId ? tenantApi.sendRoomRedPacket(tenantAgentId, payload) : agentApi.sendChatRedPacket(payload),
    setLotteryRoomStatus: (gameId: string, enabled: boolean) => tenantAgentId ? tenantApi.setRoomLotteryStatus(tenantAgentId, gameId, enabled) : agentApi.setLotteryRoomStatus(gameId, enabled),
    runRobotOnce: () => tenantAgentId ? tenantApi.runRoomRobotOnce(tenantAgentId) : agentApi.runRobotOnce(),
  }), [tenantAgentId])
  const selectedRef = useRef<AdminChatConversation | null>(null)
  useEffect(() => { selectedRef.current = selected }, [selected])

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const head = await roomApi.dashboard(); setDashboard(head); setRoomName(head.room_name ?? ''); setRoomLogo(head.room_logo ?? '')
      if (section === 'users') setUsers((await roomApi.users({ query })).items)
      if (section === 'applications' || section === 'room-reviews') setApplications((await roomApi.applications({ query, type: applicationCategory })).items)
      if (section === 'bets') setBets((await roomApi.bets({ query })).items)
      if (chatSection) {
        const channel = lotteryChat ? 'lottery' : chatMode
        const rows = (await roomApi.chatConversations({ query, channel, roomType: channel === 'service' ? 'service' : 'group' })).items
        setConversations(rows)
        setSelected(current => {
          if (!current) return rows[0] ?? null
          return rows.find(row => row.scope === current.scope
            && row.room_scope === current.room_scope
            && row.game_id === current.game_id
            && row.room_type === current.room_type) ?? rows[0] ?? null
        })
      }
    } catch (reason) { setError(reason instanceof Error ? reason.message : '读取房间数据失败') }
    finally { setLoading(false) }
  }, [applicationCategory, chatMode, chatSection, lotteryChat, query, roomApi, section])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])
  const loadChatMessages = useCallback(async (conversation: AdminChatConversation) => {
    try {
      const result = await roomApi.chatMessages({ scope: conversation.scope, roomScope: conversation.room_scope, gameId: conversation.game_id, roomType: conversation.room_type })
      setMessages(result.items)
    } catch { /* The live channel will retry after reconnecting. */ }
  }, [roomApi])

  useEffect(() => {
    if (!selected) return
    const initial = window.setTimeout(() => { setMessages([]); void loadChatMessages(selected) }, 0)
    return () => window.clearTimeout(initial)
  }, [loadChatMessages, selected])

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
        void loadChatMessages(current)
      }
      void load()
    }
    window.addEventListener(MANAGEMENT_WS_EVENT, onRealtime)
    return () => window.removeEventListener(MANAGEMENT_WS_EVENT, onRealtime)
  }, [chatSection, load, loadChatMessages])

  const cards = useMemo(() => dashboard ? [
    ['房间成员', `${dashboard.active_member_count} / ${dashboard.member_count}`], ['成员余额', `¥ ${money(dashboard.member_balance)}`],
    ['今日投注', `¥ ${money(dashboard.today_stake)}`], ['今日派彩', `¥ ${money(dashboard.today_payout)}`],
    ['今日净额', `¥ ${money(dashboard.today_net)}`], ['待处理', `${dashboard.pending_applications} 申请 · ${dashboard.pending_bets} 注单`],
  ] : [], [dashboard])

  const adjustBalance = async () => {
    if (!balanceUser || !Number(amount) || !remark.trim()) return
    try { await roomApi.adjustUserBalance(balanceUser.id, Number(amount), remark.trim()); showMessage('余额调整成功'); setBalanceUser(null); setAmount(''); setRemark(''); await load() }
    catch (reason) { showMessage(reason instanceof Error ? reason.message : '余额调整失败', 'error') }
  }
  const saveRoomProfile = async () => {
    const next = roomName.trim()
    if (next.length < 2 || next.length > 30) {
      showMessage('房间名称长度需为 2–30 个字符', 'error')
      return
    }
    setRoomSaving(true)
    try {
      const result = await roomApi.updateRoomSettings(next, roomLogo)
      setDashboard(result)
      setRoomName(result.room_name)
      setRoomLogo(result.room_logo ?? '')
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
  const review = async (decision: 'approved' | 'rejected') => {
    if (!reviewing) return
    try { await roomApi.reviewApplication(reviewing.id, { decision, received_amount: decision === 'approved' ? reviewing.requested_amount : 0, remark: decision === 'approved' ? '房间审核通过' : '房间审核拒绝' }); showMessage('申请已处理'); setReviewing(null); await load() }
    catch (reason) { showMessage(reason instanceof Error ? reason.message : '审核失败', 'error') }
  }
  const sendReply = async () => {
    if (!selected || !reply.trim()) return
    try { const row = await roomApi.replyChat({ scope: selected.scope, room_scope: selected.room_scope, game_id: selected.game_id, room_type: selected.room_type, content: reply.trim() }); setMessages(current => [...current, row]); setReply('') }
    catch (reason) { showMessage(reason instanceof Error ? reason.message : '发送失败', 'error') }
  }
  const openRedPacket = () => {
    if (lotteryChat || !selected || selected.room_type !== 'group') return
    setRedPacketCount('10')
    setRedPacketTotal('100')
    setRedPacketGreeting('恭喜发财')
    setRedPacketCover('classic')
    setRedPacketMinTurnover('0')
    setRedPacketOpen(true)
  }
  const sendRedPacket = async () => {
    const count = Number(redPacketCount)
    const total = Number(redPacketTotal)
    if (lotteryChat || !selected || !Number.isInteger(count) || count < 1 || total < count * .01) return
    try { const row = await roomApi.sendChatRedPacket({ game_id: selected.game_id, count, total_amount: total, min_daily_turnover: Math.max(0, Number(redPacketMinTurnover) || 0), greeting: redPacketGreeting.trim() || '恭喜发财', cover: redPacketCover }); setMessages(current => [...current, row]); setRedPacketOpen(false); showMessage('红包已发送到当前聊天室') }
    catch (reason) { showMessage(reason instanceof Error ? reason.message : '发送红包失败', 'error') }
  }
  const setLotteryRoomEnabled = async (conversation: AdminChatConversation, enabled: boolean) => {
    setSwitchingLotteryGame(conversation.game_id)
    try {
      const result = await roomApi.setLotteryRoomStatus(conversation.game_id, enabled)
      setConversations(current => current.map(row => row.game_id === conversation.game_id ? { ...row, enabled: result.enabled } : row))
      setSelected(current => current?.game_id === conversation.game_id ? { ...current, enabled: result.enabled } : current)
      showMessage(`${conversation.title}已${result.enabled ? '开启' : '关闭'}`)
    } catch (reason) {
      showMessage(reason instanceof Error ? reason.message : '保存彩票室状态失败', 'error')
    } finally {
      setSwitchingLotteryGame('')
    }
  }

  return <Box p={embedded ? 0 : { xs: 1.5, md: 2.5 }}>
    <Stack direction="row" justifyContent="flex-end" mb={1.5}>
      <Button variant="outlined" onClick={() => void load()}>刷新</Button>
    </Stack>
    {error && <Alert severity="error" sx={{ mb: 2 }} action={<Button onClick={() => void load()}>重试</Button>}>{error}</Alert>}
    {loading && <Box py={1}><CircularProgress size={20} /></Box>}

    {section === 'dashboard' && <>
      <Card sx={{ mb: 2 }}><CardContent><Stack direction={{ xs: 'column', md: 'row' }} alignItems={{ md: 'center' }} gap={1.5}><Avatar src={roomLogo || undefined} variant="rounded" sx={{ width: 64, height: 64, bgcolor: 'primary.main', fontWeight: 900, fontSize: 24 }}>{(roomName || '房').slice(0, 1)}</Avatar><Box flex={1}><Typography fontWeight={850}>房间资料</Typography><Typography variant="caption" color="text.secondary">房间号 {dashboard?.room_code || '—'} · 名称和 Logo 会显示给进入该房间的用户</Typography><Stack direction="row" gap={.5} mt={.7}><Button component="label" size="small" startIcon={<PhotoCameraRounded />}>{roomLogo ? '更换 Logo' : '选择 Logo'}<input hidden type="file" accept="image/png,image/jpeg,image/webp" onChange={chooseRoomLogo} /></Button>{roomLogo && <Button color="error" size="small" startIcon={<DeleteOutlineRounded />} onClick={() => setRoomLogo('')}>移除</Button>}</Stack></Box><TextField size="small" label="房间名称" value={roomName} onChange={event => setRoomName(event.target.value)} inputProps={{ maxLength: 30 }} sx={{ width: { xs: '100%', md: 260 } }} /><Button variant="contained" disabled={roomSaving || !dashboard} onClick={() => void saveRoomProfile()}>{roomSaving ? '保存中…' : '保存资料'}</Button></Stack></CardContent></Card>
      <Box display="grid" gridTemplateColumns={{ xs: '1fr 1fr', lg: 'repeat(3,1fr)' }} gap={1.5}>{cards.map(([label, value]) => <Card key={label}><CardContent><Typography variant="caption" color="text.secondary">{label}</Typography><Typography variant="h6" fontWeight={850}>{value}</Typography></CardContent></Card>)}</Box>
      <Card sx={{ mt: 2 }}><CardContent><Typography fontWeight={800}>房间机器人</Typography><Typography variant="body2" color="text.secondary" mb={1.5}>使用普通会员身份在本房间持久化下注和聊天，可立即执行一轮。</Typography><Button disabled={robotRunning} variant="contained" onClick={() => { setRobotRunning(true); void roomApi.runRobotOnce().then(() => showMessage('本房间已执行一轮')).catch(reason => showMessage(reason instanceof Error ? reason.message : '执行失败', 'error')).finally(() => setRobotRunning(false)) }}>{robotRunning ? '执行中…' : '立即执行'}</Button></CardContent></Card>
    </>}

    {section === 'reports' && <OperatingReportPanel agent tenantAgentId={tenantAgentId} />}

    {section !== 'dashboard' && !chatSection && section !== 'reports' && <Paper variant="outlined" sx={{ p: 1.3, mb: 1.5 }}><Stack direction="row" gap={1}><TextField size="small" fullWidth placeholder="搜索当前房间数据" value={query} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') void load() }} /><Button variant="contained" onClick={() => void load()}>查询</Button></Stack></Paper>}

    {section === 'users' && <Card><TableContainer><Table size="small"><TableHead><TableRow><TableCell>用户</TableCell><TableCell>余额</TableCell><TableCell>状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>{users.map(row => <TableRow key={row.id}><TableCell><Typography fontWeight={750}>{row.nickname || row.username}</Typography><Typography variant="caption" color="text.secondary">@{row.username} · {row.public_id}</Typography></TableCell><TableCell>¥ {money(row.balance)}</TableCell><TableCell><FormControlLabel control={<Switch size="small" checked={row.status === 1} onChange={async () => { await roomApi.setUserStatus(row.id, row.status === 1 ? 0 : 1); await load() }} />} label={row.status === 1 ? '正常' : '停用'} /></TableCell><TableCell align="right"><Button size="small" onClick={() => setBalanceUser(row)}>调整余额</Button></TableCell></TableRow>)}</TableBody></Table></TableContainer></Card>}

    {(section === 'applications' || section === 'room-reviews') && <Stack gap={1.25}>
      <Paper variant="outlined" sx={{ borderRadius: 2.5, overflow: 'hidden' }}><Tabs value={applicationCategory} onChange={(_, next: ApplicationCategory) => { setApplicationCategory(next); setQuery(''); setApplications([]) }} variant="fullWidth" sx={{ minHeight: 56, '& .MuiTab-root': { minHeight: 56, fontSize: { xs: 12, sm: 14 }, fontWeight: 800 } }}><Tab value="wallet" label="上下分申请" /><Tab value="join" label="入房申请" /><Tab value="entertainment" label="娱乐上下分" /></Tabs></Paper>
      <Card><TableContainer><Table size="small"><TableHead><TableRow><TableCell>用户</TableCell><TableCell>{applicationCategory === 'join' ? '目标房间' : applicationCategory === 'entertainment' ? '娱乐平台' : '类型'}</TableCell>{applicationCategory !== 'join' && <TableCell>金额</TableCell>}<TableCell>状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>{applications.map(row => <TableRow key={row.id}><TableCell>{row.username}<Typography variant="caption" display="block" color="text.secondary">{time(row.created_at)}</Typography></TableCell><TableCell>{applicationCategory === 'join' ? row.target_room_code || '当前房间' : applicationCategory === 'entertainment' ? row.game_id || '未标记平台' : row.request_type === 'credit' ? '上分' : row.request_type === 'debit' ? '下分' : '入房'}</TableCell>{applicationCategory !== 'join' && <TableCell>¥ {money(row.requested_amount)}</TableCell>}<TableCell><Chip size="small" color={row.status === 'pending' ? 'warning' : row.status === 'approved' ? 'success' : 'default'} label={row.status === 'pending' ? '待审核' : row.status === 'approved' ? '已通过' : '已拒绝'} /></TableCell><TableCell align="right">{row.status === 'pending' && <Button size="small" onClick={() => setReviewing(row)}>审核</Button>}</TableCell></TableRow>)}{!loading && applications.length === 0 && <TableRow><TableCell colSpan={5}><Box py={5} textAlign="center"><Typography color="text.secondary">暂无{applicationCategory === 'wallet' ? '上下分' : applicationCategory === 'join' ? '入房' : '娱乐上下分'}申请</Typography></Box></TableCell></TableRow>}</TableBody></Table></TableContainer></Card>
    </Stack>}

    {section === 'bets' && <Card><TableContainer><Table size="small"><TableHead><TableRow><TableCell>注单</TableCell><TableCell>玩法</TableCell><TableCell>金额</TableCell><TableCell>赔率</TableCell><TableCell>状态</TableCell></TableRow></TableHead><TableBody>{bets.map(row => <TableRow key={row.id}><TableCell>#{row.id}<Typography variant="caption" display="block" color="text.secondary">{row.game_id} · {row.issue}</Typography></TableCell><TableCell>{row.play_name} · {row.selection}</TableCell><TableCell>¥ {money(row.amount)}</TableCell><TableCell>{row.odds}</TableCell><TableCell>{row.status}</TableCell></TableRow>)}</TableBody></Table></TableContainer></Card>}

    {chatSection && <Paper variant="outlined" sx={{
      width: lotteryChat ? '100%' : { xs: '100%', md: selected ? 660 : 260 },
      maxWidth: lotteryChat ? 1380 : '100%',
      mx: lotteryChat ? 'auto' : 0,
      height: lotteryChat ? { xs: 'auto', md: 'calc(100dvh - 190px)' } : { xs: 'auto', md: 352 },
      minHeight: lotteryChat ? { md: 560 } : 0,
      maxHeight: lotteryChat ? { md: 860 } : { md: 352 },
      alignSelf: 'flex-start',
      overflow: 'hidden',
      borderRadius: 2.5,
      transition: theme => theme.transitions.create('width', { duration: theme.transitions.duration.shorter }),
    }}><Box sx={{
      display: 'grid',
      gridTemplateColumns: lotteryChat
        ? { xs: '1fr', md: selected ? '320px minmax(0, 1fr)' : '360px' }
        : { xs: '1fr', md: selected ? '230px minmax(0, 430px)' : '260px' },
      width: '100%',
      height: { md: '100%' },
      minHeight: 0,
      overflow: { md: 'hidden' },
    }}>
      <Box sx={{ height: { md: '100%' }, minHeight: 0, overflow: 'hidden', borderRight: { md: 1 }, borderColor: 'divider' }}>
        <CardContent sx={{ p: 0, height: { md: '100%' }, minHeight: 0, display: 'flex', flexDirection: 'column', '&:last-child': { pb: 0 } }}>
          <Box sx={{ p: .85 }}>
            <Typography fontWeight={850} fontSize={lotteryChat ? 17 : 13.5} mb={.6}>{lotteryChat ? '彩票室' : '房间会话'}</Typography>
            {!lotteryChat && <Tabs value={chatMode} onChange={(_, next: 'service' | 'room') => { setChatMode(next); setSelected(null); setMessages([]) }} variant="fullWidth" sx={{ minHeight: 32, mb: .7, '& .MuiTab-root': { minHeight: 32, py: .3, fontSize: 11 } }}>
              <Tab value="service" label="在线客服" />
              <Tab value="room" label="房间群聊" />
            </Tabs>}
            <TextField size="small" fullWidth placeholder="搜索会话" value={query} onChange={event => setQuery(event.target.value)} />
          </Box>
          <Divider />
          <List disablePadding sx={{ flex: 1, minHeight: 0, overflowY: 'auto', scrollbarWidth: 'thin', p: .5 }}>
            {conversations.map(row => <ListItemButton
              key={`${row.scope}:${row.game_id}:${row.room_type}`}
              selected={selected?.scope === row.scope && selected?.game_id === row.game_id && selected?.room_type === row.room_type}
              onClick={() => { setMessages([]); setSelected(row) }}
              sx={{ borderRadius: 1.4, py: lotteryChat ? .75 : .2, px: lotteryChat ? 1.1 : .8, mb: lotteryChat ? .35 : .05, minHeight: lotteryChat ? 56 : 40 }}
            ><ListItemText
              primary={row.title}
              secondary={`${row.subtitle} · ${row.latest_text}`}
              primaryTypographyProps={{ fontWeight: 750, fontSize: lotteryChat ? 14 : 12.5, noWrap: true, sx: { opacity: lotteryChat && !row.enabled ? .52 : 1 } }}
              secondaryTypographyProps={{ noWrap: true, fontSize: lotteryChat ? 11.5 : 10 }}
            />{lotteryChat && <Switch size="small" checked={row.enabled} disabled={switchingLotteryGame === row.game_id} onClick={event => event.stopPropagation()} onChange={event => void setLotteryRoomEnabled(row, event.target.checked)} />}</ListItemButton>)}
            {conversations.length === 0 && !loading && <Box sx={{ px: 1.2, py: 2.5, textAlign: 'center' }}>
              <Typography fontSize={13} fontWeight={750} color="text.secondary">暂无房间会话</Typography>
              <Typography fontSize={11} color="text.disabled" mt={.4}>新消息到达后会显示在这里</Typography>
            </Box>}
          </List>
        </CardContent>
      </Box>

      {selected && <Box sx={{ height: { md: '100%' }, minHeight: { xs: 300, md: 0 }, overflow: 'hidden' }}>
        <CardContent sx={{ p: 0, display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0, minWidth: 0, '&:last-child': { pb: 0 } }}>
          <Box sx={{ px: 1.25, py: .55, minHeight: 44, display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
            <Box minWidth={0}>
              <Typography fontWeight={800} noWrap>{selected.title}</Typography>
              <Typography variant="caption" color="text.secondary" noWrap display="block">{selected.subtitle}</Typography>
            </Box>
            <Stack direction="row" gap={.4}>{!lotteryChat && selected.room_type === 'group' && <Button size="small" color="error" startIcon={<CardGiftcardRounded />} onClick={openRedPacket} sx={{ minWidth: 64 }}>红包</Button>}<Button size="small" color="inherit" onClick={() => { setSelected(null); setMessages([]) }} sx={{ minWidth: 42 }}>收起</Button></Stack>
          </Box>
          <Divider />
          <Stack flex={1} minHeight={0} gap={.8} sx={{ overflowY: 'auto', p: 1.15, width: '100%' }}>
            {selected && messages.length === 0 && !loading && <Box sx={{ mt: 2.5, textAlign: 'center', color: 'text.secondary' }}><Typography fontWeight={750}>暂无消息</Typography><Typography variant="body2">该聊天室暂时没有历史消息</Typography></Box>}
            {messages.map(row => <Box key={row.id} alignSelf={row.is_staff ? 'flex-end' : 'flex-start'} sx={{ width: 'fit-content', minWidth: 0, maxWidth: lotteryChat ? { xs: '88%', md: 620 } : { xs: '88%', md: 340 } }}>{row.message_type === 'redpacket' ? <AdminRedPacketCard count={row.red_packet_count || 1} total={Number(row.red_packet_total || 0)} minTurnover={Number(row.red_packet_min_turnover || 0)} greeting={row.content} cover={row.red_packet_cover} time={time(row.created_at)} /> : <Paper variant="outlined" sx={{ px: 1.15, pt: .75, pb: .45, bgcolor: row.is_staff ? 'primary.main' : 'background.paper', color: row.is_staff ? 'primary.contrastText' : 'text.primary', borderRadius: row.is_staff ? '13px 3px 13px 13px' : '3px 13px 13px 13px' }}><Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', lineHeight: 1.45 }}>{row.content}</Typography><Typography fontSize={9} sx={{ opacity: .7, textAlign: 'right', mt: .1 }}>{time(row.created_at)}</Typography></Paper>}</Box>)}
          </Stack>
          {selected && <Stack direction="row" alignItems="center" gap={.75} sx={{ p: .85, borderTop: 1, borderColor: 'divider' }}><TextField size="small" fullWidth value={reply} onChange={event => setReply(event.target.value)} placeholder="回复当前会话" onKeyDown={event => { if (event.key === 'Enter') void sendReply() }} /><Button size="small" variant="contained" onClick={() => void sendReply()} sx={{ flex: '0 0 auto', minWidth: 72, height: 36, px: 1.25, whiteSpace: 'nowrap' }}>发送</Button></Stack>}
        </CardContent>
      </Box>}
    </Box></Paper>}

    <Dialog open={!lotteryChat && redPacketOpen} onClose={() => setRedPacketOpen(false)} fullWidth maxWidth="sm" slotProps={{ paper: { sx: { width: 'min(560px, calc(100% - 24px))', maxHeight: 'calc(100dvh - 32px)', borderRadius: 2, overflow: 'hidden' } } }}><DialogTitle sx={{ color: '#fff', background: 'linear-gradient(135deg,#d94b45,#ed7954)' }}><Typography fontSize={18} fontWeight={900}>发送房间红包</Typography><Typography fontSize={10.5} sx={{ opacity: .82 }}>红包会实时发送到当前房间聊天室</Typography></DialogTitle><DialogContent sx={{ pt: '18px !important', bgcolor: 'background.default' }}><RedPacketForm count={redPacketCount} total={redPacketTotal} greeting={redPacketGreeting} cover={redPacketCover} minTurnover={redPacketMinTurnover} onCount={setRedPacketCount} onTotal={setRedPacketTotal} onGreeting={setRedPacketGreeting} onCover={setRedPacketCover} onMinTurnover={setRedPacketMinTurnover} /></DialogContent><DialogActions sx={{ px: 2.5, py: 1.25, bgcolor: 'background.paper' }}><Button size="small" onClick={() => setRedPacketOpen(false)}>取消</Button><Button size="small" variant="contained" color="error" disabled={!Number.isInteger(Number(redPacketCount)) || Number(redPacketCount) < 1 || Number(redPacketTotal) < Number(redPacketCount) * .01 || Number(redPacketMinTurnover) < 0} onClick={() => void sendRedPacket()} sx={{ minWidth: 88, height: 34, px: 1.5 }}>发送红包</Button></DialogActions></Dialog>
    <Dialog open={Boolean(balanceUser)} onClose={() => setBalanceUser(null)} fullWidth maxWidth="xs"><DialogTitle>调整余额 · {balanceUser?.nickname || balanceUser?.username}</DialogTitle><DialogContent><Stack gap={2} pt={1}><TextField type="number" label="调整金额" helperText="正数为上分，负数为下分" value={amount} onChange={event => setAmount(event.target.value)} /><TextField label="原因" value={remark} onChange={event => setRemark(event.target.value)} /></Stack></DialogContent><DialogActions><Button onClick={() => setBalanceUser(null)}>取消</Button><Button variant="contained" onClick={() => void adjustBalance()}>确认</Button></DialogActions></Dialog>
    <Dialog open={Boolean(reviewing)} onClose={() => setReviewing(null)} fullWidth maxWidth="xs"><DialogTitle>{reviewing?.request_type === 'join' ? '审核入房申请' : '审核上下分申请'} #{reviewing?.id}</DialogTitle><DialogContent><Typography>用户：{reviewing?.username}</Typography>{reviewing?.request_type === 'join' ? <Typography>目标房间：{reviewing.target_room_code || '当前房间'}</Typography> : <><Typography>金额：¥ {money(reviewing?.requested_amount ?? 0)}</Typography><TextField select fullWidth label="收款方式" value={reviewing?.payment_type ?? 'manual'} disabled sx={{ mt: 2 }}><MenuItem value={reviewing?.payment_type ?? 'manual'}>{reviewing?.payment_type ?? '人工'}</MenuItem></TextField></>}</DialogContent><DialogActions><Button color="error" onClick={() => void review('rejected')}>拒绝</Button><Button variant="contained" onClick={() => void review('approved')}>通过</Button></DialogActions></Dialog>
  </Box>
}
