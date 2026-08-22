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
import { useState } from 'react'
import { adminApi } from '../api'
import { saveSession, type AuthUser } from '../auth'

export function LoginPage({ onSuccess }: { onSuccess: (user: AuthUser) => void }) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('123456')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setLoading(true)
    setError('')
    try {
      const result = await adminApi.login(username.trim(), password)
      saveSession({ token: result.token, user: result.user })
      onSuccess(result.user)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Box minHeight="100vh" display="grid" sx={{ placeItems: 'center', px: 2, background: 'radial-gradient(circle at 20% 20%, #d7f3ef, transparent 36%), radial-gradient(circle at 80% 0%, #c7e4f4, transparent 40%), linear-gradient(160deg, #f4f8fb, #e8f1f6)' }}>
      <Card sx={{ width: '100%', maxWidth: 420, borderRadius: 3 }}>
        <CardContent sx={{ p: { xs: 3, sm: 4 } }}>
          <Stack alignItems="center" gap={1} mb={3}>
            <Box sx={{ width: 52, height: 52, borderRadius: 2.5, display: 'grid', placeItems: 'center', color: '#fff', fontWeight: 900, fontSize: 22, background: 'linear-gradient(145deg,#1684ad,#29bdb0)' }}>曜</Box>
            <Typography variant="h5" fontWeight={850}>曜图管理中心</Typography>
            <Typography variant="body2" color="text.secondary">请使用管理员账号登录</Typography>
          </Stack>
          {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
          <Stack component="form" gap={2} onSubmit={event => void submit(event)}>
            <TextField label="用户名" value={username} onChange={event => setUsername(event.target.value)} autoComplete="username" required />
            <TextField label="密码" type="password" value={password} onChange={event => setPassword(event.target.value)} autoComplete="current-password" required />
            <Button type="submit" variant="contained" size="large" disabled={loading || !username.trim() || !password}>
              {loading ? <CircularProgress size={22} color="inherit" /> : '登录'}
            </Button>
          </Stack>
          <Typography variant="caption" color="text.secondary" display="block" textAlign="center" mt={2}>
            默认账号 admin / 123456
          </Typography>
        </CardContent>
      </Card>
    </Box>
  )
}
