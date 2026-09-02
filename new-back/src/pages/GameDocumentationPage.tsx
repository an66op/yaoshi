import {
  Alert, Box, Card, CardContent, Chip, CircularProgress, Divider, List, ListItemButton,
  ListItemText, Paper, Stack, Tab, Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  Tabs, TextField, Typography,
} from '@mui/material'
import ArticleRounded from '@mui/icons-material/ArticleRounded'
import CompareArrowsRounded from '@mui/icons-material/CompareArrowsRounded'
import MenuBookRounded from '@mui/icons-material/MenuBookRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import { useEffect, useMemo, useRef, useState } from 'react'
import { adminApi, type AdminGame, type GameOddsLimits, type PlayCatalogItem } from '../api'
import {
  currentRuleBindingReady, currentRuleProfileForGame, differenceStatusLabel, originalNamedGameCount, originalRuleDocumentLineCount, parseOriginalRuleDocument,
  type OriginalRuleSection, type RuleDifferenceStatus,
} from '../gameRuleDocumentation'

type DocumentationTab = 'original' | 'current' | 'differences'

const ORIGINAL_RULES_URL = '/game-docs/original.txt'

const statusColor: Record<RuleDifferenceStatus, 'success' | 'warning' | 'info' | 'default'> = {
  same: 'success',
  different: 'warning',
  'current-only': 'info',
  pending: 'default',
}

const formatOdds = (value: number | undefined) => value && value > 0 ? value.toFixed(3).replace(/0+$/, '').replace(/\.$/, '') : '待配置'

