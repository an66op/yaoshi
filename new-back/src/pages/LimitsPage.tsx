import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogContent,
  DialogTitle,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material'
import SaveRounded from '@mui/icons-material/SaveRounded'
import RestartAltRounded from '@mui/icons-material/RestartAltRounded'
import SyncRounded from '@mui/icons-material/SyncRounded'
import MenuBookRounded from '@mui/icons-material/MenuBookRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminApi, type AdminGame, type PlayCatalogItem, type PlayLimitItem } from '../api'
import { PageHeader } from '../components/PageHeader'
import { GameOddsNavigation, PlatformOddsGrid } from '../components/OddsEditors'
import { useFeedback } from '../components/feedback'

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
      setItems(Array.isArray(result?.items) ? result.items : [])
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
    if (!gameId) return
    const timer = window.setTimeout(() => void loadLimits(gameId), 0)
    return () => window.clearTimeout(timer)
  }, [gameId, loadLimits])

  const save = async () => {
    if (!gameId) return
    setSaving(true)
    setError('')
    try {
      const result = await adminApi.updateOddsLimits(gameId, items)
      setItems(Array.isArray(result?.items) ? result.items : [])
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
      setItems(Array.isArray(result?.items) ? result.items : [])
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
    <Box p={{ xs: 1.5, lg: 2 }}>
      <PageHeader
        eyebrow="游戏运营 / 风控"
        title="赔率与限额"
        description=""
        actions={
          <Button variant="contained" startIcon={<SaveRounded />} disabled={!gameId || loading || saving || Boolean(busy)} onClick={() => void save()}>{saving ? '保存中…' : '保存设置'}</Button>
        }
      />

      <Paper variant="outlined" sx={{ mt: 1.25, p: 1, borderRadius: 1.25 }}>
        <Stack direction={{ xs: 'column', lg: 'row' }} gap={.8} alignItems={{ lg: 'center' }} mb={.8}>
          <Box flex={1}>
            <Typography fontSize={13} fontWeight={900}>平台默认层</Typography>
            <Typography fontSize={10} color="text.secondary">房间赔率可覆盖平台值，会员单独赔率优先级最高。</Typography>
          </Box>
          <Stack direction="row" gap={.5} flexWrap="wrap" useFlexGap>
            <Button size="small" variant="text" startIcon={<MenuBookRounded />} onClick={() => setGuideOpen(true)}>玩法说明</Button>
            <Button size="small" variant="outlined" startIcon={<SyncRounded />} disabled={Boolean(busy) || saving} onClick={() => void syncAllGames()}>{busy === 'sync' ? '同步中…' : '补全彩种'}</Button>
            <Button size="small" variant="outlined" startIcon={<RestartAltRounded />} disabled={!gameId || Boolean(busy) || saving} onClick={() => void resetDefaults()}>{busy === 'reset' ? '恢复中…' : '恢复当前'}</Button>
          </Stack>
        </Stack>
        <GameOddsNavigation games={games.map(game => ({ ...game, enabled: true }))} gameId={gameId} onSelect={setGameId} />
        {currentGame && (
          <Stack direction="row" gap={.6} flexWrap="wrap" mt={.75}>
            <Chip size="small" color="primary" label={currentGame.name} sx={{ height: 22 }} />
            <Chip size="small" variant="outlined" label={`${items.length} 个玩法`} sx={{ height: 22 }} />
            <Chip size="small" variant="outlined" color={currentGame.enabled ? 'success' : 'default'} label={currentGame.enabled ? '运行中' : '已停用'} sx={{ height: 22 }} />
          </Stack>
        )}
      </Paper>
      {error && <Alert severity="error" sx={{ mt: 1.5 }}>{error}</Alert>}

      <Paper variant="outlined" sx={{ mt: 1, p: { xs: .75, md: 1 }, borderRadius: 1.25 }}>
        {loading ? <Box minHeight={220} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={24} /></Box>
          : items.length ? <PlatformOddsGrid items={items} catalog={catalogMap} onChange={setItems} />
            : <Stack minHeight={180} alignItems="center" justifyContent="center" color="text.secondary">暂无玩法配置，请选择游戏或补全默认玩法</Stack>}
      </Paper>

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
