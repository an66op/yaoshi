import { Alert, Avatar, Box, Button, Card, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Divider, IconButton, InputAdornment, MenuItem, Paper, Stack, Tab, Tabs, TextField, Tooltip, Typography } from '@mui/material'
import SupportAgentRounded from '@mui/icons-material/SupportAgentRounded'
import ForumRounded from '@mui/icons-material/ForumRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SendRounded from '@mui/icons-material/SendRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import VolumeOffRounded from '@mui/icons-material/VolumeOffRounded'
import VolumeUpRounded from '@mui/icons-material/VolumeUpRounded'
import CampaignRounded from '@mui/icons-material/CampaignRounded'
import ArrowUpwardRounded from '@mui/icons-material/ArrowUpwardRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, type AdminChatConversation, type AdminChatMessage } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

type ChatMode = '' | 'service' | 'group'

const dateTime = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value)) : '—'

export function ChatPage() {
  const [mode, setMode] = useState<ChatMode>('service')
  const [query, setQuery] = useState('')
  const [appliedQuery, setAppliedQuery] = useState('')
  const [conversations, setConversations] = useState<AdminChatConversation[]>([])
  const [selected, setSelected] = useState<AdminChatConversation | null>(null)
  const [messages, setMessages] = useState<AdminChatMessage[]>([])
  const [nextBeforeID, setNextBeforeID] = useState<number | undefined>()
  const [hasMore, setHasMore] = useState(false)
  const [draft, setDraft] = useState('')
  const [announcement, setAnnouncement] = useState('')
  const [muteOpen, setMuteOpen] = useState(false)
  const [muteMinutes, setMuteMinutes] = useState('60')
  const [muteReason, setMuteReason] = useState('')
  const [loading, setLoading] = useState(true)
  const [messageLoading, setMessageLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const loadConversations = useCallback(async (preserve = true) => {
    setLoading(true)
    setError('')
    try {
      const result = await adminApi.chatConversations({ roomType: mode, query: appliedQuery, page: 1, pageSize: 60 })
      setConversations(result.items)
      setSelected(current => {
        if (!preserve) return result.items[0] ?? null
        return result.items.find(item => item.scope === current?.scope && item.room_type === current?.room_type) ?? result.items[0] ?? null
      })
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取会话失败')
    } finally { setLoading(false) }
  }, [appliedQuery, mode])

  const loadMessages = useCallback(async (conversation: AdminChatConversation, beforeId?: number, prepend = false) => {
    setMessageLoading(true)
    try {
      const result = await adminApi.chatMessages({ scope: conversation.scope, roomType: conversation.room_type, beforeId, limit: 50 })
      setMessages(current => prepend ? [...result.items, ...current] : result.items)
      setHasMore(result.has_more)
      setNextBeforeID(result.next_before_id)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取聊天记录失败')
    } finally { setMessageLoading(false) }
  }, [])

  useEffect(() => { const timer = window.setTimeout(() => void loadConversations(false), 0); return () => window.clearTimeout(timer) }, [loadConversations])
  useEffect(() => { if (selected) void loadMessages(selected) }, [selected, loadMessages])

  const selectedMuted = Boolean(selected?.muted_until && new Date(selected.muted_until).getTime() > Date.now())
  const title = selected?.title ?? '选择一个会话'
  const subtitle = selected?.subtitle ?? '从左侧选择需要处理的会话'
  const conversationCount = useMemo(() => conversations.length, [conversations])

  const reply = async () => {
    if (!selected || !draft.trim()) return
    setSaving(true)
    try {
      const message = await adminApi.replyChat({ scope: selected.scope, room_type: selected.room_type, content: draft.trim() })
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
    if (!selected?.user_id) return
    setSaving(true)
    try {
      const next = await adminApi.setChatMute(selected.user_id, { minutes, reason: muteReason.trim() })
      setSelected(current => current ? { ...current, muted_until: next.muted_until } : current)
      setConversations(current => current.map(item => item.scope === selected.scope && item.room_type === selected.room_type ? { ...item, muted_until: next.muted_until } : item))
      setMuteOpen(false)
      showMessage(minutes > 0 ? '已设置群聊禁言' : '已解除群聊禁言')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '更新禁言失败') } finally { setSaving(false) }
  }

  const saveAnnouncement = async () => {
    setSaving(true)
    try { await adminApi.setChatAnnouncement(announcement); showMessage('大厅公告已更新') }
    catch (reason) { setError(reason instanceof Error ? reason.message : '更新公告失败') }
    finally { setSaving(false) }
  }

  return <Box p={{ xs: 2, lg: 2.5 }}>
    <PageHeader eyebrow="客户服务 / 会话" title="客服与聊天室" description="处理会员专属客服、房间群聊、禁言与大厅公告；聊天记录按会话隔离。" actions={<><Button variant="outlined" startIcon={<CampaignRounded />} onClick={() => { setAnnouncement(''); document.getElementById('chat-announcement')?.focus() }}>大厅公告</Button><Button variant="outlined" startIcon={<RefreshRounded />} disabled={loading} onClick={() => void loadConversations()}>刷新</Button></>} />
    {error && <Alert severity="error" onClose={() => setError('')} sx={{ mt: 2 }}>{error}</Alert>}
    <Card sx={{ mt: 2.5, minHeight: { lg: 650 }, overflow: 'hidden' }}>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'minmax(300px, 35%) minmax(0, 65%)' }, minHeight: { lg: 650 } }}>
        <Box sx={{ borderRight: { lg: 1 }, borderBottom: { xs: 1, lg: 0 }, borderColor: 'divider', minHeight: { xs: 310, lg: 650 } }}>
          <Tabs value={mode} onChange={(_, next: ChatMode) => { setMode(next); setSelected(null) }} variant="fullWidth">
            <Tab value="service" icon={<SupportAgentRounded />} iconPosition="start" label="在线客服" />
            <Tab value="group" icon={<ForumRounded />} iconPosition="start" label="房间群聊" />
          </Tabs>
          <Box p={1.5}><TextField fullWidth size="small" value={query} placeholder="搜索会员、昵称或消息内容" onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') setAppliedQuery(query.trim()) }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} /></Box>
          <Divider />
          {loading ? <Box p={3} textAlign="center"><CircularProgress size={24} /></Box> : <Box sx={{ maxHeight: { xs: 350, lg: 562 }, overflowY: 'auto' }}>
            {conversations.map(item => <Box key={`${item.room_type}:${item.scope}`} component="button" type="button" onClick={() => setSelected(item)} sx={{ display: 'block', width: '100%', border: 0, textAlign: 'left', cursor: 'pointer', px: 1.8, py: 1.5, bgcolor: selected?.scope === item.scope && selected?.room_type === item.room_type ? 'action.selected' : 'transparent', color: 'inherit', '&:hover': { bgcolor: 'action.hover' } }}>
              <Stack direction="row" gap={1.1} alignItems="center"><Avatar sx={{ bgcolor: item.room_type === 'service' ? 'primary.main' : 'secondary.main', width: 38, height: 38 }}>{item.room_type === 'service' ? <SupportAgentRounded fontSize="small" /> : <ForumRounded fontSize="small" />}</Avatar><Box flex={1} minWidth={0}><Stack direction="row" gap={.5} alignItems="center"><Typography fontSize={12} fontWeight={800} noWrap>{item.title}</Typography>{item.muted_until && new Date(item.muted_until).getTime() > Date.now() && <Tooltip title="已禁言"><VolumeOffRounded color="warning" sx={{ fontSize: 14 }} /></Tooltip>}<Typography ml="auto" fontSize={9} color="text.secondary">{dateTime(item.latest_at)}</Typography></Stack><Typography fontSize={10} color="text.secondary" noWrap>{item.subtitle}</Typography><Typography fontSize={11} mt={.35} color="text.secondary" noWrap>{item.latest_text}</Typography></Box></Stack>
            </Box>)}
            {!conversations.length && <Box textAlign="center" py={7} color="text.secondary"><ForumRounded sx={{ opacity: .35, fontSize: 36 }} /><Typography fontSize={12}>暂无待处理会话</Typography></Box>}
          </Box>}
          <Box px={1.8} py={1} borderTop={1} borderColor="divider"><Typography fontSize={10} color="text.secondary">当前 {conversationCount} 个会话 · 服务会话仅管理员可查看</Typography></Box>
        </Box>
        <Box sx={{ minWidth: 0, display: 'flex', flexDirection: 'column', minHeight: { xs: 480, lg: 650 } }}>
          <Stack direction="row" alignItems="center" gap={1} px={2} py={1.35} borderBottom={1} borderColor="divider"><Avatar sx={{ bgcolor: selected?.room_type === 'group' ? 'secondary.main' : 'primary.main', width: 36, height: 36 }}>{selected?.room_type === 'group' ? <ForumRounded fontSize="small" /> : <SupportAgentRounded fontSize="small" />}</Avatar><Box minWidth={0} flex={1}><Typography fontSize={13} fontWeight={850} noWrap>{title}</Typography><Typography fontSize={10} color="text.secondary" noWrap>{subtitle}</Typography></Box>{selected?.user_id && <>{selectedMuted ? <Button size="small" color="success" startIcon={<VolumeUpRounded />} disabled={saving} onClick={() => void saveMute(0)}>解除禁言</Button> : <Button size="small" color="warning" startIcon={<VolumeOffRounded />} onClick={() => { setMuteReason(''); setMuteMinutes('60'); setMuteOpen(true) }}>禁言</Button>}</>}</Stack>
          <Box sx={{ flex: 1, overflowY: 'auto', p: 2, bgcolor: 'background.default' }}>
            {selected && hasMore && <Box textAlign="center" mb={1.5}><Button size="small" startIcon={messageLoading ? <CircularProgress size={13} /> : <ArrowUpwardRounded />} disabled={messageLoading} onClick={() => void loadMessages(selected, nextBeforeID, true)}>加载更早消息</Button></Box>}
            {!selected ? <Box height="100%" display="grid" sx={{ placeItems: 'center' }} color="text.secondary"><Typography>请选择一个会话开始处理</Typography></Box> : <Stack gap={1.4}>{messages.map(item => <Stack key={item.id} direction={item.is_staff ? 'row-reverse' : 'row'} gap={.8} alignItems="flex-start"><Avatar sx={{ width: 31, height: 31, fontSize: 11, bgcolor: item.is_staff ? 'primary.main' : 'grey.500' }}>{(item.nickname || item.username).slice(0, 1)}</Avatar><Box sx={{ maxWidth: '78%' }}><Stack direction="row" gap={.7} justifyContent={item.is_staff ? 'flex-end' : 'flex-start'}><Typography fontSize={10} color="text.secondary">{item.is_staff ? '客服 · ' : ''}{item.nickname || item.username}</Typography><Typography fontSize={9} color="text.secondary">{dateTime(item.created_at)}</Typography></Stack><Paper sx={{ mt: .35, p: 1.15, bgcolor: item.is_staff ? 'primary.main' : 'background.paper', color: item.is_staff ? 'primary.contrastText' : 'text.primary', borderRadius: item.is_staff ? '14px 3px 14px 14px' : '3px 14px 14px 14px', boxShadow: 'none' }}><Stack direction="row" gap={.8} alignItems="flex-start"><Typography fontSize={13} whiteSpace="pre-wrap" flex={1}>{item.content}</Typography><Tooltip title="撤回"><IconButton size="small" disabled={saving} onClick={() => void deleteMessage(item.id)} sx={{ color: item.is_staff ? 'inherit' : 'text.secondary', mt: -.5, mr: -.5 }}><DeleteOutlineRounded sx={{ fontSize: 15 }} /></IconButton></Tooltip></Stack></Paper></Box></Stack>)}</Stack>}
            {selected && !messages.length && !messageLoading && <Box textAlign="center" py={8} color="text.secondary"><Typography fontSize={12}>暂无聊天记录</Typography></Box>}
          </Box>
          <Divider />
          <Box p={1.4}><Stack direction="row" gap={1} alignItems="flex-end"><TextField fullWidth multiline maxRows={4} placeholder={selected ? '输入回复内容，Enter 发送，Shift + Enter 换行' : '请先选择会话'} disabled={!selected || saving} value={draft} onChange={event => setDraft(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void reply() } }} inputProps={{ maxLength: 500 }} /><Button variant="contained" startIcon={<SendRounded />} disabled={!selected || !draft.trim() || saving} onClick={() => void reply()}>发送</Button></Stack></Box>
        </Box>
      </Box>
    </Card>
    <Card sx={{ mt: 1.5, p: 1.8 }}><Stack direction={{ xs: 'column', md: 'row' }} gap={1.2} alignItems={{ md: 'center' }}><Stack direction="row" gap={1} alignItems="center"><CampaignRounded color="primary" /><Box><Typography fontSize={12} fontWeight={800}>大厅公告</Typography><Typography fontSize={10} color="text.secondary">所有会员房间顶部实时读取该公告。</Typography></Box></Stack><TextField id="chat-announcement" size="small" fullWidth placeholder="输入大厅公告；留空可清除" value={announcement} onChange={event => setAnnouncement(event.target.value)} inputProps={{ maxLength: 500 }} /><Button variant="contained" disabled={saving} onClick={() => void saveAnnouncement()}>发布公告</Button></Stack></Card>
    <Dialog open={muteOpen} onClose={() => !saving && setMuteOpen(false)} fullWidth maxWidth="xs"><DialogTitle>设置群聊禁言</DialogTitle><DialogContent><Stack gap={1.5} pt={1}><Alert severity="warning">禁言仅限制会员在房间群聊发言，专属客服仍可正常联系。</Alert><TextField select label="禁言时长" value={muteMinutes} onChange={event => setMuteMinutes(event.target.value)}><MenuItem value="10">10 分钟</MenuItem><MenuItem value="60">1 小时</MenuItem><MenuItem value="480">8 小时</MenuItem><MenuItem value="1440">24 小时</MenuItem><MenuItem value="10080">7 天</MenuItem></TextField><TextField label="禁言原因（可选）" value={muteReason} onChange={event => setMuteReason(event.target.value)} multiline minRows={2} inputProps={{ maxLength: 300 }} /></Stack></DialogContent><DialogActions><Button disabled={saving} onClick={() => setMuteOpen(false)}>取消</Button><Button variant="contained" color="warning" disabled={saving} onClick={() => void saveMute(Number(muteMinutes))}>{saving ? '设置中…' : '确认禁言'}</Button></DialogActions></Dialog>
  </Box>
}
