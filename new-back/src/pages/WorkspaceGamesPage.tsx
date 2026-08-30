import {
  Alert, Avatar, Box, Button, Chip, CircularProgress, InputAdornment, Paper, Stack, Switch, TextField, Tooltip, Typography,
} from '@mui/material'
import SearchRounded from '@mui/icons-material/SearchRounded'
import SportsEsportsRounded from '@mui/icons-material/SportsEsportsRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { agentApi, tenantApi, type WorkspaceGame } from '../api'
import { getStoredUser } from '../auth'
import { PageHeader } from '../components/PageHeader'
import { useFeedback } from '../components/feedback'
import { gameLogo } from '../gameLogos'
import { workspaceGameAvailability } from '../utils/workspaceGameAvailability'

type GameFilter = 'all' | 'enabled' | 'disabled'

export function WorkspaceGamesPage() {
  const role = getStoredUser()?.role === 'tenant' ? 'tenant' : 'agent'
  const api = role === 'tenant' ? tenantApi : agentApi
  const { showMessage } = useFeedback()
  const [games, setGames] = useState<WorkspaceGame[]>([])
  const [category, setCategory] = useState('全部')
  const [filter, setFilter] = useState<GameFilter>('all')
  const [query, setQuery] = useState('')
  const [pending, setPending] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setGames(await api.games())
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取房间游戏失败')
    } finally {
      setLoading(false)
    }
  }, [api])

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer) }, [load])

  const categories = useMemo(() => ['全部', ...Array.from(new Set(games.map(game => game.lobby_category?.trim() || '未分类')))], [games])
  const visibleGames = useMemo(() => games.filter(game => {
    if (category !== '全部' && (game.lobby_category?.trim() || '未分类') !== category) return false
    const available = workspaceGameAvailability(game).available
    if (filter === 'enabled' && !available) return false
    if (filter === 'disabled' && available) return false
    const keyword = query.trim().toLowerCase()
    return !keyword || game.name.toLowerCase().includes(keyword) || game.id.toLowerCase().includes(keyword)
  }), [category, filter, games, query])

  const toggle = async (game: WorkspaceGame, enabled: boolean) => {
    const availability = workspaceGameAvailability(game)
    if (enabled && !availability.canEnable) {
      setError(availability.detail)
      return
    }
    setPending(game.id)
    setError('')
    let saved = false
    try {
      await api.setGameStatus(game.id, enabled)
      saved = true
      const nextGames = await api.games()
      setGames(nextGames)
      const current = nextGames.find(item => item.id === game.id)
      const status = current && workspaceGameAvailability(current)
      showMessage(status ? `${game.name}：${status.label}` : `${game.name}房间设置已保存`, status?.color === 'warning' ? 'warning' : 'success')
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : '更新游戏状态失败'
      setError(saved ? `开关已保存，但游戏状态刷新失败，请刷新页面确认：${message}` : message)
    } finally {
      setPending('')
    }
  }

  const enabledCount = games.filter(game => workspaceGameAvailability(game).available).length

  return <Box p={{ xs: 1.5, md: 2.5 }}>
    <PageHeader eyebrow="房间运营" title="游戏列表" description="" />
    {error && <Alert severity="error" sx={{ mt: 1.5 }}>{error}</Alert>}
    <Paper variant="outlined" sx={{ mt: 1.5, p: 1.25, borderRadius: 2.5 }}>
      <Stack direction={{ xs: 'column', md: 'row' }} gap={1} alignItems={{ md: 'center' }}>
        <TextField size="small" placeholder="搜索游戏名称或编号" value={query} onChange={event => setQuery(event.target.value)} sx={{ flex: 1, minWidth: 220 }} slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small" /></InputAdornment> } }} />
        <Stack direction="row" gap={.6} flexWrap="wrap">
          {(['all', 'enabled', 'disabled'] as const).map(value => <Button key={value} size="small" variant={filter === value ? 'contained' : 'outlined'} onClick={() => setFilter(value)}>{value === 'all' ? `全部 ${games.length}` : value === 'enabled' ? `已开放 ${enabledCount}` : `未开放 ${games.length - enabledCount}`}</Button>)}
        </Stack>
      </Stack>
      <Stack direction="row" gap={.65} mt={1.1} pb={.15} sx={{ overflowX: 'auto', scrollbarWidth: 'thin' }}>
        {categories.map(item => <Button key={item} size="small" variant={category === item ? 'contained' : 'text'} onClick={() => setCategory(item)} sx={{ flex: '0 0 auto', whiteSpace: 'nowrap' }}>{item}</Button>)}
      </Stack>
      <Typography variant="caption" color="text.secondary" display="block" mt={1}>分类沿用平台；新房间游戏默认关闭，可按需开启。未分类游戏暂不上架。</Typography>
    </Paper>

    {loading && <Box py={5} display="grid" sx={{ placeItems: 'center' }}><CircularProgress size={24} /></Box>}
    {!loading && <Box mt={1.25} sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,minmax(0,1fr))', xl: 'repeat(3,minmax(0,1fr))' }, gap: 1 }}>
      {visibleGames.map(game => {
        const globallyClosed = !game.platform_enabled
        const availability = workspaceGameAvailability(game)
        return <Paper key={game.id} variant="outlined" sx={{ p: 1.25, borderRadius: 2.4, opacity: globallyClosed ? .62 : 1 }}>
          <Stack direction="row" alignItems="center" gap={1.1}>
            <Avatar src={gameLogo(game.id)} alt={game.name} sx={{ width: 44, height: 44, bgcolor: 'action.hover', border: 1, borderColor: 'divider' }}>{game.name.slice(0, 2)}</Avatar>
            <Box flex={1} minWidth={0}>
              <Stack direction="row" alignItems="center" gap={.6}><Typography fontSize={14.5} fontWeight={850} noWrap>{game.name}</Typography>{globallyClosed && <Chip size="small" label="平台停用" sx={{ height: 19, fontSize: 9 }} />}</Stack>
              <Typography fontSize={10.5} color="text.secondary" noWrap>{game.lobby_category?.trim() || '未分类'} · {game.source_kind === 'official' ? '官方源' : game.source_kind === 'external' ? '外部源' : '平台彩'}</Typography>
            </Box>
            <Tooltip title={availability.detail}><span><Switch checked={game.room_enabled} disabled={(!availability.canEnable && !game.room_enabled) || pending !== ''} onChange={event => void toggle(game, event.target.checked)} inputProps={{ 'aria-label': `${game.name}房间开关` }} /></span></Tooltip>
          </Stack>
          <Stack direction="row" justifyContent="space-between" alignItems="center" mt={1} pt={.9} borderTop={1} borderColor="divider">
            <Typography fontSize={10} color="text.secondary">{game.id}</Typography>
            <Tooltip title={availability.detail}><Chip size="small" color={availability.color} variant={availability.available ? 'filled' : 'outlined'} icon={<SportsEsportsRounded />} label={availability.label} sx={{ height: 23, fontSize: 9.5 }} /></Tooltip>
          </Stack>
        </Paper>
      })}
    </Box>}
    {!loading && visibleGames.length === 0 && <Paper variant="outlined" sx={{ mt: 1.25, py: 8, textAlign: 'center', color: 'text.secondary' }}>当前条件没有游戏</Paper>}
  </Box>
}
