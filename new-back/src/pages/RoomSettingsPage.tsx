import { Alert, Box, Button, Card, CardContent, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Divider, FormControlLabel, IconButton, MenuItem, Paper, Stack, Switch, TextField, Typography } from '@mui/material'
import SaveRounded from '@mui/icons-material/SaveRounded'
import AddRounded from '@mui/icons-material/AddRounded'
import CampaignRounded from '@mui/icons-material/CampaignRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import PushPinRounded from '@mui/icons-material/PushPinRounded'
import { useCallback, useEffect, useMemo, useState, type ChangeEvent } from 'react'
import { agentApi, tenantApi, type OpsActivity, type PaymentChannel, type PaymentChannelPayload, type RoomTradingConfig, type SystemSettings, type WorkspaceGame } from '../api'
import { getStoredUser } from '../auth'
import { PageHeader } from '../components/PageHeader'
import { RoomLogoPicker } from '../components/RoomLogoPicker'
import { GameOddsNavigation, OddsOverrideGrid } from '../components/OddsEditors'
import { useFeedback } from '../components/feedback'
import { PlanManagementPanel } from '../components/PlanManagementPanel'
import { prepareRoomLogo } from '../utils/roomLogo'

type Section = 'room' | 'content' | 'limits' | 'wallet'
type Announcement = SystemSettings['announcements'][number]
const titles: Record<Section, [string, string]> = {
  room: ['房间配置', '房间设置'], content: ['内容与服务', '公告与活动'], limits: ['彩票运营', '赔率与回水'], wallet: ['申请与财务', '收款方式'],
}

const emptyActivity = { type: 'promotion', title: '', subtitle: '', status: 'draft', cover: '', reward: 0, sort_order: 100, config: {} }
const emptyChannel: PaymentChannelPayload = { provider: 'manual', name: '', merchant_no: '-', credit_type: 'manual', fee_rate: 0, min_amount: 1, max_amount: 100000, status: 'enabled', remark: '', sort_order: 0, mode: 'manual', api_base: '', create_order_path: '', query_order_path: '', callback_path: '', secret_key: '', timeout_seconds: 10 }
const pinOptions = [
  { id: 'service', label: '在线客服' },
  { id: 'group', label: '聊天室' },
  { id: 'system', label: '系统通知' },
  { id: 'activity', label: '活动通知' },
  { id: 'plan', label: '计划群' },
]