function GameList({ games, selectedID, query, onQuery, onSelect }: {
  games: AdminGame[]
  selectedID: string
  query: string
  onQuery: (value: string) => void
  onSelect: (id: string) => void
}) {
  const visible = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return games
    return games.filter(game => `${game.name} ${game.id} ${game.category}`.toLowerCase().includes(keyword))
  }, [games, query])
  return <Paper variant="outlined" sx={{ width: { xs: '100%', md: 278 }, height: { md: '100%' }, minHeight: { xs: 420, md: 0 }, flex: '0 0 auto', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
    <Box p={1.2}><TextField fullWidth size="small" value={query} onChange={event => onQuery(event.target.value)} placeholder="搜索彩种" InputProps={{ startAdornment: <SearchRounded color="action" sx={{ mr: .8, fontSize: 19 }} /> }} /></Box>
    <Divider />
    <List dense disablePadding sx={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
      {visible.map(game => {
        const ready = currentRuleBindingReady(game)
        return <ListItemButton key={game.id} selected={game.id === selectedID} onClick={() => onSelect(game.id)} sx={{ py: .85 }}>
          <ListItemText primary={game.name} secondary={`${game.rule_version || '未绑定'} · ${ready ? '规则已接入' : '暂停受理'}`} primaryTypographyProps={{ fontWeight: 750, fontSize: 13 }} secondaryTypographyProps={{ fontSize: 10.5 }} />
          <Chip size="small" color={ready ? 'success' : 'default'} variant="outlined" label={game.enabled ? '已显示' : '已关闭'} sx={{ height: 22, fontSize: 10 }} />
        </ListItemButton>
      })}
      {!visible.length && <Box p={2}><Typography color="text.secondary" fontSize={12}>没有匹配的彩种</Typography></Box>}
    </List>
  </Paper>
}

function OriginalDocument({ source, sections, loading, error }: { source: string; sections: OriginalRuleSection[]; loading: boolean; error: string }) {
  const [query, setQuery] = useState('')
  const [selectedID, setSelectedID] = useState('')
  const visibleSections = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return sections
    return sections.filter(section => `${section.title}\n${section.content}`.toLowerCase().includes(keyword))
  }, [query, sections])
  const effectiveSelectedID = selectedID || sections[0]?.id || '__all__'
  const selected = sections.find(section => section.id === effectiveSelectedID)
  const showFullSource = effectiveSelectedID === '__all__'
  const content = showFullSource ? source : selected?.rulesContent ?? source

  if (loading) return <Box minHeight={320} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={30} /></Box>
  if (error) return <Alert severity="error">{error}</Alert>
  return <Stack direction={{ xs: 'column', md: 'row' }} gap={1.5} sx={{ height: { md: 'calc(100dvh - 245px)' }, minHeight: { md: 540 } }}>
    <Paper variant="outlined" sx={{ width: { xs: '100%', md: 278 }, flex: '0 0 auto', overflow: 'hidden', display: 'flex', flexDirection: 'column', minHeight: { xs: 420, md: 0 } }}>
      <Box p={1.2}><TextField fullWidth size="small" value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索原版正文" InputProps={{ startAdornment: <SearchRounded color="action" sx={{ mr: .8, fontSize: 19 }} /> }} /></Box>
      <Divider />
      <List dense disablePadding sx={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
        <Box px={1.8} pt={1.1} pb={.5}><Typography variant="overline" color="text.secondary" fontWeight={850}>具体游戏（{sections.length}）</Typography></Box>
        {visibleSections.map((section, index) => <ListItemButton key={section.id} selected={effectiveSelectedID === section.id} onClick={() => setSelectedID(section.id)}>
          <ListItemText primary={`${index + 1}. ${section.title}`} secondary={`第 ${section.startLine.toLocaleString()}–${section.endLine.toLocaleString()} 行`} primaryTypographyProps={{ fontSize: 13, fontWeight: 700 }} secondaryTypographyProps={{ fontSize: 10.5 }} />
        </ListItemButton>)}
        <Divider />
        <ListItemButton selected={effectiveSelectedID === '__all__'} onClick={() => setSelectedID('__all__')}><ListItemText primary="原始附件全文（辅助）" secondary={`${originalRuleDocumentLineCount(source).toLocaleString()} 行 · ${new Blob([source]).size.toLocaleString()} 字节`} primaryTypographyProps={{ fontSize: 12.5 }} secondaryTypographyProps={{ fontSize: 10.5 }} /></ListItemButton>
      </List>
    </Paper>
    <Paper variant="outlined" sx={{ flex: 1, minWidth: 0, minHeight: { xs: 520, md: 0 }, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1} px={2} py={1.35} bgcolor="action.hover">
        <Box><Typography fontWeight={850}>{selected?.title ?? '原始附件全文（辅助）'}</Typography><Typography variant="caption" color="text.secondary">附件原文快照 · 按具体游戏拆分 · 未改写、未判断、未套用为当前规则</Typography></Box>
        <Chip size="small" icon={<ArticleRounded />} label="原版资料" color="warning" variant="outlined" />
      </Stack>
      {showFullSource ? <Box component="pre" sx={{ m: 0, p: 2, flex: 1, minHeight: 0, overflow: 'auto', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 12, lineHeight: 1.75 }}>{content}</Box>
        : <Box sx={{ flex: 1, minHeight: 0, display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'minmax(0, 2fr) minmax(0, 1fr)' }, overflow: 'hidden' }}>
          <Box sx={{ minWidth: 0, minHeight: { xs: 340, md: 0 }, display: 'flex', flexDirection: 'column', borderRight: { md: 1 }, borderBottom: { xs: 1, md: 0 }, borderColor: 'divider' }}>
            <Box px={2} py={1.05} bgcolor="action.hover"><Typography fontWeight={850} fontSize={13.5}>玩法说明</Typography></Box>
            <Box component="pre" sx={{ m: 0, p: 2, flex: 1, minHeight: 0, overflow: 'auto', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 12, lineHeight: 1.75 }}>{content || '原版未单独提供玩法说明。'}</Box>
          </Box>
          <Box sx={{ minWidth: 0, minHeight: { xs: 340, md: 0 }, display: 'flex', flexDirection: 'column' }}>
            <Box px={2} py={1.05} bgcolor="action.hover"><Typography fontWeight={850} fontSize={13.5}>赔率 / 倍率</Typography></Box>
            {selected?.odds.length ? <TableContainer sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }}><Table stickyHeader size="small" sx={{ width: '100%', tableLayout: 'fixed' }}><TableHead><TableRow><TableCell>玩法</TableCell><TableCell align="right" width={82}>倍率</TableCell></TableRow></TableHead><TableBody>{selected.odds.map((item, index) => <TableRow key={`${index}-${item.play}`} hover><TableCell sx={{ overflowWrap: 'anywhere' }}><Typography fontSize={12.5}>{item.play}</Typography></TableCell><TableCell align="right"><Typography fontWeight={850} color="warning.main" whiteSpace="nowrap">{item.multiplier}</Typography></TableCell></TableRow>)}</TableBody></Table></TableContainer>
              : <Alert severity="info" sx={{ m: 1.5 }}>原版这一段没有独立赔率表。</Alert>}
          </Box>
        </Box>}
    </Paper>
  </Stack>
}

