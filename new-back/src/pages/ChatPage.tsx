import { Alert, Avatar, Box, Button, Card, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Divider, IconButton, InputAdornment, MenuItem, Paper, Stack, Switch, Tab, Tabs, TextField, Tooltip, Typography } from '@mui/material'
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
import { adminApi, type AdminChatConversation, type AdminChatMessage, type AgentItem } from '../api'
import { useFeedback } from '../components/feedback'
import { AdminRedPacketCard, RedPacketForm, type RedPacketCover } from '../components/RedPacketForm'
import { MANAGEMENT_WS_EVENT } from '../hooks/useManagementWebSocket'
import type { ManagementWsEvent } from '../api'

type ChatMode = 'service' | 'room' | 'lottery'

const dateTime = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value)) : '—'
const sameConversation = (left: AdminChatConversation | null, right: AdminChatConversation) => Boolean(left
  && left.scope === right.scope
  && left.room_scope === right.room_scope
  && left.game_id === right.game_id
  && left.room_type === right.room_type)

const conversationPreview = (item: AdminChatConversation) => {
  if (!item.latest_at || item.latest_text === '暂无聊天记录') return '暂无聊天记录'
  if (item.latest_message_type === 'redpacket') return `红包 · ${item.latest_text || '恭喜发财'}`
  return `${item.latest_is_staff ? '客服' : '会员'}：${item.latest_text}`
}

