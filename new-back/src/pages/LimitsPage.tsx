import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  Dialog,
  DialogContent,
  DialogTitle,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import SaveRounded from '@mui/icons-material/SaveRounded'
import RestartAltRounded from '@mui/icons-material/RestartAltRounded'
import SyncRounded from '@mui/icons-material/SyncRounded'
import MenuBookRounded from '@mui/icons-material/MenuBookRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, type AdminGame, type PlayCatalogItem, type PlayLimitItem } from '../api'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'

const categoryColor: Record<string, 'default' | 'primary' | 'secondary' | 'success' | 'warning' | 'info'> = {
  两面盘: 'primary',
  号码: 'success',
  龙虎: 'warning',
  总和: 'info',
  形态: 'secondary',
}

export function LimitsPage() {
  const [games, setGames] = useState<AdminGame[]>([])
  const [gameId, setGameId] = useState('')
  const [items, setItems] = useState<PlayLimitItem[]>([])
  const [catalog, setCatalog] = useState<PlayCatalogItem[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [busy, setBusy] = useState<'reset' | 'sync' | null>(null)
  const [error, setError] = useState('')
  const [guideOpen, setGuideOpen] = useState(false)
  const { showMessage } = useFeedback()

  const catalogMap = useMemo(() => Object.fromEntries(catalog.map(item => [item.play_code, item])), [catalog])

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
          const [_, catalogItems] = await Promise.all([loadGames(), adminApi.playCatalog()])
          setCatalog(catalogItems)
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

  const resetDefaults = async () => {
    if (!gameId || !window.confirm('将当前彩种恢复为系统默认玩法与赔率，自定义修改会丢失。继续？')) return
    setBusy('reset')
    setError('')
    try {
      const result = await adminApi.resetOddsLimits(gameId)
      setItems(result.items)
      showMessage(`${result.game_name} 已恢复默认玩法`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '恢复默认失败')
    } finally {
      setBusy(null)
    }
  }

  const syncAllGames = async () => {
    setBusy('sync')
    setError('')
    try {
      const result = await adminApi.syncOddsLimits()
      showMessage(result.seeded_games.length
        ? `已为 ${result.seeded_games.length} 个彩种补全默认玩法（共 ${result.game_count} 个彩种）`
        : `全部 ${result.game_count} 个彩种已有玩法配置`)
      if (gameId) await loadLimits(gameId)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '同步玩法失败')
    } finally {
      setBusy(null)
    }
  }

  const currentGame = games.find(game => game.id === gameId)

  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="游戏运营 / 风控"
        title="赔率与限额"
        description="配置各彩种玩法赔率、单注与单期限额；玩法编号需与前台解析和结算逻辑一致。"
        actions={
          <>
            <TextField
              select
              size="small"
              label="彩种"
              value={gameId}
              onChange={event => setGameId(event.target.value)}
              sx={{ minWidth: 180 }}
              disabled={!games.length || loading || saving || Boolean(busy)}
            >
              {games.map(game => <MenuItem key={game.id} value={game.id}>{game.name}</MenuItem>)}
            </TextField>
            <Button variant="outlined" startIcon={<MenuBookRounded />} onClick={() => setGuideOpen(true)}>玩法说明</Button>
            <Button variant="outlined" startIcon={<SyncRounded />} disabled={Boolean(busy) || saving} onClick={() => void syncAllGames()}>{busy === 'sync' ? '同步中…' : '补全全部彩种'}</Button>
            <Button variant="outlined" startIcon={<RestartAltRounded />} disabled={!gameId || Boolean(busy) || saving} onClick={() => void resetDefaults()}>{busy === 'reset' ? '恢复中…' : '恢复默认'}</Button>
            <Button variant="outlined" startIcon={<RefreshRounded />} disabled={!gameId || loading || saving || Boolean(busy)} onClick={() => void loadLimits(gameId, true)}>刷新</Button>
            <Button variant="contained" startIcon={<SaveRounded />} disabled={!gameId || loading || saving || Boolean(busy)} onClick={() => void save()}>{saving ? '保存中…' : '保存设置'}</Button>
          </>
        }
      />

      <Alert severity="info" sx={{ mt: 2.5 }}>
        前台快捷输入示例：<b>1大/100</b>、<b>3/7/100</b>、<b>冠亚和小/50</b>、<b>12345/200</b>。
        用户详情可配置单独赔率覆盖本页房间默认值。
      </Alert>
      {currentGame && (
        <Stack direction="row" gap={1} flexWrap="wrap" mt={1.5}>
          <Chip size="small" label={`当前：${currentGame.name}`} />
          <Chip size="small" variant="outlined" label={`${items.length} 个玩法`} />
          <Chip size="small" variant="outlined" color={currentGame.enabled ? 'success' : 'default'} label={currentGame.enabled ? '运行中' : '已停用'} />
        </Stack>
      )}
      {error && <Alert severity="error" sx={{ mt: 1.5 }}>{error}</Alert>}

      <Card sx={{ mt: 1.5 }}>
        {loading && <Box px={2} py={1}><CircularProgress size={18} /></Box>}
        <TableContainer>
          <Table size="small" sx={{ minWidth: 980 }}>
            <TableHead>
              <TableRow>
                {['玩法编号', '玩法名称', '分类', '说明 / 示例', '赔率', '单注最低', '单注最高', '个人单期最高', '单期总限额'].map(column => (
                  <TableCell key={column}>{column}</TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {items.map((item, index) => {
                const meta = catalogMap[item.play_code]
                return (
                  <TableRow key={item.play_code}>
                    <TableCell>
                      <Typography fontFamily="monospace" fontSize={11}>{item.play_code}</Typography>
                    </TableCell>
                    <TableCell>{item.play_name}</TableCell>
                    <TableCell>
                      {meta?.category
                        ? <Chip size="small" label={meta.category} color={categoryColor[meta.category] ?? 'default'} />
                        : '—'}
                    </TableCell>
                    <TableCell sx={{ maxWidth: 260 }}>
                      <Typography fontSize={11} color="text.secondary">{meta?.description ?? '—'}</Typography>
                      {meta?.example && <Typography fontSize={10} color="text.disabled" mt={0.5}>例：{meta.example}</Typography>}
                    </TableCell>
                    <TableCell>
                      <TextField type="number" value={item.odds} onChange={event => updateItem(index, { odds: Number(event.target.value) })} sx={{ width: 90 }} inputProps={{ step: 0.001, min: 1.001 }} />
                      {meta && Math.abs(item.odds - meta.default_odds) > 0.001 && (
                        <Typography fontSize={10} color="warning.main">默认 {meta.default_odds}</Typography>
                      )}
                    </TableCell>
                    <TableCell><TextField type="number" value={item.min_bet} onChange={event => updateItem(index, { min_bet: Number(event.target.value) })} sx={{ width: 100 }} /></TableCell>
                    <TableCell><TextField type="number" value={item.max_bet} onChange={event => updateItem(index, { max_bet: Number(event.target.value) })} sx={{ width: 100 }} /></TableCell>
                    <TableCell><TextField type="number" value={item.max_user_period} onChange={event => updateItem(index, { max_user_period: Number(event.target.value) })} sx={{ width: 100 }} /></TableCell>
                    <TableCell><TextField type="number" value={item.max_period_total} onChange={event => updateItem(index, { max_period_total: Number(event.target.value) })} sx={{ width: 100 }} /></TableCell>
                  </TableRow>
                )
              })}
              {!loading && !items.length && (
                <TableRow>
                  <TableCell colSpan={9}>
                    <Stack minHeight={180} alignItems="center" justifyContent="center" color="text.secondary">
                      暂无玩法配置，请选择彩种或点击「补全全部彩种」
                    </Stack>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
      </Card>

      <Dialog open={guideOpen} onClose={() => setGuideOpen(false)} fullWidth maxWidth="md">
        <DialogTitle>系统支持的玩法</DialogTitle>
        <DialogContent dividers>
          <Table size="small">
            <TableHead>
              <TableRow>
                {['编号', '名称', '分类', '说明', '前台示例', '默认赔率'].map(column => <TableCell key={column}>{column}</TableCell>)}
              </TableRow>
            </TableHead>
            <TableBody>
              {catalog.map(item => (
                <TableRow key={item.play_code}>
                  <TableCell><Typography fontFamily="monospace" fontSize={11}>{item.play_code}</Typography></TableCell>
                  <TableCell>{item.play_name}</TableCell>
                  <TableCell>{item.category}</TableCell>
                  <TableCell>{item.description}</TableCell>
                  <TableCell>{item.example}</TableCell>
                  <TableCell>{item.default_odds}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>
    </Box>
  )
}
