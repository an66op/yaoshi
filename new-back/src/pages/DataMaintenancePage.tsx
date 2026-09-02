import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  MenuItem,
  Paper,
  Stack,
  Switch,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tabs,
  TextField,
  Typography,
} from '@mui/material'
import ArchiveRounded from '@mui/icons-material/ArchiveRounded'
import DeleteForeverRounded from '@mui/icons-material/DeleteForeverRounded'
import DeleteSweepRounded from '@mui/icons-material/DeleteSweepRounded'
import HistoryRounded from '@mui/icons-material/HistoryRounded'
import PreviewRounded from '@mui/icons-material/PreviewRounded'
import RestoreRounded from '@mui/icons-material/RestoreRounded'
import SaveRounded from '@mui/icons-material/SaveRounded'
import ShieldRounded from '@mui/icons-material/ShieldRounded'
import WarningAmberRounded from '@mui/icons-material/WarningAmberRounded'
import { useCallback, useEffect, useState } from 'react'
import {
  adminApi,
  type CleanupExecution,
  type CleanupPreview,
  type CleanupPreviewItem,
  type CleanupResultItem,
  type CleanupRunView,
  type DataMaintenanceSummary,
  type LifecycleArchiveKind,
  type LifecycleArchiveRecord,
  type LifecycleDataClass,
  type LifecycleDeleteMode,
  type RetentionPolicyView,
  type UpdateRetentionPolicyInput,
} from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'
import { createRequestId } from '../utils/requestId'

type WorkspaceOption = { id: number; label: string; kind: '租户' | '代理' }
type ScopeMode = '' | 'workspace' | 'all'
type RestoreKind = 'soft' | 'robot'
type PolicySave = { workspaceID: number; updates: Array<{ dataClass: LifecycleDataClass; input: UpdateRetentionPolicyInput }> }

const DATA_CLASSES: ReadonlyArray<{ value: LifecycleDataClass; label: string }> = [
  { value: 'chat_messages', label: '普通聊天（非游戏房）' },
  { value: 'robot_chat_messages', label: '机器人普通聊天（非游戏房）' },
  { value: 'game_chat_messages', label: '游戏房聊天' },
  { value: 'notifications', label: '通知消息' },
  { value: 'audit_logs', label: '审计日志' },
  { value: 'robot_test_data', label: '机器人测试数据' },
]

const dataClassLabels: Record<string, string> = Object.fromEntries(DATA_CLASSES.map(item => [item.value, item.label]))
const actionLabels: Record<string, string> = {
  soft_delete: '软删除',
  hard_delete: '永久删除',
  archive_then_purge_hot: '归档后移出热表',
  cold_archive: '冷归档',
}
const HARD_DELETE_CLASSES: LifecycleDataClass[] = ['chat_messages', 'robot_chat_messages', 'game_chat_messages', 'notifications']
const statusLabels: Record<string, string> = {
  previewed: '待确认',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
}