export function CurrentRules({ game, catalog, limits, loading, error }: { game?: AdminGame; catalog: PlayCatalogItem[]; limits: GameOddsLimits | null; loading: boolean; error: string }) {
  if (!game) return <Alert severity="info">后台暂时没有彩种资料。</Alert>
  const profile = currentRuleProfileForGame(game)
  const bindingReady = currentRuleBindingReady(game)
  const configured = new Map((limits?.items ?? []).map(item => [item.play_code, item.odds]))
  return <Stack gap={1.5}>
    <Card variant="outlined"><CardContent>
      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1.2}>
        <Box><Typography variant="overline" color="primary.main" fontWeight={900}>当前系统事实</Typography><Typography variant="h5" fontWeight={900}>{game.name}</Typography><Typography mt={.7} color="text.secondary" fontSize={13.5} lineHeight={1.75}>{profile.summary}</Typography></Box>
        <Stack direction="row" flexWrap="wrap" gap={.7} alignContent="flex-start">
          <Chip size="small" color={bindingReady ? 'success' : 'warning'} label={bindingReady ? '可识别规则已绑定' : '规则未安全绑定'} />
          <Chip size="small" variant="outlined" label={game.rule_version || profile.expectedVersion} />
          <Chip size="small" variant="outlined" label={profile.modes} />
        </Stack>
      </Stack>
      {!bindingReady && <Alert severity="warning" sx={{ mt: 1.5 }}>{game.rules_message || '该彩种尚未配置完整玩法，暂不受理投注。'}</Alert>}
    </CardContent></Card>
    <Paper variant="outlined" sx={{ p: 2 }}><Typography fontWeight={850}>当前结算与识别规则</Typography><Typography variant="caption" color="text.secondary">规则族：{profile.family}</Typography><Stack component="ul" gap={.8} pl={2.5} mb={0}>{profile.rules.map(rule => <Typography component="li" key={rule} fontSize={13} lineHeight={1.7}>{rule}</Typography>)}</Stack></Paper>
    <Paper variant="outlined" sx={{ overflow: 'hidden' }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center" px={2} py={1.35} bgcolor="action.hover"><Box><Typography fontWeight={850}>服务端当前玩法目录</Typography><Typography variant="caption" color="text.secondary">仅列出下注与结算同时实现的玩法；赔率读取当前后台配置</Typography></Box><Chip size="small" label={`${catalog.length} 项`} /></Stack>
      {loading ? <Box minHeight={160} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={26} /></Box>
        : error ? <Alert severity="error" sx={{ m: 1.5 }}>{error}</Alert>
        : catalog.length ? <TableContainer sx={{ maxHeight: 480 }}><Table stickyHeader size="small"><TableHead><TableRow><TableCell>玩法</TableCell><TableCell>分类</TableCell><TableCell>说明 / 示例</TableCell><TableCell align="right">当前赔率</TableCell></TableRow></TableHead><TableBody>{catalog.map(item => <TableRow key={item.play_code} hover><TableCell><Typography fontWeight={750} fontSize={12.5}>{item.play_name}</Typography><Typography fontFamily="ui-monospace,monospace" fontSize={10} color="text.secondary">{item.play_code}</Typography></TableCell><TableCell>{item.category}</TableCell><TableCell><Typography fontSize={12}>{item.description}</Typography>{item.example && <Typography fontSize={10.5} color="text.secondary">例：{item.example}</Typography>}</TableCell><TableCell align="right"><Typography fontWeight={800} color={(configured.get(item.play_code) ?? 0) > 1 ? 'success.main' : 'warning.main'}>{formatOdds(configured.get(item.play_code))}</Typography></TableCell></TableRow>)}</TableBody></Table></TableContainer>
        : <Alert severity="info" sx={{ m: 1.5 }}>当前没有可提交的玩法目录；这不代表原版没有玩法，只表示本系统尚未绑定安全的下注与结算规则。</Alert>}
    </Paper>
  </Stack>
}