const newAnnouncement = (order: number): Announcement => ({
  id: `announcement-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
  title: '新公告',
  content: '',
  enabled: true,
  popup_on_login: true,
  sort_order: order,
})

export function RoomSettingsPage({ section = 'room' }: { section?: Section }) {
  const role = getStoredUser()?.role ?? 'agent'
  const api = useMemo(() => role === 'tenant' ? tenantApi : agentApi, [role])
  const [data, setData] = useState<SystemSettings | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
	const [activities, setActivities] = useState<OpsActivity[]>([])
	const [channels, setChannels] = useState<PaymentChannel[]>([])
	const [activityDraft, setActivityDraft] = useState<(typeof emptyActivity) & { id?: number } | null>(null)
	const [channelDraft, setChannelDraft] = useState<(PaymentChannelPayload & { id?: number }) | null>(null)
	const [pendingDelete, setPendingDelete] = useState<{ kind: 'activity' | 'channel'; id: number; label: string } | null>(null)
	const [games, setGames] = useState<WorkspaceGame[]>([])
	const [trading, setTrading] = useState<RoomTradingConfig | null>(null)
	const [tradingLoading, setTradingLoading] = useState(false)
	const [tradingDirty, setTradingDirty] = useState(false)
  const { showMessage } = useFeedback()
  const load = useCallback(async () => {
    setError('')
		try {
			const settings = await api.settings()
			setData(settings)
			if (section === 'limits') {
				const [roomGames, roomTrading] = await Promise.all([api.games(), api.trading()])
				setGames(roomGames)
				setTrading(roomTrading)
				setTradingDirty(false)
			}
			if (section === 'content') setActivities(await api.activities('all'))
			if (section === 'wallet') setChannels(await api.walletChannels({ status: 'all' }))
		} catch (reason) { setError(reason instanceof Error ? reason.message : '读取房间设置失败') }
  }, [api, section])
  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => window.clearTimeout(timer)
  }, [load])
  const patch = <K extends keyof SystemSettings>(key: K, value: SystemSettings[K]) => setData(current => current ? { ...current, [key]: value } : current)
  const announcements = data?.announcements ?? []
  const pinnedRows = Array.isArray(data?.game.message_pinned_rows)
    ? data.game.message_pinned_rows.filter((item): item is string => typeof item === 'string')
    : ['service', 'group']
  const updateAnnouncement = (id: string, values: Partial<Announcement>) => setData(current => current ? {
    ...current,
    announcements: current.announcements.map(item => item.id === id ? { ...item, ...values } : item),
  } : current)
  const addAnnouncement = () => setData(current => {
    if (!current) return current
    const order = current.announcements.reduce((maximum, item) => Math.max(maximum, item.sort_order), 0) + 10
    return { ...current, announcements: [...current.announcements, newAnnouncement(order)] }
  })
  const removeAnnouncement = (id: string) => setData(current => current ? {
    ...current,
    announcements: current.announcements.filter(item => item.id !== id),
  } : current)
  const togglePin = (id: string, checked: boolean) => setData(current => {
    if (!current) return current
    const existing = Array.isArray(current.game.message_pinned_rows)
      ? current.game.message_pinned_rows.filter((item): item is string => typeof item === 'string')
      : ['service', 'group']
    const next = checked ? Array.from(new Set([...existing, id])) : existing.filter(item => item !== id)
    return { ...current, game: { ...current.game, message_pinned_rows: next } }
  })
  const save = async () => {
    if (!data) return
    if (section === 'content' && data.announcements.some(item => !item.title.trim() || !item.content.trim())) {
      setError('每条公告都需要填写标题和内容')
      return
    }
    setSaving(true); setError('')
    try {
			const payload = section === 'content' ? {
				...data,
				announcements: data.announcements.map(item => ({ ...item, title: item.title.trim(), content: item.content.trim() })),
			} : data
			setData(await api.updateSettings(payload))
			if (section === 'limits' && trading) {
				setTrading(await api.updateTrading({ rebate_rate: trading.rebate_rate, game_id: trading.game_id, odds: trading.odds.map(item => ({ play_code: item.play_code, override: item.has_override ? item.override : null })) }))
				setTradingDirty(false)
			}
			showMessage('当前房间设置已保存')
		}
    catch (reason) { setError(reason instanceof Error ? reason.message : '保存失败') }
    finally { setSaving(false) }
  }
	const chooseRoomLogo = async (event: ChangeEvent<HTMLInputElement>) => {
		const file = event.target.files?.[0]
		event.target.value = ''
		if (!file) return
		try { patch('room_logo', await prepareRoomLogo(file)) }
		catch (reason) { setError(reason instanceof Error ? reason.message : '处理房间 Logo 失败') }
	}
	const saveActivity = async () => {
		if (!activityDraft?.title.trim()) return
		setSaving(true); setError('')
		try {
			if (activityDraft.id) await api.updateActivity(activityDraft.id, activityDraft)
			else await api.createActivity(activityDraft)
			setActivityDraft(null); setActivities(await api.activities('all')); showMessage('活动已保存')
		} catch (reason) { setError(reason instanceof Error ? reason.message : '保存活动失败') }
		finally { setSaving(false) }
	}
	const loadTrading = async (gameId: string) => {
		if (tradingDirty) {
			showMessage('请先保存当前游戏赔率，再切换其他游戏', 'warning')
			return
		}
		setTradingLoading(true)
		setError('')
		try { setTrading(await api.trading(gameId)); setTradingDirty(false) }
		catch (reason) { setError(reason instanceof Error ? reason.message : '读取房间赔率失败') }
		finally { setTradingLoading(false) }
	}
	const saveTrading = async () => {
		if (!trading) return
		setSaving(true); setError('')
		try {
			setTrading(await api.updateTrading({ rebate_rate: trading.rebate_rate, game_id: trading.game_id, odds: trading.odds.map(item => ({ play_code: item.play_code, override: item.has_override ? item.override : null })) }))
			setTradingDirty(false)
			showMessage(`${trading.game_name} 房间赔率已保存`)
		} catch (reason) { setError(reason instanceof Error ? reason.message : '保存房间赔率失败') }
		finally { setSaving(false) }
	}
	const saveChannel = async () => {
		if (!channelDraft?.name.trim()) return
		setSaving(true); setError('')
		try {
			if (channelDraft.id) await api.updateWalletChannel(channelDraft.id, channelDraft)
			else await api.createWalletChannel(channelDraft)
			setChannelDraft(null); setChannels(await api.walletChannels({ status: 'all' })); showMessage('收款方式已保存')
		} catch (reason) { setError(reason instanceof Error ? reason.message : '保存收款方式失败') }
		finally { setSaving(false) }
	}
	const removeItem = async () => {
		if (!pendingDelete) return
		setSaving(true); setError('')
		try {
			if (pendingDelete.kind === 'activity') { await api.deleteActivity(pendingDelete.id); setActivities(await api.activities('all')) }
			else { await api.deleteWalletChannel(pendingDelete.id); setChannels(await api.walletChannels({ status: 'all' })) }
			setPendingDelete(null); showMessage('已删除')
		} catch (reason) { setError(reason instanceof Error ? reason.message : '删除失败') }
		finally { setSaving(false) }
	}
  const [eyebrow, title] = titles[section]
  return <Box p={{ xs: 1.5, md: section === 'limits' ? 2 : 2.5 }}>
			<PageHeader eyebrow={eyebrow} title={title} description="" actions={<Stack direction="row" gap={1}>{section === 'content' && <Button variant="outlined" startIcon={<CampaignRounded />} onClick={addAnnouncement}>新增公告</Button>}{section === 'content' && <Button variant="outlined" startIcon={<AddRounded />} onClick={() => setActivityDraft({ ...emptyActivity })}>新增活动</Button>}{section === 'wallet' && <Button variant="outlined" startIcon={<AddRounded />} onClick={() => setChannelDraft({ ...emptyChannel })}>新增收款方式</Button>}<Button variant="contained" startIcon={<SaveRounded />} disabled={!data || saving} onClick={() => void save()}>{saving ? '保存中…' : '保存设置'}</Button></Stack>} />
    {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
		    {data && <Card sx={{ mt: section === 'limits' ? 1.25 : 2, maxWidth: section === 'limits' ? 'none' : 1080, borderRadius: section === 'limits' ? 1.5 : undefined }}><CardContent sx={{ p: section === 'limits' ? { xs: 1, md: 1.25 } : { xs: 1.5, md: 2.5 } }}><Stack gap={section === 'limits' ? 1 : 2}>
      {section === 'room' && <>
        <Typography fontWeight={850}>入房与聊天</Typography>
				<Box sx={{ p: 1.4, border: 1, borderColor: 'divider', borderRadius: 2.2, bgcolor: data.room_enabled ? 'success.50' : 'action.hover' }}><FormControlLabel sx={{ m: 0 }} control={<Switch checked={data.room_enabled} onChange={event => patch('room_enabled', event.target.checked)} />} label={<Box><Typography fontWeight={850}>{data.room_enabled ? '房间营业中' : '房间已关闭'}</Typography><Typography variant="caption" color="text.secondary">关闭后会员不能进入或下注，管理账号仍可登录修改配置。</Typography></Box>} /></Box>
        <FormControlLabel control={<Switch checked={data.require_join_review} onChange={event => patch('require_join_review', event.target.checked)} />} label="入房审核" />
        <Typography variant="caption" color="text.secondary">开启后，会员提交申请并审核通过才能进入当前房间；新房间默认开启。</Typography>
        <Paper variant="outlined" sx={{ p: 1.4, borderRadius: 2.2 }}><RoomLogoPicker value={data.room_logo} fallback={data.room_name || '房'} heading={data.room_name || '当前房间'} description={`${data.chat_nickname || '开奖员'} · 将显示在彩票室和工作人员消息旁`} previewSize={58} onChange={room_logo => patch('room_logo', room_logo)} onUpload={chooseRoomLogo} /></Paper>
        <Stack direction={{ xs: 'column', md: 'row' }} gap={1.5}><TextField fullWidth label="房间名称" value={data.room_name} onChange={event => patch('room_name', event.target.value)} inputProps={{ maxLength: 30 }} /><TextField fullWidth label="客服 / 开奖员头衔" value={data.chat_nickname} onChange={event => patch('chat_nickname', event.target.value)} inputProps={{ maxLength: 80 }} helperText="用于后台发送消息时显示自己的身份" /></Stack>
        <Stack direction={{ xs: 'column', md: 'row' }} gap={1.5}><TextField fullWidth type="number" label="最低发言积分" value={data.min_chat_score} onChange={event => patch('min_chat_score', Number(event.target.value))} /><TextField fullWidth type="number" label="昵称显示长度" value={data.nickname_display_length} onChange={event => patch('nickname_display_length', Number(event.target.value))} /></Stack>
      </>}
	      {section === 'content' && <>
	        <Stack direction="row" alignItems="center" gap={1}><PushPinRounded color="primary" fontSize="small" /><Typography fontWeight={850}>消息入口置顶</Typography></Stack>
	        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,minmax(0,1fr))', md: 'repeat(5,minmax(0,1fr))' }, gap: 1 }}>
	          {pinOptions.map(item => <Paper key={item.id} variant="outlined" sx={{ px: 1.2, py: .7, borderColor: pinnedRows.includes(item.id) ? 'primary.main' : 'divider', bgcolor: pinnedRows.includes(item.id) ? 'action.selected' : 'background.paper' }}><FormControlLabel sx={{ m: 0, width: '100%', justifyContent: 'space-between', '& .MuiFormControlLabel-label': { fontSize: 12, fontWeight: 750 } }} labelPlacement="start" label={item.label} control={<Switch size="small" checked={pinnedRows.includes(item.id)} onChange={event => togglePin(item.id, event.target.checked)} />} /></Paper>)}
	        </Box>
	        <Divider />
	        <Stack direction="row" justifyContent="space-between" alignItems="center"><Box><Typography fontWeight={850}>大厅公告</Typography><Typography fontSize={10.5} color="text.secondary">公告只推送到当前房间；按排序从小到大展示。</Typography></Box><Chip size="small" color="primary" variant="outlined" label={`${announcements.filter(item => item.enabled).length}/${announcements.length} 展示`} /></Stack>
	        <Stack gap={1.2}>
	          {announcements.map((item, index) => <Paper key={item.id} variant="outlined" sx={{ p: { xs: 1.4, md: 1.7 }, borderColor: item.enabled ? 'primary.main' : 'divider', opacity: item.enabled ? 1 : .7 }}>
	            <Stack direction="row" alignItems="center" gap={1} mb={1.2}>
	              <Box width={27} height={27} borderRadius={1.5} display="grid" sx={{ placeItems: 'center', bgcolor: 'primary.main', color: 'primary.contrastText', fontSize: 11, fontWeight: 900 }}>{index + 1}</Box>
	              <Typography fontWeight={850} flex={1}>{item.title || '未命名公告'}</Typography>
	              <FormControlLabel label="启用" control={<Switch size="small" checked={item.enabled} onChange={event => updateAnnouncement(item.id, { enabled: event.target.checked })} />} sx={{ mr: 0, '& .MuiFormControlLabel-label': { fontSize: 11 } }} />
	              <IconButton size="small" color="error" aria-label="删除公告" onClick={() => removeAnnouncement(item.id)}><DeleteOutlineRounded fontSize="small" /></IconButton>
	            </Stack>
	            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'minmax(180px,.55fr) minmax(0,1.45fr) 120px' }, gap: 1.2 }}>
	              <TextField size="small" label="公告标题" value={item.title} inputProps={{ maxLength: 80 }} onChange={event => updateAnnouncement(item.id, { title: event.target.value })} />
	              <TextField size="small" multiline minRows={2} label="公告内容" value={item.content} inputProps={{ maxLength: 2000 }} onChange={event => updateAnnouncement(item.id, { content: event.target.value })} />
	              <Stack gap={.5}><TextField size="small" type="number" label="排序" value={item.sort_order} onChange={event => updateAnnouncement(item.id, { sort_order: Number(event.target.value) || 0 })} /><FormControlLabel label="登录后弹窗" control={<Switch size="small" checked={item.popup_on_login} onChange={event => updateAnnouncement(item.id, { popup_on_login: event.target.checked })} />} sx={{ m: 0, '& .MuiFormControlLabel-label': { fontSize: 10.5 } }} /></Stack>
	            </Box>
	          </Paper>)}
	          {!announcements.length && <Box textAlign="center" py={4}><CampaignRounded sx={{ fontSize: 36, color: 'text.disabled' }} /><Typography color="text.secondary" fontSize={12}>暂无公告，点击上方“新增公告”创建</Typography></Box>}
	        </Stack>
	        <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}><FormControlLabel control={<Switch checked={data.sound_enabled} onChange={event => patch('sound_enabled', event.target.checked)} />} label="通知声音" /><FormControlLabel control={<Switch checked={data.abnormal_login_alert} onChange={event => patch('abnormal_login_alert', event.target.checked)} />} label="异常登录提醒" /></Stack>
				<Divider /><Stack direction="row" justifyContent="space-between" alignItems="center"><Typography fontWeight={850}>活动列表</Typography><Chip size="small" label={`${activities.length} 项`} /></Stack>
				<Box display="grid" gridTemplateColumns={{ xs: '1fr', lg: 'repeat(2,minmax(0,1fr))' }} gap={1.2}>{activities.map(item => <Card key={item.id} variant="outlined" sx={{ borderRadius: 2.2 }}><CardContent sx={{ p: '14px !important' }}><Stack direction="row" gap={1.2} alignItems="center"><Box flex={1} minWidth={0}><Stack direction="row" gap={.8} alignItems="center"><Typography fontWeight={850} noWrap>{item.title}</Typography><Chip size="small" color={item.status === 'active' ? 'success' : 'default'} label={item.status === 'active' ? '启用' : item.status === 'ended' ? '已结束' : '草稿'} /></Stack><Typography fontSize={11} color="text.secondary" noWrap>{item.subtitle || '暂无副标题'} · 排序 {item.sort_order}</Typography></Box><Button size="small" onClick={() => setActivityDraft({ id: item.id, type: item.type, title: item.title, subtitle: item.subtitle, status: item.status, cover: item.cover, reward: item.reward, sort_order: item.sort_order, config: item.config })}>编辑</Button><Switch size="small" checked={item.status === 'active'} onChange={async event => { await api.setActivityStatus(item.id, event.target.checked ? 'active' : 'draft'); setActivities(await api.activities('all')) }} /><IconButton size="small" color="error" onClick={() => setPendingDelete({ kind: 'activity', id: item.id, label: item.title })}><DeleteOutlineRounded fontSize="small" /></IconButton></Stack></CardContent></Card>)}</Box>
	      </>}
      {section === 'limits' && <>
		        <Paper variant="outlined" sx={{ px: 1, py: .7, borderRadius: 1.1, bgcolor: 'action.hover' }}>
					  <Stack direction={{ xs: 'column', md: 'row' }} alignItems={{ md: 'center' }} gap={{ xs: .5, md: 1 }}>
					    <Box flex={1}><Typography fontSize={13} fontWeight={900}>房间规则</Typography><Typography fontSize={9.8} color="text.secondary">仅作用于当前房间，会员单独配置优先。</Typography></Box>
					    <Stack direction="row" gap={{ xs: .5, sm: 1 }} flexWrap="wrap" sx={{ '& .MuiFormControlLabel-root': { m: 0 }, '& .MuiFormControlLabel-label': { fontSize: 11.5, fontWeight: 700 } }}><FormControlLabel control={<Switch size="small" checked={data.show_odds} onChange={event => patch('show_odds', event.target.checked)} />} label="显示赔率" /><FormControlLabel control={<Switch size="small" checked={data.prediction_enabled} onChange={event => patch('prediction_enabled', event.target.checked)} />} label="预测" /><FormControlLabel control={<Switch size="small" checked={data.security_password_check} onChange={event => patch('security_password_check', event.target.checked)} />} label="安全校验" /></Stack>
					  </Stack>
					</Paper>
					<GameOddsNavigation games={games.filter(game => game.platform_enabled).map(game => ({ ...game, enabled: game.platform_enabled }))} gameId={trading?.game_id ?? ''} onSelect={gameId => void loadTrading(gameId)} />
					{!trading || tradingLoading ? <Box py={5} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={24} /></Box> : <>
					  <Paper variant="outlined" sx={{ p: .8, borderRadius: 1.1 }}>
					    <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} gap={.8}>
					      <Box flex={1}><Typography fontSize={13.5} fontWeight={900}>{trading.game_name}</Typography><Typography fontSize={9.8} color="text.secondary">房间 {trading.room_code} · {trading.odds.filter(item => item.has_override).length} 项单独赔率</Typography></Box>
					      <TextField size="small" type="number" label="默认回水 %" value={trading.rebate_rate} onChange={event => { setTrading(current => current ? { ...current, rebate_rate: Number(event.target.value) } : current); setTradingDirty(true) }} inputProps={{ min: 0, max: 100, step: 0.01 }} sx={{ width: { xs: '100%', sm: 170 }, '& .MuiOutlinedInput-root': { borderRadius: 1 } }} />
					      <Button size="small" variant="contained" startIcon={<SaveRounded />} disabled={saving || !tradingDirty} onClick={() => void saveTrading()} sx={{ minHeight: 36, whiteSpace: 'nowrap' }}>{saving ? '保存中…' : tradingDirty ? '保存当前游戏' : '已保存'}</Button>
					    </Stack>
					  </Paper>
					  <OddsOverrideGrid level="room" items={trading.odds} onChange={odds => { setTrading(current => current ? { ...current, odds } : current); setTradingDirty(true) }} />
					</>}
					<Paper variant="outlined" sx={{ px: 1, py: .65, borderRadius: 1, bgcolor: 'action.hover' }}><Stack direction={{ xs: 'column', sm: 'row' }} gap={.65} alignItems={{ sm: 'center' }}><Chip size="small" color="primary" variant="outlined" label="生效顺序" sx={{ height: 21, width: 'fit-content' }} /><Typography fontSize={10.5} color="text.secondary">会员单独赔率 → 当前房间赔率 → 平台默认；会员换房后不会带走本房配置。</Typography></Stack></Paper>
      </>}
      {section === 'wallet' && <>
        <Typography fontWeight={850}>人工上下分门槛</Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth type="number" label="最低上分金额" value={data.min_credit_amount} onChange={event => patch('min_credit_amount', Number(event.target.value))} /><TextField fullWidth type="number" label="最低下分金额" value={data.min_debit_amount} onChange={event => patch('min_debit_amount', Number(event.target.value))} /></Stack>
        <Alert severity="info">上下分只创建人工申请，不直接连接真实支付；审批通过后才在同一事务内变更余额并记录流水。</Alert>
				<Divider /><Stack direction="row" justifyContent="space-between" alignItems="center"><Typography fontWeight={850}>房间收款方式</Typography><Chip size="small" label={`${channels.filter(item => item.status === 'enabled').length}/${channels.length} 启用`} /></Stack>
				<Box display="grid" gridTemplateColumns={{ xs: '1fr', lg: 'repeat(2,minmax(0,1fr))' }} gap={1.2}>{channels.map(item => <Card key={item.id} variant="outlined" sx={{ borderRadius: 2.2 }}><CardContent sx={{ p: '14px !important' }}><Stack direction="row" gap={1.2} alignItems="center"><Box flex={1} minWidth={0}><Stack direction="row" gap={.8} alignItems="center"><Typography fontWeight={850} noWrap>{item.name}</Typography><Chip size="small" variant="outlined" label={item.credit_type} /></Stack><Typography fontSize={11} color="text.secondary">{item.min_amount}–{item.max_amount} · {item.mode === 'manual' ? '人工审核' : '接口模式'}</Typography></Box><Button size="small" onClick={() => setChannelDraft({ ...item, secret_key: '' })}>编辑</Button><Switch size="small" checked={item.status === 'enabled'} onChange={async event => { await api.setWalletChannelStatus(item.id, event.target.checked ? 'enabled' : 'disabled'); setChannels(await api.walletChannels({ status: 'all' })) }} /><IconButton size="small" color="error" onClick={() => setPendingDelete({ kind: 'channel', id: item.id, label: item.name })}><DeleteOutlineRounded fontSize="small" /></IconButton></Stack></CardContent></Card>)}</Box>
      </>}
    </Stack></CardContent></Card>}
		{section === 'content' && <PlanManagementPanel />}
		<Dialog open={Boolean(activityDraft)} onClose={() => !saving && setActivityDraft(null)} fullWidth maxWidth="sm"><DialogTitle>{activityDraft?.id ? '编辑活动' : '新增活动'}</DialogTitle><DialogContent><Stack gap={1.5} pt={1}><TextField select label="活动类型" value={activityDraft?.type ?? 'promotion'} onChange={event => setActivityDraft(current => current && ({ ...current, type: event.target.value }))}>{[['promotion','推广活动'],['checkin','每日签到'],['invite','邀请活动'],['redpacket','红包活动'],['banner','轮播内容']].map(([value,label]) => <MenuItem key={value} value={value}>{label}</MenuItem>)}</TextField><TextField label="标题" value={activityDraft?.title ?? ''} onChange={event => setActivityDraft(current => current && ({ ...current, title: event.target.value }))} /><TextField label="副标题" value={activityDraft?.subtitle ?? ''} onChange={event => setActivityDraft(current => current && ({ ...current, subtitle: event.target.value }))} /><TextField label="封面地址" value={activityDraft?.cover ?? ''} onChange={event => setActivityDraft(current => current && ({ ...current, cover: event.target.value }))} /><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth type="number" label="奖励积分" value={activityDraft?.reward ?? 0} onChange={event => setActivityDraft(current => current && ({ ...current, reward: Number(event.target.value) }))} /><TextField fullWidth type="number" label="排序" value={activityDraft?.sort_order ?? 0} onChange={event => setActivityDraft(current => current && ({ ...current, sort_order: Number(event.target.value) }))} /></Stack></Stack></DialogContent><DialogActions><Button onClick={() => setActivityDraft(null)}>取消</Button><Button variant="contained" disabled={saving || !activityDraft?.title.trim()} onClick={() => void saveActivity()}>保存</Button></DialogActions></Dialog>
		<Dialog open={Boolean(channelDraft)} onClose={() => !saving && setChannelDraft(null)} fullWidth maxWidth="sm"><DialogTitle>{channelDraft?.id ? '编辑收款方式' : '新增收款方式'}</DialogTitle><DialogContent><Stack gap={1.5} pt={1}><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth label="名称" value={channelDraft?.name ?? ''} onChange={event => setChannelDraft(current => current && ({ ...current, name: event.target.value }))} /><TextField fullWidth label="渠道标识" value={channelDraft?.provider ?? ''} onChange={event => setChannelDraft(current => current && ({ ...current, provider: event.target.value }))} /></Stack><TextField select label="收款类型" value={channelDraft?.credit_type ?? 'manual'} onChange={event => setChannelDraft(current => current && ({ ...current, credit_type: event.target.value }))}>{[['manual','人工处理'],['bank','银行卡'],['alipay','支付宝'],['wechat','微信'],['usdt','USDT']].map(([value,label]) => <MenuItem key={value} value={value}>{label}</MenuItem>)}</TextField><Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField fullWidth type="number" label="最低金额" value={channelDraft?.min_amount ?? 0} onChange={event => setChannelDraft(current => current && ({ ...current, min_amount: Number(event.target.value) }))} /><TextField fullWidth type="number" label="最高金额" value={channelDraft?.max_amount ?? 0} onChange={event => setChannelDraft(current => current && ({ ...current, max_amount: Number(event.target.value) }))} /></Stack><TextField label="说明" value={channelDraft?.remark ?? ''} onChange={event => setChannelDraft(current => current && ({ ...current, remark: event.target.value }))} multiline minRows={2} /></Stack></DialogContent><DialogActions><Button onClick={() => setChannelDraft(null)}>取消</Button><Button variant="contained" disabled={saving || !channelDraft?.name.trim()} onClick={() => void saveChannel()}>保存</Button></DialogActions></Dialog>
		<Dialog open={Boolean(pendingDelete)} onClose={() => !saving && setPendingDelete(null)}><DialogTitle>确认删除</DialogTitle><DialogContent><Typography>删除“{pendingDelete?.label}”后不会再向当前房间展示。</Typography></DialogContent><DialogActions><Button onClick={() => setPendingDelete(null)}>取消</Button><Button color="error" variant="contained" disabled={saving} onClick={() => void removeItem()}>删除</Button></DialogActions></Dialog>
  </Box>
}
