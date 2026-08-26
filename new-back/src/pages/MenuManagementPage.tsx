import {
  Alert, Box, Button, Card, CardContent, Chip, CircularProgress, IconButton, MenuItem, Paper, Stack, Switch,
  TextField, Tooltip, Typography,
} from '@mui/material'
import ArrowDownwardRounded from '@mui/icons-material/ArrowDownwardRounded'
import ArrowUpwardRounded from '@mui/icons-material/ArrowUpwardRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import RestartAltRounded from '@mui/icons-material/RestartAltRounded'
import SaveRounded from '@mui/icons-material/SaveRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { ADMIN_MENU_GROUPS, normalizeAdminMenu, resetAdminMenu, type AdminMenuItemConfig } from '../adminMenu'
import { adminApi, type SystemSettings } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

export function MenuManagementPage() {
  const [settings, setSettings] = useState<SystemSettings | null>(null)
  const [items, setItems] = useState<AdminMenuItemConfig[]>(() => resetAdminMenu())
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const current = await adminApi.settings()
      setSettings(current)
      setItems(normalizeAdminMenu(current.game?.admin_menu))
      if (notify) showMessage('菜单配置已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取菜单配置失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const grouped = useMemo(() => {
    const map = new Map<string, AdminMenuItemConfig[]>()
    items.slice().sort((a, b) => a.order - b.order).forEach(item => map.set(item.group, [...(map.get(item.group) ?? []), item]))
    return [...map.entries()]
  }, [items])

  const update = (path: string, patch: Partial<AdminMenuItemConfig>) => setItems(current => current.map(item => item.path === path ? { ...item, ...patch } : item))
  const move = (path: string, direction: -1 | 1) => setItems(current => {
    const sorted = current.slice().sort((a, b) => a.order - b.order)
    const index = sorted.findIndex(item => item.path === path)
    const target = index + direction
    if (index < 0 || target < 0 || target >= sorted.length) return current
    const left = sorted[index]
    const right = sorted[target]
    return current.map(item => item.path === left.path ? { ...item, order: right.order } : item.path === right.path ? { ...item, order: left.order } : item)
  })

  const save = async () => {
    if (!settings) return
    setSaving(true)
    setError('')
    try {
      const normalized = normalizeAdminMenu(items)
      const saved = await adminApi.updateSettings({ ...settings, game: { ...settings.game, admin_menu: normalized } })
      setSettings(saved)
      setItems(normalizeAdminMenu(saved.game?.admin_menu))
      window.dispatchEvent(new Event('yaotu-admin-menu-updated'))
      showMessage('菜单名称、分组、顺序和显示状态已保存')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存菜单配置失败')
    } finally {
      setSaving(false)
    }
  }

  return <Box p={{ xs: 2, lg: 2.5 }}>
    <PageHeader
      eyebrow="系统管理 / 导航"
      title="菜单管理"
      description="总管理员可统一调整后台菜单的名称、分组、排序和显示状态，保存后侧栏立即更新。"
      actions={<><Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)} disabled={loading}>刷新</Button><Button variant="contained" startIcon={saving ? <CircularProgress color="inherit" size={16} /> : <SaveRounded />} onClick={() => void save()} disabled={saving || loading}>保存菜单</Button></>}
    />
    {error && <Alert severity="error" sx={{ mt: 2 }} onClose={() => setError('')}>{error}</Alert>}
    <Stack direction={{ xs: 'column', md: 'row' }} gap={1.2} mt={2}>
      <Card sx={{ flex: 1 }}><CardContent><Typography variant="caption" color="text.secondary">后台页面</Typography><Typography fontSize={25} fontWeight={900}>{items.length}</Typography><Typography variant="caption" color="text.secondary">所有可管理的固定功能入口</Typography></CardContent></Card>
      <Card sx={{ flex: 1 }}><CardContent><Typography variant="caption" color="text.secondary">当前显示</Typography><Typography fontSize={25} fontWeight={900} color="success.main">{items.filter(item => item.visible).length}</Typography><Typography variant="caption" color="text.secondary">隐藏只影响侧栏，不删除页面</Typography></CardContent></Card>
      <Card sx={{ flex: 1 }}><CardContent><Typography variant="caption" color="text.secondary">导航分组</Typography><Typography fontSize={25} fontWeight={900}>{grouped.length}</Typography><Typography variant="caption" color="text.secondary">按业务流程整理，避免菜单混在一起</Typography></CardContent></Card>
    </Stack>
    <Stack gap={1.5} mt={1.5}>
      {grouped.map(([group, entries]) => <Paper key={group} variant="outlined" sx={{ overflow: 'hidden' }}>
        <Stack direction="row" alignItems="center" justifyContent="space-between" px={1.6} py={1.15} bgcolor="action.hover">
          <Box><Typography fontWeight={850}>{group}</Typography><Typography variant="caption" color="text.secondary">{entries.length} 个入口 · {entries.filter(item => item.visible).length} 个显示</Typography></Box>
          <Chip size="small" label={group} color="primary" variant="outlined" />
        </Stack>
        <Stack divider={<Box sx={{ borderTop: 1, borderColor: 'divider' }} />}>
          {entries.map((item, index) => <Box key={item.path} sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr auto', md: 'minmax(180px,1.35fr) minmax(160px,1fr) 150px 100px 96px' }, gap: 1.2, alignItems: 'center', px: 1.6, py: 1.1, opacity: item.visible ? 1 : .65 }}>
            <TextField size="small" label="菜单名称" value={item.label} onChange={event => update(item.path, { label: event.target.value })} />
            <TextField size="small" select label="所属分组" value={item.group} onChange={event => update(item.path, { group: event.target.value })} sx={{ gridColumn: { xs: '1 / -1', md: 'auto' } }}>{ADMIN_MENU_GROUPS.map(option => <MenuItem key={option} value={option}>{option}</MenuItem>)}</TextField>
            <Box sx={{ gridColumn: { xs: '1 / -1', md: 'auto' } }}><Typography fontSize={10} color="text.secondary">页面地址</Typography><Typography fontSize={12} fontFamily="ui-monospace,monospace" noWrap>{item.path}</Typography></Box>
            <Stack direction="row" alignItems="center"><Switch size="small" checked={item.visible} disabled={item.path === '/menu-management'} onChange={event => update(item.path, { visible: event.target.checked })} /><Typography fontSize={11}>{item.visible ? '显示' : '隐藏'}</Typography></Stack>
            <Stack direction="row" justifyContent="flex-end">
              <Tooltip title="上移"><span><IconButton size="small" disabled={index === 0 && grouped[0]?.[0] === group} onClick={() => move(item.path, -1)}><ArrowUpwardRounded fontSize="small" /></IconButton></span></Tooltip>
              <Tooltip title="下移"><span><IconButton size="small" disabled={items.slice().sort((a, b) => a.order - b.order).at(-1)?.path === item.path} onClick={() => move(item.path, 1)}><ArrowDownwardRounded fontSize="small" /></IconButton></span></Tooltip>
            </Stack>
          </Box>)}
        </Stack>
      </Paper>)}
    </Stack>
    <Stack direction="row" justifyContent="flex-end" mt={1.5}><Button color="inherit" startIcon={<RestartAltRounded />} onClick={() => setItems(resetAdminMenu())}>恢复默认分组与名称</Button></Stack>
  </Box>
}
