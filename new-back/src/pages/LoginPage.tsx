import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Stack,
  TextField,
  Typography,
} from '@mui/material'
import { useEffect, useRef, useState } from 'react'
import { adminApi, type ManagementLoginRole } from '../api'
import { clearLegacyAdminSession, type AuthUser } from '../auth'
import { createDevLoginPresets, type DevLoginPreset, type LoginIdentity } from '../devLoginPresets'
import {
  MANAGEMENT_LOGIN_PASSWORD_MAX_BYTES,
  MANAGEMENT_LOGIN_USERNAME_MAX_RUNES,
  truncateCodePoints,
  validateManagementLoginInput,
} from '../loginLimits'
import { loadTestLogin, type ManagementTestLogins } from '../utils/testLogin'
import { LOGIN_CAPTCHA_LENGTH, useLoginCaptcha } from '../hooks/useLoginCaptcha'

const identityOptions: Array<{
  id: LoginIdentity
  code: string
  label: string
}> = [
  { id: 'platform', code: 'admin', label: '平台管理员' },
  { id: 'tenant', code: 'tenant', label: '租户' },
  { id: 'agent', code: 'agent', label: '代理' },
]

const identityFromCode = (value: string): LoginIdentity | null => {
  const code = value.trim().toLowerCase()
  if (code === 'admin' || code === 'platform') return 'platform'
  if (code === 'tenant') return 'tenant'
  if (code === 'agent') return 'agent'
  return null
}

const codeForIdentity = (identity: LoginIdentity) => identityOptions.find(item => item.id === identity)?.code ?? ''
const roleForIdentity = (identity: LoginIdentity): ManagementLoginRole => identity === 'platform' ? 'admin' : identity

const localPresets: Partial<Record<LoginIdentity, DevLoginPreset>> = import.meta.env.DEV
  ? createDevLoginPresets(true, import.meta.env)
  : {}

const localPreset = (identity: LoginIdentity) => localPresets[identity]

