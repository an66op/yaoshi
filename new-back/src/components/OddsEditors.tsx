import { Alert, Avatar, Box, Button, Checkbox, Chip, FormControlLabel, InputAdornment, MenuItem, Paper, Stack, TextField, Typography } from '@mui/material'
import KeyboardArrowRightRounded from '@mui/icons-material/KeyboardArrowRightRounded'
import RestartAltRounded from '@mui/icons-material/RestartAltRounded'
import { useMemo, useState } from 'react'
import type { AdminGame, PlayLimitItem } from '../api'
import { gameLogo } from '../gameLogos'
import { applyOddsBatch, oddsEditableFields, type OddsEditableField } from '../oddsEditing'

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
  dragon_tiger_tie: '龙虎',
  sum: '冠亚和',
  leopard: '形态',
  straight: '形态',
  pair: '形态',
  half_straight: '形态',
  mixed: '形态',
}

function categoryForPlay(playCode: string, playName = '') {
  if (/^sum_(?:big|small|odd|even|\d+)$/.test(playCode)) return '冠亚和'
  if (playCode === 'sum' && playName.includes('总和')) return '总和'
  return playCategory[playCode] || '其他玩法'
}

function oddsConfirmationLabel(item: PlayLimitItem, configured: boolean) {
  if (item.configuration_source === 'pending_admin_save') return '待保存确认'
  if (item.configuration_source === 'rule_version_mismatch') return '规则已变更'
  return configured ? '后台已确认' : '未配置 / 停用'
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
  // Selection may be refused by the parent while a draft or write is pending.
  // Only show a new category after the parent accepts its game selection.
  const category = selectedGame ? selectedGame.lobby_category?.trim() || '未分类' : categories[0] || '未分类'

  const gamesInCategory = availableGames.filter(game => (game.lobby_category?.trim() || '未分类') === category)

  return <Paper variant="outlined" sx={{ borderRadius: 1.25, overflow: 'hidden', bgcolor: 'background.paper' }}>
    <Stack direction="row" gap={.35} px={.75} pt={.6} sx={{ overflowX: 'auto', scrollbarWidth: 'thin' }}>
      {categories.map(item => {
        const selected = item === category
        return <Button
          key={item}
          size="small"
          variant={selected ? 'contained' : 'text'}
          onClick={() => {
            const first = availableGames.find(game => (game.lobby_category?.trim() || '未分类') === item)
            if (first && first.id !== gameId) onSelect(first.id)
          }}
          sx={{
            flex: '0 0 auto', minWidth: 68, minHeight: 30, px: 1, whiteSpace: 'nowrap', fontWeight: 850,
            borderRadius: '6px 6px 0 0', boxShadow: 'none',
          }}
        >{item}</Button>
      })}
    </Stack>
    <Stack direction="row" gap={.5} px={.75} py={.65} sx={{ overflowX: 'auto', borderTop: 1, borderColor: 'divider', scrollbarWidth: 'thin' }}>
      {gamesInCategory.map(game => {
        const selected = game.id === gameId
        return <Button
          key={game.id}
          onClick={() => onSelect(game.id)}
          variant={selected ? 'outlined' : 'text'}
          endIcon={selected ? <KeyboardArrowRightRounded /> : undefined}
          sx={{
            flex: '0 0 auto', minWidth: 112, minHeight: 36, justifyContent: 'flex-start', borderRadius: 1,
            px: .75, py: .45, color: selected ? 'primary.main' : 'text.primary',
            bgcolor: selected ? 'action.selected' : 'transparent', borderColor: selected ? 'primary.main' : 'transparent',
          }}
        >
          <Avatar src={gameLogo(game.id)} alt="" sx={{ width: 24, height: 24, mr: .65, bgcolor: 'action.hover' }}>{game.name.slice(0, 1)}</Avatar>
          <Box textAlign="left" minWidth={0}>
            <Typography fontSize={11.5} fontWeight={850} noWrap>{game.name}</Typography>
            <Typography fontSize={8.5} lineHeight={1.1} color="text.secondary" noWrap>{game.source_kind === 'official' ? '官方源' : game.source_kind === 'external' ? '外部源' : '平台彩'}</Typography>
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
      const category = categoryForPlay(item.play_code, item.play_name)
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

  return <Stack gap={.85}>
    <Stack direction="row" justifyContent="space-between" alignItems="center" gap={1}>
      <Box>
        <Typography fontSize={14} fontWeight={900}>{level === 'member' ? '会员单独赔率' : '当前游戏赔率'}</Typography>
        <Typography fontSize={10} color="text.secondary">留空即{inheritedLabel}，高亮项为单独设置。</Typography>
      </Box>
      <Button size="small" variant="text" startIcon={<RestartAltRounded />} onClick={() => onChange(items.map(item => ({
        ...item,
        override: null,
        has_override: false,
        effective: level === 'member' ? (item.room_odds ?? item.base_odds) : item.base_odds,
      })))}>全部继承</Button>
    </Stack>
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(2,minmax(0,1fr))', xl: 'repeat(3,minmax(0,1fr))' }, gap: .85, alignItems: 'start' }}>
      {groups.map(([category, rows]) => <Paper key={category} variant="outlined" sx={{ borderRadius: 1.1, overflow: 'hidden' }}>
        <Stack direction="row" alignItems="center" justifyContent="space-between" px={1} py={.55} bgcolor="action.hover">
          <Typography fontSize={11.5} fontWeight={900}>{category}</Typography>
          <Typography fontSize={9.5} color="text.secondary">{rows.length} 项</Typography>
        </Stack>
        {rows.map(({ item, index }) => {
          const base = level === 'member' ? (item.room_odds ?? item.base_odds) : item.base_odds
          return <Box key={item.play_code} sx={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(112px,145px)', gap: .75, alignItems: 'center', px: 1, py: .7, borderTop: 1, borderColor: 'divider', bgcolor: item.has_override ? 'action.selected' : 'background.paper' }}>
            <Box minWidth={0}>
              <Stack direction="row" gap={.45} alignItems="center">
                <Typography fontSize={11.5} fontWeight={850} noWrap>{item.play_name}</Typography>
                <Chip size="small" variant="outlined" label={`${baseLabel} ${base}`} sx={{ height: 18, fontSize: 8, '& .MuiChip-label': { px: .55 } }} />
              </Stack>
              <Typography fontSize={8.5} lineHeight={1.2} color="text.secondary" noWrap>{item.play_code}</Typography>
            </Box>
            <TextField
              fullWidth
              size="small"
              type="number"
              placeholder={inheritedLabel}
              value={item.has_override ? (item.override ?? '') : ''}
              onChange={event => update(index, event.target.value)}
              inputProps={{ min: 1.001, step: 0.001, 'aria-label': `${item.play_name}${level === 'member' ? '会员' : '房间'}赔率` }}
              slotProps={{ input: { endAdornment: <InputAdornment position="end">倍</InputAdornment> } }}
              sx={{ '& .MuiOutlinedInput-root': { borderRadius: 1 } }}
            />
          </Box>
        })}
      </Paper>)}
    </Box>
  </Stack>
}

export function PlatformOddsGrid({ items, catalog, onChange, disabled = false }: {
  items: PlayLimitItem[]
  catalog: Record<string, { category?: string; description?: string; example?: string } | undefined>
  onChange: (items: PlayLimitItem[]) => void
  disabled?: boolean
}) {
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('all')
  const [selected, setSelected] = useState<string[]>([])
  const [batchField, setBatchField] = useState<OddsEditableField>('odds')
  const [batchValue, setBatchValue] = useState('')
  const [batchError, setBatchError] = useState('')
  const categoryFor = (item: PlayLimitItem) => catalog[item.play_code]?.category || categoryForPlay(item.play_code, item.play_name)
  const categories = Array.from(new Set(items.map(categoryFor)))
  const visible = items.filter(item => (category === 'all' || categoryFor(item) === category) && `${item.play_code} ${item.play_name}`.toLowerCase().includes(query.trim().toLowerCase()))
  const selectedCodes = selected.filter(code => items.some(item => item.play_code === code))
  const allVisibleSelected = visible.length > 0 && visible.every(item => selectedCodes.includes(item.play_code))
  const someVisibleSelected = visible.some(item => selectedCodes.includes(item.play_code))
  const update = (code: string, patch: Partial<PlayLimitItem>) => {
    if (!disabled) onChange(items.map(item => item.play_code === code ? { ...item, ...patch } : item))
  }
  const applyBatch = () => {
    if (disabled || !selectedCodes.length) return
    try {
      onChange(applyOddsBatch(items, selectedCodes, batchField, batchValue))
      setBatchError('')
    } catch (reason) {
      setBatchError(reason instanceof Error ? reason.message : '批量数值不正确')
    }
  }
  return <Stack gap={.65}>
    <Paper variant="outlined" sx={{ p: 1, borderRadius: 1 }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} gap={.75}>
        <TextField size="small" label="搜索玩法" value={query} onChange={event => setQuery(event.target.value)} sx={{ flex: 1 }} />
        <TextField size="small" select label="玩法分类" value={category} onChange={event => setCategory(event.target.value)} sx={{ minWidth: 130 }}>
          <MenuItem value="all">全部分类</MenuItem>{categories.map(name => <MenuItem key={name} value={name}>{name}</MenuItem>)}
        </TextField>
      </Stack>
      <Stack direction="row" gap={.75} alignItems="center" flexWrap="wrap" mt={.8}>
        <FormControlLabel label="选择当前筛选" control={<Checkbox size="small" disabled={disabled || !visible.length} checked={allVisibleSelected} indeterminate={!allVisibleSelected && someVisibleSelected} onChange={(_, checked) => {
          const visibleCodes = new Set(visible.map(item => item.play_code))
          setSelected(checked ? Array.from(new Set([...selectedCodes, ...visibleCodes])) : selectedCodes.filter(code => !visibleCodes.has(code)))
        }} />} sx={{ mr: 0, '& .MuiFormControlLabel-label': { fontSize: 11 } }} />
        <Typography fontSize={11} color="text.secondary">已选 {selectedCodes.length} 项 / 当前显示 {visible.length} 项</Typography>
        <TextField size="small" select label="批量字段" value={batchField} onChange={event => setBatchField(event.target.value as OddsEditableField)} disabled={disabled} sx={{ minWidth: 165 }}>
          {oddsEditableFields.map(field => <MenuItem key={field.key} value={field.key}>{field.label}</MenuItem>)}
        </TextField>
        <TextField size="small" type="number" label="批量数值" value={batchValue} onChange={event => setBatchValue(event.target.value)} disabled={disabled} inputProps={{ min: 0, step: batchField === 'odds' ? .0001 : .01 }} sx={{ width: 135 }} />
        <Button size="small" variant="outlined" disabled={disabled || !selectedCodes.length || !batchValue.trim()} onClick={applyBatch}>应用到所选</Button>
      </Stack>
      <Typography fontSize={10} color="text.secondary" mt={.5}>批量操作只修改草稿，需点击“保存设置”后才生效。赔率最多四位小数，限额最多两位小数。</Typography>
      {batchError && <Alert severity="error" sx={{ mt: .75 }}>{batchError}</Alert>}
    </Paper>
    {visible.map(item => {
      const meta = catalog[item.play_code]
      const modified = item.configuration_source === 'pending_admin_save'
      const configured = item.configured === true && item.configuration_source === 'admin_save' && Number.isFinite(item.odds) && item.odds > 1
      const confirmationLabel = oddsConfirmationLabel(item, configured)
      return <Paper key={item.play_code} variant="outlined" sx={{ p: .75, borderRadius: 1, borderColor: modified ? 'warning.main' : !configured ? 'error.light' : 'divider' }}>
        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr 1fr', lg: 'minmax(155px,1.2fr) repeat(5,minmax(92px,1fr))' }, gap: .65, alignItems: 'center' }}>
          <Box sx={{ gridColumn: { xs: '1 / -1', lg: 'auto' } }}>
            <Stack direction="row" gap={.4} alignItems="center" flexWrap="wrap"><Checkbox size="small" checked={selectedCodes.includes(item.play_code)} disabled={disabled} inputProps={{ 'aria-label': `选择${item.play_name}` }} onChange={(_, checked) => setSelected(checked ? [...selectedCodes, item.play_code] : selectedCodes.filter(code => code !== item.play_code))} sx={{ p: .25 }} /><Typography fontSize={12.5} fontWeight={900}>{item.play_name}</Typography><Chip size="small" variant="outlined" label={meta?.category || categoryForPlay(item.play_code, item.play_name)} sx={{ height: 19, fontSize: 8.5 }} /><Chip size="small" color={modified ? 'warning' : configured ? 'success' : 'error'} variant="outlined" label={confirmationLabel} sx={{ height: 19, fontSize: 8.5 }} /></Stack>
            <Typography fontSize={9} color="text.secondary">{item.play_code}{meta?.example ? ` · 例：${meta.example}` : ''}</Typography>
          </Box>
          {oddsEditableFields.map(field => <TextField
            key={field.key}
            size="small"
            type="number"
            label={field.label}
            value={item[field.key]}
            disabled={disabled}
            onChange={event => {
              const next = Number(event.target.value)
              update(item.play_code, { [field.key]: next })
            }}
            inputProps={{ min: 0, step: field.step, 'aria-label': `${item.play_name}${field.label}` }}
            sx={{ '& .MuiOutlinedInput-root': { borderRadius: 1 } }}
          />)}
        </Box>
      </Paper>
    })}
    {!visible.length && <Typography color="text.secondary" textAlign="center" py={3}>没有符合筛选条件的玩法</Typography>}
  </Stack>
}
