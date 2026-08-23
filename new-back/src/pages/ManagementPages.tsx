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
  Stack,
  TextField,
  Typography,
} from '@mui/material'
import AddRounded from '@mui/icons-material/AddRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SportsEsportsRounded from '@mui/icons-material/SportsEsportsRounded'
import StarsRounded from '@mui/icons-material/StarsRounded'
import InboxRounded from '@mui/icons-material/InboxRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type EntertainmentPlatform, type OpsActivity, type SpecialOverview } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const typeLabel: Record<string, string> = { checkin: '签到', banner: '轮播', invite: '邀请', redpacket: '红包' }
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
  const [form, setForm] = useState({ type: 'checkin', title: '', subtitle: '', status: 'draft', reward: 0, sort_order: 0, pool: 88, min_amount: 1, max_amount: 8.8 })
  const [saving, setSaving] = useState(false)
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
    setForm({ type: 'checkin', title: '', subtitle: '', status: 'draft', reward: 0, sort_order: 0, pool: 88, min_amount: 1, max_amount: 8.8 })
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
    reward: form.reward,
    sort_order: form.sort_order,
    config: form.type === 'redpacket'
      ? { pool: form.pool, min_amount: form.min_amount, max_amount: form.max_amount }
      : {},
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

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="游戏运营 / 活动"
        title="活动管理"
        description="管理签到、轮播、邀请和红包活动。"
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
            <Box sx={{ height: 140, p: 2.5, color: '#fff', display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', background: 'radial-gradient(circle at 80% 20%,#8ff5e5,#29aeb4 34%,#0e5488)' }}>
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
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState({ code: '', name: '', category: '其他', merchant_no: '', api_base: '', launch_path: '/portal', secret_key: 'demo', status: 'disabled', remark: '', sort_order: 0 })
  const [saving, setSaving] = useState(false)
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      setItems(await adminApi.entertainment())
      if (notify) showMessage('娱乐平台已刷新')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取娱乐平台失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

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
        eyebrow="扩展服务 / 娱乐"
        title="彩票娱乐"
        description="管理开元 / PG / AG 等第三方娱乐平台配置与启停。"
        actions={
          <>
            <Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)}>刷新</Button>
            <Button variant="contained" startIcon={<SportsEsportsRounded />} onClick={() => { setForm({ code: '', name: '', category: '其他', merchant_no: '', api_base: '', launch_path: '/portal', secret_key: 'demo', status: 'disabled', remark: '', sort_order: items.length + 1 }); setOpen(true) }}>接入新平台</Button>
          </>
        }
      />
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
      {loading && <Box mt={2}><CircularProgress size={20} /></Box>}
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'repeat(2,1fr)' }, gap: 1.5, mt: 2.5 }}>
        {items.map(item => (
          <Card key={item.id}>
            <CardContent>
              <Stack direction="row" gap={1.5}>
                <Box sx={{ width: 50, height: 50, display: 'grid', placeItems: 'center', borderRadius: 3, color: '#fff', bgcolor: 'primary.main', fontWeight: 800 }}>{item.name.slice(0, 1)}</Box>
                <Box flex={1} minWidth={0}>
                  <Typography variant="caption" color="primary">{item.category}</Typography>
                  <Typography fontWeight={750}>{item.name}</Typography>
                  <Typography variant="caption" color="text.secondary">{item.remark || item.merchant_no || '尚未填写商户信息'}</Typography>
                </Box>
                <Chip size="small" color={statusColor(item.status)} label={statusLabel(item.status)} />
              </Stack>
              <Divider sx={{ my: 2 }} />
              <Stack direction="row" justifyContent="flex-end" gap={1} flexWrap="wrap">
                <Button size="small" variant="outlined" onClick={() => { setForm({ code: item.code, name: item.name, category: item.category, merchant_no: item.merchant_no, api_base: item.api_base, launch_path: item.launch_path ?? '/portal', secret_key: item.secret_key ?? '', status: item.status, remark: item.remark, sort_order: item.sort_order }); setOpen(true) }}>编辑配置</Button>
                {item.status !== 'enabled' && <Button size="small" variant="contained" onClick={() => void adminApi.setEntertainmentStatus(item.id, 'enabled').then(() => load()).then(() => showMessage('已启用'))}>启用</Button>}
                {item.status === 'enabled' && <Button size="small" onClick={() => void adminApi.setEntertainmentStatus(item.id, 'maintenance').then(() => load()).then(() => showMessage('已设为维护'))}>维护</Button>}
                {item.status !== 'disabled' && <Button size="small" color="inherit" onClick={() => void adminApi.setEntertainmentStatus(item.id, 'disabled').then(() => load()).then(() => showMessage('已停用'))}>停用</Button>}
              </Stack>
            </CardContent>
          </Card>
        ))}
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
            <TextField label="签名密钥" value={form.secret_key} onChange={e => setForm(current => ({ ...current, secret_key: e.target.value }))} helperText="用于生成 launch token" />
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
  const [campaignTitle, setCampaignTitle] = useState('')
  const [campaignRule, setCampaignRule] = useState('')
  const [grant, setGrant] = useState({ campaign_id: 0, resource_id: 0, user_id: 0 })
  const { showMessage } = useFeedback()

  const load = useCallback(async (notify = false) => {
    setLoading(true)
    setError('')
    try {
      const overview = await adminApi.specialOverview()
      setData(overview)
      if (overview.campaigns[0]) setGrant(current => ({ ...current, campaign_id: overview.campaigns[0].id }))
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

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="扩展服务 / 房间号"
        title="房间靓号"
        description="靓号即代理房间号。用户输入该号进入对应代理房间；发放时用户自动升为代理。"
        actions={<Button variant="outlined" startIcon={<RefreshRounded />} onClick={() => void load(true)}>刷新</Button>}
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
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: '1.1fr 1fr' }, gap: 1.5, mt: 1.5 }}>
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
        <Card>
          <CardContent>
            <Typography fontWeight={750} mb={1.5}>创建活动 / 发放给代理</Typography>
            <Stack gap={1.5}>
              <TextField label="活动标题" value={campaignTitle} onChange={e => setCampaignTitle(e.target.value)} />
              <TextField label="规则说明" value={campaignRule} onChange={e => setCampaignRule(e.target.value)} />
              <Button variant="contained" startIcon={<StarsRounded />} onClick={() => void adminApi.createSpecialCampaign({ title: campaignTitle, rule_text: campaignRule, status: 'active' }).then(() => { showMessage('活动已创建'); setCampaignTitle(''); setCampaignRule(''); return load() })}>创建活动</Button>
              <Divider />
              <TextField select label="活动" value={grant.campaign_id || ''} onChange={e => setGrant(current => ({ ...current, campaign_id: Number(e.target.value) }))}>
                {(data?.campaigns ?? []).map(item => <MenuItem key={item.id} value={item.id}>{item.title}</MenuItem>)}
              </TextField>
              <TextField select label="可用房间号" value={grant.resource_id || ''} onChange={e => setGrant(current => ({ ...current, resource_id: Number(e.target.value) }))}>
                {(data?.resources ?? []).filter(item => item.status === 'available').map(item => <MenuItem key={item.id} value={item.id}>{item.number}</MenuItem>)}
              </TextField>
              <TextField type="number" label="用户 ID（将升为代理）" value={grant.user_id || ''} onChange={e => setGrant(current => ({ ...current, user_id: Number(e.target.value) }))} />
              <Button variant="outlined" onClick={() => void adminApi.grantSpecialNumber(grant).then(() => { showMessage('已发放房间号并升为代理'); return load() })}>发放房间号</Button>
            </Stack>
            <Divider sx={{ my: 2 }} />
            <Typography fontWeight={750} mb={1}>活动列表</Typography>
            {(data?.campaigns ?? []).length === 0 ? <EmptyState message="暂无房间号活动" /> : (
              <Stack gap={1}>
                {data?.campaigns.map(item => (
                  <Stack key={item.id} direction="row" justifyContent="space-between">
                    <Box>
                      <Typography fontWeight={700}>{item.title}</Typography>
                      <Typography variant="caption" color="text.secondary">已发放 {item.granted_count}</Typography>
                    </Box>
                    <Chip size="small" color={statusColor(item.status)} label={statusLabel(item.status)} />
                  </Stack>
                ))}
              </Stack>
            )}
          </CardContent>
        </Card>
      </Box>
    </Box>
  )
}

export function ManagementPage({ path }: { path: string }) {
  if (path === '/activities') return <ActivitiesPage />
  if (path === '/entertainment') return <EntertainmentPage />
  if (path === '/special-numbers') return <SpecialPage />
  return <Box p={3}><EmptyState message="页面正在建设中" /></Box>
}
