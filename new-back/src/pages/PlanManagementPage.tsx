import { Alert, Avatar, Box, Button, Card, Checkbox, Chip, CircularProgress, Divider, FormControlLabel, MenuItem, Paper, Stack, Switch, TextField, Typography } from '@mui/material'
import FactCheckRounded from '@mui/icons-material/FactCheckRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SaveRounded from '@mui/icons-material/SaveRounded'
import { useEffect, useMemo, useState } from 'react'
import { adminApi, type PlanAutomationConfig, type WorkspaceGame } from '../api'
import { getStoredUser } from '../auth'
import { useFeedback } from '../components/feedback'
import { PlanManagementPanel } from '../components/PlanManagementPanel'
import { PlanAutomationExperts } from '../components/PlanAutomationExperts'
import { PlanVariantSettings } from '../components/PlanVariantSettings'
import { buildPlanAutomationPayload, canManagePlanAutomation, hasPlanAutomationChanges } from '../utils/planAutomation'
import { loadPlanWorkspaces, loadPlanWorkspaceGames, type PlanWorkspaceOption } from '../utils/planWorkspaces'
import { workspaceGameAvailability } from '../utils/workspaceGameAvailability'

const timeText = (value: string | null) => {
  if (!value) return '尚未运行'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '时间不可用' : date.toLocaleString('zh-CN', { hour12: false })
}

export function PlanGenerationPolicy({ config }: { config: PlanAutomationConfig }) {
  return <Paper variant="outlined" sx={{ p: 1.3 }}>
    <Typography fontSize={12} fontWeight={800}>按访问生成 · 无人浏览不推进</Typography>
    <Typography color="text.secondary" fontSize={11} mt={.5}>仅会员计划页可见时每 15 秒请求当前所选计划；多个会员共享同一计划，隐藏或离开即暂停请求。默认冠军计划也不常驻，访问租期 {config.stream_ttl_seconds ?? 60} 秒。</Typography>
    <Typography color="text.secondary" fontSize={11} mt={.5}>会员默认展示最近 {config.history_default_periods ?? 6} 期，接口最多 {config.history_max_periods ?? 10} 期；每组自动计划仅保留最近 {config.history_retention_periods ?? 20} 期，不补造历史推荐。</Typography>
  </Paper>
}