export function ChatPage({ view = 'support' }: { view?: 'support' | 'lottery' }) {
  const [mode, setMode] = useState<ChatMode>(view === 'lottery' ? 'lottery' : 'room')
  const [rooms, setRooms] = useState<AgentItem[]>([])
  const [roomScope, setRoomScope] = useState('')
  const [query, setQuery] = useState('')
  const [appliedQuery, setAppliedQuery] = useState('')
  const [conversations, setConversations] = useState<AdminChatConversation[]>([])
  const [selected, setSelected] = useState<AdminChatConversation | null>(null)
  const [messages, setMessages] = useState<AdminChatMessage[]>([])
  const [nextBeforeID, setNextBeforeID] = useState<number | undefined>()
  const [hasMore, setHasMore] = useState(false)
  const [draft, setDraft] = useState('')
  const [muteOpen, setMuteOpen] = useState(false)
  const [muteMinutes, setMuteMinutes] = useState('60')
  const [muteReason, setMuteReason] = useState('')
  const [loading, setLoading] = useState(true)
  const [messageLoading, setMessageLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [redPacketOpen, setRedPacketOpen] = useState(false)
  const [redPacketCount, setRedPacketCount] = useState('10')
  const [redPacketTotal, setRedPacketTotal] = useState('100')
  const [redPacketGreeting, setRedPacketGreeting] = useState('恭喜发财')
  const [redPacketCover, setRedPacketCover] = useState<RedPacketCover>('classic')
  const [redPacketMinTurnover, setRedPacketMinTurnover] = useState('0')
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()
  const selectedRef = useRef<AdminChatConversation | null>(null)
  useEffect(() => { selectedRef.current = selected }, [selected])

  const loadConversations = useCallback(async (preserve = true) => {
    if (!roomScope) {
      setConversations([])
      setSelected(null)
      setLoading(false)
      return
    }
    setLoading(true)
    setError('')
    try {
      const result = await adminApi.chatConversations({ roomType: mode === 'service' ? 'service' : 'group', roomScope, channel: mode, query: appliedQuery, page: 1, pageSize: 60 })
      setConversations(result.items)
      setSelected(current => {
        if (!preserve) return result.items[0] ?? null
        return result.items.find(item => item.scope === current?.scope && item.room_scope === current?.room_scope && item.game_id === current?.game_id && item.room_type === current?.room_type) ?? result.items[0] ?? null
      })
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取会话失败')
    } finally { setLoading(false) }
  }, [appliedQuery, mode, roomScope])

  const loadMessages = useCallback(async (conversation: AdminChatConversation, beforeId?: number, prepend = false) => {
    setMessageLoading(true)
    try {
      const result = await adminApi.chatMessages({ scope: conversation.scope, roomScope: conversation.room_scope, gameId: conversation.game_id, roomType: conversation.room_type, beforeId, limit: 50 })
      setMessages(current => prepend ? [...result.items, ...current] : result.items)
      setHasMore(result.has_more)
      setNextBeforeID(result.next_before_id)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取聊天记录失败')
    } finally { setMessageLoading(false) }
  }, [])

  useEffect(() => { const timer = window.setTimeout(() => void loadConversations(false), 0); return () => window.clearTimeout(timer) }, [loadConversations])
  useEffect(() => {
    let active = true
    void (async () => {
      const first = await adminApi.agents({ page: 1, pageSize: 100 })
      const all = [...first.items]
      for (let page = 2; all.length < first.total; page += 1) {
        const next = await adminApi.agents({ page, pageSize: 100 })
        all.push(...next.items)
        if (next.items.length === 0) break
      }
      return all
    })().then(items => {
      if (!active) return
      const available = items.filter(item => item.room_code.trim())
      setRooms(available)
      setRoomScope(current => available.some(item => `agent:${item.id}` === current) ? current : (available[0] ? `agent:${available[0].id}` : ''))
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
        void loadMessages(current)
      }
      // Also refresh the left list so a member's first message creates a new
      // customer-service conversation without any manual reload.
      void loadConversations()
    }
    window.addEventListener(MANAGEMENT_WS_EVENT, onRealtime)
    return () => window.removeEventListener(MANAGEMENT_WS_EVENT, onRealtime)
  }, [loadConversations, loadMessages, roomScope])

  const selectedMuted = Boolean(selected?.muted_until)
  const title = selected?.title ?? '选择一个会话'
  const subtitle = selected?.subtitle ?? '从左侧选择需要处理的会话'
  const conversationCount = useMemo(() => conversations.length, [conversations])

  const reply = async () => {
    if (!selected || !draft.trim()) return
    setSaving(true)
    try {
      const message = await adminApi.replyChat({ scope: selected.scope, room_scope: selected.room_scope, game_id: selected.game_id, room_type: selected.room_type, content: draft.trim() })
      setMessages(current => [...current, message])
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

  const saveMute = async (minutes: number) => {
    if (view === 'lottery' || !selected?.user_id) return
    setSaving(true)
    try {
      const next = await adminApi.setChatMute(selected.user_id, { minutes, reason: muteReason.trim() })
      setSelected(current => current ? { ...current, muted_until: next.muted_until } : current)
      setConversations(current => current.map(item => item.scope === selected.scope && item.room_type === selected.room_type ? { ...item, muted_until: next.muted_until } : item))
      setMuteOpen(false)
      showMessage(minutes > 0 ? '已设置群聊禁言' : '已解除群聊禁言')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '更新禁言失败') } finally { setSaving(false) }
  }

  const openRedPacket = () => {
    if (view === 'lottery' || !selected || selected.room_type !== 'group') return
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
    if (view === 'lottery' || !selected || !Number.isInteger(count) || count < 1 || total < count * .01) return
    setSaving(true)
    try {
      const message = await adminApi.sendChatRedPacket({
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
      setRedPacketOpen(false)
      showMessage('红包已发送到当前聊天室')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '发送红包失败') }
    finally { setSaving(false) }
  }

  const setLotteryRoomEnabled = async (item: AdminChatConversation, enabled: boolean) => {
    setSaving(true)
    try {
      const result = await adminApi.setLotteryRoomStatus(item.room_scope, item.game_id, enabled)
      setConversations(current => current.map(row => sameConversation(row, item) ? { ...row, enabled: result.enabled } : row))
      setSelected(current => current && sameConversation(current, item) ? { ...current, enabled: result.enabled } : current)
      showMessage(`${item.title}已${result.enabled ? '开启' : '关闭'}`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存彩票室状态失败')
    } finally {
      setSaving(false)
    }
  }

  const lotteryView = view === 'lottery'

  return <Box p={{ xs: 2, lg: 2.5 }}>
    {error && <Alert severity="error" onClose={() => setError('')} sx={{ mt: 2 }}>{error}</Alert>}
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
          borderRight: { md: 1 },
          borderBottom: { xs: 1, md: 0 },
          borderColor: 'divider',
          minHeight: { xs: 280, md: 0 },
          height: { md: '100%' },
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}>
          <Box px={1.3} py={1.1} borderBottom={1} borderColor="divider">
            <TextField fullWidth select size="small" label="房间号" value={roomScope} onChange={event => { setMessages([]); setSelected(null); setRoomScope(event.target.value) }}>
              {rooms.length ? rooms.map(room => <MenuItem key={room.id} value={`agent:${room.id}`}>{room.room_code} · {room.room_name || room.nickname || room.username}{room.status === 1 ? '' : '（停用）'}</MenuItem>) : <MenuItem value="" disabled>暂无可用房间</MenuItem>}
            </TextField>
          </Box>
          {lotteryView ? <Stack direction="row" alignItems="center" gap={1.15} px={1.8} py={1.45}><Avatar sx={{ bgcolor: 'secondary.main', width: 38, height: 38 }}><ForumRounded fontSize="small" /></Avatar><Box><Typography fontWeight={850} fontSize={16}>彩票室</Typography><Typography fontSize={11} color="text.secondary">按房间 · 按彩种隔离</Typography></Box></Stack> : <Tabs value={mode} onChange={(_, next: ChatMode) => { setMode(next); setMessages([]); setSelected(null) }} variant="fullWidth">
            <Tab value="service" icon={<SupportAgentRounded />} iconPosition="start" label="在线客服" />
            <Tab value="room" icon={<ForumRounded />} iconPosition="start" label="房间群聊" />
          </Tabs>}
          <Box p={1.5}><TextField fullWidth size="small" value={query} placeholder={lotteryView ? '搜索彩种、房间号或消息内容' : '搜索会员、昵称或消息内容'} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') setAppliedQuery(query.trim()) }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /></Box>
          <Divider />
          {loading ? <Box p={3} textAlign="center"><CircularProgress size={24} /></Box> : <Box sx={{ flex: 1, minHeight: 0, maxHeight: { xs: 300, md: 'none' }, overflowY: 'auto', overscrollBehavior: 'contain' }}>
            {conversations.map(item => <Box key={`${item.room_type}:${item.scope}:${item.room_scope}:${item.game_id}`} component="div" role="button" tabIndex={0} onClick={() => {
              if (sameConversation(selected, item)) return
              setMessages([])
              setSelected(item)
            }} onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); setMessages([]); setSelected(item) } }} sx={{ display: 'block', width: '100%', border: 0, textAlign: 'left', cursor: 'pointer', px: 1.8, py: lotteryView ? 1.7 : 1.5, bgcolor: selected?.scope === item.scope && selected?.room_scope === item.room_scope && selected?.game_id === item.game_id && selected?.room_type === item.room_type ? 'action.selected' : 'transparent', color: 'inherit', '&:hover': { bgcolor: 'action.hover' } }}>
              <Stack direction="row" gap={1.1} alignItems="center"><Avatar sx={{ bgcolor: item.room_type === 'service' ? 'primary.main' : 'secondary.main', width: lotteryView ? 42 : 38, height: lotteryView ? 42 : 38, opacity: lotteryView && !item.enabled ? .5 : 1 }}>{item.room_type === 'service' ? <SupportAgentRounded fontSize="small" /> : <ForumRounded fontSize="small" />}</Avatar><Box flex={1} minWidth={0}><Stack direction="row" gap={.5} alignItems="center"><Typography fontSize={lotteryView ? 14 : 12} fontWeight={800} sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', opacity: lotteryView && !item.enabled ? .55 : 1 }}>{item.title}</Typography>{item.pinned && <Tooltip title="固定置顶"><PushPinRounded color="primary" sx={{ fontSize: 13, transform: 'rotate(-18deg)' }} /></Tooltip>}{!lotteryView && item.muted_until && <Tooltip title="已禁言"><VolumeOffRounded color="warning" sx={{ fontSize: 14 }} /></Tooltip>}<Typography ml="auto" fontSize={lotteryView ? 10 : 9} color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>{dateTime(item.latest_at)}</Typography></Stack><Typography fontSize={lotteryView ? 11.5 : 10} color="text.secondary" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{item.subtitle}</Typography><Typography fontSize={lotteryView ? 12 : 11} mt={.35} color="text.secondary" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{conversationPreview(item)}</Typography></Box>{lotteryView && <Tooltip title={item.enabled ? '关闭彩票室' : '开启彩票室'}><Switch size="small" checked={item.enabled} disabled={saving} onClick={event => event.stopPropagation()} onChange={event => void setLotteryRoomEnabled(item, event.target.checked)} /></Tooltip>}</Stack>
            </Box>)}
            {!conversations.length && <Box textAlign="center" py={7} color="text.secondary"><ForumRounded sx={{ opacity: .35, fontSize: 36 }} /><Typography fontSize={12}>暂无待处理会话</Typography></Box>}
          </Box>}
          <Box px={1.8} py={1} borderTop={1} borderColor="divider"><Typography fontSize={10} color="text.secondary">当前 {conversationCount} 个{lotteryView ? '彩票房间' : '会话'}{!lotteryView && ' · 服务会话仅管理员可查看'}</Typography></Box>
        </Box>
        <Box sx={{
          minWidth: 0,
          minHeight: { xs: 420, md: 0 },
          height: { md: '100%' },
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}>
          <Stack direction="row" alignItems="center" gap={1} px={2} py={1.35} borderBottom={1} borderColor="divider"><Avatar sx={{ bgcolor: selected?.room_type === 'group' ? 'secondary.main' : 'primary.main', width: 36, height: 36 }}>{selected?.room_type === 'group' ? <ForumRounded fontSize="small" /> : <SupportAgentRounded fontSize="small" />}</Avatar><Box minWidth={0} flex={1}><Typography fontSize={13} fontWeight={850} noWrap>{title}</Typography><Typography fontSize={10} color="text.secondary" noWrap>{subtitle}</Typography></Box>{!lotteryView && selected?.room_type === 'group' && <Button size="small" color="secondary" startIcon={<CardGiftcardRounded />} onClick={openRedPacket}>发红包</Button>}{!lotteryView && selected?.user_id && <>{selectedMuted ? <Button size="small" color="success" startIcon={<VolumeUpRounded />} disabled={saving} onClick={() => void saveMute(0)}>解除禁言</Button> : <Button size="small" color="warning" startIcon={<VolumeOffRounded />} onClick={() => { setMuteReason(''); setMuteMinutes('60'); setMuteOpen(true) }}>禁言</Button>}</>}</Stack>
          <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overscrollBehavior: 'contain', p: { xs: 1.5, md: 2 }, bgcolor: 'background.default' }}>
            {selected && hasMore && <Box textAlign="center" mb={1.5}><Button size="small" startIcon={messageLoading ? <CircularProgress size={13} /> : <ArrowUpwardRounded />} disabled={messageLoading} onClick={() => void loadMessages(selected, nextBeforeID, true)}>加载更早消息</Button></Box>}
            {!selected ? <Box height="100%" display="grid" sx={{ placeItems: 'center' }} color="text.secondary"><Typography>请选择一个会话开始处理</Typography></Box> : <Stack gap={1.25} sx={{ width: '100%', maxWidth: 860, mx: 'auto' }}>
              {messages.map(item => <Stack key={item.id} direction={item.is_staff ? 'row-reverse' : 'row'} gap={.8} alignItems="flex-start">
                <Avatar sx={{ width: 31, height: 31, fontSize: 11, bgcolor: item.is_staff ? 'primary.main' : 'grey.500', flexShrink: 0 }}>{(item.nickname || item.username).slice(0, 1)}</Avatar>
                <Box sx={{ width: 'fit-content', minWidth: 0, maxWidth: { xs: '86%', md: lotteryView ? 680 : 560 } }}>
                  <Stack direction="row" gap={.7} justifyContent={item.is_staff ? 'flex-end' : 'flex-start'}>
                    <Typography fontSize={10} color="text.secondary" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: { xs: 170, md: 220 } }}>{item.is_staff ? '客服 · ' : ''}{item.nickname || item.username}</Typography>
                  </Stack>
                  {item.message_type === 'redpacket' ? <Box mt={.3}>
                    <AdminRedPacketCard
                      count={item.red_packet_count || 1}
                      total={Number(item.red_packet_total || 0)}
                      minTurnover={Number(item.red_packet_min_turnover || 0)}
                      greeting={item.content}
                      cover={item.red_packet_cover}
                      time={dateTime(item.created_at)}
                      action={<Tooltip title="撤回"><IconButton size="small" disabled={saving} onClick={() => void deleteMessage(item.id)} sx={{ color: 'inherit', opacity: .8, p: .1 }}><DeleteOutlineRounded sx={{ fontSize: 14 }} /></IconButton></Tooltip>}
                    />
                  </Box> : <Paper sx={{ mt: .3, px: 1.25, pt: 1, pb: .65, bgcolor: item.is_staff ? 'primary.main' : 'background.paper', color: item.is_staff ? 'primary.contrastText' : 'text.primary', borderRadius: item.is_staff ? '14px 3px 14px 14px' : '3px 14px 14px 14px', boxShadow: 'none' }}>
                    <Typography fontSize={13} sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', wordBreak: 'break-word', lineHeight: 1.55 }}>{item.content}</Typography>
                    <Stack direction="row" justifyContent="flex-end" alignItems="center" gap={.35} mt={.2}><Typography fontSize={9} sx={{ opacity: .7 }}>{dateTime(item.created_at)}</Typography><Tooltip title="撤回"><IconButton size="small" disabled={saving} onClick={() => void deleteMessage(item.id)} sx={{ color: 'inherit', opacity: .68, p: .15 }}><DeleteOutlineRounded sx={{ fontSize: 14 }} /></IconButton></Tooltip></Stack>
                  </Paper>}
                </Box>
              </Stack>)}
            </Stack>}
            {selected && !messages.length && !messageLoading && <Box textAlign="center" py={8} color="text.secondary"><Typography fontSize={12}>暂无聊天记录</Typography></Box>}
          </Box>
          <Divider />
          <Box p={1.4}><Stack direction="row" gap={1} alignItems="flex-end"><TextField fullWidth multiline maxRows={4} placeholder={selected ? '输入回复内容，Enter 发送，Shift + Enter 换行' : '请先选择会话'} disabled={!selected || saving} value={draft} onChange={event => setDraft(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void reply() } }} inputProps={{ maxLength: 500 }} /><Button size="small" variant="contained" startIcon={<SendRounded sx={{ fontSize: '17px !important' }} />} disabled={!selected || !draft.trim() || saving} onClick={() => void reply()} sx={{ flex: '0 0 auto', minWidth: 78, height: 40, px: 1.3, whiteSpace: 'nowrap', fontSize: 12 }}>发送</Button></Stack></Box>
        </Box>
      </Box>
    </Card>
    <Dialog open={!lotteryView && redPacketOpen} onClose={() => !saving && setRedPacketOpen(false)} fullWidth maxWidth="sm" slotProps={{ paper: { sx: { width: 'min(560px, calc(100% - 24px))', maxHeight: 'calc(100dvh - 32px)', borderRadius: 2, overflow: 'hidden' } } }}><DialogTitle sx={{ color: '#fff', background: 'linear-gradient(135deg,#d94b45,#ed7954)' }}><Typography fontSize={18} fontWeight={900}>发送房间红包</Typography><Typography fontSize={10.5} sx={{ opacity: .82 }}>红包会实时发送到当前房间聊天室</Typography></DialogTitle><DialogContent sx={{ pt: '18px !important', bgcolor: 'background.default' }}><RedPacketForm count={redPacketCount} total={redPacketTotal} greeting={redPacketGreeting} cover={redPacketCover} minTurnover={redPacketMinTurnover} onCount={setRedPacketCount} onTotal={setRedPacketTotal} onGreeting={setRedPacketGreeting} onCover={setRedPacketCover} onMinTurnover={setRedPacketMinTurnover} /></DialogContent><DialogActions sx={{ px: 2.5, py: 1.25, bgcolor: 'background.paper' }}><Button size="small" onClick={() => setRedPacketOpen(false)}>取消</Button><Button size="small" variant="contained" color="error" disabled={saving || !Number.isInteger(Number(redPacketCount)) || Number(redPacketCount) < 1 || Number(redPacketTotal) < Number(redPacketCount) * .01 || Number(redPacketMinTurnover) < 0} onClick={() => void sendRedPacket()} sx={{ minWidth: 88, height: 34, px: 1.5 }}>{saving ? '发送中…' : '发送红包'}</Button></DialogActions></Dialog>
    <Dialog open={!lotteryView && muteOpen} onClose={() => !saving && setMuteOpen(false)} fullWidth maxWidth="xs"><DialogTitle>设置群聊禁言</DialogTitle><DialogContent><Stack gap={1.5} pt={1}><Alert severity="warning">禁言仅限制会员在房间群聊发言，专属客服仍可正常联系。</Alert><TextField select label="禁言时长" value={muteMinutes} onChange={event => setMuteMinutes(event.target.value)}><MenuItem value="10">10 分钟</MenuItem><MenuItem value="60">1 小时</MenuItem><MenuItem value="480">8 小时</MenuItem><MenuItem value="1440">24 小时</MenuItem><MenuItem value="10080">7 天</MenuItem></TextField><TextField label="禁言原因（可选）" value={muteReason} onChange={event => setMuteReason(event.target.value)} multiline minRows={2} inputProps={{ maxLength: 300 }} /></Stack></DialogContent><DialogActions><Button disabled={saving} onClick={() => setMuteOpen(false)}>取消</Button><Button variant="contained" color="warning" disabled={saving} onClick={() => void saveMute(Number(muteMinutes))}>{saving ? '设置中…' : '确认禁言'}</Button></DialogActions></Dialog>
  </Box>
}