function Differences({ game, games }: { game?: AdminGame; games: AdminGame[] }) {
  if (!game) return <Alert severity="info">后台暂时没有彩种资料。</Alert>
  const profile = currentRuleProfileForGame(game)
  return <Stack gap={1.5}>
    <Alert severity="info">这里只记录原版资料与当前实现的事实差异，不判断哪一版正确，也不会自动修改规则或赔率。</Alert>
    <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.2}>
      <Card variant="outlined" sx={{ flex: 1 }}><CardContent><Typography variant="caption" color="text.secondary">原版附件</Typography><Typography fontSize={25} fontWeight={900}>{originalNamedGameCount}</Typography><Typography variant="caption" color="text.secondary">个命名彩种或玩法版本</Typography></CardContent></Card>
      <Card variant="outlined" sx={{ flex: 1 }}><CardContent><Typography variant="caption" color="text.secondary">当前彩种目录</Typography><Typography fontSize={25} fontWeight={900}>{games.length}</Typography><Typography variant="caption" color="text.secondary">以后端实时返回为准</Typography></CardContent></Card>
      <Card variant="outlined" sx={{ flex: 1 }}><CardContent><Typography variant="caption" color="text.secondary">当前规则已绑定</Typography><Typography fontSize={25} fontWeight={900} color="success.main">{games.filter(currentRuleBindingReady).length}</Typography><Typography variant="caption" color="text.secondary">其余保留为待映射/暂停受理</Typography></CardContent></Card>
    </Stack>
    <Alert severity="warning">原版与当前目录不是一一对应：原版中的乐彩快3、动物运动会等目前没有本地彩种ID；当前的宾果时时彩（二至四）、宾果赛车(B)及部分官方彩也没有可直接认定的原版映射。</Alert>
    <Paper variant="outlined" sx={{ overflow: 'hidden' }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1} px={2} py={1.5} bgcolor="action.hover"><Box><Typography variant="h6" fontWeight={900}>{game.name} · 差异对照</Typography><Typography variant="caption" color="text.secondary">当前规则版本：{game.rule_version || profile.expectedVersion}</Typography></Box><Chip icon={<CompareArrowsRounded />} label={`${profile.differences.length} 项记录`} color="primary" variant="outlined" /></Stack>
      <TableContainer><Table><TableHead><TableRow><TableCell width="16%">对照项</TableCell><TableCell width="31%">原版资料</TableCell><TableCell width="37%">当前系统</TableCell><TableCell width="16%">事实状态</TableCell></TableRow></TableHead><TableBody>{profile.differences.map(item => <TableRow key={item.topic} sx={{ verticalAlign: 'top' }}><TableCell><Typography fontWeight={800} fontSize={12.5}>{item.topic}</Typography></TableCell><TableCell><Typography fontSize={12.5} lineHeight={1.7}>{item.original}</Typography></TableCell><TableCell><Typography fontSize={12.5} lineHeight={1.7}>{item.current}</Typography></TableCell><TableCell><Chip size="small" color={statusColor[item.status]} label={differenceStatusLabel[item.status]} /></TableCell></TableRow>)}</TableBody></Table></TableContainer>
    </Paper>
  </Stack>
}