function WorkspacePlanManagement({ workspace }: { workspace: PlanWorkspaceOption }) {
  const [config, setConfig] = useState<PlanAutomationConfig | null>(null)
  const [games, setGames] = useState<WorkspaceGame[]>([])
  const [enabled, setEnabled] = useState(false)
  const [gameIds, setGameIds] = useState<string[]>([])
  const [positions, setPositions] = useState<number[]>([])
  const [planKeys, setPlanKeys] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [loadKey, setLoadKey] = useState(0)
  const [refreshKey, setRefreshKey] = useState(0)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setLoading(true)
      setError('')
      try {
        const [settings, rows] = await Promise.all([adminApi.planAutomation(workspace.id), loadPlanWorkspaceGames(workspace)])
        if (cancelled) return
        setConfig(settings)
        setEnabled(settings.enabled)
        setGameIds(settings.game_ids)
        setPositions(settings.positions)
        setPlanKeys(settings.plan_keys)
        setGames(rows)
      } catch (reason) {
        if (!cancelled) setError(reason instanceof Error ? reason.message : '读取自动推荐配置失败')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [workspace, loadKey])

  const candidates = useMemo(() => games.filter(game => config?.supported_categories?.includes(game.category)), [config, games])
  const missingGameIds = gameIds.filter(id => !candidates.some(game => game.id === id))
  const gameNames = new Map(games.map(game => [game.id, game.name]))
  const racingSelection = { positions, plan_keys: planKeys }
  const dirty = config ? hasPlanAutomationChanges(config, enabled, gameIds, racingSelection) : false
  const busy = saving
  const noSelectedGame = enabled && gameIds.length === 0
  const noSelectedPlan = enabled && gameIds.includes('speed-racing') && (!positions.length || !planKeys.length)

  const save = async () => {
    if (!config || busy) return
    setError('')
    setSaving(true)
    try {
      const result = await adminApi.updatePlanAutomation(buildPlanAutomationPayload(workspace.id, enabled, gameIds, racingSelection))
      setConfig(result)
      setEnabled(result.enabled)
      setGameIds(result.game_ids)
      setPositions(result.positions)
      setPlanKeys(result.plan_keys)
      setRefreshKey(current => current + 1)
      showMessage(result.enabled ? '自动推荐已开启，会员访问计划页时按需生成' : '自动推荐已关闭，已有推荐保留')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存自动推荐配置失败')
    } finally { setSaving(false) }
  }

  const toggleGame = (id: string, checked: boolean) => {
    setGameIds(current => checked ? [...new Set([...current, id])] : current.filter(item => item !== id))
  }

  return <>
    <Card variant="outlined" sx={{ p: { xs: 1.5, md: 2 } }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} gap={1} mb={1.5}>
        <Box flex={1}>
          <Typography fontWeight={900}>自动推荐</Typography>
          <Typography fontSize={12} color="text.secondary">仅总管理员可配置，默认关闭；只对当前房间生效。</Typography>
        </Box>
        <Chip size="small" label={!config ? '未加载配置' : config.enabled ? '已保存：开启' : config.updated_at ? '已保存：关闭' : '默认关闭'} color={config?.enabled ? 'success' : 'default'} />
        <FormControlLabel label="开启自动推荐" control={<Switch checked={enabled} disabled={loading || busy || !config} onChange={event => setEnabled(event.target.checked)} />} />
      </Stack>
      <Alert severity="warning" sx={{ mb: 1.5 }}>
        {config?.notice || '系统自动生成，仅供娱乐参考，不保证命中。'}不展示虚构命中率。
      </Alert>
      {error && <Alert severity="error" sx={{ mb: 1.5 }} action={!config && !loading ? <Button color="inherit" size="small" onClick={() => setLoadKey(current => current + 1)}>重试</Button> : undefined}>{error}</Alert>}
      {loading ? <Box textAlign="center" py={3}><CircularProgress size={24} /><Typography color="text.secondary" mt={1}>正在读取当前房间配置…</Typography></Box> : config && <Stack gap={1.5}>
        <Box>
          <Typography fontWeight={800} fontSize={13} mb={1}>自动生成彩种</Typography>
          <Typography fontSize={12} color="text.secondary" mb={1}>开启前至少选择一个彩种。会员访问计划页时，仅在房间、平台和房间彩种均开放且存在真实开放期时生成，不会自动开房、开游戏或创建期号。</Typography>
          <Box display="grid" gridTemplateColumns={{ xs: '1fr', sm: 'repeat(2,minmax(0,1fr))', lg: 'repeat(3,minmax(0,1fr))' }} gap={1}>
            {candidates.map(game => {
              const availability = workspaceGameAvailability(game)
              return <Paper key={game.id} variant="outlined" sx={{ px: 1, py: .5 }}>
                <FormControlLabel sx={{ m: 0, width: '100%', alignItems: 'center' }} control={<Checkbox size="small" checked={gameIds.includes(game.id)} disabled={busy} onChange={event => toggleGame(game.id, event.target.checked)} />} label={<Box><Typography fontSize={13} fontWeight={750}>{game.name}</Typography><Typography fontSize={11} color={availability.available ? 'success.main' : 'text.secondary'}>{availability.label}</Typography></Box>} />
              </Paper>
            })}
          </Box>
          {!candidates.length && <Alert severity="info">当前房间没有支持自动推荐的彩种，可继续使用下方手工推荐。</Alert>}
          {missingGameIds.length > 0 && <Alert severity="warning" sx={{ mt: 1 }}>
            已保存的以下彩种当前不可选，保存前可移除：
            <Stack direction="row" gap={1} mt={1} flexWrap="wrap">{missingGameIds.map(id => <Chip key={id} size="small" label={gameNames.get(id) || id} onDelete={busy ? undefined : () => toggleGame(id, false)} />)}</Stack>
          </Alert>}
          {noSelectedGame && <Typography color="error" fontSize={12} mt={1}>开启自动推荐前，请至少选择一个彩种。</Typography>}
        </Box>
        {gameIds.includes('speed-racing') && <>
          <PlanVariantSettings positions={positions} planKeys={planKeys} availablePositions={config.available_positions} options={config.options} maxActiveStreams={config.max_active_streams} disabled={busy} onPositionsChange={setPositions} onPlanKeysChange={setPlanKeys} />
          {noSelectedPlan && <Alert severity="error">极速赛车至少选择一个名次和一种计划。</Alert>}
        </>}
        <Divider />
        <PlanAutomationExperts masters={config.masters} />
        <PlanGenerationPolicy config={config} />
        <Paper variant="outlined" sx={{ p: 1.3 }}>
          <Typography fontSize={12}>最近运行：{timeText(config.last_run_at)} · 最近新增 {config.last_created_count} 条</Typography>
          {config.last_error && <Typography color="error" fontSize={12} mt={.5}>{config.last_error}</Typography>}
          <Typography color="text.secondary" fontSize={11} mt={.5}>同一房间、彩种、名次、计划和期号不会重复发布；关闭自动推荐会停止生成，不改写已发布记录。</Typography>
        </Paper>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1} alignItems={{ sm: 'center' }}>
          <Button variant="contained" startIcon={<SaveRounded />} disabled={busy || noSelectedGame || noSelectedPlan} onClick={() => void save()}>{saving ? '保存中…' : '保存自动配置'}</Button>
          <Typography color="text.secondary" fontSize={11}>{dirty ? '配置尚未保存。保存仅更新规则，不会生成推荐。' : '管理页不触发生成。请在会员端打开计划页查看；下方保留手工及其他彩种推荐。'}</Typography>
        </Stack>
      </Stack>}
    </Card>
    <PlanManagementPanel workspaceId={workspace.id} refreshKey={refreshKey} />
  </>
}