const safeArray = <T,>(value: T[] | null | undefined): T[] => Array.isArray(value) ? value : []
const asCount = (value: number | null | undefined) => Number.isFinite(Number(value)) ? Number(value) : 0
const dateTime = (value?: string | null) => {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(date)
}
const moneyFromCents = (value?: number | null) => `¥ ${(asCount(value) / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
const newRequestID = () => {
  const random = createRequestId().replaceAll('-', '').slice(0, 16)
  return `cleanup-${Date.now()}-${random}`
}

function ItemTable({ items, result = false }: { items: CleanupPreviewItem[] | CleanupResultItem[] | null | undefined; result?: boolean }) {
  const rows = (Array.isArray(items) ? items : []) as Array<CleanupPreviewItem | CleanupResultItem>
  if (!rows.length) return <Typography color="text.secondary" fontSize={13} py={2} textAlign="center">暂无明细</Typography>
  return <TableContainer><Table size="small" sx={{ minWidth: result ? 560 : 880 }}>
    <TableHead><TableRow>
      <TableCell>数据类</TableCell><TableCell>动作</TableCell>
      {result ? <TableCell align="right">处理数量</TableCell> : <><TableCell align="right">候选</TableCell><TableCell align="right">本批计划</TableCell><TableCell align="right">受保护</TableCell><TableCell>保留/截止</TableCell></>}
      <TableCell>说明</TableCell>
    </TableRow></TableHead>
    <TableBody>{rows.map((raw, index) => {
      const item = raw as CleanupPreviewItem & CleanupResultItem
      return <TableRow key={`${item.data_class}-${index}`}>
        <TableCell><Typography fontWeight={800} fontSize={13}>{dataClassLabels[item.data_class] ?? item.data_class}</Typography></TableCell>
        <TableCell><Chip size="small" variant="outlined" label={actionLabels[item.action] ?? item.action ?? '—'} /></TableCell>
        {result
          ? <TableCell align="right"><Typography fontWeight={900}>{asCount(item.affected_count).toLocaleString()}</Typography></TableCell>
          : <>
            <TableCell align="right">{asCount(item.eligible_count).toLocaleString()}</TableCell>
            <TableCell align="right"><Typography fontWeight={900} color="primary.main">{asCount(item.planned_count).toLocaleString()}</Typography></TableCell>
            <TableCell align="right"><Typography fontWeight={800} color={asCount(item.protected_from_deletion) ? 'warning.main' : 'text.secondary'}>{asCount(item.protected_from_deletion).toLocaleString()}</Typography></TableCell>
            <TableCell><Typography fontSize={12}>{asCount(item.retention_days)} 天</Typography><Typography variant="caption" color="text.secondary">{dateTime(item.cutoff_at)}</Typography></TableCell>
          </>}
        <TableCell><Typography fontSize={12} color="text.secondary">{result ? item.note || '处理完成' : item.enabled === false ? '策略未启用，本次不会处理' : item.description || '—'}</Typography></TableCell>
      </TableRow>
    })}</TableBody>
  </Table></TableContainer>
}

export function DataMaintenancePage() {
  const [tab, setTab] = useState<'policies' | 'cleanup' | 'runs'>('policies')
  const [workspaces, setWorkspaces] = useState<WorkspaceOption[]>([])
  const [workspaceError, setWorkspaceError] = useState('')
  const [policyWorkspace, setPolicyWorkspace] = useState(0)
  const [policies, setPolicies] = useState<RetentionPolicyView[]>([])
  const [policyLoading, setPolicyLoading] = useState(true)
  const [policySaving, setPolicySaving] = useState(false)
  const [policyDirty, setPolicyDirty] = useState<Set<string>>(new Set())
  const [pendingPolicySave, setPendingPolicySave] = useState<PolicySave | null>(null)
  const [policyConfirmWord, setPolicyConfirmWord] = useState('')
  const [scopeMode, setScopeMode] = useState<ScopeMode>('')
  const [cleanupWorkspace, setCleanupWorkspace] = useState(0)
  const [selectedClasses, setSelectedClasses] = useState<LifecycleDataClass[]>(DATA_CLASSES.map(item => item.value))
  const [deleteMode, setDeleteMode] = useState<LifecycleDeleteMode>('soft')
  const [batchLimit, setBatchLimit] = useState(5000)
  const [preview, setPreview] = useState<CleanupPreview | null>(null)
  const [execution, setExecution] = useState<CleanupExecution | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [executing, setExecuting] = useState(false)
  const [executeOpen, setExecuteOpen] = useState(false)
  const [executeWord, setExecuteWord] = useState('')
  const [runs, setRuns] = useState<CleanupRunView[]>([])
  const [runsLoading, setRunsLoading] = useState(true)
  const [runsHasMore, setRunsHasMore] = useState(false)
  const [runsBeforeID, setRunsBeforeID] = useState<number | undefined>()
  const [runWorkspace, setRunWorkspace] = useState(0)
  const [selectedRun, setSelectedRun] = useState<CleanupRunView | null>(null)
  const [runDetailLoading, setRunDetailLoading] = useState(false)
  const [restoreKind, setRestoreKind] = useState<RestoreKind | null>(null)
  const [restoring, setRestoring] = useState(false)
  const [archiveRun, setArchiveRun] = useState<CleanupRunView | null>(null)
  const [archiveKind, setArchiveKind] = useState<LifecycleArchiveKind>('bets')
  const [archives, setArchives] = useState<LifecycleArchiveRecord[]>([])
  const [archiveBeforeID, setArchiveBeforeID] = useState<number | undefined>()
  const [archiveHasMore, setArchiveHasMore] = useState(false)
  const [archivesLoading, setArchivesLoading] = useState(false)
  const [error, setError] = useState('')
  const [summary, setSummary] = useState<DataMaintenanceSummary | null>(null)
  const [summaryLoading, setSummaryLoading] = useState(true)
  const { showMessage } = useFeedback()

  const workspaceLabel = useCallback((id: number) => workspaces.find(item => item.id === id)?.label ?? `工作区 ${id}`, [workspaces])

  const loadSummary = useCallback(async () => {
    setSummaryLoading(true)
    try {
      setSummary(await adminApi.dataMaintenanceSummary())
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取维护概况失败')
    } finally {
      setSummaryLoading(false)
    }
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => void loadSummary(), 0)
    return () => window.clearTimeout(timer)
  }, [loadSummary])

  useEffect(() => {
    let cancelled = false
    const loadTenantPages = async () => {
      const first = await adminApi.tenants({ page: 1, pageSize: 100 })
      const pageSize = Math.max(1, asCount(first?.page_size) || safeArray(first?.items).length || 100)
      const pages = Math.ceil(asCount(first?.total) / pageSize)
      if (pages <= 1) return safeArray(first?.items)
      const rest = await Promise.all(Array.from({ length: pages - 1 }, (_, index) => adminApi.tenants({ page: index + 2, pageSize })))
      return [...safeArray(first?.items), ...rest.flatMap(page => safeArray(page?.items))]
    }
    const loadAgentPages = async () => {
      const first = await adminApi.agents({ page: 1, pageSize: 100 })
      const pageSize = Math.max(1, asCount(first?.page_size) || safeArray(first?.items).length || 100)
      const pages = Math.ceil(asCount(first?.total) / pageSize)
      if (pages <= 1) return safeArray(first?.items)
      const rest = await Promise.all(Array.from({ length: pages - 1 }, (_, index) => adminApi.agents({ page: index + 2, pageSize })))
      return [...safeArray(first?.items), ...rest.flatMap(page => safeArray(page?.items))]
    }
    void Promise.all([loadTenantPages(), loadAgentPages()])
      .then(([tenantItems, agentItems]) => {
        if (cancelled) return
        const options: WorkspaceOption[] = [
          ...tenantItems.filter(item => asCount(item.workspace_id) > 0).map(item => ({ id: item.workspace_id, label: `${item.room_code || item.workspace_id} · ${item.room_name || item.nickname || item.username}`, kind: '租户' as const })),
          ...agentItems.filter(item => asCount(item.workspace_id) > 0).map(item => ({ id: item.workspace_id, label: `${item.room_code || item.workspace_id} · ${item.room_name || item.nickname || item.username}`, kind: '代理' as const })),
        ]
        const unique = [...new Map(options.map(item => [item.id, item])).values()].sort((a, b) => a.label.localeCompare(b.label, 'zh-CN'))
        setWorkspaces(unique)
        setWorkspaceError('')
      })
      .catch(reason => {
        if (!cancelled) setWorkspaceError(reason instanceof Error ? reason.message : '读取工作区失败')
      })
    return () => { cancelled = true }
  }, [])

  const loadPolicies = useCallback(async (workspaceID: number) => {
    setPolicyLoading(true)
    setError('')
    try {
      const value = await adminApi.retentionPolicies(workspaceID)
      setPolicies(safeArray(value))
      setPolicyDirty(new Set())
    } catch (reason) {
      setPolicies([])
      setError(reason instanceof Error ? reason.message : '读取保留策略失败')
    } finally {
      setPolicyLoading(false)
    }
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => void loadPolicies(policyWorkspace), 0)
    return () => window.clearTimeout(timer)
  }, [loadPolicies, policyWorkspace])

  const loadRuns = useCallback(async (append = false, beforeID?: number) => {
    setRunsLoading(true)
    setError('')
    try {
      const page = await adminApi.dataCleanupRuns({ beforeId: append ? beforeID : undefined, limit: 30, workspaceId: runWorkspace || undefined })
      const next = safeArray(page?.items)
      setRuns(current => append ? [...current, ...next.filter(item => !current.some(row => row.id === item.id))] : next)
      setRunsHasMore(Boolean(page?.has_more))
      setRunsBeforeID(page?.next_before_id || undefined)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取运行记录失败')
      if (!append) setRuns([])
    } finally {
      setRunsLoading(false)
    }
  }, [runWorkspace])

  useEffect(() => {
    const timer = window.setTimeout(() => void loadRuns(false), 0)
    return () => window.clearTimeout(timer)
  }, [loadRuns])

  const patchPolicy = (dataClass: string, patch: Partial<RetentionPolicyView>) => {
    setPolicies(current => current.map(item => item.data_class === dataClass ? { ...item, ...patch } : item))
    setPolicyDirty(current => new Set(current).add(dataClass))
  }

  const persistPolicies = async (save: PolicySave) => {
    if (policySaving) return
    setPolicySaving(true)
    setError('')
    try {
      await Promise.all(save.updates.map(item => adminApi.updateRetentionPolicy(item.dataClass, item.input)))
      setPendingPolicySave(null)
      setPolicyConfirmWord('')
      await loadPolicies(save.workspaceID)
      showMessage('数据保留策略已保存')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存保留策略失败')
    } finally {
      setPolicySaving(false)
    }
  }

  const savePolicies = () => {
    const dirty = policies.filter(item => policyDirty.has(item.data_class))
    if (!dirty.length || policySaving) return
    const save: PolicySave = { workspaceID: policyWorkspace, updates: dirty.map(item => ({
      dataClass: item.data_class,
      input: {
        workspace_id: policyWorkspace,
        enabled: Boolean(item.enabled),
        retention_days: Math.min(3650, Math.max(item.data_class === 'audit_logs' ? 365 : 1, Math.round(asCount(item.retention_days)))),
        ...(item.data_class === 'game_chat_messages' ? { purge_after_days: Math.min(3650, Math.max(0, Math.round(asCount(item.purge_after_days)))) } : {}),
      },
    })) }
    if (save.updates.some(item => item.dataClass === 'game_chat_messages' && item.input.enabled && asCount(item.input.purge_after_days) > 0)) {
      setPendingPolicySave(save)
      setPolicyConfirmWord('')
      return
    }
    void persistPolicies(save)
  }

  const closePolicyConfirmation = () => {
    if (policySaving) return
    setPendingPolicySave(null)
    setPolicyConfirmWord('')
  }

  const invalidatePreview = () => {
    setPreview(null)
    setExecution(null)
    setExecuteOpen(false)
    setExecuteWord('')
  }

  const applyPreset = (mode: LifecycleDeleteMode, classes: LifecycleDataClass[]) => {
    setDeleteMode(mode)
    setSelectedClasses(classes)
    invalidatePreview()
  }

  const changeDeleteMode = (mode: LifecycleDeleteMode) => {
    setDeleteMode(mode)
    if (mode === 'hard') {
      setSelectedClasses(current => {
        const allowed = current.filter(item => HARD_DELETE_CLASSES.includes(item))
        return allowed
      })
    }
    invalidatePreview()
  }

  const runPreview = async () => {
    if (!scopeMode) { setError('请先明确选择一个工作区，或选择全部工作区'); return }
    if (scopeMode === 'workspace' && !cleanupWorkspace) { setError('请选择需要维护的工作区'); return }
    if (!selectedClasses.length) { setError('请至少选择一个数据类'); return }
    setPreviewing(true)
    setError('')
    try {
      const result = await adminApi.previewDataCleanup({
        request_id: newRequestID(),
        workspace_id: scopeMode === 'workspace' ? cleanupWorkspace : undefined,
        all_workspaces: scopeMode === 'all',
        data_classes: selectedClasses,
        batch_limit: Math.min(20000, Math.max(1, Math.round(batchLimit || 1))),
        delete_mode: deleteMode,
      })
      setPreview({ ...result, items: safeArray(result?.items) })
      setExecution(null)
      showMessage('预览已冻结，请核对后再执行', 'info')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '生成清理预览失败')
    } finally {
      setPreviewing(false)
    }
  }

  const executeCleanup = async () => {
    const requiredWord = preview?.delete_mode === 'hard' ? '永久删除' : '执行'
    if (!preview || executeWord !== requiredWord) return
    setExecuting(true)
    setError('')
    try {
      const result = await adminApi.executeDataCleanup(preview.request_id)
      setExecution({ ...result, items: safeArray(result?.items) })
      setExecuteOpen(false)
      setExecuteWord('')
      await Promise.all([loadRuns(false), loadSummary()])
      showMessage('数据维护任务已完成')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '执行数据维护失败')
    } finally {
      setExecuting(false)
    }
  }

  const openRun = async (run: CleanupRunView) => {
    setSelectedRun({ ...run, preview: safeArray(run.preview), result: safeArray(run.result), soft_restore_result: safeArray(run.soft_restore_result), financial_restore_result: safeArray(run.financial_restore_result) })
    setRunDetailLoading(true)
    setError('')
    try {
      const detail = await adminApi.dataCleanupRun(run.request_id)
      setSelectedRun({ ...detail, preview: safeArray(detail?.preview), result: safeArray(detail?.result), soft_restore_result: safeArray(detail?.soft_restore_result), financial_restore_result: safeArray(detail?.financial_restore_result) })
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取任务详情失败')
    } finally {
      setRunDetailLoading(false)
    }
  }

  const restore = async () => {
    if (!selectedRun || !restoreKind) return
    setRestoring(true)
    setError('')
    try {
      if (restoreKind === 'soft') await adminApi.restoreSoftDeleted(selectedRun.request_id)
      else await adminApi.restoreRobotArchive(selectedRun.request_id)
      const detail = await adminApi.dataCleanupRun(selectedRun.request_id)
      const normalized = { ...detail, preview: safeArray(detail?.preview), result: safeArray(detail?.result), soft_restore_result: safeArray(detail?.soft_restore_result), financial_restore_result: safeArray(detail?.financial_restore_result) }
      setSelectedRun(normalized)
      setRuns(current => current.map(item => item.request_id === normalized.request_id ? normalized : item))
      setRestoreKind(null)
      showMessage(restoreKind === 'soft' ? '软删除数据已恢复' : '机器人冷归档已恢复')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '恢复数据失败')
    } finally {
      setRestoring(false)
    }
  }

  const loadArchives = useCallback(async (run: CleanupRunView, kind: LifecycleArchiveKind, append = false) => {
    setArchivesLoading(true)
    setError('')
    try {
      const page = await adminApi.dataCleanupArchives(run.request_id, kind, append ? archiveBeforeID : undefined, 50)
      const next = safeArray(page?.items)
      setArchives(current => append ? [...current, ...next.filter(item => !current.some(row => row.id === item.id))] : next)
      setArchiveHasMore(Boolean(page?.has_more))
      setArchiveBeforeID(page?.next_before_id || undefined)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取冷归档失败')
      if (!append) setArchives([])
    } finally {
      setArchivesLoading(false)
    }
  }, [archiveBeforeID])

  const openArchives = (run: CleanupRunView) => {
    const initialKind: LifecycleArchiveKind = safeArray(run.preview).some(item => item.data_class === 'robot_test_data') ? 'bets' : 'audit'
    setArchiveRun(run)
    setArchiveKind(initialKind)
    setArchives([])
    setArchiveBeforeID(undefined)
    void loadArchives(run, initialKind, false)
  }

  const previewItems = safeArray(preview?.items)
  const plannedTotal = previewItems.reduce((sum, item) => sum + asCount(item.planned_count), 0)
  const protectedTotal = previewItems.reduce((sum, item) => sum + asCount(item.protected_from_deletion), 0)
  const selectedRunClasses = safeArray(selectedRun?.preview).map(item => item.data_class)
  const canRestoreSoft = selectedRun?.status === 'completed' && selectedRun.delete_mode !== 'hard' && !selectedRun.soft_restored_at && !selectedRun.content_purged_at && asCount(selectedRun.content_purge_count) === 0 && selectedRunClasses.some(item => HARD_DELETE_CLASSES.includes(item))
  const canRestoreRobot = selectedRun?.status === 'completed' && !selectedRun.financial_restored_at && selectedRunClasses.includes('robot_test_data')
  const runHasArchive = (run: CleanupRunView) => safeArray(run.preview).some(item => item.data_class === 'robot_test_data' || item.data_class === 'audit_logs')
  const selectedScopeText = preview?.all_workspaces ? '全部工作区' : preview ? workspaceLabel(preview.workspace_id) : '—'
  const confirmWord = preview?.delete_mode === 'hard' ? '永久删除' : '执行'
  const recycleBinTotal = asCount(summary?.soft_deleted_chat_count) + asCount(summary?.soft_deleted_robot_chat_count) + asCount(summary?.soft_deleted_game_chat_count) + asCount(summary?.soft_deleted_notification_count)
  const protectedEvidenceTotal = asCount(summary?.protected_bet_count) + asCount(summary?.protected_ledger_count) + asCount(summary?.protected_audit_count)

  return <Box p={{ xs: 1.5, md: 2, xl: 2.5 }}>
    <PageHeader eyebrow="系统管理" title="数据维护" description="" />
    {(error || workspaceError) && <Alert severity="error" sx={{ mt: 1.5 }} onClose={() => { setError(''); setWorkspaceError('') }}>{error || workspaceError}</Alert>}

    <Box mt={1.5} display="grid" gridTemplateColumns={{ xs: '1fr', sm: 'repeat(2, minmax(0, 1fr))', xl: 'repeat(4, minmax(0, 1fr))' }} gap={1}>
      {[
        { title: '回收站内容', value: recycleBinTotal, note: `普通聊天 ${asCount(summary?.soft_deleted_chat_count)} · 机器人普通聊天 ${asCount(summary?.soft_deleted_robot_chat_count)} · 游戏房 ${asCount(summary?.soft_deleted_game_chat_count)} · 通知 ${asCount(summary?.soft_deleted_notification_count)}`, icon: <DeleteSweepRounded color="warning" /> },
        { title: '自动恢复中的请求', value: asCount(summary?.stale_idempotency_count), note: '超时的下注幂等请求由后台自动恢复', icon: <HistoryRounded color="info" /> },
        { title: '待整理临时记录', value: asCount(summary?.delivered_session_receipt_count) + asCount(summary?.orphan_chat_cursor_count), note: `已投递会话票据 ${asCount(summary?.delivered_session_receipt_count)} · 孤立游标 ${asCount(summary?.orphan_chat_cursor_count)}`, icon: <ArchiveRounded color="primary" /> },
        { title: '永久保护凭证', value: protectedEvidenceTotal, note: '注单、余额流水和审计不可直接硬删', icon: <ShieldRounded color="success" /> },
      ].map(card => <Paper key={card.title} variant="outlined" sx={{ p: 1.35, borderRadius: 2, minWidth: 0 }}>
        <Stack direction="row" justifyContent="space-between" alignItems="flex-start" gap={1}>
          <Box minWidth={0}><Typography variant="caption" color="text.secondary" fontWeight={750}>{card.title}</Typography><Typography fontSize={24} lineHeight={1.25} fontWeight={900}>{summaryLoading ? '—' : card.value.toLocaleString()}</Typography></Box>
          {card.icon}
        </Stack>
        <Typography variant="caption" color="text.secondary" display="block" mt={.45} noWrap title={card.note}>{card.note}</Typography>
      </Paper>)}
    </Box>

    <Alert severity="info" sx={{ mt: 1.5 }}>
      游戏房聊天默认保留 7 天、策略默认停用。启用后每小时第 10 分钟小批处理，每批 1,000 条，每房间每种模式最多 5 批，整轮最多运行 2 分钟。
      正式注单、余额流水、下注命令与回执、红包及申请记录受保护，不是清空整张聊天表。
    </Alert>

    <Paper variant="outlined" sx={{ mt: 1.5, overflow: 'hidden', borderRadius: 2 }}>
      <Tabs value={tab} onChange={(_, value: typeof tab) => setTab(value)} variant="fullWidth">
        <Tab value="policies" label="保留策略" />
        <Tab value="cleanup" label="清理任务" />
        <Tab value="runs" label="运行历史" />
      </Tabs>
    </Paper>

    {tab === 'policies' && <Stack gap={1.25} mt={1.5}>
      <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2 }}>
        <TextField select size="small" label="策略范围" value={policyWorkspace} disabled={policySaving || Boolean(pendingPolicySave)} onChange={event => setPolicyWorkspace(Number(event.target.value))} sx={{ minWidth: { xs: '100%', sm: 340 } }}>
          <MenuItem value={0}>平台默认策略</MenuItem>
          {workspaces.map(item => <MenuItem key={item.id} value={item.id}>{item.label} · {item.kind}</MenuItem>)}
        </TextField>
      </Paper>
      <Paper variant="outlined" sx={{ overflow: 'hidden', borderRadius: 2 }}>
        {policyLoading ? <Box minHeight={180} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={26} /></Box> : <TableContainer><Table size="small" sx={{ minWidth: 1040 }}>
          <TableHead><TableRow><TableCell>数据类</TableCell><TableCell>状态</TableCell><TableCell>保留天数</TableCell><TableCell>回收站保留天数</TableCell><TableCell>处理动作</TableCell><TableCell>规则</TableCell></TableRow></TableHead>
          <TableBody>{safeArray(policies).map(item => <TableRow key={item.data_class}>
            <TableCell><Typography fontSize={13} fontWeight={850}>{dataClassLabels[item.data_class] ?? item.data_class}</Typography>{item.inherited && <Chip size="small" variant="outlined" label="继承平台" sx={{ mt: .5 }} />}</TableCell>
            <TableCell><Stack direction="row" alignItems="center"><Switch size="small" checked={Boolean(item.enabled)} disabled={policySaving || Boolean(pendingPolicySave)} onChange={event => patchPolicy(item.data_class, { enabled: event.target.checked, inherited: false })} /><Typography fontSize={12}>{item.enabled ? '已启用' : '已停用'}</Typography></Stack></TableCell>
            <TableCell><TextField size="small" type="number" value={asCount(item.retention_days)} disabled={policySaving || Boolean(pendingPolicySave)} onChange={event => patchPolicy(item.data_class, { retention_days: Number(event.target.value), inherited: false })} slotProps={{ htmlInput: { 'aria-label': `${dataClassLabels[item.data_class]}保留天数`, min: item.data_class === 'audit_logs' ? 365 : 1, max: 3650, step: 1 } }} sx={{ width: 120 }} /></TableCell>
            <TableCell>{item.data_class === 'game_chat_messages' ? <TextField size="small" type="number" label="回收站保留天数" value={asCount(item.purge_after_days)} disabled={policySaving || Boolean(pendingPolicySave)} onChange={event => patchPolicy(item.data_class, { purge_after_days: Number(event.target.value), inherited: false })} slotProps={{ htmlInput: { min: 0, max: 3650, step: 1 } }} helperText="0 = 不自动永久清理；正数 = 软删除后等待该天数再永久清除，不可恢复。" sx={{ width: 220, my: 1 }} /> : '—'}</TableCell>
            <TableCell><Chip size="small" color={item.enabled ? 'primary' : 'default'} variant="outlined" label={actionLabels[item.action] ?? item.action} /></TableCell>
            <TableCell><Typography fontSize={12} color="text.secondary">{item.description || '—'}</Typography></TableCell>
          </TableRow>)}</TableBody>
        </Table></TableContainer>}
        <Stack direction="row" justifyContent="flex-end" p={1.25} borderTop={1} borderColor="divider">
          <Button size="small" variant="contained" startIcon={policySaving ? <CircularProgress color="inherit" size={15} /> : <SaveRounded />} disabled={policySaving || policyLoading || !policyDirty.size} onClick={() => void savePolicies()}>保存策略</Button>
        </Stack>
      </Paper>
    </Stack>}

    {tab === 'cleanup' && <Stack gap={1.25} mt={1.5}>
      <Paper variant="outlined" sx={{ p: 1.4, borderRadius: 2 }}>
        <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" gap={1} mb={1.2}>
          <Stack direction="row" flexWrap="wrap" gap={.7}>
            <Button size="small" variant="outlined" onClick={() => applyPreset('soft', [...HARD_DELETE_CLASSES])}>日常内容清理</Button>
            <Button size="small" variant="outlined" onClick={() => applyPreset('soft', ['audit_logs', 'robot_test_data'])}>安全归档</Button>
            <Button size="small" color="error" variant="outlined" startIcon={<DeleteForeverRounded />} onClick={() => applyPreset('hard', [...HARD_DELETE_CLASSES])}>永久清理回收站</Button>
          </Stack>
          <TextField select size="small" label="删除方式" value={deleteMode} onChange={event => changeDeleteMode(event.target.value as LifecycleDeleteMode)} sx={{ minWidth: { xs: '100%', md: 220 } }}>
            <MenuItem value="soft">软删除 · 可恢复</MenuItem>
            <MenuItem value="hard">硬删除 · 不可恢复</MenuItem>
          </TextField>
        </Stack>
        <Alert severity={deleteMode === 'hard' ? 'error' : 'info'} sx={{ mb: 1.2 }}>
          {deleteMode === 'hard'
            ? '仅永久清除已经软删除、超过保留期的普通聊天、游戏房聊天和非账务通知；下注命令与回执、红包、申请、注单、余额流水及审计均受保护。'
            : '聊天与通知进入回收站后可按任务恢复；审计和机器人财务数据沿用经过校验的只读归档流程。'}
        </Alert>
        <Stack direction={{ xs: 'column', lg: 'row' }} gap={1.2} alignItems={{ lg: 'center' }}>
          <TextField select size="small" label="维护范围" value={scopeMode} onChange={event => { setScopeMode(event.target.value as ScopeMode); invalidatePreview() }} sx={{ minWidth: 200 }}>
            <MenuItem value="" disabled>请选择范围</MenuItem>
            <MenuItem value="workspace">指定工作区</MenuItem>
            <MenuItem value="all">全部工作区</MenuItem>
          </TextField>
          {scopeMode === 'workspace' && <TextField select size="small" label="工作区" value={cleanupWorkspace || ''} onChange={event => { setCleanupWorkspace(Number(event.target.value)); invalidatePreview() }} sx={{ minWidth: { xs: '100%', lg: 330 } }}>
            <MenuItem value="" disabled>请选择工作区</MenuItem>
            {workspaces.map(item => <MenuItem key={item.id} value={item.id}>{item.label} · {item.kind}</MenuItem>)}
          </TextField>}
          {scopeMode === 'all' && <Alert severity="warning" icon={<WarningAmberRounded />} sx={{ py: 0, flex: 1 }}>全部工作区按平台默认策略计算，归属不明的历史数据不会处理。</Alert>}
          <TextField size="small" type="number" label="单类批次上限" value={batchLimit} onChange={event => { setBatchLimit(Number(event.target.value)); invalidatePreview() }} slotProps={{ htmlInput: { min: 1, max: 20000, step: 100 } }} sx={{ width: { xs: '100%', lg: 170 } }} />
        </Stack>
        <Stack direction="row" flexWrap="wrap" gap={.6} mt={1.2}>
          {DATA_CLASSES.filter(item => deleteMode === 'soft' || HARD_DELETE_CLASSES.includes(item.value)).map(item => <FormControlLabel key={item.value} sx={{ m: 0, pr: 1, border: 1, borderColor: selectedClasses.includes(item.value) ? deleteMode === 'hard' ? 'error.main' : 'primary.main' : 'divider', borderRadius: 1.5 }} control={<Checkbox size="small" color={deleteMode === 'hard' ? 'error' : 'primary'} checked={selectedClasses.includes(item.value)} onChange={event => { setSelectedClasses(current => event.target.checked ? [...current, item.value] : current.filter(value => value !== item.value)); invalidatePreview() }} />} label={<Typography fontSize={12.5} fontWeight={700}>{item.label}</Typography>} />)}
        </Stack>
        <Stack direction="row" justifyContent="flex-end" mt={1.2}>
          <Button variant="contained" startIcon={previewing ? <CircularProgress color="inherit" size={15} /> : <PreviewRounded />} disabled={previewing || executing} onClick={() => void runPreview()}>生成预览</Button>
        </Stack>
      </Paper>

      {preview && <Paper variant="outlined" sx={{ overflow: 'hidden', borderRadius: 2 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1} alignItems={{ sm: 'center' }} px={1.4} py={1.15} bgcolor="action.hover">
          <Box><Stack direction="row" gap={.7} alignItems="center"><Typography fontWeight={850}>已冻结预览</Typography><Chip size="small" color={preview.delete_mode === 'hard' ? 'error' : 'success'} label={preview.delete_mode === 'hard' ? '不可恢复' : '可恢复/可核验'} /></Stack><Typography variant="caption" color="text.secondary">{selectedScopeText} · 请求号 {preview.request_id}</Typography></Box>
          <Stack direction="row" gap={.7}><Chip size="small" label={`计划 ${plannedTotal.toLocaleString()} 条`} color={preview.delete_mode === 'hard' ? 'error' : 'primary'} /><Chip size="small" label={`保护 ${protectedTotal.toLocaleString()} 条`} color={protectedTotal ? 'warning' : 'default'} /></Stack>
        </Stack>
        <ItemTable items={previewItems} />
        {execution ? <Box borderTop={1} borderColor="divider"><Typography fontWeight={850} px={1.4} pt={1.2}>执行结果</Typography><ItemTable items={execution.items} result /></Box> : <Stack direction="row" justifyContent="flex-end" p={1.25} borderTop={1} borderColor="divider">
          <Button color="error" variant="contained" startIcon={<WarningAmberRounded />} disabled={!plannedTotal || executing} onClick={() => setExecuteOpen(true)}>确认执行</Button>
        </Stack>}
      </Paper>}
    </Stack>}

    {tab === 'runs' && <Stack gap={1.25} mt={1.5}>
      <Paper variant="outlined" sx={{ p: 1.25, borderRadius: 2 }}>
        <TextField select size="small" label="运行范围" value={runWorkspace} onChange={event => { setRunWorkspace(Number(event.target.value)); setRunsBeforeID(undefined) }} sx={{ minWidth: { xs: '100%', sm: 340 } }}>
          <MenuItem value={0}>全部任务</MenuItem>
          {workspaces.map(item => <MenuItem key={item.id} value={item.id}>{item.label} · {item.kind}</MenuItem>)}
        </TextField>
      </Paper>
      <Paper variant="outlined" sx={{ overflow: 'hidden', borderRadius: 2 }}>
        <TableContainer><Table size="small" sx={{ minWidth: 940 }}><TableHead><TableRow><TableCell>请求号</TableCell><TableCell>范围</TableCell><TableCell>方式</TableCell><TableCell>状态</TableCell><TableCell>执行人</TableCell><TableCell>处理结果</TableCell><TableCell>创建时间</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead>
          <TableBody>{safeArray(runs).map(run => <TableRow key={run.id} hover>
            <TableCell><Typography fontFamily="ui-monospace, monospace" fontSize={12}>{run.request_id}</Typography></TableCell>
            <TableCell>{run.all_workspaces ? '全部工作区' : workspaceLabel(run.workspace_id)}</TableCell>
            <TableCell><Chip size="small" color={run.delete_mode === 'hard' ? 'error' : 'default'} variant={run.delete_mode === 'hard' ? 'filled' : 'outlined'} label={run.delete_mode === 'hard' ? '永久删除' : '可恢复/归档'} /></TableCell>
            <TableCell><Chip size="small" color={run.status === 'completed' ? 'success' : run.status === 'failed' ? 'error' : 'warning'} label={statusLabels[run.status] ?? run.status ?? '未知'} /></TableCell>
            <TableCell><Typography fontSize={13}>{run.executed_by_name || '尚未执行'}</Typography><Typography variant="caption" color="text.secondary">预览：{run.actor_name || `用户 ${run.actor_id || '—'}`}</Typography></TableCell>
            <TableCell>{safeArray(run.result).reduce((sum, item) => sum + asCount(item.affected_count), 0).toLocaleString()} 条</TableCell>
            <TableCell>{dateTime(run.created_at)}</TableCell>
            <TableCell align="right"><Stack direction="row" justifyContent="flex-end" gap={.5}><Button size="small" startIcon={<HistoryRounded />} onClick={() => void openRun(run)}>详情</Button>{runHasArchive(run) && <Button size="small" startIcon={<ArchiveRounded />} onClick={() => openArchives(run)}>归档</Button>}</Stack></TableCell>
          </TableRow>)}</TableBody>
        </Table></TableContainer>
        {!runs.length && !runsLoading && <Typography color="text.secondary" textAlign="center" py={5}>暂无运行记录</Typography>}
        <Stack direction="row" justifyContent="center" p={1.2} borderTop={runs.length ? 1 : 0} borderColor="divider">
          {runsLoading ? <CircularProgress size={22} /> : runsHasMore ? <Button size="small" onClick={() => void loadRuns(true, runsBeforeID)}>加载更多</Button> : runs.length ? <Typography variant="caption" color="text.secondary">已加载全部记录</Typography> : null}
        </Stack>
      </Paper>
    </Stack>}

    <Dialog open={Boolean(pendingPolicySave)} onClose={closePolicyConfirmation} fullWidth maxWidth="sm">
      <DialogTitle>确认启用自动永久清理</DialogTitle>
      <DialogContent><Stack gap={1.5} pt={.5}>
        <Alert severity="error">该策略会自动永久删除达到回收站保留天数的游戏房聊天，删除后不可恢复。保存后由定时任务执行，不会再次逐批询问确认。</Alert>
        <Typography fontSize={13}>策略范围：{pendingPolicySave?.workspaceID ? workspaceLabel(pendingPolicySave.workspaceID) : '平台默认策略（影响继承该策略的房间）'}</Typography>
        {pendingPolicySave?.updates.filter(item => item.dataClass === 'game_chat_messages' && item.input.enabled && asCount(item.input.purge_after_days) > 0).map(item => <Typography key={item.dataClass} fontSize={13}>游戏房聊天保留 {item.input.retention_days} 天后软删除，再在回收站保留 {item.input.purge_after_days} 天后永久清除。</Typography>)}
        <TextField autoFocus label="输入“永久删除”确认自动清理" value={policyConfirmWord} disabled={policySaving} onChange={event => setPolicyConfirmWord(event.target.value)} error={Boolean(policyConfirmWord && policyConfirmWord !== '永久删除')} helperText="仅开启可恢复软删除时，请取消并将回收站保留天数设为 0。" />
        {error && <Alert severity="error">{error}</Alert>}
      </Stack></DialogContent>
      <DialogActions><Button onClick={closePolicyConfirmation} disabled={policySaving}>取消</Button><Button color="error" variant="contained" disabled={policySaving || policyConfirmWord !== '永久删除'} onClick={() => { if (pendingPolicySave && policyConfirmWord === '永久删除') void persistPolicies(pendingPolicySave) }}>{policySaving ? '保存中…' : '确认启用自动永久清理'}</Button></DialogActions>
    </Dialog>

    <Dialog open={executeOpen} onClose={() => !executing && setExecuteOpen(false)} fullWidth maxWidth="sm">
      <DialogTitle>{preview?.delete_mode === 'hard' ? '二次确认永久删除' : '二次确认数据维护'}</DialogTitle>
      <DialogContent><Stack gap={1.5} pt={.5}>
        <Alert severity={preview?.delete_mode === 'hard' ? 'error' : 'warning'}>{preview?.delete_mode === 'hard' ? '本操作不可恢复。' : ''}将对“{selectedScopeText}”按冻结预览执行，计划处理 {plannedTotal.toLocaleString()} 条，受保护 {protectedTotal.toLocaleString()} 条。</Alert>
        <TextField autoFocus label={`输入“${confirmWord}”继续`} value={executeWord} onChange={event => setExecuteWord(event.target.value.trim())} error={Boolean(executeWord && executeWord !== confirmWord)} helperText="执行只接受当前预览的请求号和删除方式；修改条件后必须重新预览。" />
      </Stack></DialogContent>
      <DialogActions><Button onClick={() => setExecuteOpen(false)} disabled={executing}>取消</Button><Button color="error" variant="contained" disabled={executing || executeWord !== confirmWord} onClick={() => void executeCleanup()}>{executing ? '执行中…' : preview?.delete_mode === 'hard' ? '永久删除' : '执行维护'}</Button></DialogActions>
    </Dialog>

    <Dialog open={Boolean(selectedRun)} onClose={() => !restoring && setSelectedRun(null)} fullWidth maxWidth="md">
      <DialogTitle>运行详情</DialogTitle>
      <DialogContent dividers>{selectedRun && <Stack gap={1.2}>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={.8} flexWrap="wrap">
          <Chip size="small" label={statusLabels[selectedRun.status] ?? selectedRun.status} color={selectedRun.status === 'completed' ? 'success' : selectedRun.status === 'failed' ? 'error' : 'warning'} />
          <Chip size="small" color={selectedRun.delete_mode === 'hard' ? 'error' : 'default'} variant="outlined" label={selectedRun.delete_mode === 'hard' ? '永久删除 · 不可恢复' : '软删除/安全归档'} />
          <Chip size="small" variant="outlined" label={selectedRun.all_workspaces ? '全部工作区' : workspaceLabel(selectedRun.workspace_id)} />
          <Chip size="small" variant="outlined" label={`批次上限 ${asCount(selectedRun.batch_limit).toLocaleString()}`} />
          <Chip size="small" variant="outlined" label={`预览：${selectedRun.actor_name || `用户 ${selectedRun.actor_id || '—'}`}`} />
          <Chip size="small" variant="outlined" label={`执行：${selectedRun.executed_by_name || '尚未执行'}`} />
          {selectedRun.soft_restored_at && <Chip size="small" variant="outlined" label={`软恢复：${selectedRun.soft_restored_by_name || '未知管理员'}`} />}
          {selectedRun.financial_restored_at && <Chip size="small" variant="outlined" label={`归档恢复：${selectedRun.financial_restored_by_name || '未知管理员'}`} />}
          {asCount(selectedRun.content_purge_count) > 0 && <Chip size="small" color="error" variant="outlined" label={`已永久清理 ${asCount(selectedRun.content_purge_count).toLocaleString()} 条`} />}
        </Stack>
        <Typography fontFamily="ui-monospace, monospace" fontSize={12} color="text.secondary">{selectedRun.request_id}</Typography>
        {selectedRun.last_content_purge_request_id && <Typography fontFamily="ui-monospace, monospace" fontSize={12} color="error.main">永久清理请求号：{selectedRun.last_content_purge_request_id}</Typography>}
        {selectedRun.last_error && <Alert severity="error">{selectedRun.last_error}</Alert>}
        {runDetailLoading ? <Box display="grid" sx={{ placeItems: 'center' }} py={4}><CircularProgress size={24} /></Box> : <>
          <Typography fontWeight={850}>冻结预览</Typography><Paper variant="outlined"><ItemTable items={selectedRun.preview} /></Paper>
          <Typography fontWeight={850}>执行结果</Typography><Paper variant="outlined"><ItemTable items={selectedRun.result} result /></Paper>
          {safeArray(selectedRun.soft_restore_result).length > 0 && <><Typography fontWeight={850}>软删除恢复结果</Typography><Paper variant="outlined"><ItemTable items={selectedRun.soft_restore_result} result /></Paper></>}
          {safeArray(selectedRun.financial_restore_result).length > 0 && <><Typography fontWeight={850}>机器人归档恢复结果</Typography><Paper variant="outlined"><ItemTable items={selectedRun.financial_restore_result} result /></Paper></>}
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1}>
            <Typography variant="caption" color="text.secondary">创建 {dateTime(selectedRun.created_at)} · 完成 {dateTime(selectedRun.completed_at)}</Typography>
            <Stack direction="row" gap={.7}>
              {runHasArchive(selectedRun) && <Button size="small" startIcon={<ArchiveRounded />} onClick={() => openArchives(selectedRun)}>浏览归档</Button>}
              {canRestoreSoft && <Button size="small" color="warning" variant="outlined" startIcon={<RestoreRounded />} onClick={() => setRestoreKind('soft')}>恢复软删除</Button>}
              {canRestoreRobot && <Button size="small" color="warning" variant="outlined" startIcon={<RestoreRounded />} onClick={() => setRestoreKind('robot')}>恢复机器人归档</Button>}
            </Stack>
          </Stack>
        </>}
      </Stack>}</DialogContent>
      <DialogActions><Button onClick={() => setSelectedRun(null)} disabled={restoring}>关闭</Button></DialogActions>
    </Dialog>

    <Dialog open={Boolean(restoreKind)} onClose={() => !restoring && setRestoreKind(null)} fullWidth maxWidth="xs">
      <DialogTitle>确认恢复数据</DialogTitle>
      <DialogContent><Alert severity="warning">{restoreKind === 'soft' ? '将恢复本任务软删除的普通聊天、机器人普通聊天、游戏房聊天与通知数据。' : '将把本任务归档的机器人注单与安全流水恢复到热表。'}恢复操作按任务幂等执行。</Alert></DialogContent>
      <DialogActions><Button onClick={() => setRestoreKind(null)} disabled={restoring}>取消</Button><Button variant="contained" color="warning" disabled={restoring} onClick={() => void restore()}>{restoring ? '恢复中…' : '确认恢复'}</Button></DialogActions>
    </Dialog>

    <Dialog open={Boolean(archiveRun)} onClose={() => !archivesLoading && setArchiveRun(null)} fullWidth maxWidth="lg">
      <DialogTitle>冷归档 · {archiveRun?.request_id}</DialogTitle>
      <DialogContent dividers>
        <Tabs value={archiveKind} onChange={(_, value: LifecycleArchiveKind) => { if (!archiveRun) return; setArchiveKind(value); setArchiveBeforeID(undefined); setArchives([]); void loadArchives(archiveRun, value, false) }} sx={{ mb: 1 }}><Tab value="bets" label="机器人注单" /><Tab value="ledger" label="安全流水" /><Tab value="audit" label="审计日志" /></Tabs>
        {archiveKind === 'audit' && <Alert severity="info" sx={{ mb: 1 }}>审计归档为只读凭证，仅支持核验和浏览。</Alert>}
        <TableContainer><Table size="small" sx={{ minWidth: 960 }}><TableHead><TableRow><TableCell>ID</TableCell><TableCell>工作区/用户</TableCell><TableCell>标识/关联</TableCell><TableCell>状态/类型</TableCell><TableCell>金额</TableCell><TableCell>原始时间</TableCell><TableCell>归档时间</TableCell><TableCell>校验哈希</TableCell></TableRow></TableHead>
          <TableBody>{safeArray(archives).map(row => <TableRow key={row.id}><TableCell>{row.id}</TableCell><TableCell>{row.workspace_id} / {row.user_id || '—'}</TableCell><TableCell>{row.game_id || row.reference || '—'}<Typography variant="caption" display="block" color="text.secondary">{row.issue || (row.game_id ? row.reference : '') || '—'}</Typography></TableCell><TableCell>{row.status || '—'}<Typography variant="caption" display="block" color="text.secondary">{row.type || '—'}</Typography></TableCell><TableCell>{moneyFromCents(row.amount_cents)}</TableCell><TableCell>{dateTime(row.created_at)}</TableCell><TableCell>{dateTime(row.archived_at)}</TableCell><TableCell><Typography fontFamily="ui-monospace, monospace" fontSize={11} title={row.row_hash}>{row.row_hash ? `${row.row_hash.slice(0, 12)}…` : '—'}</Typography></TableCell></TableRow>)}</TableBody>
        </Table></TableContainer>
        {!archives.length && !archivesLoading && <Typography textAlign="center" color="text.secondary" py={5}>本任务暂无此类归档</Typography>}
        <Stack alignItems="center" py={1.2}>{archivesLoading ? <CircularProgress size={22} /> : archiveHasMore && archiveRun ? <Button size="small" onClick={() => void loadArchives(archiveRun, archiveKind, true)}>加载更多</Button> : archives.length ? <Typography variant="caption" color="text.secondary">已加载全部归档</Typography> : null}</Stack>
      </DialogContent>
      <DialogActions><Button onClick={() => setArchiveRun(null)}>关闭</Button></DialogActions>
    </Dialog>
  </Box>
}
