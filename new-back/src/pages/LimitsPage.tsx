import {
  Alert,
  Box,
  Button,
  Card,
  CircularProgress,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
} from '@mui/material'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SaveRounded from '@mui/icons-material/SaveRounded'
import { useCallback, useEffect, useState } from 'react'
import { adminApi, type AdminGame, type PlayLimitItem } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

export function LimitsPage() {
  const [games, setGames] = useState<AdminGame[]>([])
  const [gameId, setGameId] = useState('')
  const [items, setItems] = useState<PlayLimitItem[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const { showMessage } = useFeedback()

  const loadGames = useCallback(async () => {
    const list = await adminApi.games()
    setGames(list)
    setGameId(current => current || list[0]?.id || '')
    return list
  }, [])

  const loadLimits = useCallback(async (id: string, notify = false) => {
    if (!id) {
      setItems([])
      return
    }
    setLoading(true)
    setError('')
    try {
      const result = await adminApi.oddsLimits(id)
      setItems(result.items)
      if (notify) showMessage(`${result.game_name} 赔率限额已刷新`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取赔率限额失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          setLoading(true)
          setError('')
          await loadGames()
        } catch (reason) {
          setError(reason instanceof Error ? reason.message : '读取游戏列表失败')
          setLoading(false)
        }
      })()
    }, 0)
    return () => window.clearTimeout(timer)
  }, [loadGames])

  useEffect(() => {
    if (!gameId) {
      setLoading(false)
      setItems([])
      return
    }
    const timer = window.setTimeout(() => void loadLimits(gameId), 0)
    return () => window.clearTimeout(timer)
  }, [gameId, loadLimits])

  const updateItem = (index: number, patch: Partial<PlayLimitItem>) => {
    setItems(current => current.map((item, i) => i === index ? { ...item, ...patch } : item))
  }

  const save = async () => {
    if (!gameId) return
    setSaving(true)
    setError('')
    try {
      const result = await adminApi.updateOddsLimits(gameId, items)
      setItems(result.items)
      showMessage(`${result.game_name} 赔率限额已保存`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存赔率限额失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="游戏运营 / 风控"
        title="赔率与限额"
        description="配置玩法赔率、单注限额和单期总限额。"
        actions={
          <>
            <TextField
              select
              size="small"
              label="彩种"
              value={gameId}
              onChange={event => setGameId(event.target.value)}
              sx={{ minWidth: 180 }}
              disabled={!games.length || loading || saving}
            >
              {games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}
            </TextField>
            <Button variant="outlined" startIcon={<RefreshRounded />} disabled={!gameId || loading || saving} onClick={() => void loadLimits(gameId, true)}>刷新</Button>
            <Button variant="contained" startIcon={<SaveRounded />} disabled={!gameId || loading || saving} onClick={() => void save()}>{saving ? '保存中…' : '保存设置'}</Button>
          </>
        }
      />
      <Alert severity="warning" sx={{ mt: 2.5 }}>修改赔率会直接影响前台展示，请确认后保存。用户详情中可配置单独赔率覆盖本页房间默认值。</Alert>
      {error && <Alert severity="error" sx={{ mt: 1.5 }}>{error}</Alert>}
      <Card sx={{ mt: 1.5 }}>
        {loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}
        <TableContainer>
          <Table size="small" sx={{ minWidth: 850 }}>
            <TableHead>
              <TableRow>
                {['玩法', '赔率', '单注最低', '单注最高', '个人单期最高', '单期总限额'].map(column => <TableCell key={column}>{column}</TableCell>)}
              </TableRow>
            </TableHead>
            <TableBody>
              {items.map((item, index) => (
                <TableRow key={item.play_code}>
                  <TableCell>{item.play_name}</TableCell>
                  <TableCell><TextField type="number" value={item.odds} onChange={event => updateItem(index, { odds: Number(event.target.value) })} sx={{ width: 90 }} inputProps={{ step: 0.001, min: 1.001 }} /></TableCell>
                  <TableCell><TextField type="number" value={item.min_bet} onChange={event => updateItem(index, { min_bet: Number(event.target.value) })} sx={{ width: 110 }} /></TableCell>
                  <TableCell><TextField type="number" value={item.max_bet} onChange={event => updateItem(index, { max_bet: Number(event.target.value) })} sx={{ width: 110 }} /></TableCell>
                  <TableCell><TextField type="number" value={item.max_user_period} onChange={event => updateItem(index, { max_user_period: Number(event.target.value) })} sx={{ width: 110 }} /></TableCell>
                  <TableCell><TextField type="number" value={item.max_period_total} onChange={event => updateItem(index, { max_period_total: Number(event.target.value) })} sx={{ width: 110 }} /></TableCell>
                </TableRow>
              ))}
              {!loading && !items.length && (
                <TableRow>
                  <TableCell colSpan={6}>
                    <Stack minHeight={180} alignItems="center" justifyContent="center" color="text.secondary">暂无赔率配置，请先选择彩种</Stack>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
      </Card>
    </Box>
  )
}