function AdminPlanManagementPage() {
  const [workspaces, setWorkspaces] = useState<PlanWorkspaceOption[]>([])
  const [workspaceId, setWorkspaceId] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [loadKey, setLoadKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setLoading(true)
      setError('')
      try {
        const rows = await loadPlanWorkspaces()
        if (cancelled) return
        setWorkspaces(rows)
        setWorkspaceId(current => rows.some(row => row.id === current) ? current : rows[0]?.id || 0)
      } catch (reason) {
        if (!cancelled) setError(reason instanceof Error ? reason.message : '读取房间失败')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [loadKey])

  const workspace = workspaces.find(item => item.id === workspaceId)
  return <Box p={{ xs: 2, lg: 2.5 }}>
    <Stack gap={1.5} maxWidth={1080} mx="auto">
      <Card sx={{ p: { xs: 1.5, md: 2 } }}>
        <Stack direction={{ xs: 'column', md: 'row' }} gap={1.5} alignItems={{ md: 'center' }}>
          <Avatar variant="rounded" sx={{ bgcolor: 'primary.main' }}><FactCheckRounded /></Avatar>
          <Box flex={1}><Typography fontSize={18} fontWeight={900}>计划管理</Typography><Typography fontSize={12} color="text.secondary">按房间管理专家推荐与自动生成配置，供会员端「消息 → 计划群」展示。</Typography></Box>
          <TextField select size="small" label="配置房间" value={workspaceId || ''} disabled={loading || workspaces.length === 0} onChange={event => setWorkspaceId(Number(event.target.value))} sx={{ minWidth: { md: 290 } }}>
            {workspaces.map(item => <MenuItem key={item.id} value={item.id}>{item.label}</MenuItem>)}
          </TextField>
        </Stack>
      </Card>
      {error && <Alert severity="error" action={<Button color="inherit" size="small" startIcon={<RefreshRounded />} onClick={() => setLoadKey(current => current + 1)}>重试</Button>}>{error}</Alert>}
      {loading && <Typography color="text.secondary" textAlign="center" py={4}>正在读取房间…</Typography>}
      {!loading && !error && !workspace && <Alert severity="info">暂无可管理房间，请先创建租户直属房间或代理房间。</Alert>}
      {workspace && <WorkspacePlanManagement key={workspace.id} workspace={workspace} />}
    </Stack>
  </Box>
}

export function PlanManagementPage() {
  if (!canManagePlanAutomation(getStoredUser()?.role)) return <Box p={2}><Alert severity="warning">仅总管理员可访问计划自动化管理。房间手工推荐仍可在公告与活动中维护。</Alert></Box>
  return <AdminPlanManagementPage />
}
