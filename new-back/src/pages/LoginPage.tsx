import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material'
import AccountBalanceRounded from '@mui/icons-material/AccountBalanceRounded'
import BusinessRounded from '@mui/icons-material/BusinessRounded'
import StorefrontRounded from '@mui/icons-material/StorefrontRounded'
import { useEffect, useRef, useState } from 'react'
import { adminApi } from '../api'
import { clearLegacyAdminSession, type AuthUser } from '../auth'
import { createDevLoginPresets, type DevLoginPreset, type LoginIdentity } from '../devLoginPresets'
import {
  MANAGEMENT_LOGIN_PASSWORD_MAX_BYTES,
  MANAGEMENT_LOGIN_USERNAME_MAX_RUNES,
  MANAGEMENT_LOGIN_WORKSPACE_MAX_RUNES,
  truncateCodePoints,
  validateManagementLoginInput,
} from '../loginLimits'
import { loadTestLogin, type ManagementTestLogins } from '../utils/testLogin'

const identityOptions: Array<{
  id: LoginIdentity
  label: string
  caption: string
  workspace: string
  icon: typeof AccountBalanceRounded
}> = [
  { id: 'platform', label: '平台管理员', caption: '管理全平台', workspace: '平台', icon: AccountBalanceRounded },
  { id: 'tenant', label: '租户', caption: '开通并管理房间', workspace: '平台', icon: BusinessRounded },
  { id: 'agent', label: '代理', caption: '管理所属房间', workspace: '', icon: StorefrontRounded },
]

const localPresets: Partial<Record<LoginIdentity, DevLoginPreset>> = import.meta.env.DEV
  ? createDevLoginPresets(true, import.meta.env)
  : {}

const localPreset = (identity: LoginIdentity) => localPresets[identity]

