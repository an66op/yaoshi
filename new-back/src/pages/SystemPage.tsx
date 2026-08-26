import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Divider,
  FormControlLabel,
  IconButton,
  MenuItem,
  Paper,
  Stack,
  Switch,
  Tab,
  Tabs,
  TextField,
  Typography,
} from '@mui/material'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SaveRounded from '@mui/icons-material/SaveRounded'
import AddRounded from '@mui/icons-material/AddRounded'
import DeleteRounded from '@mui/icons-material/DeleteRounded'
import PhotoCameraRounded from '@mui/icons-material/PhotoCameraRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import { useCallback, useEffect, useState, type ChangeEvent } from 'react'
import { adminApi, type AuditLogPage, type RebatePreview, type ReconciliationSummary, type RoomActivityStatus, type SystemSettings } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'
import { prepareRoomLogo } from '../utils/roomLogo'

const emptySettings = (): SystemSettings => ({
  room_name: '王者大厅',
  room_logo: '',
  chat_nickname: '群主',
  nickname_display_length: 0,
  min_chat_score: 0,
  min_credit_amount: 0,
  min_debit_amount: 0,
  require_join_review: true,
  sound_enabled: true,
  show_odds: true,
  prediction_enabled: true,
  abnormal_login_alert: false,
  security_password_check: false,
  room_notice: '',
  announcements: [],
  game: {
    seal_seconds: 30,
    allow_cancel: true,
    default_fly_rate: 0,
    max_open_games: 8,
    room_activity_enabled: true,
    room_activity_interval_secs: 10,
    room_activity_bots_per_room: 6,
    room_activity_bets_per_cycle: 2,
    room_activity_chat_chance_percent: 0,
    show_member_turnover: true,
    show_member_profit: true,
    show_member_rebate: true,
    web_keyboard_enabled: true,
    show_mipai_tool: true,
    show_orders_tool: true,
    show_streak_tool: true,
    show_prediction_tool: true,
  },
  quick_replies: [],
  rebate: { enabled: true, rate_percent: 0.5, min_turnover: 0, settle_mode: 'daily', auto_credit: false },
})

const numberFields: Array<{ key: keyof SystemSettings; label: string }> = [
  { key: 'nickname_display_length', label: '昵称显示长度' },
  { key: 'min_chat_score', label: '最低发言分数' },
  { key: 'min_credit_amount', label: '最低上分金额' },
  { key: 'min_debit_amount', label: '最低下分金额' },
]

const toggles: Array<{ key: keyof SystemSettings; label: string }> = [
  { key: 'require_join_review', label: '入房审核' },
  { key: 'sound_enabled', label: '提示音' },
  { key: 'show_odds', label: '显示游戏赔率' },
  { key: 'prediction_enabled', label: '预测功能' },
  { key: 'abnormal_login_alert', label: '异常登录提醒' },
  { key: 'security_password_check', label: '安全密码校验' },
]

const roomDisplayToggles: Array<{ key: keyof SystemSettings['game']; label: string; caption: string }> = [
  { key: 'show_member_turnover', label: '显示会员流水', caption: '游戏顶部显示今日有效投注流水' },
  { key: 'show_member_profit', label: '显示会员输赢', caption: '游戏顶部显示今日已结算输赢' },
  { key: 'show_member_rebate', label: '显示会员回水', caption: '游戏顶部显示今日回水' },
  { key: 'web_keyboard_enabled', label: '快捷投注键盘', caption: '关闭后改用手机或电脑系统输入法' },
  { key: 'show_mipai_tool', label: '咪牌', caption: '显示游戏房间的咪牌工具' },
  { key: 'show_orders_tool', label: '注单', caption: '显示本彩种注单工具' },
  { key: 'show_streak_tool', label: '长龙', caption: '显示历史长龙走势工具' },
  { key: 'show_prediction_tool', label: '预测', caption: '需同时开启平台预测功能' },
]

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
const activityTime = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', {
  month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
}).format(new Date(value)) : '尚未执行'

