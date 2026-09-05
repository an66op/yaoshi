import { Alert, Box, Button, Card, CardContent, Chip, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, MenuItem, Paper, Stack, Switch, TextField, Typography } from '@mui/material'
import AddRounded from '@mui/icons-material/AddRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import EditRounded from '@mui/icons-material/EditRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, agentApi, tenantApi, type AdminGame, type PlanRecommendation, type WorkspaceGame } from '../api'
import { getStoredUser } from '../auth'
import { loadPlanWorkspaces } from '../utils/planWorkspaces'
import { buildPlanRecommendationPayload, isRacingPlanGame, isSupportedManualPlanGame, planRecommendationNumberError, planRecommendationSelection, type PlanRecommendationDraft } from '../utils/planRecommendation'
import { PlanRecommendationNumberFields } from './PlanRecommendationNumberFields'
import { useFeedback } from './feedback'

type Draft = PlanRecommendationDraft

const blankDraft = (workspaceId: number): Draft => ({
  workspace_id: workspaceId, game_id: '', issue: '', master_name: '', master_title: '', master_color: '#2aa9b3',
  numbersText: '', size: '', parity: '', result: 'pending', note: '', enabled: true, sort_order: 100,
})

export function PlanManagementPanel({ workspaceId: controlledWorkspaceId, refreshKey = 0 }: { workspaceId?: number; refreshKey?: number } = {}) {
  const role = getStoredUser()?.role ?? 'agent'
  const [selectedWorkspaceId, setWorkspaceId] = useState(0)
  const workspaceId = controlledWorkspaceId ?? selectedWorkspaceId
  const [workspaces, setWorkspaces] = useState<Array<{ id: number; label: string }>>([])
  const [games, setGames] = useState<Array<AdminGame | WorkspaceGame>>([])
  const [items, setItems] = useState<PlanRecommendation[]>([])
  const [draft, setDraft] = useState<Draft | null>(null)
  const [pendingDelete, setPendingDelete] = useState<PlanRecommendation | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const loadSequence = useRef(0)
  const { showMessage } = useFeedback()

  const loadWorkspaces = useCallback(async () => {
    if (role !== 'admin' || controlledWorkspaceId !== undefined) return
    const rows = await loadPlanWorkspaces()
    setWorkspaces(rows)
    setWorkspaceId(current => current || rows[0]?.id || 0)
  }, [role, controlledWorkspaceId])

  const load = useCallback(async () => {
    const sequence = ++loadSequence.current
    if (role === 'admin' && !workspaceId) { setItems([]); setLoading(false); return }
    setLoading(true); setError('')
    try {
      const [rows, gameRows] = role === 'admin'
        ? await Promise.all([adminApi.plans(workspaceId), adminApi.games()])
        : role === 'tenant'
          ? await Promise.all([tenantApi.plans(), tenantApi.games()])
          : await Promise.all([agentApi.plans(), agentApi.games()])
      if (sequence !== loadSequence.current) return
      setItems(Array.isArray(rows) ? rows : [])
      setGames(Array.isArray(gameRows) ? gameRows : [])
    } catch (reason) { if (sequence === loadSequence.current) setError(reason instanceof Error ? reason.message : '读取计划推荐失败') }
    finally { if (sequence === loadSequence.current) setLoading(false) }
  }, [role, workspaceId])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadWorkspaces().catch(reason => setError(reason instanceof Error ? reason.message : '读取房间失败'))
    }, 0)
    return () => window.clearTimeout(timer)
  }, [loadWorkspaces])
  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => { window.clearTimeout(timer); loadSequence.current += 1 }
  }, [load, refreshKey])

  const gameNames = useMemo(() => new Map(games.map(game => [game.id, game.name])), [games])
  const manualGames = useMemo(() => games.filter(game => isSupportedManualPlanGame(game.id) && !isRacingPlanGame(game.id)), [games])
  const openEdit = (item: PlanRecommendation) => {
    if (item.source === 'demo' || !isSupportedManualPlanGame(item.game_id) || isRacingPlanGame(item.game_id)) return
    setDraft({ ...item, numbersText: item.numbers.join(','), workspace_id: item.workspace_id })
  }
  const save = async () => {
    if (!draft) return
    setSaving(true); setError('')
    try {
      const payload = buildPlanRecommendationPayload(draft, role === 'admin' ? workspaceId : draft.workspace_id)
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
        <Box flex={1}><Typography fontWeight={900}>计划群推荐</Typography><Typography fontSize={11} color="text.secondary">非赛车彩种可手工发布；赛车类统一使用上方名次与方案矩阵。会员页展示最近 6 期真实发布与可信开奖统计。</Typography></Box>
        {role === 'admin' && controlledWorkspaceId === undefined && <TextField select size="small" label="配置房间" value={workspaceId} disabled={saving} onChange={event => { loadSequence.current += 1; setItems([]); setDraft(null); setPendingDelete(null); setWorkspaceId(Number(event.target.value)) }} sx={{ minWidth: 260 }}>{workspaces.map(item => <MenuItem key={item.id} value={item.id}>{item.label}</MenuItem>)}</TextField>}
        <Button variant="outlined" startIcon={<RefreshRounded />} disabled={loading || saving || (role === 'admin' && !workspaceId)} onClick={() => void load()}>刷新推荐</Button>
        <Button variant="contained" startIcon={<AddRounded />} disabled={role === 'admin' && !workspaceId} onClick={() => setDraft(blankDraft(workspaceId))}>新增推荐</Button>
      </Stack>
      {error && <Alert severity="error" sx={{ mb: 1.5 }}>{error}</Alert>}
      <Box display="grid" gridTemplateColumns={{ xs: '1fr', md: 'repeat(2,minmax(0,1fr))' }} gap={1}>
        {items.map(item => <Paper key={item.id} variant="outlined" sx={{ p: 1.3, opacity: item.enabled ? 1 : .6 }}>
          <Stack direction="row" alignItems="center" gap={1}>
            <Box width={32} height={32} borderRadius={1.7} display="grid" sx={{ placeItems: 'center', bgcolor: item.master_color, color: '#fff', fontWeight: 900 }}>{item.master_name.slice(0, 1)}</Box>
            <Box flex={1} minWidth={0}><Stack direction="row" gap={.7} alignItems="center" flexWrap="wrap"><Typography fontWeight={850} noWrap>{gameNames.get(item.game_id) || item.game_id}</Typography><Chip size="small" label={item.enabled ? '已发布' : '已停用'} color={item.enabled ? 'success' : 'default'} />{item.source === 'demo' && <Chip size="small" label="自动生成" color="warning" variant="outlined" />}{isRacingPlanGame(item.game_id) && item.source === 'manual' && <Chip size="small" label="旧手工记录 · 仅后台存档" variant="outlined" />}</Stack><Typography fontSize={10.5} color="text.secondary" noWrap>第 {item.issue} 期 · {item.master_name} · {planRecommendationSelection(item)}</Typography></Box>
            <IconButton size="small" disabled={item.source === 'demo' || !isSupportedManualPlanGame(item.game_id) || isRacingPlanGame(item.game_id)} title={item.source === 'demo' ? '系统自动推荐不能手工修改' : isRacingPlanGame(item.game_id) ? '赛车类请使用自动计划矩阵' : !isSupportedManualPlanGame(item.game_id) ? '该彩种尚未配置可验证的推荐规则' : '编辑推荐'} onClick={() => openEdit(item)}><EditRounded fontSize="small" /></IconButton>
            <IconButton size="small" color="error" disabled={item.source === 'demo'} title={item.source === 'demo' ? '系统自动推荐不能手工删除' : '删除推荐'} onClick={() => item.source !== 'demo' && setPendingDelete(item)}><DeleteOutlineRounded fontSize="small" /></IconButton>
          </Stack>
        </Paper>)}
      </Box>
      {!loading && items.length === 0 && <Typography color="text.secondary" textAlign="center" py={4}>当前房间暂无计划推荐</Typography>}
      {loading && <Typography color="text.secondary" textAlign="center" py={4}>正在加载…</Typography>}
    </CardContent>

    <Dialog open={Boolean(draft)} onClose={() => !saving && setDraft(null)} fullWidth maxWidth="sm">
      <DialogTitle>{draft?.id ? '编辑计划推荐' : '新增计划推荐'}</DialogTitle>
      <DialogContent><Stack gap={1.3} pt={1}>
        <Alert severity="info">推荐发布后，结果只按已验证开奖号码自动结算，后台不能手工标记命中或未命中。赛车类彩种请在上方自动计划中配置。</Alert>
        <TextField select label="彩种" value={draft?.game_id ?? ''} onChange={event => { const game = manualGames.find(item => item.id === event.target.value); setDraft(current => current && ({ ...current, game_id: event.target.value, issue: current.issue || game?.current_issue || game?.issue || '' })) }}>{manualGames.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}</TextField>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.3}><TextField fullWidth label="期号" value={draft?.issue ?? ''} onChange={event => setDraft(current => current && ({ ...current, issue: event.target.value }))} /><TextField fullWidth label="专家名称" value={draft?.master_name ?? ''} onChange={event => setDraft(current => current && ({ ...current, master_name: event.target.value }))} /></Stack>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.3}><TextField fullWidth label="专家标签" value={draft?.master_title ?? ''} onChange={event => setDraft(current => current && ({ ...current, master_title: event.target.value }))} /><TextField fullWidth type="color" label="标识颜色" value={draft?.master_color ?? '#2aa9b3'} onChange={event => setDraft(current => current && ({ ...current, master_color: event.target.value }))} /></Stack>
        <PlanRecommendationNumberFields gameId={draft?.game_id ?? ''} value={{ numbersText: draft?.numbersText ?? '', size: draft?.size ?? '', parity: draft?.parity ?? '' }} onChange={patch => setDraft(current => current && ({ ...current, ...patch }))} />
        <TextField multiline minRows={2} label="推荐说明" value={draft?.note ?? ''} onChange={event => setDraft(current => current && ({ ...current, note: event.target.value }))} />
        <Stack direction="row" alignItems="center" justifyContent="space-between"><TextField size="small" type="number" label="排序" value={draft?.sort_order ?? 100} onChange={event => setDraft(current => current && ({ ...current, sort_order: Number(event.target.value) }))} /><Stack direction="row" alignItems="center"><Typography fontSize={12}>发布到前端</Typography><Switch checked={draft?.enabled ?? true} onChange={event => setDraft(current => current && ({ ...current, enabled: event.target.checked }))} /></Stack></Stack>
      </Stack></DialogContent>
      <DialogActions><Button onClick={() => setDraft(null)}>取消</Button><Button variant="contained" disabled={saving || !draft?.game_id || !draft.issue.trim() || !draft.master_name.trim() || !draft.numbersText.trim() || Boolean(planRecommendationNumberError(draft.game_id, draft.numbersText))} onClick={() => void save()}>保存</Button></DialogActions>
    </Dialog>
    <Dialog open={Boolean(pendingDelete)} onClose={() => !saving && setPendingDelete(null)}><DialogTitle>删除计划推荐</DialogTitle><DialogContent><Typography>删除后前端不再显示这条推荐，历史记录保留在数据库软删除中。</Typography></DialogContent><DialogActions><Button onClick={() => setPendingDelete(null)}>取消</Button><Button color="error" variant="contained" disabled={saving} onClick={() => void remove()}>确认删除</Button></DialogActions></Dialog>
  </Card>
}
