import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  LinearProgress,
  MenuItem,
  Paper,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material'
import AddRounded from '@mui/icons-material/AddRounded'
import CloudUploadRounded from '@mui/icons-material/CloudUploadRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SportsEsportsRounded from '@mui/icons-material/SportsEsportsRounded'
import InboxRounded from '@mui/icons-material/InboxRounded'
import SendRounded from '@mui/icons-material/SendRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, resolveApiAsset, type AdminGame, type AdminUser, type EntertainmentPlatform, type OpsActivity, type SpecialOverview } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const typeLabel: Record<string, string> = { checkin: '签到', banner: '轮播', promotion: '推广活动', invite: '邀请', redpacket: '红包' }
const statusColor = (status: string): 'default' | 'success' | 'warning' | 'error' => {
  if (status === 'active' || status === 'enabled' || status === 'available') return 'success'
  if (status === 'maintenance' || status === 'draft' || status === 'reserved') return 'warning'
  if (status === 'ended' || status === 'disabled' || status === 'granted') return 'default'
  return 'default'
}
const statusLabel = (status: string) => ({ draft: '草稿', active: '进行中', ended: '已结束', enabled: '已启用', maintenance: '维护中', disabled: '未接入', available: '可用', reserved: '预留', granted: '已发放' }[status] ?? status)

function EmptyState({ message = '暂无符合条件的数据', description = '数据接口接入后会显示在这里' }: { message?: string; description?: string }) {
  return (
    <Stack minHeight={220} alignItems="center" justifyContent="center" color="text.secondary">
      <Box sx={{ width: 58, height: 58, display: 'grid', placeItems: 'center', borderRadius: 3, bgcolor: 'action.hover', color: 'primary.main' }}><InboxRounded /></Box>
      <Typography mt={1.5} fontSize={13} fontWeight={700}>{message}</Typography>
      <Typography variant="caption" textAlign="center" px={2}>{description}</Typography>
    </Stack>
  )
}

