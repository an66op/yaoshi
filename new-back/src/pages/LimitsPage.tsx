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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, type AdminGame, type OddsRiskWarning, type PlayCatalogItem, type PlayLimitItem } from '../api'
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
  const [rulesReady, setRulesReady] = useState(false)
  const [rulesMessage, setRulesMessage] = useState('')
  const [riskWarnings, setRiskWarnings] = useState<OddsRiskWarning[]>([])
  const loadSequence = useRef(0)
  const writeLocked = useRef(false)
  const { showMessage } = useFeedback()

  const catalogMap = useMemo(() => Object.fromEntries(catalog.map(item => [item.play_code, item])), [catalog])

  const loadGames = useCallback(async () => {
    const list = await adminApi.games()
    setGames(list)
    setGameId(current => current || list[0]?.id || '')
    return list
  }, [])

  const loadLimits = useCallback(async (id: string, notify = false) => {
    const sequence = ++loadSequence.current
    if (!id) {
      setItems([])
      setRiskWarnings([])
      return
    }
    setLoading(true)
    setError('')
    try {
      const [result, catalogItems] = await Promise.all([adminApi.oddsLimits(id), adminApi.playCatalog(id)])
      if (sequence !== loadSequence.current) return
      setItems(Array.isArray(result?.items) ? result.items : [])
      setCatalog(catalogItems)
      setRulesReady(result.rules_ready !== false && result.items.length > 0)
      setRulesMessage(result.rules_message || '')
      setRiskWarnings(result.risk_warnings || [])
      if (notify) showMessage(`${result.game_name} 赔率限额已刷新`)
    } catch (reason) {
      if (sequence !== loadSequence.current) return
      setError(reason instanceof Error ? reason.message : '读取赔率限额失败')
      setRulesReady(false)
      setRiskWarnings([])
    } finally {
      if (sequence === loadSequence.current) setLoading(false)
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
    if (!gameId) return
    const timer = window.setTimeout(() => void loadLimits(gameId), 0)
    return () => {
      window.clearTimeout(timer)
      // This ref is a request-generation counter, not a rendered DOM node.
      // eslint-disable-next-line react-hooks/exhaustive-deps
      loadSequence.current++
    }
  }, [gameId, loadLimits])

  const save = async () => {
    if (writeLocked.current || !gameId || !rulesReady || loading || saving || busy) return
    writeLocked.current = true
    setSaving(true)
    setError('')
    try {
      const result = await adminApi.updateOddsLimits(gameId, items)
      setItems(Array.isArray(result?.items) ? result.items : [])
      setRiskWarnings(result.risk_warnings || [])
      showMessage(`${result.game_name} 赔率限额已保存`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存赔率限额失败')
    } finally {
      writeLocked.current = false
      setSaving(false)
    }
  }

  const resetDefaults = async () => {
    if (writeLocked.current || !gameId || !rulesReady || loading || saving || busy || !window.confirm('将当前彩种恢复为系统默认玩法与赔率，自定义修改会丢失。继续？')) return
    writeLocked.current = true
    setBusy('reset')
    setError('')
    try {
      const result = await adminApi.resetOddsLimits(gameId)
      setItems(Array.isArray(result?.items) ? result.items : [])
      setRiskWarnings(result.risk_warnings || [])
      showMessage(`${result.game_name} 已恢复默认玩法`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '恢复默认失败')
    } finally {
      writeLocked.current = false
      setBusy(null)
    }
  }

  const syncAllGames = async () => {
    if (writeLocked.current || saving || busy) return
    writeLocked.current = true
    setBusy('sync')
    setError('')
    try {
      const result = await adminApi.syncOddsLimits()
      showMessage(result.seeded_games.length
        ? `已为 ${result.seeded_games.length} 个彩种补全默认玩法（共 ${result.game_count} 个彩种）`
        : '当前已建模彩种的默认玩法已配置；待核对彩种保持暂停受理')
      if (gameId) await loadLimits(gameId)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '同步玩法失败')
    } finally {
      writeLocked.current = false
      setBusy(null)
    }
  }

  const currentGame = games.find(game => game.id === gameId)
  const selectGame = (id: string) => {
    if (writeLocked.current || saving || busy || id === gameId) return
    loadSequence.current++
    setLoading(true)
    setItems([])
    setCatalog([])
    setRulesReady(false)
    setRulesMessage('')
    setRiskWarnings([])
    setGameId(id)
  }

  return (
    <Box p={{ xs: 1.5, lg: 2 }}>
      <PageHeader
        eyebrow="游戏运营 / 风控"
        title="赔率与限额"
        description=""
        actions={
          <Button variant="contained" startIcon={<SaveRounded />} disabled={!gameId || !rulesReady || loading || saving || Boolean(busy)} onClick={() => void save()}>{saving ? '保存中…' : '保存设置'}</Button>
        }
      />

      <Paper variant="outlined" sx={{ mt: 1.25, p: 1, borderRadius: 1.25 }}>
        <Stack direction={{ xs: 'column', lg: 'row' }} gap={.8} alignItems={{ lg: 'center' }} mb={.8}>
          <Box flex={1}>
            <Typography fontSize={13} fontWeight={900}>平台默认层</Typography>
            <Typography fontSize={10} color="text.secondary">平台赔率为0表示关闭玩法；只有平台已启用的玩法才能按房间、会员顺序覆盖。</Typography>
          </Box>
          <Stack direction="row" gap={.5} flexWrap="wrap" useFlexGap>
            <Button size="small" variant="text" startIcon={<MenuBookRounded />} disabled={loading || !catalog.length} onClick={() => setGuideOpen(true)}>玩法说明</Button>
            <Button size="small" variant="outlined" startIcon={<SyncRounded />} disabled={Boolean(busy) || saving} onClick={() => void syncAllGames()}>{busy === 'sync' ? '同步中…' : '补全彩种'}</Button>
            <Button size="small" variant="outlined" startIcon={<RestartAltRounded />} disabled={!gameId || !rulesReady || loading || Boolean(busy) || saving} onClick={() => void resetDefaults()}>{busy === 'reset' ? '恢复中…' : '恢复当前'}</Button>
          </Stack>
        </Stack>
        <GameOddsNavigation games={games.map(game => ({ ...game, enabled: true }))} gameId={gameId} onSelect={selectGame} />
        {currentGame && (
          <Stack direction="row" gap={.6} flexWrap="wrap" mt={.75}>
            <Chip size="small" color="primary" label={currentGame.name} sx={{ height: 22 }} />
            <Chip size="small" variant="outlined" label={`${items.length} 个玩法`} sx={{ height: 22 }} />
            <Chip size="small" variant="outlined" color={rulesReady && currentGame.enabled ? 'success' : 'default'} label={loading ? '加载中' : !rulesReady ? '玩法待配置' : currentGame.enabled ? '运行中' : '已停用'} sx={{ height: 22 }} />
          </Stack>
        )}
      </Paper>
      {error && <Alert severity="error" sx={{ mt: 1.5 }}>{error}</Alert>}
      {!loading && rulesMessage && <Alert severity="warning" sx={{ mt: 1.5 }}>{rulesMessage}。现有赔率配置保留，确认专属规则后再开放。</Alert>}
      {!loading && riskWarnings.map(warning => <Alert key={warning.code} severity="warning" sx={{ mt: 1.5 }}>
        <Typography component="div" fontWeight={700} fontSize={13}>已保存配置 · 赔率风险</Typography>
        {warning.message}
      </Alert>)}

      <Paper variant="outlined" sx={{ mt: 1, p: { xs: .75, md: 1 }, borderRadius: 1.25 }}>
        {loading ? <Box minHeight={220} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={24} /></Box>
          : items.length ? <Box sx={{ pointerEvents: !rulesReady || saving || busy ? 'none' : 'auto', opacity: !rulesReady || saving || busy ? .6 : 1 }}><PlatformOddsGrid items={items} catalog={catalogMap} onChange={value => { if (!writeLocked.current && rulesReady && !loading && !saving && !busy) setItems(value) }} /></Box>
            : <Stack minHeight={180} alignItems="center" justifyContent="center" color="text.secondary">{rulesMessage ? '此彩种的专属玩法待核对，当前不提供投注赔率' : '暂无玩法配置，请选择游戏或补全默认玩法'}</Stack>}
      </Paper>

      <Dialog open={guideOpen} onClose={() => setGuideOpen(false)} fullWidth maxWidth="md">
        <DialogTitle>{currentGame?.name || '当前彩种'} · 玩法说明</DialogTitle>
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
