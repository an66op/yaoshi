import { Avatar, Box, Button, Chip, InputAdornment, Paper, Stack, TextField, Typography } from '@mui/material'
import KeyboardArrowRightRounded from '@mui/icons-material/KeyboardArrowRightRounded'
import RestartAltRounded from '@mui/icons-material/RestartAltRounded'
import { useEffect, useMemo, useState } from 'react'
import type { AdminGame, PlayLimitItem } from '../api'
import { gameLogo } from '../gameLogos'

export type OddsOverrideItem = {
  play_code: string
  play_name: string
  base_odds: number
  room_odds?: number
  override: number | null
  effective: number
  has_override: boolean
}

type GameOption = Pick<AdminGame, 'id' | 'name' | 'lobby_category' | 'enabled'> & Partial<Pick<AdminGame, 'source_kind'>>

const playCategory: Record<string, string> = {
  two_sided: '两面盘',
  ball_1_5: '号码',
  dragon_tiger: '龙虎',
  sum: '冠亚和',
  leopard: '形态',
  straight: '形态',
  pair: '形态',
  half_straight: '形态',
  mixed: '形态',
}

const categoryOrder = ['彩票', '168', '宾果', 'PC', '六合彩', '高频彩', '境外彩', '全国彩', '未分类']

export function GameOddsNavigation({ games, gameId, onSelect }: {
  games: GameOption[]
  gameId: string
  onSelect: (gameId: string) => void
}) {
  const availableGames = useMemo(() => games.filter(game => game.enabled !== false), [games])
  const categories = useMemo(() => Array.from(new Set(availableGames.map(game => game.lobby_category?.trim() || '未分类')))
    .sort((left, right) => {
      const leftIndex = categoryOrder.indexOf(left)
      const rightIndex = categoryOrder.indexOf(right)
      return (leftIndex < 0 ? 999 : leftIndex) - (rightIndex < 0 ? 999 : rightIndex)
    }), [availableGames])
  const selectedGame = availableGames.find(game => game.id === gameId)
  const selectedCategory = selectedGame?.lobby_category?.trim() || categories[0] || '未分类'
  const [category, setCategory] = useState(selectedCategory)

  useEffect(() => {
    if (selectedCategory && selectedCategory !== category) setCategory(selectedCategory)
    // category intentionally follows a game selection made outside this component.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedCategory])

  const gamesInCategory = availableGames.filter(game => (game.lobby_category?.trim() || '未分类') === category)

  return <Paper variant="outlined" sx={{ borderRadius: 2.5, overflow: 'hidden', bgcolor: 'background.paper' }}>
    <Stack direction="row" gap={.45} px={1} pt={.9} sx={{ overflowX: 'auto', scrollbarWidth: 'thin' }}>
      {categories.map(item => {
        const selected = item === category
        return <Button
          key={item}
          size="small"
          variant={selected ? 'contained' : 'text'}
          onClick={() => {
            setCategory(item)
            const first = availableGames.find(game => (game.lobby_category?.trim() || '未分类') === item)
            if (first && first.id !== gameId) onSelect(first.id)
          }}
          sx={{
            flex: '0 0 auto', minWidth: 72, whiteSpace: 'nowrap', fontWeight: 850,
            borderRadius: '10px 10px 0 0', boxShadow: selected ? '0 7px 18px rgba(8,145,178,.18)' : 'none',
          }}
        >{item}</Button>
      })}
    </Stack>
    <Stack direction="row" gap={.75} px={1.1} py={1} sx={{ overflowX: 'auto', borderTop: 1, borderColor: 'divider', scrollbarWidth: 'thin' }}>
      {gamesInCategory.map(game => {
        const selected = game.id === gameId
        return <Button
          key={game.id}
          onClick={() => onSelect(game.id)}
          variant={selected ? 'outlined' : 'text'}
          endIcon={selected ? <KeyboardArrowRightRounded /> : undefined}
          sx={{
            flex: '0 0 auto', minWidth: 132, justifyContent: 'flex-start', borderRadius: 2,
            px: 1, py: .7, color: selected ? 'primary.main' : 'text.primary',
            bgcolor: selected ? 'action.selected' : 'transparent', borderColor: selected ? 'primary.main' : 'transparent',
          }}
        >
          <Avatar src={gameLogo(game.id)} alt="" sx={{ width: 27, height: 27, mr: .75, bgcolor: 'action.hover' }}>{game.name.slice(0, 1)}</Avatar>
          <Box textAlign="left" minWidth={0}>
            <Typography fontSize={11.5} fontWeight={850} noWrap>{game.name}</Typography>
            <Typography fontSize={8.8} color="text.secondary" noWrap>{game.source_kind === 'official' ? '官方源' : game.source_kind === 'external' ? '外部源' : '平台彩'}</Typography>
          </Box>
        </Button>
      })}
    </Stack>
  </Paper>
}

export function OddsOverrideGrid({ items, level, onChange }: {
  items: OddsOverrideItem[]
  level: 'room' | 'member'
  onChange: (items: OddsOverrideItem[]) => void
}) {
  const groups = useMemo(() => {
    const grouped = new Map<string, Array<{ item: OddsOverrideItem; index: number }>>()
    items.forEach((item, index) => {
      const category = playCategory[item.play_code] || '其他玩法'
      grouped.set(category, [...(grouped.get(category) ?? []), { item, index }])
    })
    return Array.from(grouped.entries())
  }, [items])
  const inheritedLabel = level === 'member' ? '跟随房间' : '跟随平台'
  const baseLabel = level === 'member' ? '房间' : '平台'

  const update = (index: number, raw: string) => {
    const next = [...items]
    const source = level === 'member' ? (next[index].room_odds ?? next[index].base_odds) : next[index].base_odds
    next[index] = raw.trim() === ''
      ? { ...next[index], override: null, has_override: false, effective: source }
      : { ...next[index], override: Number(raw), has_override: true, effective: Number(raw) }
    onChange(next)
  }

  return <Stack gap={1.2}>
    <Stack direction="row" justifyContent="space-between" alignItems="center" gap={1}>
      <Box>
        <Typography fontWeight={900}>{level === 'member' ? '会员单独赔率' : '当前游戏赔率'}</Typography>
        <Typography fontSize={10.5} color="text.secondary">留空即{inheritedLabel}；已单独修改的输入框会高亮。</Typography>
      </Box>
      <Button size="small" startIcon={<RestartAltRounded />} onClick={() => onChange(items.map(item => ({
        ...item,
        override: null,
        has_override: false,
        effective: level === 'member' ? (item.room_odds ?? item.base_odds) : item.base_odds,
      })))}>全部继承</Button>
    </Stack>
    {groups.map(([category, rows]) => <Paper key={category} variant="outlined" sx={{ borderRadius: 2.2, overflow: 'hidden' }}>
      <Stack direction="row" alignItems="center" justifyContent="space-between" px={1.25} py={.8} bgcolor="action.hover">
        <Typography fontSize={12} fontWeight={900}>{category}</Typography>
        <Chip size="small" variant="outlined" label={`${rows.length} 项`} sx={{ height: 20, fontSize: 9 }} />
      </Stack>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,minmax(0,1fr))', xl: 'repeat(3,minmax(0,1fr))' }, gap: 1, p: 1 }}>
        {rows.map(({ item, index }) => {
          const base = level === 'member' ? (item.room_odds ?? item.base_odds) : item.base_odds
          return <Paper key={item.play_code} variant="outlined" sx={{ p: 1, borderRadius: 1.7, borderColor: item.has_override ? 'primary.main' : 'divider', bgcolor: item.has_override ? 'action.selected' : 'background.paper' }}>
            <Stack direction="row" alignItems="center" justifyContent="space-between" gap={1} mb={.65}>
              <Box minWidth={0}>
                <Typography fontSize={11.5} fontWeight={850} noWrap>{item.play_name}</Typography>
                <Typography fontSize={8.8} color="text.secondary">{item.play_code}</Typography>
              </Box>
              <Chip size="small" variant="outlined" label={`${baseLabel} ${base}`} sx={{ height: 20, fontSize: 8.8 }} />
            </Stack>
            <TextField
              fullWidth
              size="small"
              type="number"
              placeholder={inheritedLabel}
              value={item.has_override ? (item.override ?? '') : ''}
              onChange={event => update(index, event.target.value)}
              inputProps={{ min: 1.001, step: 0.001, 'aria-label': `${item.play_name}${level === 'member' ? '会员' : '房间'}赔率` }}
              slotProps={{ input: { endAdornment: <InputAdornment position="end">倍</InputAdornment> } }}
            />
          </Paper>
        })}
      </Box>
    </Paper>)}
  </Stack>
}

export function PlatformOddsGrid({ items, catalog, onChange }: {
  items: PlayLimitItem[]
  catalog: Record<string, { category?: string; description?: string; example?: string; default_odds?: number } | undefined>
  onChange: (items: PlayLimitItem[]) => void
}) {
  const update = (index: number, patch: Partial<PlayLimitItem>) => onChange(items.map((item, rowIndex) => rowIndex === index ? { ...item, ...patch } : item))
  const fields: Array<{ key: keyof Pick<PlayLimitItem, 'odds' | 'min_bet' | 'max_bet' | 'max_user_period' | 'max_period_total'>; label: string; step?: number }> = [
    { key: 'odds', label: '平台赔率', step: .001 },
    { key: 'min_bet', label: '单注最低' },
    { key: 'max_bet', label: '单注最高' },
    { key: 'max_user_period', label: '会员单期' },
    { key: 'max_period_total', label: '全房单期' },
  ]
  return <Stack gap={1}>
    {items.map((item, index) => {
      const meta = catalog[item.play_code]
      const modified = typeof meta?.default_odds === 'number' && Math.abs(item.odds - meta.default_odds) > .001
      return <Paper key={item.play_code} variant="outlined" sx={{ p: 1.1, borderRadius: 2, borderColor: modified ? 'warning.main' : 'divider' }}>
        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr 1fr', lg: 'minmax(170px,1.35fr) repeat(5,minmax(105px,1fr))' }, gap: 1, alignItems: 'center' }}>
          <Box sx={{ gridColumn: { xs: '1 / -1', lg: 'auto' } }}>
            <Stack direction="row" gap={.6} alignItems="center"><Typography fontSize={12.5} fontWeight={900}>{item.play_name}</Typography><Chip size="small" variant="outlined" label={meta?.category || '玩法'} sx={{ height: 19, fontSize: 8.5 }} /></Stack>
            <Typography fontSize={9} color="text.secondary">{item.play_code}{meta?.example ? ` · 例：${meta.example}` : ''}</Typography>
          </Box>
          {fields.map(field => <TextField
            key={field.key}
            size="small"
            type="number"
            label={field.label}
            value={item[field.key]}
            onChange={event => update(index, { [field.key]: Number(event.target.value) })}
            inputProps={{ min: field.key === 'odds' ? 1.001 : 0, step: field.step ?? 1, 'aria-label': `${item.play_name}${field.label}` }}
          />)}
        </Box>
      </Paper>
    })}
  </Stack>
}