export function LoginPage({ onSuccess }: { onSuccess: (user: AuthUser) => void }) {
  const [username, setUsername] = useState(() => import.meta.env.DEV ? localPreset('platform')?.username ?? '' : '')
  const [password, setPassword] = useState(() => import.meta.env.DEV ? localPreset('platform')?.password ?? '' : '')
  const [identity, setIdentity] = useState<LoginIdentity>('platform')
  const [identityCode, setIdentityCode] = useState(() => codeForIdentity('platform'))
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [testPresets, setTestPresets] = useState<ManagementTestLogins>({})
  const [testPrefilled, setTestPrefilled] = useState(false)
  const edited = useRef(false)
  const selectedIdentity = useRef<LoginIdentity>('platform')
  const submitting = useRef(false)
  const { captcha, code: captchaCode, setCode: setCaptchaCode, refresh: refreshCaptcha, imageLoaded, imageFailed, takeSubmission, mounted } = useLoginCaptcha()

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
    if (submitting.current) return
    const option = identityOptions.find(item => item.id === next)
    if (!option) return
    selectedIdentity.current = next
    edited.current = false
    setIdentity(next)
    setIdentityCode(codeForIdentity(next))
    const preset = import.meta.env.DEV ? localPreset(next) : testPresets[next]
    setUsername(preset?.username ?? '')
    setPassword(preset?.password ?? '')
    setTestPrefilled(!import.meta.env.DEV && !!preset)
    setCaptchaCode('')
    setError('')
  }

  const changeCaptcha = () => {
    if (submitting.current) return
    void refreshCaptcha()
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (submitting.current) return
    const requestedIdentity = identityFromCode(identityCode)
    if (!requestedIdentity) {
      setError('登录身份必须是 admin、tenant 或 agent')
      return
    }
    const validationError = validateManagementLoginInput(username, password)
    if (validationError) {
      setError(validationError)
      return
    }
    let proof: { captcha_id: string; captcha_code: string }
    try { proof = takeSubmission() }
    catch (reason) {
      const message = reason instanceof Error ? reason.message : '请填写验证码'
      setError(message === '验证码已过期，已自动刷新' ? '' : message)
      return
    }
    submitting.current = true
    setLoading(true)
    setError('')
    try {
      // The server matches this role against the authenticated account. Local
      // shortcuts only fill credentials; they never bypass that role check.
      const result = await adminApi.login(roleForIdentity(requestedIdentity), username.trim(), password, proof)
	  if (!mounted.current) return
	  clearLegacyAdminSession()
      onSuccess(result.user)
    } catch (reason) {
      if (!mounted.current) return
      setError(reason instanceof Error ? reason.message : '登录失败')
      // Every attempted login consumes its challenge, including bad passwords
      // and uncertain network failures. Preserve credentials, not the answer.
      void refreshCaptcha()
    } finally {
      submitting.current = false
      if (mounted.current) setLoading(false)
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
            <Typography variant="body2" color="text.secondary">输入身份代号后填写管理账号</Typography>
          </Stack>
          {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
          {testPrefilled && <Alert severity="info" sx={{ mb: 2 }}>测试环境 · 已填充体验账号<Typography component="span" display="block" variant="caption">体验账号公开，仅用于测试，请勿录入真实业务资料。</Typography></Alert>}
          <Stack component="form" gap={2} onSubmit={event => void submit(event)}>
            <TextField
              label="登录身份"
              value={identityCode}
              disabled={loading}
              onChange={event => {
                const nextCode = event.target.value.replace(/\s/g, '').toLowerCase().slice(0, 16)
                setIdentityCode(nextCode)
                const nextIdentity = identityFromCode(nextCode)
                if (nextIdentity) chooseIdentity(nextIdentity)
                else setError('')
              }}
              autoComplete="off"
              required
              slotProps={{ htmlInput: { inputMode: 'text', spellCheck: false } }}
            />
            {import.meta.env.DEV && <Stack direction="row" gap={1}>
              {identityOptions.map(option => <Button
                key={option.id}
                type="button"
                size="small"
                fullWidth
                disabled={loading}
                variant={identity === option.id && identityCode === option.code ? 'contained' : 'outlined'}
                aria-label={`快捷切换 ${option.code}`}
                onClick={() => chooseIdentity(option.id)}
              >{option.code}</Button>)}
            </Stack>}
            <TextField label="登录帐号" value={username} disabled={loading} onChange={event => { markEdited(); setUsername(truncateCodePoints(event.target.value, MANAGEMENT_LOGIN_USERNAME_MAX_RUNES)) }} autoComplete="username" required />
            <TextField label="密码" type="password" value={password} disabled={loading} onChange={event => { markEdited(); setPassword(event.target.value) }} autoComplete="current-password" slotProps={{ htmlInput: { maxLength: MANAGEMENT_LOGIN_PASSWORD_MAX_BYTES } }} required />
            <Box>
              <Stack direction="row" alignItems="center" gap={1}>
                <TextField label="验证码" size="small" autoComplete="off" value={captchaCode} disabled={loading || captcha.status !== 'ready'} onChange={event => setCaptchaCode(event.target.value.replace(/\D/g, '').slice(0, LOGIN_CAPTCHA_LENGTH))} slotProps={{ htmlInput: { inputMode: 'numeric', pattern: `[0-9]{${LOGIN_CAPTCHA_LENGTH}}`, maxLength: LOGIN_CAPTCHA_LENGTH, 'aria-describedby': 'management-captcha-hint' } }} required sx={{ flex: 1, minWidth: 0 }} />
                <Button type="button" variant="outlined" aria-label="更换登录验证码" aria-busy={captcha.status === 'loading' || captcha.status === 'image'} disabled={loading} onClick={changeCaptcha} sx={{ width: 132, minWidth: 132, height: 44, p: 0, overflow: 'hidden', bgcolor: 'background.paper' }}>
                  {captcha.challenge ? <Box component="img" key={captcha.challenge.requestID} src={captcha.challenge.image} alt="登录验证码" draggable={false} onLoad={() => imageLoaded(captcha.challenge!.requestID)} onError={() => imageFailed(captcha.challenge!.requestID)} sx={{ width: '100%', height: '100%', objectFit: 'contain', opacity: captcha.status === 'used' ? .45 : 1 }} />
                    : captcha.status === 'loading' ? <CircularProgress size={20} /> : '点击重试'}
                </Button>
              </Stack>
              <Stack direction="row" alignItems="center" justifyContent="space-between" gap={.75} mt={.25}>
                <Typography id="management-captcha-hint" variant="caption" aria-live="polite" color={captcha.status === 'error' ? 'error.main' : 'text.secondary'}>{captcha.message || `请输入图中${LOGIN_CAPTCHA_LENGTH}位数字`}</Typography>
                <Button type="button" size="small" disabled={loading} onClick={changeCaptcha} sx={{ px: 0, whiteSpace: 'nowrap', flexShrink: 0 }}>看不清？换一张</Button>
              </Stack>
            </Box>
            <Button type="submit" variant="contained" size="large" disabled={loading || !identityFromCode(identityCode) || !username.trim() || !password || captcha.status !== 'ready' || !new RegExp(`^\\d{${LOGIN_CAPTCHA_LENGTH}}$`).test(captchaCode)}>
              {loading ? <CircularProgress size={22} color="inherit" /> : `登录${identityOptions.find(item => item.id === identity)?.label ?? '管理后台'}`}
            </Button>
          </Stack>
        </CardContent>
      </Card>
    </Box>
  )
}
