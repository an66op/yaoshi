import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Divider,
  FormControlLabel,
  IconButton,
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
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type RebatePreview, type SystemSettings } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const emptySettings = (): SystemSettings => ({
  room_name: '曜图',
  room_code: '1231',
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
  game: { seal_seconds: 30, allow_cancel: true, default_fly_rate: 0, max_open_games: 8 },
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
  { key: 'require_join_review', label: '入群审核' },
  { key: 'sound_enabled', label: '提示音' },
  { key: 'show_odds', label: '显示游戏赔率' },
  { key: 'prediction_enabled', label: '预测功能' },
  { key: 'abnormal_login_alert', label: '异常登录提醒' },
  { key: 'security_password_check', label: '安全密码校验' },
]

const money = (value: number) => new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)

export function SystemPage() {
  const [tab, setTab] = useState(0)
  const [settings, setSettings] = useState<SystemSettings>(emptySettings)
  const [rebatePreview, setRebatePreview] = useState<RebatePreview | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const [result, preview] = await Promise.all([adminApi.settings(), adminApi.rebatePreview()])
      setSettings({
        ...emptySettings(),
        ...result,
        game: { ...emptySettings().game, ...(result.game ?? {}) },
        quick_replies: Array.isArray(result.quick_replies) ? result.quick_replies : [],
        rebate: { ...emptySettings().rebate, ...(result.rebate ?? {}) },
      })
      setRebatePreview(preview)
      if (notify) showMessage('系统设置已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取系统设置失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

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
      showMessage('系统设置已保存')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存系统设置失败')
    } finally {
      setSaving(false)
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
        description="管理房间资料、游戏规则、快捷回复与回水参数。"
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
        </Tabs>
        <Divider />
        <CardContent>
          {tab === 0 ? (
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)', xl: 'repeat(3,1fr)' }, gap: 2 }}>
              <TextField label="房间名称" value={settings.room_name} onChange={event => setSettings(current => ({ ...current, room_name: event.target.value }))} />
              <TextField label="房间号" value={settings.room_code} onChange={event => setSettings(current => ({ ...current, room_code: event.target.value }))} />
              <TextField label="聊天室昵称" value={settings.chat_nickname} onChange={event => setSettings(current => ({ ...current, chat_nickname: event.target.value }))} />
              {numberFields.map(field => (
                <TextField
                  key={field.key}
                  type="number"
                  label={field.label}
                  value={settings[field.key] as number}
                  onChange={event => setSettings(current => ({ ...current, [field.key]: Number(event.target.value) }))}
                />
              ))}
              {toggles.map(item => (
                <Paper variant="outlined" key={item.key} sx={{ p: 1.2 }}>
                  <FormControlLabel
                    control={<Switch checked={Boolean(settings[item.key])} onChange={event => setSettings(current => ({ ...current, [item.key]: event.target.checked }))} />}
                    label={item.label}
                  />
                </Paper>
              ))}
              <TextField
                multiline
                minRows={4}
                label="房间公告"
                value={settings.room_notice}
                onChange={event => setSettings(current => ({ ...current, room_notice: event.target.value }))}
                sx={{ gridColumn: { sm: '1/-1' } }}
              />
            </Box>
          ) : tab === 1 ? (
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)' }, gap: 2, maxWidth: 720 }}>
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
          ) : (
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
                <TextField label="结算模式" value={settings.rebate.settle_mode ?? 'daily'} onChange={e => setSettings(current => ({ ...current, rebate: { ...current.rebate, settle_mode: e.target.value } }))} helperText="当前支持 daily" />
              </Box>
              <Typography variant="caption" color="text.secondary">仪表盘「今日回水」按已结算注单流水 × 比例自动计算；点击入账后会写入用户余额并防重复结算。</Typography>
            </Stack>
          )}
        </CardContent>
      </Card>
    </Box>
  )
}
