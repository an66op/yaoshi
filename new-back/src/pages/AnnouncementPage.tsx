import { Alert, Box, Button, Card, Chip, Divider, FormControlLabel, IconButton, Paper, Stack, Switch, TextField, Typography } from '@mui/material'
import AddRounded from '@mui/icons-material/AddRounded'
import CampaignRounded from '@mui/icons-material/CampaignRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import SaveRounded from '@mui/icons-material/SaveRounded'
import PushPinRounded from '@mui/icons-material/PushPinRounded'
import { useEffect, useMemo, useState } from 'react'
import { adminApi, type SystemSettings } from '../api'
import { useFeedback } from '../components/feedback'

type Announcement = SystemSettings['announcements'][number]

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

export function AnnouncementPage() {
  const [settings, setSettings] = useState<SystemSettings | null>(null)
  const [announcements, setAnnouncements] = useState<Announcement[]>([])
  const [pinnedRows, setPinnedRows] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  useEffect(() => {
    let cancelled = false
    void adminApi.settings().then(result => {
      if (cancelled) return
      setSettings(result)
      const items = Array.isArray(result.announcements) ? result.announcements : []
      setAnnouncements([...items].sort((a, b) => a.sort_order - b.sort_order))
      setPinnedRows(Array.isArray(result.game.message_pinned_rows)
        ? result.game.message_pinned_rows.filter((item): item is string => typeof item === 'string')
        : ['service', 'group'])
    }).catch(reason => {
      if (!cancelled) setError(reason instanceof Error ? reason.message : '读取公告设置失败')
    }).finally(() => {
      if (!cancelled) setLoading(false)
    })
    return () => { cancelled = true }
  }, [])

  const enabledCount = useMemo(() => announcements.filter(item => item.enabled).length, [announcements])
  const popupCount = useMemo(() => announcements.filter(item => item.enabled && item.popup_on_login).length, [announcements])

  const updateAnnouncement = (id: string, patch: Partial<Announcement>) => {
    setAnnouncements(current => current.map(item => item.id === id ? { ...item, ...patch } : item))
  }

  const addAnnouncement = () => {
    const order = announcements.reduce((max, item) => Math.max(max, item.sort_order), 0) + 10
    setAnnouncements(current => [...current, newAnnouncement(order)])
  }

  const removeAnnouncement = (id: string) => {
    setAnnouncements(current => current.filter(item => item.id !== id))
  }

  const togglePin = (id: string, checked: boolean) => {
    setPinnedRows(current => checked ? Array.from(new Set([...current, id])) : current.filter(item => item !== id))
  }

  const save = async () => {
    if (!settings) return
    if (announcements.some(item => !item.title.trim() || !item.content.trim())) {
      setError('每条公告都需要填写标题和内容')
      return
    }
    setSaving(true)
    setError('')
    try {
      const result = await adminApi.updateSettings({
        ...settings,
        announcements: announcements.map(item => ({ ...item, title: item.title.trim(), content: item.content.trim() })),
        game: { ...settings.game, message_pinned_rows: pinnedRows },
      })
      setSettings(result)
      const items = Array.isArray(result.announcements) ? result.announcements : []
      setAnnouncements([...items].sort((a, b) => a.sort_order - b.sort_order))
      showMessage('公告和置顶设置已保存')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存公告设置失败')
    } finally {
      setSaving(false)
    }
  }

  return <Box p={{ xs: 2, lg: 2.5 }}>
    {error && <Alert severity="error" onClose={() => setError('')} sx={{ mb: 1.5 }}>{error}</Alert>}
    <Stack gap={1.5} maxWidth={1080} mx="auto">
      <Card sx={{ p: { xs: 2, md: 2.5 }, overflow: 'hidden', position: 'relative' }}>
        <Box sx={{ position: 'absolute', width: 190, height: 190, borderRadius: '50%', right: -72, top: -92, bgcolor: 'primary.main', opacity: .08 }} />
        <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} gap={1.5} position="relative">
          <Box width={48} height={48} borderRadius={2.5} display="grid" sx={{ placeItems: 'center', color: 'primary.contrastText', background: 'linear-gradient(145deg,#168bae,#27b7aa)' }}><CampaignRounded /></Box>
          <Box flex={1}><Typography fontSize={18} fontWeight={900}>公告</Typography><Typography fontSize={11} color="text.secondary">大厅公告与消息入口置顶</Typography></Box>
          <Stack direction="row" gap={.7}><Chip size="small" label={`${enabledCount} 条展示`} color="primary" variant="outlined" /><Chip size="small" label={`${popupCount} 条登录弹窗`} variant="outlined" /></Stack>
          <Button variant="contained" startIcon={<SaveRounded />} disabled={loading || saving || !settings} onClick={() => void save()}>{saving ? '保存中…' : '保存'}</Button>
        </Stack>
      </Card>

      <Card sx={{ p: { xs: 1.5, md: 2 } }}>
        <Stack direction="row" alignItems="center" gap={1} mb={1.3}><PushPinRounded color="primary" fontSize="small" /><Typography fontWeight={850}>消息入口置顶</Typography></Stack>
        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,minmax(0,1fr))', md: 'repeat(5,minmax(0,1fr))' }, gap: 1 }}>
          {pinOptions.map(item => <Paper key={item.id} variant="outlined" sx={{ px: 1.2, py: .7, borderColor: pinnedRows.includes(item.id) ? 'primary.main' : 'divider', bgcolor: pinnedRows.includes(item.id) ? 'action.selected' : 'background.paper' }}><FormControlLabel sx={{ m: 0, width: '100%', justifyContent: 'space-between', '& .MuiFormControlLabel-label': { fontSize: 12, fontWeight: 750 } }} labelPlacement="start" label={item.label} control={<Switch size="small" checked={pinnedRows.includes(item.id)} onChange={event => togglePin(item.id, event.target.checked)} />} /></Paper>)}
        </Box>
      </Card>

      <Card sx={{ overflow: 'hidden' }}>
        <Stack direction="row" alignItems="center" justifyContent="space-between" px={{ xs: 1.5, md: 2 }} py={1.4}>
          <Box><Typography fontWeight={850}>大厅公告</Typography><Typography fontSize={10.5} color="text.secondary">按排序数字从小到大展示；登录弹窗只展示已启用且勾选弹窗的公告。</Typography></Box>
          <Button size="small" variant="outlined" startIcon={<AddRounded />} onClick={addAnnouncement}>新增公告</Button>
        </Stack>
        <Divider />
        <Stack gap={1.2} p={{ xs: 1.5, md: 2 }}>
          {loading ? <Typography color="text.secondary" py={4} textAlign="center">正在加载…</Typography> : announcements.map((item, index) => <Paper key={item.id} variant="outlined" sx={{ p: { xs: 1.4, md: 1.7 }, borderColor: item.enabled ? 'primary.main' : 'divider', opacity: item.enabled ? 1 : .7 }}>
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
          {!loading && !announcements.length && <Box textAlign="center" py={5}><CampaignRounded sx={{ fontSize: 38, color: 'text.disabled' }} /><Typography color="text.secondary" fontSize={12}>暂无公告，点击右上角新增</Typography></Box>}
        </Stack>
      </Card>
    </Stack>
  </Box>
}