function ActivitiesPage() {
  const [items, setItems] = useState<OpsActivity[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<OpsActivity | null>(null)
  const [form, setForm] = useState({ type: 'checkin', title: '', subtitle: '', status: 'draft', cover: '', action_type: 'none', action_url: '', reward: 0, sort_order: 0, pool: 88, min_amount: 1, max_amount: 8.8 })
  const [saving, setSaving] = useState(false)
  const [uploading, setUploading] = useState(false)
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      setItems(await adminApi.activities())
      if (notify) showMessage('活动列表已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取活动失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const openCreate = () => {
    setEditing(null)
    setForm({ type: 'checkin', title: '', subtitle: '', status: 'draft', cover: '', action_type: 'none', action_url: '', reward: 0, sort_order: 0, pool: 88, min_amount: 1, max_amount: 8.8 })
    setOpen(true)
  }

  const openEdit = (item: OpsActivity) => {
    setEditing(item)
    const cfg = item.config ?? {}
    setForm({
      type: item.type,
      title: item.title,
      subtitle: item.subtitle,
      status: item.status,
      cover: item.cover,
      action_type: String(cfg.action_type ?? 'none'),
      action_url: String(cfg.action_url ?? ''),
      reward: item.reward,
      sort_order: item.sort_order,
      pool: Number(cfg.pool ?? item.pool_total ?? 88),
      min_amount: Number(cfg.min_amount ?? 1),
      max_amount: Number(cfg.max_amount ?? 8.8),
    })
    setOpen(true)
  }

  const buildPayload = () => ({
    type: form.type,
    title: form.title,
    subtitle: form.subtitle,
    status: form.status,
    cover: form.cover,
    reward: form.reward,
    sort_order: form.sort_order,
    config: form.type === 'redpacket'
      ? { pool: form.pool, min_amount: form.min_amount, max_amount: form.max_amount, action_type: form.action_type, action_url: form.action_url }
      : { action_type: form.action_type, action_url: form.action_url },
  })

  const save = async () => {
    if (!form.title.trim()) {
      setError('请填写活动标题')
      return
    }
    setSaving(true)
    try {
      if (editing) await adminApi.updateActivity(editing.id, buildPayload())
      else await adminApi.createActivity(buildPayload())
      setOpen(false)
      showMessage(editing ? '活动已更新' : '活动已创建')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存活动失败')
    } finally {
      setSaving(false)
    }
  }

  const uploadCover = async (file?: File) => {
    if (!file) return
    if (!/^image\/(jpeg|png|webp)$/i.test(file.type)) {
      setError('仅支持 JPG、PNG 或 WebP 图片')
      return
    }
    setUploading(true)
    setError('')
    try {
      const result = await adminApi.uploadActivityImage(file)
      setForm(current => ({ ...current, cover: result.url }))
      showMessage('活动图片已上传')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '上传活动图片失败')
    } finally {
      setUploading(false)
    }
  }

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="游戏运营 / 活动"
        title="活动管理"
        description="管理签到、轮播、推广海报、邀请和红包活动；推广海报可配置封面与点击跳转。"
        actions={
          <>
            <Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)} disabled={loading}>刷新</Button>
            <Button variant="contained" startIcon={<AddRounded />} onClick={openCreate}>新增活动</Button>
          </>
        }
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      {loading && <Box mt={2}><CircularProgress size={20} /></Box>}
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'repeat(2,1fr)' }, gap: 2, mt: 2.5 }}>
        {items.map(item => (
          <Card key={item.id}>
            <Box sx={{ height: 140, p: 2.5, color: '#fff', display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', backgroundImage: item.cover ? `linear-gradient(0deg,rgba(5,28,48,.72),transparent 74%),url(${resolveApiAsset(item.cover)})` : 'radial-gradient(circle at 80% 20%,#8ff5e5,#29aeb4 34%,#0e5488)', backgroundSize: 'cover', backgroundPosition: 'center' }}>
              <Typography variant="caption">{typeLabel[item.type] ?? item.type}</Typography>
              <Typography variant="h6" fontWeight={800}>{item.title}</Typography>
            </Box>
            <CardContent>
              <Stack direction="row" justifyContent="space-between" gap={1}>
                <Box minWidth={0}>
                  <Typography fontWeight={750}>{item.title}</Typography>
                  <Typography variant="caption" color="text.secondary">{item.subtitle || '暂无说明'}</Typography>
                </Box>
                <Chip size="small" color={statusColor(item.status)} label={statusLabel(item.status)} />
              </Stack>
              <Typography variant="caption" color="text.secondary" display="block" mt={1.5}>
                参与 {item.participants} · 奖励 {item.reward}
                {item.type === 'redpacket' && (
                  <> · 奖池 {item.pool_remaining ?? item.pool_total ?? 0}/{item.pool_total ?? 0} 元</>
                )}
              </Typography>
              <Stack direction="row" gap={1} mt={2} flexWrap="wrap">
                <Button size="small" variant="outlined" onClick={() => openEdit(item)}>编辑</Button>
                {item.status !== 'active' && <Button size="small" onClick={() => void adminApi.setActivityStatus(item.id, 'active').then(() => load()).then(() => showMessage('已上线'))}>上线</Button>}
                {item.status === 'active' && <Button size="small" onClick={() => void adminApi.setActivityStatus(item.id, 'ended').then(() => load()).then(() => showMessage('已结束'))}>结束</Button>}
                <Button size="small" color="error" onClick={() => void adminApi.deleteActivity(item.id).then(() => load()).then(() => showMessage('已删除'))}>删除</Button>
              </Stack>
            </CardContent>
          </Card>
        ))}
        {!loading && items.length === 0 && <Card><CardContent><EmptyState message="暂无活动" description="点击右上角新增签到/轮播/邀请/红包活动" /></CardContent></Card>}
      </Box>
      <Dialog open={open} onClose={() => setOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>{editing ? '编辑活动' : '新增活动'}</DialogTitle>
        <DialogContent>
          <Stack gap={1.5} mt={1}>
            <TextField select label="类型" value={form.type} onChange={e => setForm(current => ({ ...current, type: e.target.value }))}>
              {Object.entries(typeLabel).map(([value, label]) => <MenuItem key={value} value={value}>{label}</MenuItem>)}
            </TextField>
            <TextField label="标题" value={form.title} onChange={e => setForm(current => ({ ...current, title: e.target.value }))} />
            <TextField label="副标题" value={form.subtitle} onChange={e => setForm(current => ({ ...current, subtitle: e.target.value }))} />
            <Paper variant="outlined" sx={{ overflow: 'hidden' }}>
              {form.cover ? (
                <Box component="img" src={resolveApiAsset(form.cover)} alt="活动图片预览" sx={{ width: '100%', height: 150, display: 'block', objectFit: 'cover', bgcolor: 'action.hover' }} />
              ) : (
                <Stack height={120} alignItems="center" justifyContent="center" color="text.secondary">
                  <CloudUploadRounded color="primary" />
                  <Typography variant="caption" mt={1}>上传后会在用户端活动通知中展示</Typography>
                </Stack>
              )}
              <Stack direction="row" gap={1} p={1.25}>
                <Button component="label" variant="outlined" startIcon={<CloudUploadRounded />} disabled={uploading}>
                  {uploading ? '上传中…' : form.cover ? '更换图片' : '上传图片'}
                  <input hidden type="file" accept="image/jpeg,image/png,image/webp" onChange={event => { void uploadCover(event.target.files?.[0]); event.target.value = '' }} />
                </Button>
                {form.cover && <Button color="error" startIcon={<DeleteOutlineRounded />} onClick={() => setForm(current => ({ ...current, cover: '' }))}>移除</Button>}
              </Stack>
            </Paper>
            <TextField select label="点击后操作" value={form.action_type} onChange={e => setForm(current => ({ ...current, action_type: e.target.value }))}>
              <MenuItem value="none">仅查看活动</MenuItem>
              <MenuItem value="internal">跳转站内页面</MenuItem>
              <MenuItem value="external">跳转外部链接</MenuItem>
            </TextField>
            {form.action_type !== 'none' && <TextField label="跳转地址" value={form.action_url} onChange={e => setForm(current => ({ ...current, action_url: e.target.value }))} helperText={form.action_type === 'internal' ? '例如 /wallet/rebate 或 /games/canada-28' : '请填写完整的 https:// 地址'} />}
            <TextField select label="状态" value={form.status} onChange={e => setForm(current => ({ ...current, status: e.target.value }))}>
              <MenuItem value="draft">草稿</MenuItem>
              <MenuItem value="active">进行中</MenuItem>
              <MenuItem value="ended">已结束</MenuItem>
            </TextField>
            <TextField type="number" label="奖励金额" value={form.reward} onChange={e => setForm(current => ({ ...current, reward: Number(e.target.value) }))} />
            {form.type === 'redpacket' && (
              <>
                <TextField type="number" label="奖池总额（元）" value={form.pool} onChange={e => setForm(current => ({ ...current, pool: Number(e.target.value) }))} helperText="真实奖池，领完即止" />
                <TextField type="number" label="单次最低（元）" value={form.min_amount} onChange={e => setForm(current => ({ ...current, min_amount: Number(e.target.value) }))} />
                <TextField type="number" label="单次最高（元）" value={form.max_amount} onChange={e => setForm(current => ({ ...current, max_amount: Number(e.target.value) }))} />
              </>
            )}
            <TextField type="number" label="排序" value={form.sort_order} onChange={e => setForm(current => ({ ...current, sort_order: Number(e.target.value) }))} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>取消</Button>
          <Button variant="contained" disabled={saving} onClick={() => void save()}>{saving ? '保存中…' : '保存'}</Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

function EntertainmentPage() {
  const [items, setItems] = useState<EntertainmentPlatform[]>([])
  const [games, setGames] = useState<AdminGame[]>([])
  const [gameFilter, setGameFilter] = useState<'all' | 'enabled' | 'disabled'>('all')
  const [gameQuery, setGameQuery] = useState('')
  const [pendingGame, setPendingGame] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState({ code: '', name: '', category: '其他', merchant_no: '', api_base: '', launch_path: '/portal', secret_key: '', status: 'disabled', remark: '', sort_order: 0 })
  const [saving, setSaving] = useState(false)
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const [platforms, lotteryGames] = await Promise.all([adminApi.entertainment(), adminApi.games()])
      setItems(platforms)
      setGames(lotteryGames)
      if (notify) showMessage('游戏与彩种已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取娱乐平台失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const visibleGames = games.filter(game => {
    if (gameFilter === 'enabled' && !game.enabled) return false
    if (gameFilter === 'disabled' && game.enabled) return false
    const query = gameQuery.trim().toLowerCase()
    return !query || game.name.toLowerCase().includes(query) || game.id.toLowerCase().includes(query) || game.category.toLowerCase().includes(query)
  })
  const groupedGames = Array.from(visibleGames.reduce((map, game) => {
    map.set(game.category, [...(map.get(game.category) ?? []), game])
    return map
  }, new Map<string, AdminGame[]>()).entries()).sort(([left], [right]) => left.localeCompare(right, 'zh-CN'))

  const toggleGame = async (game: AdminGame, enabled: boolean) => {
    setPendingGame(game.id)
    setError('')
    try {
      const updated = await adminApi.updateGameStatus(game.id, enabled)
      setGames(current => current.map(item => item.id === updated.id ? { ...item, ...updated } : item))
      showMessage(`${game.name}已${enabled ? '启用' : '停用'}`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '更新彩种状态失败')
    } finally {
      setPendingGame('')
    }
  }

  const save = async () => {
    if (!form.code.trim() || !form.name.trim()) {
      setError('请填写平台编号和名称')
      return
    }
    setSaving(true)
    try {
      await adminApi.upsertEntertainment(form)
      setOpen(false)
      showMessage('娱乐平台已保存')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="彩票运营 / 游戏中心"
        title="游戏与彩种"
        description="所有已接入、已停用和维护中的游戏统一在这里管理；停用彩种仍会保留，随时可以重新开放。"
        actions={
          <>
            <Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)}>刷新</Button>
            <Button variant="contained" startIcon={<SportsEsportsRounded />} onClick={() => { setForm({ code: '', name: '', category: '其他', merchant_no: '', api_base: '', launch_path: '/portal', secret_key: '', status: 'disabled', remark: '', sort_order: items.length + 1 }); setOpen(true) }}>接入扩展游戏</Button>
          </>
        }
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      {loading && <Box mt={2}><CircularProgress size={20} /></Box>}
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,minmax(0,1fr))', md: 'repeat(4,minmax(0,1fr))' }, gap: 1.2, mt: 2 }}>
        {[['全部彩种', games.length, 'primary.main'], ['正常开放', games.filter(game => game.enabled).length, 'success.main'], ['已停用', games.filter(game => !game.enabled).length, 'text.secondary'], ['彩票分类', new Set(games.map(game => game.category)).size, 'warning.main']].map(([label, value, color]) => <Card key={String(label)}><CardContent sx={{ p: '14px !important' }}><Typography variant="caption" color="text.secondary">{label}</Typography><Typography fontSize={{ xs: 21, md: 26 }} fontWeight={900} color={String(color)}>{value}</Typography></CardContent></Card>)}
      </Box>
      <Paper variant="outlined" sx={{ mt: 1.5, p: 1.35 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1} alignItems={{ sm: 'center' }}>
          <TextField size="small" placeholder="搜索彩种名称、编号或分类" value={gameQuery} onChange={event => setGameQuery(event.target.value)} sx={{ flex: 1, minWidth: 220 }} />
          <Stack direction="row" gap={.75}>{(['all', 'enabled', 'disabled'] as const).map(filter => <Button key={filter} size="small" variant={gameFilter === filter ? 'contained' : 'outlined'} onClick={() => setGameFilter(filter)}>{filter === 'all' ? `全部 ${games.length}` : filter === 'enabled' ? `已启用 ${games.filter(game => game.enabled).length}` : `已停用 ${games.filter(game => !game.enabled).length}`}</Button>)}</Stack>
        </Stack>
      </Paper>
      <Stack gap={1.5} mt={1.5}>
        {groupedGames.map(([category, categoryGames]) => <Paper key={category} variant="outlined" sx={{ overflow: 'hidden' }}>
          <Stack direction="row" alignItems="center" justifyContent="space-between" px={1.6} py={1.2} bgcolor="action.hover">
            <Box><Typography fontWeight={850}>{category}</Typography><Typography variant="caption" color="text.secondary">{categoryGames.length} 个彩种 · {categoryGames.filter(game => game.enabled).length} 个开放</Typography></Box>
            <Chip size="small" color={categoryGames.some(game => game.enabled) ? 'primary' : 'default'} label={categoryGames.some(game => game.enabled) ? '分类已开放' : '分类已停用'} />
          </Stack>
          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,minmax(0,1fr))', xl: 'repeat(3,minmax(0,1fr))' }, gap: 1, p: 1.2 }}>
            {categoryGames.map(game => <Card key={game.id} variant="outlined" sx={{ boxShadow: 'none', opacity: game.enabled ? 1 : .68 }}>
              <CardContent sx={{ p: '13px !important' }}>
                <Stack direction="row" alignItems="flex-start" gap={1.1}>
                  <Box sx={{ width: 40, height: 40, flex: '0 0 40px', display: 'grid', placeItems: 'center', borderRadius: 2.2, color: 'white', fontSize: 12, fontWeight: 900, background: game.enabled ? 'linear-gradient(145deg,#1684ad,#29bdb0)' : 'linear-gradient(145deg,#77838d,#a3acb3)' }}>{game.name.slice(0, 2)}</Box>
                  <Box flex={1} minWidth={0}><Typography fontSize={13} fontWeight={850} noWrap>{game.name}</Typography><Typography fontSize={9} color="text.secondary" noWrap>{game.id}</Typography><Stack direction="row" gap={.5} mt={.7} flexWrap="wrap"><Chip size="small" variant="outlined" label={game.source_kind === 'official' ? '官方源' : game.source_kind === 'external' ? '外部源' : '平台彩'} sx={{ height: 20, fontSize: 9 }} /><Chip size="small" label={game.enabled ? '已启用' : '已停用'} color={game.enabled ? 'success' : 'default'} sx={{ height: 20, fontSize: 9 }} /></Stack></Box>
                  <Switch size="small" checked={game.enabled} disabled={pendingGame === game.id} onChange={event => void toggleGame(game, event.target.checked)} inputProps={{ 'aria-label': `${game.name}状态` }} />
                </Stack>
                {game.source_kind !== 'platform' && <Typography mt={1} fontSize={9} color={game.sync_status === 'error' ? 'error.main' : 'text.secondary'} noWrap>{game.sync_status === 'error' ? game.last_sync_error || '开奖源异常' : game.last_sync_at ? `最近同步 ${new Date(game.last_sync_at).toLocaleString('zh-CN', { hour12: false })}` : game.source_name || '等待首次同步'}</Typography>}
              </CardContent>
            </Card>)}
          </Box>
        </Paper>)}
        {!loading && groupedGames.length === 0 && <Paper variant="outlined"><EmptyState message="没有符合条件的彩种" description="可切换“全部”或清除搜索条件" /></Paper>}
      </Stack>
      <Stack direction="row" alignItems="center" justifyContent="space-between" mt={3} mb={1.2}><Box><Typography fontSize={17} fontWeight={900}>扩展娱乐服务</Typography><Typography variant="caption" color="text.secondary">捕鱼、体育、真人、电子和电竞等第三方服务；未配置的项目保持停用。</Typography></Box><Chip size="small" label={`${items.length} 个平台`} /></Stack>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'repeat(2,1fr)' }, gap: 1.5 }}>
        {items.map(item => (
          <Card key={item.id}>
            <CardContent>
              <Stack direction="row" gap={1.5}>
                <Box sx={{ width: 50, height: 50, display: 'grid', placeItems: 'center', borderRadius: 3, color: '#fff', bgcolor: 'primary.main', fontWeight: 800 }}>{item.name.slice(0, 1)}</Box>
                <Box flex={1} minWidth={0}>
                  <Typography variant="caption" color="primary">{item.category}</Typography>
                  <Typography fontWeight={750}>{item.name}</Typography>
                  <Typography variant="caption" color="text.secondary">{item.remark || item.merchant_no || '尚未填写商户信息'} · {item.has_secret ? '密钥已配置' : '密钥未配置'}</Typography>
                </Box>
                <Chip size="small" color={statusColor(item.status)} label={statusLabel(item.status)} />
              </Stack>
              <Divider sx={{ my: 2 }} />
              <Stack direction="row" justifyContent="flex-end" gap={1} flexWrap="wrap">
                <Button size="small" variant="outlined" onClick={() => { setForm({ code: item.code, name: item.name, category: item.category, merchant_no: item.merchant_no, api_base: item.api_base, launch_path: item.launch_path ?? '/portal', secret_key: '', status: item.status, remark: item.remark, sort_order: item.sort_order }); setOpen(true) }}>编辑配置</Button>
                {item.status !== 'enabled' && <Button size="small" variant="contained" onClick={() => void adminApi.setEntertainmentStatus(item.id, 'enabled').then(() => load()).then(() => showMessage('已启用'))}>启用</Button>}
                {item.status === 'enabled' && <Button size="small" onClick={() => void adminApi.setEntertainmentStatus(item.id, 'maintenance').then(() => load()).then(() => showMessage('已设为维护'))}>维护</Button>}
                {item.status !== 'disabled' && <Button size="small" color="inherit" onClick={() => void adminApi.setEntertainmentStatus(item.id, 'disabled').then(() => load()).then(() => showMessage('已停用'))}>停用</Button>}
              </Stack>
            </CardContent>
          </Card>
        ))}
        {!loading && items.length === 0 && <Paper variant="outlined"><EmptyState message="暂无扩展娱乐平台" description="彩票彩种不受影响；需要时可在右上角接入捕鱼、体育或真人平台" /></Paper>}
      </Box>
      <Dialog open={open} onClose={() => setOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>娱乐平台配置</DialogTitle>
        <DialogContent>
          <Stack gap={1.5} mt={1}>
            <TextField label="平台编号" value={form.code} onChange={e => setForm(current => ({ ...current, code: e.target.value }))} helperText="例如 kaiyuan / pg / ag" />
            <TextField label="名称" value={form.name} onChange={e => setForm(current => ({ ...current, name: e.target.value }))} />
            <TextField label="分类" value={form.category} onChange={e => setForm(current => ({ ...current, category: e.target.value }))} />
            <TextField label="商户号" value={form.merchant_no} onChange={e => setForm(current => ({ ...current, merchant_no: e.target.value }))} />
            <TextField label="API 地址" value={form.api_base} onChange={e => setForm(current => ({ ...current, api_base: e.target.value }))} helperText="留空则使用本地桥接页；也可填第三方根地址" />
            <TextField label="Launch 路径" value={form.launch_path} onChange={e => setForm(current => ({ ...current, launch_path: e.target.value }))} helperText="例如 /portal 或完整 URL 模板" />
            <TextField label="签名密钥" type="password" autoComplete="new-password" value={form.secret_key} onChange={e => setForm(current => ({ ...current, secret_key: e.target.value }))} helperText="仅写入时接收；编辑留空会保留现有密钥，后台不会回显原文" />
            <TextField select label="状态" value={form.status} onChange={e => setForm(current => ({ ...current, status: e.target.value }))}>
              <MenuItem value="enabled">已启用</MenuItem>
              <MenuItem value="maintenance">维护中</MenuItem>
              <MenuItem value="disabled">未接入</MenuItem>
            </TextField>
            <TextField label="备注" value={form.remark} onChange={e => setForm(current => ({ ...current, remark: e.target.value }))} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>取消</Button>
          <Button variant="contained" disabled={saving} onClick={() => void save()}>保存</Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

function SpecialPage() {
  const [data, setData] = useState<SpecialOverview | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [numbers, setNumbers] = useState('')
  const [level, setLevel] = useState('normal')
  const [assignOpen, setAssignOpen] = useState(false)
  const [assigning, setAssigning] = useState(false)
  const [candidates, setCandidates] = useState<AdminUser[]>([])
  const [assignResourceID, setAssignResourceID] = useState(0)
  const [assignUserID, setAssignUserID] = useState(0)
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const overview = await adminApi.specialOverview()
      setData(overview)
      if (notify) showMessage('房间靓号已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取房间号失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const total = (data?.available ?? 0) + (data?.reserved ?? 0) + (data?.granted ?? 0)
  const progress = total ? ((data?.granted ?? 0) / total) * 100 : 0
  const availableResources = (data?.resources ?? []).filter(item => item.status === 'available')

  const openAssign = async () => {
    setError('')
    try {
      const result = await adminApi.users({ role: 'member', status: 'active', page: 1, pageSize: 100 })
      setCandidates(result.items)
      setAssignResourceID(availableResources[0]?.id ?? 0)
      setAssignUserID(result.items[0]?.id ?? 0)
      setAssignOpen(true)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取可发放会员失败')
    }
  }

  const assignRoom = async () => {
    if (!assignResourceID || !assignUserID) return
    setAssigning(true)
    try {
      await adminApi.assignAgentRoom({ resource_id: assignResourceID, user_id: assignUserID })
      setAssignOpen(false)
      showMessage('房间号已发放，会员已升级为代理')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '发放房间号失败')
    } finally {
      setAssigning(false)
    }
  }

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="扩展服务 / 房间号"
        title="房间靓号"
        description="靓号即代理房间号。用户输入该号进入对应代理房间；发放时用户自动升为代理。"
        actions={<><Button variant="contained" startIcon={<SendRounded />} disabled={!availableResources.length} onClick={() => void openAssign()}>发放给代理</Button><Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)}>刷新</Button></>}
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      {loading && <Box mt={2}><CircularProgress size={20} /></Box>}
      <Card sx={{ mt: 2.5, color: '#fff', background: 'radial-gradient(circle at 85% 25%,#7ee8da,#26a9b0 30%,#116b9b)' }}>
        <CardContent sx={{ p: 3 }}>
          <Typography variant="caption">当前可售房间号</Typography>
          <Typography fontSize={36} fontWeight={850}>{data?.available ?? 0}</Typography>
          <Typography variant="body2">已分配 {data?.granted ?? 0} · 预留 {data?.reserved ?? 0}</Typography>
          <LinearProgress variant="determinate" value={progress} sx={{ mt: 2, maxWidth: 380, bgcolor: 'rgba(255,255,255,.18)' }} />
        </CardContent>
      </Card>
      <Box sx={{ mt: 1.5, maxWidth: 860 }}>
        <Card>
          <CardContent>
            <Typography fontWeight={750} mb={1.5}>批量添加房间号</Typography>
            <Stack gap={1.5}>
              <TextField multiline minRows={3} label="房间号列表" placeholder="每行一个，或用逗号分隔" value={numbers} onChange={e => setNumbers(e.target.value)} />
              <TextField select label="等级" value={level} onChange={e => setLevel(e.target.value)}>
                <MenuItem value="normal">普通</MenuItem>
                <MenuItem value="rare">稀有</MenuItem>
                <MenuItem value="epic">史诗</MenuItem>
              </TextField>
              <Button variant="contained" startIcon={<AddRounded />} onClick={() => void adminApi.addSpecialNumbers({ numbers, level }).then(r => { showMessage(`已添加 ${r.created} 个房间号`); setNumbers(''); return load() })}>添加资源</Button>
            </Stack>
            <Divider sx={{ my: 2 }} />
            <Typography fontWeight={750} mb={1}>房间号资源</Typography>
            {(data?.resources ?? []).length === 0 ? <EmptyState message="尚未添加房间号" /> : (
              <Stack gap={1} maxHeight={280} overflow="auto">
                {data?.resources.map(item => (
                  <Stack key={item.id} direction="row" justifyContent="space-between" alignItems="center">
                    <Box>
                      <Typography fontWeight={700}>{item.number}</Typography>
                      <Typography variant="caption" color="text.secondary">{item.level}{item.owner_username ? ` · 代理 ${item.owner_username}` : ''}</Typography>
                    </Box>
                    <Chip size="small" color={statusColor(item.status)} label={statusLabel(item.status)} />
                  </Stack>
                ))}
              </Stack>
            )}
          </CardContent>
        </Card>
      </Box>
      <Dialog open={assignOpen} onClose={() => !assigning && setAssignOpen(false)} fullWidth maxWidth="xs">
        <DialogTitle>发放房间号给代理</DialogTitle>
        <DialogContent>
          <Stack gap={1.5} pt={1}>
            <Alert severity="info">选择会员并发放房间号后，该会员会升级为代理并获得独立房间工作台。</Alert>
            <TextField select label="房间号" value={assignResourceID || ''} onChange={event => setAssignResourceID(Number(event.target.value))}>
              {availableResources.map(item => <MenuItem key={item.id} value={item.id}>{item.number} · {item.level === 'rare' ? '稀有' : item.level === 'epic' ? '史诗' : '普通'}</MenuItem>)}
            </TextField>
            <TextField select label="升级为代理的会员" value={assignUserID || ''} onChange={event => setAssignUserID(Number(event.target.value))}>
              {candidates.map(item => <MenuItem key={item.id} value={item.id}>{item.nickname || item.username} · @{item.username} · ID {item.public_id}</MenuItem>)}
            </TextField>
            {!candidates.length && <Alert severity="warning">暂无可升级的正常会员，请先在用户管理中创建会员。</Alert>}
          </Stack>
        </DialogContent>
        <DialogActions><Button disabled={assigning} onClick={() => setAssignOpen(false)}>取消</Button><Button variant="contained" disabled={assigning || !assignResourceID || !assignUserID} onClick={() => void assignRoom()}>{assigning ? '发放中…' : '确认发放'}</Button></DialogActions>
      </Dialog>
    </Box>
  )
}

export function ManagementPage({ path }: { path: string }) {
  if (path === '/activities') return <ActivitiesPage />
  if (path === '/entertainment') return <EntertainmentPage />
  if (path === '/special-numbers') return <SpecialPage />
  return <Box p={3}><EmptyState message="页面正在建设中" /></Box>
}