export function GameDocumentationPage() {
  const [tab, setTab] = useState<DocumentationTab>('original')
  const [games, setGames] = useState<AdminGame[]>([])
  const [selectedID, setSelectedID] = useState('')
  const [gameQuery, setGameQuery] = useState('')
  const [catalog, setCatalog] = useState<PlayCatalogItem[]>([])
  const [limits, setLimits] = useState<GameOddsLimits | null>(null)
  const [gamesLoading, setGamesLoading] = useState(true)
  const [detailLoading, setDetailLoading] = useState(true)
  const [gamesError, setGamesError] = useState('')
  const [detailError, setDetailError] = useState('')
  const [original, setOriginal] = useState('')
  const [originalLoading, setOriginalLoading] = useState(true)
  const [originalError, setOriginalError] = useState('')
  const detailRequest = useRef(0)

  useEffect(() => {
    let active = true
    void adminApi.games().then(result => {
      if (!active) return
      const next = Array.isArray(result) ? result : []
      setGames(next)
      setSelectedID(current => current || next[0]?.id || '')
    }).catch(reason => { if (active) setGamesError(reason instanceof Error ? reason.message : '读取当前彩种失败') }).finally(() => { if (active) setGamesLoading(false) })
    return () => { active = false }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void fetch(ORIGINAL_RULES_URL, { cache: 'no-store', signal: controller.signal }).then(response => {
      if (!response.ok) throw new Error(`原版资料读取失败（${response.status}）`)
      return response.text()
    }).then(setOriginal).catch(reason => { if (!controller.signal.aborted) setOriginalError(reason instanceof Error ? reason.message : '原版资料读取失败') }).finally(() => { if (!controller.signal.aborted) setOriginalLoading(false) })
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (!selectedID) return
    const requestID = ++detailRequest.current
    void Promise.all([adminApi.playCatalog(selectedID), adminApi.oddsLimits(selectedID)]).then(([nextCatalog, nextLimits]) => {
      if (requestID !== detailRequest.current) return
      setCatalog(Array.isArray(nextCatalog) ? nextCatalog : [])
      setLimits(nextLimits)
    }).catch(reason => {
      if (requestID !== detailRequest.current) return
      setCatalog([])
      setLimits(null)
      setDetailError(reason instanceof Error ? reason.message : '读取当前玩法失败')
    }).finally(() => { if (requestID === detailRequest.current) setDetailLoading(false) })
  }, [selectedID])

  const selectedGame = games.find(game => game.id === selectedID)
  const sections = useMemo(() => parseOriginalRuleDocument(original), [original])
  const selectGame = (id: string) => {
    if (id === selectedID) return
    setDetailLoading(true)
    setDetailError('')
    setSelectedID(id)
  }

  return <Box p={{ xs: 1.5, lg: 2.5 }}>
    <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" gap={1.5} mb={2}>
      <Box><Typography variant="overline" color="primary.main" fontWeight={900}>系统管理 / 规则资料</Typography><Typography variant="h4" fontWeight={950}>游戏说明</Typography><Typography mt={.6} color="text.secondary" fontSize={13}>原版附件、当前系统规则与事实差异分别保存；本页面只读，不会改变赔率、开关或结算。</Typography></Box>
      <Stack direction="row" gap={.8} alignItems="flex-start" flexWrap="wrap"><Chip icon={<ArticleRounded />} label="原版原文" variant="outlined" /><Chip icon={<MenuBookRounded />} label={`${games.length} 个当前彩种`} variant="outlined" color="primary" /></Stack>
    </Stack>
    {gamesError && <Alert severity="error" sx={{ mb: 1.5 }}>{gamesError}</Alert>}
    <Paper variant="outlined" sx={{ mb: 1.5, overflow: 'hidden' }}><Tabs value={tab} onChange={(_, value: DocumentationTab) => setTab(value)} variant="scrollable" scrollButtons="auto"><Tab value="original" label="原版说明" /><Tab value="current" label="当前所有规则" /><Tab value="differences" label="与原版的差异" /></Tabs></Paper>
    {tab === 'original' ? <OriginalDocument source={original} sections={sections} loading={originalLoading} error={originalError} />
      : gamesLoading ? <Box minHeight={340} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={30} /></Box>
      : <Stack direction={{ xs: 'column', md: 'row' }} gap={1.5} alignItems="stretch" sx={{ height: { md: 'calc(100dvh - 245px)' }, minHeight: { md: 540 } }}>
        <GameList games={games} selectedID={selectedID} query={gameQuery} onQuery={setGameQuery} onSelect={selectGame} />
        <Paper variant="outlined" sx={{ flex: 1, minWidth: 0, width: '100%', height: { md: '100%' }, minHeight: { xs: 520, md: 0 }, overflow: 'hidden' }}>
          <Box sx={{ height: '100%', overflowY: 'auto', overflowX: 'hidden', p: 1.5 }}>
            {tab === 'current' ? <CurrentRules game={selectedGame} catalog={catalog} limits={limits} loading={detailLoading} error={detailError} /> : <Differences game={selectedGame} games={games} />}
          </Box>
        </Paper>
      </Stack>}
  </Box>
}
