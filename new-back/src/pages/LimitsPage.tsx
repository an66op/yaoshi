import { Alert, Box, Button, Chip, CircularProgress, Dialog, DialogContent, DialogTitle, Paper, Stack, Table, TableBody, TableCell, TableHead, TableRow, Typography } from '@mui/material'
import SaveRounded from '@mui/icons-material/SaveRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import MenuBookRounded from '@mui/icons-material/MenuBookRounded'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, type AdminGame, type GameOddsLimits, type OddsMutationGuard, type OddsRiskWarning, type PlayCatalogItem, type PlayLimitItem } from '../api'
import { PageHeader } from '../components/PageHeader'
import { GameOddsNavigation, PlatformOddsGrid } from '../components/OddsEditors'
import { useFeedback } from '../components/feedback'
import { isOddsConfigurationConflict, oddsDirtyCodes, oddsDraftItems, validateOddsDraft } from '../oddsEditing'

export function LimitsPage() {
  const [games, setGames] = useState<AdminGame[]>([])
  const [gameId, setGameId] = useState('')
  const [items, setItems] = useState<PlayLimitItem[]>([])
  const [savedItems, setSavedItems] = useState<PlayLimitItem[]>([])
  const [catalog, setCatalog] = useState<PlayCatalogItem[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [error, setError] = useState('')
  const [conflict, setConflict] = useState(false)
  const [guideOpen, setGuideOpen] = useState(false)
  const [rulesReady, setRulesReady] = useState(false)
  const [rulesMessage, setRulesMessage] = useState('')
  const [guard, setGuard] = useState<OddsMutationGuard>({ expected_rule_version: '', expected_revision: '' })
  const [riskWarnings, setRiskWarnings] = useState<OddsRiskWarning[]>([])
  const loadSequence = useRef(0)
  const loadController = useRef<AbortController | null>(null)
  const writeLocked = useRef(false)
  const loadedGame = useRef('')
  const draftRef = useRef<PlayLimitItem[]>([])
  const savedRef = useRef<PlayLimitItem[]>([])
  const { showMessage } = useFeedback()

  const catalogMap = useMemo(() => Object.fromEntries(catalog.map(item => [item.play_code, item])), [catalog])
  const dirtyCount = useMemo(() => oddsDirtyCodes(items, savedItems).length, [items, savedItems])
  const confirmedCount = savedItems.filter(item => item.configured === true && item.configuration_source === 'admin_save' && item.odds > 1).length
  const versionReady = Boolean(guard.expected_rule_version && guard.expected_revision)
  const unavailable = !gameId || !rulesReady || !versionReady || loading || saving || clearing || conflict

  const acceptSaved = useCallback((result: GameOddsLimits) => {
    const next = Array.isArray(result.items) ? result.items : []
    draftRef.current = next
    savedRef.current = next
    loadedGame.current = result.game_id
    setItems(next)
    setSavedItems(next)
    setRulesReady(result.rules_ready === true && next.length > 0)
    setRulesMessage(result.rules_message || '')
    setGuard({ expected_rule_version: result.rule_version || '', expected_revision: result.config_revision || '' })
    setRiskWarnings(result.risk_warnings || [])
    setConflict(false)
  }, [])

  const loadLimits = useCallback(async (id: string, notify = false) => {
    const sequence = ++loadSequence.current
    loadController.current?.abort()
    const controller = new AbortController()
    loadController.current = controller
    loadedGame.current = ''
    setLoading(true)
    setError('')
    try {
      const [result, catalogItems] = await Promise.all([
        adminApi.oddsLimits(id, controller.signal),
        adminApi.playCatalog(id, controller.signal),
      ])
      if (sequence !== loadSequence.current || controller.signal.aborted) return
      if (!result || result.game_id !== id) throw new Error('返回的彩种配置不匹配，请重新刷新')
      acceptSaved(result)
      setCatalog(catalogItems)
      if (notify) showMessage(`${result.game_name} 赔率限额已刷新`)
    } catch (reason) {
      const cancelled = controller.signal.aborted || (reason instanceof Error && reason.name === 'AbortError')
      // One failed endpoint makes the pair unusable; stop its sibling too.
      controller.abort()
      if (sequence !== loadSequence.current || cancelled) return
      setError(reason instanceof Error ? reason.message : '读取赔率限额失败')
      setRulesReady(false)
      setRiskWarnings([])
      setGuard({ expected_rule_version: '', expected_revision: '' })
    } finally {
      if (sequence === loadSequence.current) {
        setLoading(false)
        if (loadController.current === controller) loadController.current = null
      }
    }
  }, [acceptSaved, showMessage])

  useEffect(() => {
    let cancelled = false
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      void adminApi.games(controller.signal).then(list => {
        if (cancelled) return
        setGames(list)
        setGameId(current => current || list[0]?.id || '')
        if (!list.length) setLoading(false)
      }).catch(reason => {
        if (cancelled) return
        setError(reason instanceof Error ? reason.message : '读取游戏列表失败')
        setLoading(false)
      })
    }, 0)
    return () => { cancelled = true; window.clearTimeout(timer); controller.abort() }
  }, [])

  useEffect(() => {
    if (!gameId) return
    const timer = window.setTimeout(() => void loadLimits(gameId), 0)
    return () => {
      window.clearTimeout(timer)
      // A request-generation counter, not a rendered DOM node.
      // eslint-disable-next-line react-hooks/exhaustive-deps
      loadSequence.current++
      loadController.current?.abort()
      loadController.current = null
    }
  }, [gameId, loadLimits])

  useEffect(() => {
    const hasDraft = () => oddsDirtyCodes(draftRef.current, savedRef.current).length > 0
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (!writeLocked.current && !hasDraft()) return
      event.preventDefault()
      event.returnValue = ''
    }
    const beforeNavigate = (event: Event) => {
      if (writeLocked.current || (hasDraft() && !window.confirm('赔率有未保存修改，离开将放弃这些修改。继续？'))) event.preventDefault()
    }
    window.addEventListener('beforeunload', beforeUnload)
    window.addEventListener('yaotu-before-navigate', beforeNavigate)
    return () => {
      window.removeEventListener('beforeunload', beforeUnload)
      window.removeEventListener('yaotu-before-navigate', beforeNavigate)
    }
  }, [])

  const handleWriteError = (reason: unknown, fallback: string) => {
    setError(reason instanceof Error ? reason.message : fallback)
    if (isOddsConfigurationConflict(reason)) setConflict(true)
  }

  const save = async () => {
    if (writeLocked.current || unavailable || loadedGame.current !== gameId || !oddsDirtyCodes(draftRef.current, savedRef.current).length) return
    const validation = draftRef.current.length !== catalog.length ? '必须提交当前彩种的完整玩法目录，请刷新后重试' : validateOddsDraft(draftRef.current)
    if (validation) { setError(validation); return }
    writeLocked.current = true
    setSaving(true)
    setError('')
    try {
      const result = await adminApi.updateOddsLimits(gameId, { ...guard, items: draftRef.current })
      if (!result || result.game_id !== gameId) throw new Error('保存响应不匹配，请刷新确认当前配置')
      acceptSaved(result)
      showMessage(`${result.game_name} 赔率限额已保存`)
    } catch (reason) {
      handleWriteError(reason, '保存赔率限额失败')
    } finally {
      writeLocked.current = false
      setSaving(false)
    }
  }

  const clearCurrent = async () => {
    if (writeLocked.current || unavailable || loadedGame.current !== gameId) return
    if (!window.confirm('清空当前彩种全部平台赔率及房间、会员覆盖后，所有玩法将停止受理；未保存修改也会丢弃。继续？')) return
    writeLocked.current = true
    setClearing(true)
    setError('')
    try {
      const result = await adminApi.resetOddsLimits(gameId, guard)
      if (!result || result.game_id !== gameId) throw new Error('清空响应不匹配，请刷新确认当前配置')
      acceptSaved(result)
      showMessage(`${result.game_name} 赔率已清空，全部玩法暂停受理`)
    } catch (reason) {
      handleWriteError(reason, '清空赔率失败')
    } finally {
      writeLocked.current = false
      setClearing(false)
    }
  }

  const refresh = async () => {
    if (writeLocked.current || !gameId) return
    if (oddsDirtyCodes(draftRef.current, savedRef.current).length && !window.confirm('赔率有未保存修改，刷新将放弃这些修改。继续？')) return
    await loadLimits(gameId, true)
  }

  const selectGame = (id: string) => {
    if (writeLocked.current || saving || clearing || id === gameId) return
    if (oddsDirtyCodes(draftRef.current, savedRef.current).length && !window.confirm('赔率有未保存修改，切换彩种将放弃这些修改。继续？')) return
    loadSequence.current++
    loadController.current?.abort()
    loadController.current = null
    loadedGame.current = ''
    draftRef.current = []
    savedRef.current = []
    setLoading(true)
    setItems([])
    setSavedItems([])
    setCatalog([])
    setRulesReady(false)
    setRulesMessage('')
    setRiskWarnings([])
    setGuard({ expected_rule_version: '', expected_revision: '' })
    setConflict(false)
    setError('')
    setGameId(id)
  }

  const editItems = useCallback((next: PlayLimitItem[]) => {
    if (writeLocked.current || unavailable || loadedGame.current !== gameId) return
    const draft = oddsDraftItems(next, savedRef.current)
    draftRef.current = draft
    setItems(draft)
    setError('')
  }, [gameId, unavailable])
  const currentGame = games.find(game => game.id === gameId)

  return <Box p={{ xs: 1.5, lg: 2 }}>
    <PageHeader eyebrow="游戏运营 / 风控" title="赔率与限额" description=""
      actions={<Button variant="contained" startIcon={<SaveRounded />} disabled={unavailable || !dirtyCount} onClick={() => void save()}>{saving ? '保存中…' : '保存设置'}</Button>} />

    <Paper variant="outlined" sx={{ mt: 1.25, p: 1, borderRadius: 1.25 }}>
      <Stack direction={{ xs: 'column', lg: 'row' }} gap={.8} alignItems={{ lg: 'center' }} mb={.8}>
        <Box flex={1}>
          <Typography fontSize={13} fontWeight={900}>平台配置层</Typography>
          <Typography fontSize={10} color="text.secondary">所有赔率须后台显式保存后生效，没有系统默认赔率；填 0 关闭玩法，房间和会员覆盖不能重新启用。</Typography>
        </Box>
        <Stack direction="row" gap={.5} flexWrap="wrap" useFlexGap>
          <Button size="small" startIcon={<MenuBookRounded />} disabled={loading || !catalog.length} onClick={() => setGuideOpen(true)}>玩法说明</Button>
          <Button size="small" variant="outlined" startIcon={<RefreshRounded />} disabled={!gameId || saving || clearing} onClick={() => void refresh()}>{loading ? '取消并重试' : '刷新配置'}</Button>
          <Button size="small" color="error" variant="outlined" startIcon={<DeleteOutlineRounded />} disabled={unavailable} onClick={() => void clearCurrent()}>{clearing ? '清空中…' : '清空当前'}</Button>
        </Stack>
      </Stack>
      <GameOddsNavigation games={games.map(game => ({ ...game, enabled: true }))} gameId={gameId} onSelect={selectGame} />
      {currentGame && <Stack direction="row" gap={.6} flexWrap="wrap" mt={.75}>
        <Chip size="small" color="primary" label={currentGame.name} sx={{ height: 22 }} />
        <Chip size="small" variant="outlined" label={loading ? '正在读取配置' : `${items.length} 个玩法 · ${confirmedCount} 项已启用`} sx={{ height: 22 }} />
        <Chip size="small" variant="outlined" color={rulesReady && currentGame.enabled && confirmedCount > 0 ? 'success' : 'default'} label={loading ? '加载中' : !rulesReady ? '玩法待配置' : !currentGame.enabled ? '已停用' : confirmedCount > 0 ? '运行中' : '赔率待配置'} sx={{ height: 22 }} />
        {!loading && guard.expected_rule_version && <Chip size="small" variant="outlined" label={guard.expected_rule_version} sx={{ height: 22 }} />}
        {dirtyCount > 0 && <Chip size="small" color="warning" label={`${dirtyCount} 项未保存`} sx={{ height: 22 }} />}
      </Stack>}
    </Paper>
    {error && <Alert severity="error" sx={{ mt: 1.5 }}>{error}</Alert>}
    {conflict && <Alert severity="warning" sx={{ mt: 1 }}>配置或玩法版本已被其他操作更新。你的草稿仍保留，尚未覆盖服务器；请记录需要保留的修改，再点“刷新配置”读取最新版本后重新编辑。</Alert>}
    {!loading && rulesReady && !versionReady && <Alert severity="warning" sx={{ mt: 1.5 }}>配置版本信息缺失，已禁止保存与清空。请刷新；若仍无版本信息，请更新后端。</Alert>}
    {!loading && rulesMessage && <Alert severity="warning" sx={{ mt: 1.5 }}>{rulesMessage}。现有赔率配置保留，确认专属规则后再开放。</Alert>}
    {!loading && riskWarnings.map(warning => <Alert key={warning.code} severity="warning" sx={{ mt: 1.5 }}>
      <Typography component="div" fontWeight={700} fontSize={13}>已保存配置 · 赔率风险</Typography>{warning.message}
    </Alert>)}

    <Paper variant="outlined" sx={{ mt: 1, p: { xs: .75, md: 1 }, borderRadius: 1.25 }}>
      {loading ? <Stack minHeight={220} alignItems="center" justifyContent="center" gap={1} role="status"><CircularProgress size={24} /><Typography fontSize={12} color="text.secondary">正在读取配置，可切换彩种或取消并重试</Typography></Stack>
        : items.length ? <PlatformOddsGrid key={gameId} items={items} catalog={catalogMap} onChange={editItems} disabled={unavailable} />
          : <Stack minHeight={180} alignItems="center" justifyContent="center" color="text.secondary">{rulesMessage ? '此彩种的专属玩法待核对，当前不提供投注赔率' : '暂无玩法目录，请选择其他彩种或刷新配置'}</Stack>}
    </Paper>

    <Dialog open={guideOpen} onClose={() => setGuideOpen(false)} fullWidth maxWidth="md">
      <DialogTitle>{currentGame?.name || '当前彩种'} · 玩法说明</DialogTitle>
      <DialogContent dividers>
        <Typography variant="body2" color="text.secondary" mb={1}>以下仅为玩法目录；投注赔率以当前版本的后台保存配置为准。</Typography>
        <Table size="small"><TableHead><TableRow>{['编号', '名称', '分类', '说明', '前台示例'].map(column => <TableCell key={column}>{column}</TableCell>)}</TableRow></TableHead>
          <TableBody>{catalog.map(item => <TableRow key={item.play_code}>
            <TableCell><Typography fontFamily="monospace" fontSize={11}>{item.play_code}</Typography></TableCell>
            <TableCell>{item.play_name}</TableCell><TableCell>{item.category}</TableCell><TableCell>{item.description}</TableCell><TableCell>{item.example}</TableCell>
          </TableRow>)}</TableBody>
        </Table>
      </DialogContent>
    </Dialog>
  </Box>
}