export function LoginPage({ onSuccess }: { onSuccess: (user: AuthUser) => void }) {
  const [username, setUsername] = useState(() => import.meta.env.DEV ? localPreset('platform')?.username ?? '' : '')
  const [password, setPassword] = useState(() => import.meta.env.DEV ? localPreset('platform')?.password ?? '' : '')
  const [workspace, setWorkspace] = useState(() => import.meta.env.DEV ? localPreset('platform')?.workspace ?? '平台' : '平台')
  const [identity, setIdentity] = useState<LoginIdentity>('platform')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [testPresets, setTestPresets] = useState<ManagementTestLogins>({})
  const [testPrefilled, setTestPrefilled] = useState(false)
  const edited = useRef(false)
  const selectedIdentity = useRef<LoginIdentity>('platform')

  useEffect(() => {
    if (import.meta.env.DEV) return
    const request = new AbortController()
    void loadTestLogin(request.signal).then(presets => {
      if (!presets || request.signal.aborted) return
      setTestPresets(presets)
      const preset = presets[selectedIdentity.current]
      if (!preset || edited.current) return
      setUsername(preset.username)
      setPassword(preset.password)
      setWorkspace(preset.workspace)
      setTestPrefilled(true)
    })
    return () => request.abort()
  }, [])

  const markEdited = () => {
    edited.current = true
    setTestPrefilled(false)
    setError('')
  }

  const chooseIdentity = (next: LoginIdentity) => {
    const option = identityOptions.find(item => item.id === next)
    if (!option) return
    selectedIdentity.current = next
    edited.current = false
    setIdentity(next)
    const preset = import.meta.env.DEV ? localPreset(next) : testPresets[next]
    setWorkspace(preset?.workspace ?? option.workspace)
    setUsername(preset?.username ?? '')
    setPassword(preset?.password ?? '')
    setTestPrefilled(!import.meta.env.DEV && !!preset)
    setError('')
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    const validationError = validateManagementLoginInput(username, password, workspace)
    if (validationError) {
      setError(validationError)
      return
    }
    setLoading(true)
    setError('')
    try {
      const result = await adminApi.login(username.trim(), password, workspace.trim())
	  clearLegacyAdminSession()
      onSuccess(result.user)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Box minHeight="100vh" display="grid" sx={{
      placeItems: 'center',
      px: 2,
      background: theme => theme.palette.mode === 'dark'
        ? 'radial-gradient(circle at 16% 18%, rgba(20,130,148,.26), transparent 34%), radial-gradient(circle at 86% 4%, rgba(52,87,151,.24), transparent 38%), radial-gradient(circle at 52% 100%, rgba(21,105,108,.16), transparent 42%), linear-gradient(155deg, #04111f 0%, #071a2e 48%, #09243a 100%)'
        : 'radial-gradient(circle at 20% 20%, #d7f3ef, transparent 36%), radial-gradient(circle at 80% 0%, #c7e4f4, transparent 40%), linear-gradient(160deg, #f4f8fb, #e8f1f6)',
    }}>
      <Card sx={{
        width: '100%',
        maxWidth: 420,
        borderRadius: 3,
        boxShadow: theme => theme.palette.mode === 'dark' ? '0 24px 70px rgba(0,0,0,.48), 0 0 0 1px rgba(85,199,199,.06)' : undefined,
      }}>
        <CardContent sx={{ p: { xs: 3, sm: 4 } }}>
          <Stack alignItems="center" gap={1} mb={3}>
            <Box sx={{ width: 52, height: 52, borderRadius: 2.5, display: 'grid', placeItems: 'center', color: '#fff', fontWeight: 900, fontSize: 22, background: 'linear-gradient(145deg,#1684ad,#29bdb0)' }}>王</Box>
            <Typography variant="h5" fontWeight={850}>王者管理中心</Typography>
            <Typography variant="body2" color="text.secondary">{import.meta.env.DEV ? '选择身份后自动填充本地体验账号' : '请选择身份并输入管理账号'}</Typography>
          </Stack>
          {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
          {testPrefilled && <Alert severity="info" sx={{ mb: 2 }}>测试环境 · 已填充体验账号<Typography component="span" display="block" variant="caption">体验账号公开，仅用于测试，请勿录入真实业务资料。</Typography></Alert>}
          <Stack component="form" gap={2} onSubmit={event => void submit(event)}>
            <Box>
              <Typography component="label" fontSize={13} fontWeight={800} color="text.secondary" display="block" mb={1}>登录身份</Typography>
              <ToggleButtonGroup
                exclusive
                fullWidth
                value={identity}
                onChange={(_, next: LoginIdentity | null) => { if (next) chooseIdentity(next) }}
                aria-label="选择登录身份"
                sx={{
                  gap: 1,
                  '& .MuiToggleButtonGroup-grouped': { border: '1px solid', borderColor: 'divider', borderRadius: '12px !important', m: 0 },
                }}
              >
                {identityOptions.map(option => {
                  const IdentityIcon = option.icon
                  return <ToggleButton key={option.id} value={option.id} aria-label={option.label} sx={{ py: 1.2, px: .75, textTransform: 'none' }}>
                    <Stack alignItems="center" gap={.4} minWidth={0}>
                      <IdentityIcon fontSize="small" />
                      <Typography fontSize={12.5} fontWeight={850} noWrap>{option.label}</Typography>
                      <Typography fontSize={10.5} color="text.secondary" noWrap>{option.caption}</Typography>
                    </Stack>
                  </ToggleButton>
                })}
              </ToggleButtonGroup>
            </Box>
            <TextField
              label={identity === 'agent' ? '所属租户' : '所属平台'}
              helperText={identity === 'agent' ? '填写所属租户账号；未分配租户时填写“平台”' : '平台管理员和租户统一归属平台'}
              value={workspace}
              onChange={event => { markEdited(); setWorkspace(truncateCodePoints(event.target.value, MANAGEMENT_LOGIN_WORKSPACE_MAX_RUNES)) }}
              autoComplete="organization"
              disabled={identity !== 'agent'}
              required
            />
            <TextField label="登录帐号" value={username} onChange={event => { markEdited(); setUsername(truncateCodePoints(event.target.value, MANAGEMENT_LOGIN_USERNAME_MAX_RUNES)) }} autoComplete="username" required />
            <TextField label="密码" type="password" value={password} onChange={event => { markEdited(); setPassword(event.target.value) }} autoComplete="current-password" slotProps={{ htmlInput: { maxLength: MANAGEMENT_LOGIN_PASSWORD_MAX_BYTES } }} required />
            <Button type="submit" variant="contained" size="large" disabled={loading || !username.trim() || !password}>
              {loading ? <CircularProgress size={22} color="inherit" /> : `登录${identityOptions.find(item => item.id === identity)?.label ?? ''}`}
            </Button>
          </Stack>
          {import.meta.env.DEV && <Typography mt={2} display="block" textAlign="center" variant="caption" color="text.secondary">身份按钮仅填充本地体验账号，不影响正式账号登录</Typography>}
        </CardContent>
      </Card>
    </Box>
  )
}
