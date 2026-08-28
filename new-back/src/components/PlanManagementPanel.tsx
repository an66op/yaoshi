import { Alert, Box, Button, Card, CardContent, Chip, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, MenuItem, Paper, Stack, Switch, TextField, Typography } from '@mui/material'
import AddRounded from '@mui/icons-material/AddRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import EditRounded from '@mui/icons-material/EditRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, agentApi, tenantApi, type AdminGame, type PlanRecommendation, type PlanRecommendationPayload, type WorkspaceGame } from '../api'
import { getStoredUser } from '../auth'
import { useFeedback } from './feedback'

type Draft = Omit<PlanRecommendationPayload, 'numbers'> & { id?: number; numbersText: string }

const blankDraft = (workspaceId: number): Draft => ({
  workspace_id: workspaceId, game_id: '', issue: '', master_name: '', master_title: '', master_color: '#2aa9b3',
  numbersText: '', size: '', parity: '', result: 'pending', note: '', enabled: true, sort_order: 100,
})

export function PlanManagementPanel() {
  const role = getStoredUser()?.role ?? 'agent'
  const [workspaceId, setWorkspaceId] = useState(0)
  const [workspaces, setWorkspaces] = useState<Array<{ id: number; label: string }>>([])
  const [games, setGames] = useState<Array<AdminGame | WorkspaceGame>>([])
  const [items, setItems] = useState<PlanRecommendation[]>([])
  const [draft, setDraft] = useState<Draft | null>(null)
  const [pendingDelete, setPendingDelete] = useState<PlanRecommendation | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const loadWorkspaces = useCallback(async () => {
    if (role !== 'admin') return
    const [tenants, agents] = await Promise.all([adminApi.tenants({ pageSize: 100 }), adminApi.agents({ pageSize: 100 })])
    const rows = [
      ...(tenants.items ?? []).filter(item => item.workspace_id).map(item => ({ id: item.workspace_id, label: `租户直属 · ${item.room_code || '未分配'} · ${item.room_name || item.nickname || item.username}` })),
      ...(agents.items ?? []).filter(item => item.workspace_id).map(item => ({ id: item.workspace_id, label: `代理房间 · ${item.room_code || '未分配'} · ${item.room_name || item.nickname || item.username}` })),
    ]
    setWorkspaces(rows)
    setWorkspaceId(current => current || rows[0]?.id || 0)
  }, [role])

  const load = useCallback(async () => {
    if (role === 'admin' && !workspaceId) { setItems([]); setLoading(false); return }
    setLoading(true); setError('')
    try {
      const [rows, gameRows] = role === 'admin'
        ? await Promise.all([adminApi.plans(workspaceId), adminApi.games()])
        : role === 'tenant'
          ? await Promise.all([tenantApi.plans(), tenantApi.games()])
          : await Promise.all([agentApi.plans(), agentApi.games()])
      setItems(Array.isArray(rows) ? rows : [])
      setGames(Array.isArray(gameRows) ? gameRows : [])
    } catch (reason) { setError(reason instanceof Error ? reason.message : '读取计划推荐失败') }
    finally { setLoading(false) }
  }, [role, workspaceId])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadWorkspaces().catch(reason => setError(reason instanceof Error ? reason.message : '读取房间失败'))
    }, 0)
    return () => window.clearTimeout(timer)
  }, [loadWorkspaces])
  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const gameNames = useMemo(() => new Map(games.map(game => [game.id, game.name])), [games])
  const openEdit = (item: PlanRecommendation) => setDraft({ ...item, numbersText: item.numbers.join(','), workspace_id: item.workspace_id })
  const save = async () => {
    if (!draft) return
    const numbers = draft.numbersText.split(/[，,\s]+/).filter(Boolean).map(Number).filter(Number.isFinite)
    const payload: PlanRecommendationPayload = {
      workspace_id: role === 'admin' ? workspaceId : draft.workspace_id,
      game_id: draft.game_id, issue: draft.issue, master_name: draft.master_name,
      master_title: draft.master_title, master_color: draft.master_color, numbers,
      size: draft.size, parity: draft.parity, result: draft.result, note: draft.note,
      enabled: draft.enabled, sort_order: draft.sort_order,
    }
    setSaving(true); setError('')
    try {
      if (role === 'admin') {
        if (draft.id) await adminApi.updatePlan(draft.id, payload); else await adminApi.createPlan(payload)
      } else if (role === 'tenant') {
        if (draft.id) await tenantApi.updatePlan(draft.id, payload); else await tenantApi.createPlan(payload)
      } else {
        if (draft.id) await agentApi.updatePlan(draft.id, payload); else await agentApi.createPlan(payload)
      }
      setDraft(null); await load(); showMessage('计划推荐已保存')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '保存计划推荐失败') }
    finally { setSaving(false) }
  }
  const remove = async () => {
    if (!pendingDelete) return
    setSaving(true); setError('')
    try {
      if (role === 'admin') await adminApi.deletePlan(pendingDelete.id, workspaceId)
      else if (role === 'tenant') await tenantApi.deletePlan(pendingDelete.id)
      else await agentApi.deletePlan(pendingDelete.id)
      setPendingDelete(null); await load(); showMessage('计划推荐已删除')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '删除计划推荐失败') }
    finally { setSaving(false) }
  }

  return <Card variant="outlined" sx={{ mt: 2, maxWidth: 1080 }}>
    <CardContent>
      <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} gap={1.2} mb={1.5}>
        <Box flex={1}><Typography fontWeight={900}>计划群推荐</Typography><Typography fontSize={11} color="text.secondary">前端只显示这里已发布的房间数据，不在浏览器生成号码或命中率。</Typography></Box>
        {role === 'admin' && <TextField select size="small" label="配置房间" value={workspaceId} onChange={event => setWorkspaceId(Number(event.target.value))} sx={{ minWidth: 260 }}>{workspaces.map(item => <MenuItem key={item.id} value={item.id}>{item.label}</MenuItem>)}</TextField>}
        <Button variant="contained" startIcon={<AddRounded />} disabled={role === 'admin' && !workspaceId} onClick={() => setDraft(blankDraft(workspaceId))}>新增推荐</Button>
      </Stack>
      {error && <Alert severity="error" sx={{ mb: 1.5 }}>{error}</Alert>}
      <Box display="grid" gridTemplateColumns={{ xs: '1fr', md: 'repeat(2,minmax(0,1fr))' }} gap={1}>
        {items.map(item => <Paper key={item.id} variant="outlined" sx={{ p: 1.3, opacity: item.enabled ? 1 : .6 }}>
          <Stack direction="row" alignItems="center" gap={1}>
            <Box width={32} height={32} borderRadius={1.7} display="grid" sx={{ placeItems: 'center', bgcolor: item.master_color, color: '#fff', fontWeight: 900 }}>{item.master_name.slice(0, 1)}</Box>
            <Box flex={1} minWidth={0}><Stack direction="row" gap={.7} alignItems="center"><Typography fontWeight={850} noWrap>{gameNames.get(item.game_id) || item.game_id}</Typography><Chip size="small" label={item.enabled ? '已发布' : '已停用'} color={item.enabled ? 'success' : 'default'} /></Stack><Typography fontSize={10.5} color="text.secondary" noWrap>第 {item.issue} 期 · {item.master_name} · {item.numbers.join('、')} {item.size} {item.parity}</Typography></Box>
            <IconButton size="small" onClick={() => openEdit(item)}><EditRounded fontSize="small" /></IconButton>
            <IconButton size="small" color="error" onClick={() => setPendingDelete(item)}><DeleteOutlineRounded fontSize="small" /></IconButton>
          </Stack>
        </Paper>)}
      </Box>
      {!loading && items.length === 0 && <Typography color="text.secondary" textAlign="center" py={4}>当前房间暂无计划推荐</Typography>}
      {loading && <Typography color="text.secondary" textAlign="center" py={4}>正在加载…</Typography>}
    </CardContent>

    <Dialog open={Boolean(draft)} onClose={() => !saving && setDraft(null)} fullWidth maxWidth="sm">
      <DialogTitle>{draft?.id ? '编辑计划推荐' : '新增计划推荐'}</DialogTitle>
      <DialogContent><Stack gap={1.3} pt={1}>
        <TextField select label="彩种" value={draft?.game_id ?? ''} onChange={event => { const game = games.find(item => item.id === event.target.value); setDraft(current => current && ({ ...current, game_id: event.target.value, issue: current.issue || game?.current_issue || game?.issue || '' })) }}>{games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}</TextField>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.3}><TextField fullWidth label="期号" value={draft?.issue ?? ''} onChange={event => setDraft(current => current && ({ ...current, issue: event.target.value }))} /><TextField fullWidth label="大师名称" value={draft?.master_name ?? ''} onChange={event => setDraft(current => current && ({ ...current, master_name: event.target.value }))} /></Stack>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.3}><TextField fullWidth label="大师标签" value={draft?.master_title ?? ''} onChange={event => setDraft(current => current && ({ ...current, master_title: event.target.value }))} /><TextField fullWidth type="color" label="标识颜色" value={draft?.master_color ?? '#2aa9b3'} onChange={event => setDraft(current => current && ({ ...current, master_color: event.target.value }))} /></Stack>
        <TextField label="推荐号码" helperText="使用逗号分隔，例如 1,5,9" value={draft?.numbersText ?? ''} onChange={event => setDraft(current => current && ({ ...current, numbersText: event.target.value }))} />
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.3}><TextField select fullWidth label="大小" value={draft?.size ?? ''} onChange={event => setDraft(current => current && ({ ...current, size: event.target.value as Draft['size'] }))}><MenuItem value="">不推荐</MenuItem><MenuItem value="大">大</MenuItem><MenuItem value="小">小</MenuItem></TextField><TextField select fullWidth label="单双" value={draft?.parity ?? ''} onChange={event => setDraft(current => current && ({ ...current, parity: event.target.value as Draft['parity'] }))}><MenuItem value="">不推荐</MenuItem><MenuItem value="单">单</MenuItem><MenuItem value="双">双</MenuItem></TextField><TextField select fullWidth label="结果" value={draft?.result ?? 'pending'} onChange={event => setDraft(current => current && ({ ...current, result: event.target.value as Draft['result'] }))}><MenuItem value="pending">待开奖</MenuItem><MenuItem value="hit">命中</MenuItem><MenuItem value="miss">未命中</MenuItem></TextField></Stack>
        <TextField multiline minRows={2} label="推荐说明" value={draft?.note ?? ''} onChange={event => setDraft(current => current && ({ ...current, note: event.target.value }))} />
        <Stack direction="row" alignItems="center" justifyContent="space-between"><TextField size="small" type="number" label="排序" value={draft?.sort_order ?? 100} onChange={event => setDraft(current => current && ({ ...current, sort_order: Number(event.target.value) }))} /><Stack direction="row" alignItems="center"><Typography fontSize={12}>发布到前端</Typography><Switch checked={draft?.enabled ?? true} onChange={event => setDraft(current => current && ({ ...current, enabled: event.target.checked }))} /></Stack></Stack>
      </Stack></DialogContent>
      <DialogActions><Button onClick={() => setDraft(null)}>取消</Button><Button variant="contained" disabled={saving || !draft?.game_id || !draft.issue.trim() || !draft.master_name.trim() || !draft.numbersText.trim()} onClick={() => void save()}>保存</Button></DialogActions>
    </Dialog>
    <Dialog open={Boolean(pendingDelete)} onClose={() => !saving && setPendingDelete(null)}><DialogTitle>删除计划推荐</DialogTitle><DialogContent><Typography>删除后前端不再显示这条推荐，历史记录保留在数据库软删除中。</Typography></DialogContent><DialogActions><Button onClick={() => setPendingDelete(null)}>取消</Button><Button color="error" variant="contained" disabled={saving} onClick={() => void remove()}>确认删除</Button></DialogActions></Dialog>
  </Card>
}