export function SystemPage() {
  const [tab, setTab] = useState(0)
  const [settings, setSettings] = useState<SystemSettings>(emptySettings)
  const [rebatePreview, setRebatePreview] = useState<RebatePreview | null>(null)
  const [roomActivity, setRoomActivity] = useState<RoomActivityStatus | null>(null)
  const [auditLogs, setAuditLogs] = useState<AuditLogPage | null>(null)
  const [reconciliation, setReconciliation] = useState<ReconciliationSummary | null>(null)
  const [activityRunning, setActivityRunning] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const reconciliationErrorTotal = reconciliation ? [
    reconciliation.issue_error_count,
    reconciliation.abnormal_bet_count,
    reconciliation.pending_on_closed_count,
    reconciliation.negative_balance_count,
    reconciliation.orphan_ledger_count,
    reconciliation.duplicate_ledger_reference_count,
    reconciliation.ledger_chain_gap_count,
    reconciliation.ledger_arithmetic_error_count,
    reconciliation.latest_balance_gap_count,
    reconciliation.untracked_balance_user_count,
    reconciliation.payment_account_error_count,
    reconciliation.payment_channel_error_count,
    reconciliation.notification_financial_error_count,
    reconciliation.rebate_financial_error_count,
    reconciliation.profit_share_financial_error_count,
  ].reduce((total, value) => total + value, 0) : 0

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const [result, preview, activity, logs, check] = await Promise.all([
        adminApi.settings(), adminApi.rebatePreview(), adminApi.roomActivityStatus(), adminApi.auditLogs(), adminApi.reconciliation(),
      ])
      setSettings({
        ...emptySettings(),
        ...result,
        game: { ...emptySettings().game, ...(result.game ?? {}) },
        quick_replies: Array.isArray(result.quick_replies) ? result.quick_replies : [],
        rebate: { ...emptySettings().rebate, ...(result.rebate ?? {}) },
      })
      setRebatePreview(preview)
      setRoomActivity(activity)
      setAuditLogs(logs)
      setReconciliation(check)
      if (notify) showMessage('系统设置已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取系统设置失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  useEffect(() => {
    const timer = window.setInterval(() => {
      void adminApi.roomActivityStatus().then(setRoomActivity).catch(() => undefined)
    }, 5000)
    return () => window.clearInterval(timer)
  }, [])

  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const result = await adminApi.updateSettings(settings)
      setSettings({
        ...emptySettings(),
        ...result,
        game: { ...emptySettings().game, ...(result.game ?? {}) },
        quick_replies: Array.isArray(result.quick_replies) ? result.quick_replies : [],
        rebate: { ...emptySettings().rebate, ...(result.rebate ?? {}) },
      })
      setRebatePreview(await adminApi.rebatePreview())
      setRoomActivity(await adminApi.roomActivityStatus())
      showMessage('系统设置已保存')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存系统设置失败')
    } finally {
      setSaving(false)
    }
  }

  const chooseRoomLogo = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    try {
      const roomLogo = await prepareRoomLogo(file)
      setSettings(current => ({ ...current, room_logo: roomLogo }))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '处理房间 Logo 失败')
    }
  }

  const runRoomActivity = async () => {
    setActivityRunning(true)
    setError('')
    try {
      const status = await adminApi.runRoomActivityOnce()
      setRoomActivity(status)
      showMessage('房间活跃已执行一轮')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '执行房间活跃失败')
    } finally {
      setActivityRunning(false)
    }
  }

  const runRebate = async () => {
    setSaving(true)
    try {
      const result = await adminApi.runRebate() as { user_count?: number; total_rebate?: number }
      showMessage(`回水已入账：${result.user_count ?? 0} 人，合计 ${money(result.total_rebate ?? 0)}`)
      setRebatePreview(await adminApi.rebatePreview())
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '回水结算失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="系统 / 配置"
        title="系统设置"
        description="管理平台规则、快捷回复与回水参数。"
        actions={
          <>
            <Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)} disabled={loading || saving}>刷新</Button>
            <Button variant="contained" startIcon={<SaveRounded />} onClick={() => void save()} disabled={loading || saving}>{saving ? '保存中…' : '保存设置'}</Button>
          </>
        }
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      <Card sx={{ mt: 2.5 }}>
        {loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}
        <Tabs value={tab} onChange={(_, next) => setTab(next)} variant="scrollable" scrollButtons="auto">
          <Tab label="基础设置" />
          <Tab label="游戏设置" />
          <Tab label="快捷回复" />
          <Tab label="回水设置" />
          <Tab label="审计与对账" />
        </Tabs>
        <Divider />
        <CardContent>
          {tab === 0 ? (
            <Stack gap={2}>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography fontWeight={850} mb={.4}>账户与聊天室</Typography>
                <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} gap={1.5} my={2}>
                  <Avatar src={settings.room_logo || undefined} variant="rounded" sx={{ width: 72, height: 72, bgcolor: 'primary.main', fontSize: 28, fontWeight: 900 }}>
                    {(settings.room_name || '王').slice(0, 1)}
                  </Avatar>
                  <Stack direction="row" gap={1} flexWrap="wrap">
                    <Button component="label" variant="outlined" startIcon={<PhotoCameraRounded />}>
                      {settings.room_logo ? '更换 Logo' : '选择 Logo'}
                      <input hidden type="file" accept="image/png,image/jpeg,image/webp" onChange={chooseRoomLogo} />
                    </Button>
                    {settings.room_logo && <Button color="error" variant="text" startIcon={<DeleteOutlineRounded />} onClick={() => setSettings(current => ({ ...current, room_logo: '' }))}>移除</Button>}
                  </Stack>
                  <Typography variant="caption" color="text.secondary">未进入专属房间时使用；代理房间可单独覆盖名称和 Logo。</Typography>
                </Stack>
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)', xl: 'repeat(3,1fr)' }, gap: 2 }}>
                  <TextField label="默认大厅名称" value={settings.room_name} onChange={event => setSettings(current => ({ ...current, room_name: event.target.value }))} inputProps={{ maxLength: 30 }} />
                  <TextField label="客服与助手昵称" value={settings.chat_nickname} onChange={event => setSettings(current => ({ ...current, chat_nickname: event.target.value }))} />
                  {numberFields.map(field => (
                    <TextField
                      key={field.key}
                      type="number"
                      label={field.label}
                      value={settings[field.key] as number}
                      onChange={event => setSettings(current => ({ ...current, [field.key]: Number(event.target.value) }))}
                    />
                  ))}
                </Box>
              </Paper>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography fontWeight={850} mb={1.5}>访问、声音与安全</Typography>
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)', xl: 'repeat(3,1fr)' }, gap: 1.25 }}>
                  {toggles.map(item => (
                    <Paper variant="outlined" key={item.key} sx={{ px: 1.4, py: .65, bgcolor: 'action.hover' }}>
                      <FormControlLabel
                        sx={{ m: 0, width: '100%', justifyContent: 'space-between' }}
                        labelPlacement="start"
                        control={<Switch checked={Boolean(settings[item.key])} onChange={event => setSettings(current => ({ ...current, [item.key]: event.target.checked }))} />}
                        label={<Typography fontSize={14} fontWeight={700}>{item.label}</Typography>}
                      />
                    </Paper>
                  ))}
                </Box>
              </Paper>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography fontWeight={850} mb={.4}>游戏房间显示</Typography>
                <Typography variant="caption" color="text.secondary" display="block" mb={1.5}>这些开关直接控制用户下注页面已有功能，不增加无效的占位设置。</Typography>
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)' }, gap: 1.25 }}>
                  {roomDisplayToggles.map(item => (
                    <Paper variant="outlined" key={item.key} sx={{ px: 1.5, py: 1 }}>
                      <Stack direction="row" alignItems="center" justifyContent="space-between" gap={1.5}>
                        <Box minWidth={0}>
                          <Typography fontSize={14} fontWeight={800}>{item.label}</Typography>
                          <Typography variant="caption" color="text.secondary">{item.caption}</Typography>
                        </Box>
                        <Switch checked={settings.game[item.key] !== false} onChange={event => setSettings(current => ({ ...current, game: { ...current.game, [item.key]: event.target.checked } }))} />
                      </Stack>
                    </Paper>
                  ))}
                </Box>
              </Paper>
              <Alert severity="info">大厅公告与消息置顶已集中到“内容与服务 → 公告”管理。</Alert>
            </Stack>
          ) : tab === 1 ? (
            <Stack gap={2} maxWidth={820}>
              <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)' }, gap: 2 }}>
                <TextField type="number" label="封盘秒数" value={settings.game.seal_seconds ?? 30} onChange={e => setSettings(current => ({ ...current, game: { ...current.game, seal_seconds: Number(e.target.value) } }))} />
                <TextField type="number" label="默认可开游戏数" value={settings.game.max_open_games ?? 8} onChange={e => setSettings(current => ({ ...current, game: { ...current.game, max_open_games: Number(e.target.value) } }))} />
                <TextField type="number" label="默认飞单比例 %" value={settings.game.default_fly_rate ?? 0} onChange={e => setSettings(current => ({ ...current, game: { ...current.game, default_fly_rate: Number(e.target.value) } }))} />
                <Paper variant="outlined" sx={{ p: 1.2 }}>
                  <FormControlLabel
                    control={<Switch checked={Boolean(settings.game.allow_cancel)} onChange={e => setSettings(current => ({ ...current, game: { ...current.game, allow_cancel: e.target.checked } }))} />}
                    label="允许待结算撤单"
                  />
                </Paper>
              </Box>
              <Paper variant="outlined" sx={{ p: 2, borderColor: 'primary.main', bgcolor: 'action.hover' }}>
                <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ sm: 'center' }} gap={1} mb={2}>
                  <Box>
                    <Typography fontWeight={800}>房间自动活跃</Typography>
                    <Typography variant="body2" color="text.secondary">为每个启用房间创建独立虚拟会员，自动产生真实注单与持久化群聊；客户端不显示机器人标识。</Typography>
                  </Box>
                  <FormControlLabel
                    control={<Switch checked={Boolean(settings.game.room_activity_enabled)} onChange={e => setSettings(current => ({ ...current, game: { ...current.game, room_activity_enabled: e.target.checked } }))} />}
                    label={settings.game.room_activity_enabled ? '已启用' : '已关闭'}
                  />
                </Stack>
                <Alert
                  severity={roomActivity?.last_error ? 'error' : roomActivity?.running && roomActivity?.enabled ? 'success' : 'warning'}
                  action={
                    <Button color="inherit" size="small" disabled={activityRunning || !roomActivity?.running} onClick={() => void runRoomActivity()}>
                      {activityRunning ? '执行中…' : '立即执行一轮'}
                    </Button>
                  }
                  sx={{ mb: 2 }}
                >
                  <Typography variant="body2" fontWeight={750}>
                    {roomActivity?.last_error
                      ? roomActivity.last_error
                      : roomActivity?.running
                        ? roomActivity.enabled ? '服务运行中' : '服务在线，自动执行已关闭'
                        : '服务未运行'}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {roomActivity
                      ? `${roomActivity.target_rooms} 个房间 · ${roomActivity.bot_accounts} 个活跃账号 · ${roomActivity.enabled_games} 个彩种 · 最近执行 ${activityTime(roomActivity.last_run_at)}`
                      : '正在读取运行状态…'}
                  </Typography>
                </Alert>
                {roomActivity && (
                  <Stack direction="row" gap={3} mb={2} flexWrap="wrap">
                    <Box><Typography variant="caption" color="text.secondary">累计轮次</Typography><Typography fontWeight={800}>{roomActivity.cycles}</Typography></Box>
                    <Box><Typography variant="caption" color="text.secondary">真实注单</Typography><Typography fontWeight={800}>{roomActivity.bets_placed}</Typography></Box>
                  </Stack>
                )}
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)' }, gap: 2 }}>
                  <TextField type="number" label="执行间隔（秒）" inputProps={{ min: 5, max: 120 }} value={settings.game.room_activity_interval_secs ?? 10} onChange={e => setSettings(current => ({ ...current, game: { ...current.game, room_activity_interval_secs: Number(e.target.value) } }))} helperText="5–120 秒；保存后自动生效" />
                  <TextField type="number" label="每个房间机器人数量" inputProps={{ min: 1, max: 16 }} value={settings.game.room_activity_bots_per_room ?? 6} onChange={e => setSettings(current => ({ ...current, game: { ...current.game, room_activity_bots_per_room: Number(e.target.value) } }))} helperText="1–16 个，账户和昵称互相独立" />
                  <TextField type="number" label="每轮每房间注单数" inputProps={{ min: 1, max: 8 }} value={settings.game.room_activity_bets_per_cycle ?? 2} onChange={e => setSettings(current => ({ ...current, game: { ...current.game, room_activity_bets_per_cycle: Number(e.target.value) } }))} helperText="随机分布到启用彩种" />
                  <Alert severity="info" sx={{ gridColumn: '1 / -1' }}>
                    活跃账号只生成真实注单动态，不在游戏房间发送随机闲聊。
                  </Alert>
                </Box>
              </Paper>
            </Stack>
          ) : tab === 2 ? (
            <Stack gap={1.5}>
              {(settings.quick_replies ?? []).map((item, index) => (
                <Paper key={index} variant="outlined" sx={{ p: 1.5 }}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} gap={1} alignItems={{ sm: 'flex-start' }}>
                    <TextField label="标题" value={item.title ?? ''} onChange={e => setSettings(current => {
                      const next = [...current.quick_replies]
                      next[index] = { ...next[index], title: e.target.value }
                      return { ...current, quick_replies: next }
                    })} sx={{ minWidth: 160 }} />
                    <TextField fullWidth multiline minRows={2} label="内容" value={item.content ?? ''} onChange={e => setSettings(current => {
                      const next = [...current.quick_replies]
                      next[index] = { ...next[index], content: e.target.value }
                      return { ...current, quick_replies: next }
                    })} />
                    <IconButton aria-label="删除" onClick={() => setSettings(current => ({ ...current, quick_replies: current.quick_replies.filter((_, i) => i !== index) }))}><DeleteRounded /></IconButton>
                  </Stack>
                </Paper>
              ))}
              <Button startIcon={<AddRounded />} variant="outlined" onClick={() => setSettings(current => ({ ...current, quick_replies: [...current.quick_replies, { title: '', content: '' }] }))}>添加快捷回复</Button>
            </Stack>
          ) : tab === 3 ? (
            <Stack gap={2} maxWidth={720}>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography fontWeight={750} mb={1}>今日回水预览</Typography>
                <Typography variant="body2" color="text.secondary">日期 {rebatePreview?.biz_date ?? '—'} · 比例 {rebatePreview?.rate_percent ?? settings.rebate.rate_percent ?? 0}%</Typography>
                <Stack direction="row" gap={3} mt={1.5} flexWrap="wrap">
                  <Box><Typography variant="caption" color="text.secondary">预估</Typography><Typography fontWeight={800}>{money(rebatePreview?.estimated ?? 0)}</Typography></Box>
                  <Box><Typography variant="caption" color="text.secondary">已入账</Typography><Typography fontWeight={800}>{money(rebatePreview?.credited ?? 0)}</Typography></Box>
                  <Box><Typography variant="caption" color="text.secondary">待入账</Typography><Typography fontWeight={800}>{money(rebatePreview?.pending_credit ?? 0)}</Typography></Box>
                </Stack>
                <Button sx={{ mt: 2 }} variant="contained" disabled={saving || !(rebatePreview?.enabled ?? settings.rebate.enabled)} onClick={() => void runRebate()}>执行今日回水入账</Button>
              </Paper>
              <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)' }, gap: 2 }}>
                <Paper variant="outlined" sx={{ p: 1.2 }}>
                  <FormControlLabel
                    control={<Switch checked={Boolean(settings.rebate.enabled)} onChange={e => setSettings(current => ({ ...current, rebate: { ...current.rebate, enabled: e.target.checked } }))} />}
                    label="启用回水"
                  />
                </Paper>
                <TextField type="number" label="回水比例 %" value={settings.rebate.rate_percent ?? 0} onChange={e => setSettings(current => ({ ...current, rebate: { ...current.rebate, rate_percent: Number(e.target.value) } }))} />
                <TextField type="number" label="最低流水门槛" value={settings.rebate.min_turnover ?? 0} onChange={e => setSettings(current => ({ ...current, rebate: { ...current.rebate, min_turnover: Number(e.target.value) } }))} />
                <TextField select label="结算模式" value={settings.rebate.settle_mode ?? 'daily'} onChange={e => setSettings(current => ({ ...current, rebate: { ...current.rebate, settle_mode: e.target.value } }))}>
                  <MenuItem value="daily">每日结算</MenuItem>
                </TextField>
              </Box>
              <Typography variant="caption" color="text.secondary">仪表盘「今日回水」按已结算注单流水 × 比例自动计算；点击入账后会写入用户余额并防重复结算。</Typography>
            </Stack>
          ) : (
            <Stack gap={2}>
              <Alert severity={reconciliationErrorTotal > 0 ? 'warning' : 'success'}>
                对账时间 {reconciliation ? new Date(reconciliation.generated_at).toLocaleString('zh-CN') : '读取中'}；异常数据只进入人工核对队列，不会自动改动历史余额或开奖结果。
              </Alert>
              <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', md: 'repeat(4,1fr)' }, gap: 1.5 }}>
                {[
                  ['开奖源异常', reconciliation?.issue_error_count ?? 0],
                  ['待人工注单', reconciliation?.abnormal_bet_count ?? 0],
                  ['封闭期未结算', reconciliation?.pending_on_closed_count ?? 0],
                  ['负余额账户', reconciliation?.negative_balance_count ?? 0],
                  ['孤儿流水', reconciliation?.orphan_ledger_count ?? 0],
                  ['重复业务流水', reconciliation?.duplicate_ledger_reference_count ?? 0],
                  ['流水断点', reconciliation?.ledger_chain_gap_count ?? 0],
                  ['流水算术错误', reconciliation?.ledger_arithmetic_error_count ?? 0],
                  ['余额差异', reconciliation?.latest_balance_gap_count ?? 0],
                  ['无流水余额', reconciliation?.untracked_balance_user_count ?? 0],
                  ['收款账户异常', reconciliation?.payment_account_error_count ?? 0],
                  ['收款渠道异常', reconciliation?.payment_channel_error_count ?? 0],
                  ['通知金额异常', reconciliation?.notification_financial_error_count ?? 0],
                  ['回水记录异常', reconciliation?.rebate_financial_error_count ?? 0],
                  ['代理分成异常', reconciliation?.profit_share_financial_error_count ?? 0],
                ].map(([label, value]) => <Paper key={label} variant="outlined" sx={{ p: 1.6 }}><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontSize={24} fontWeight={850}>{value}</Typography></Paper>)}
              </Box>
              <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' }, gap: 2 }}>
                <Paper variant="outlined" sx={{ p: 2 }}>
                  <Typography fontWeight={800} mb={1}>最近开奖异常</Typography>
                  <Stack divider={<Divider flexItem />}>
                    {reconciliation?.issue_errors.length ? reconciliation.issue_errors.slice(0, 10).map(item => <Box py={1} key={item.id}><Typography fontWeight={700}>{item.game_id} · 第 {item.issue} 期</Typography><Typography variant="caption" color="error.main">{item.last_error || item.status}</Typography></Box>) : <Typography color="text.secondary">暂无开奖异常</Typography>}
                  </Stack>
                </Paper>
                <Paper variant="outlined" sx={{ p: 2 }}>
                  <Typography fontWeight={800} mb={1}>待人工核对注单</Typography>
                  <Stack divider={<Divider flexItem />}>
                    {reconciliation?.abnormal_bets.length ? reconciliation.abnormal_bets.slice(0, 10).map(item => <Box py={1} key={item.id}><Typography fontWeight={700}>#{item.id} · {item.game_id} · {item.username}</Typography><Typography variant="caption" color="text.secondary">第 {item.issue} 期 · {item.reconciliation_note || '等待人工核对'}</Typography></Box>) : <Typography color="text.secondary">暂无待核对注单</Typography>}
                  </Stack>
                </Paper>
              </Box>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography fontWeight={800} mb={1}>管理员与代理操作记录</Typography>
                <Stack divider={<Divider flexItem />}>
                  {auditLogs?.items.length ? auditLogs.items.slice(0, 20).map(item => <Stack key={item.id} py={1} direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={.5}><Box><Typography fontWeight={700}>{item.actor_name || `账号 ${item.actor_id}`} · {item.method} {item.path}</Typography><Typography variant="caption" color="text.secondary">{item.actor_role}{item.room_scope ? ` · ${item.room_scope}` : ''} · {item.ip || '本机'}</Typography></Box><Typography variant="caption" color={item.status_code >= 400 ? 'error.main' : 'success.main'}>{item.status_code} · {new Date(item.created_at).toLocaleString('zh-CN')}</Typography></Stack>) : <Typography color="text.secondary">暂无操作记录</Typography>}
                </Stack>
              </Paper>
            </Stack>
          )}
        </CardContent>
      </Card>
    </Box>
  )
}
