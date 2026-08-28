import MeetingRoomRounded from '@mui/icons-material/MeetingRoomRounded'
import VerifiedUserRounded from '@mui/icons-material/VerifiedUserRounded'
import { Alert, Avatar, Box, Button, Chip, Dialog, DialogActions, DialogContent, DialogTitle, Divider, FormControlLabel, Stack, Switch, Typography } from '@mui/material'
import { useEffect, useMemo, useState } from 'react'
import { adminApi, type SystemSettings, type WorkspaceGame } from '../api'
import { gameLogo } from '../gameLogos'

type Props = {
  open: boolean
  title: string
  target: { kind: 'tenant' | 'agent'; id: number } | null
  onClose: () => void
  onSaved: (message: string, severity?: 'success' | 'error') => void
}

export function RoomOperationsDialog({ open, title, target, onClose, onSaved }: Props) {
  const [settings, setSettings] = useState<SystemSettings | null>(null)
  const [games, setGames] = useState<WorkspaceGame[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [changingGame, setChangingGame] = useState('')
  const [error, setError] = useState('')
  const targetKind = target?.kind
  const targetID = target?.id

  useEffect(() => {
    if (!open || !targetKind || !targetID) return
    let active = true
    const timer = window.setTimeout(() => {
      if (!active) return
      setLoading(true); setSettings(null); setGames([]); setError('')
      const settingsRequest = targetKind === 'tenant' ? adminApi.tenantRoomSettings(targetID) : adminApi.agentRoomSettings(targetID)
      const gamesRequest = targetKind === 'tenant' ? adminApi.tenantRoomGames(targetID) : adminApi.agentRoomGames(targetID)
      void Promise.all([settingsRequest, gamesRequest]).then(([nextSettings, nextGames]) => {
        if (!active) return
        setSettings(nextSettings); setGames(nextGames)
      }).catch(reason => {
        if (active) setError(reason instanceof Error ? reason.message : '读取房间经营配置失败')
      }).finally(() => { if (active) setLoading(false) })
    }, 0)
    return () => { active = false; window.clearTimeout(timer) }
  }, [open, targetID, targetKind])

  const activeGames = useMemo(() => games.filter(game => game.platform_enabled && game.room_enabled).length, [games])
  const save = async () => {
    if (!settings || !target) return
    setSaving(true); setError('')
    try {
      const result = target.kind === 'tenant' ? await adminApi.updateTenantRoomSettings(target.id, settings) : await adminApi.updateAgentRoomSettings(target.id, settings)
      setSettings(result)
      onSaved('房间经营配置已保存')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '保存房间经营配置失败') }
    finally { setSaving(false) }
  }
  const toggleGame = async (game: WorkspaceGame, enabled: boolean) => {
    if (!target) return
    setChangingGame(game.id); setError('')
    try {
      if (target.kind === 'tenant') await adminApi.setTenantRoomGameStatus(target.id, game.id, enabled)
      else await adminApi.setAgentRoomGameStatus(target.id, game.id, enabled)
      setGames(current => current.map(item => item.id === game.id ? { ...item, room_enabled: enabled, enabled: item.platform_enabled && enabled } : item))
    } catch (reason) { setError(reason instanceof Error ? reason.message : '保存游戏状态失败') }
    finally { setChangingGame('') }
  }

  return <Dialog open={open} onClose={() => !saving && onClose()} fullWidth maxWidth="md">
    <DialogTitle>{title}</DialogTitle>
    <DialogContent dividers>
      {error && <Alert severity="error" sx={{ mb: 1.5 }}>{error}</Alert>}
      {loading ? <Box py={7} textAlign="center"><Typography color="text.secondary">正在读取配置…</Typography></Box> : settings && <Stack gap={2}>
        <Box display="grid" gridTemplateColumns={{ xs: '1fr', sm: '1fr 1fr' }} gap={1.2}>
          <Box sx={{ p: 1.5, border: 1, borderColor: settings.room_enabled ? 'success.light' : 'divider', borderRadius: 2.5, bgcolor: settings.room_enabled ? 'success.50' : 'action.hover' }}>
            <Stack direction="row" alignItems="center" justifyContent="space-between" gap={1}>
              <Stack direction="row" alignItems="center" gap={1}><MeetingRoomRounded color={settings.room_enabled ? 'success' : 'disabled'} /><Box><Typography fontWeight={850}>房间营业</Typography><Typography variant="caption" color="text.secondary">关闭后停止入房、聊天和下注</Typography></Box></Stack>
              <Switch checked={settings.room_enabled} onChange={(_, checked) => setSettings(current => current ? { ...current, room_enabled: checked } : current)} />
            </Stack>
          </Box>
          <Box sx={{ p: 1.5, border: 1, borderColor: 'divider', borderRadius: 2.5 }}>
            <Stack direction="row" alignItems="center" justifyContent="space-between" gap={1}>
              <Stack direction="row" alignItems="center" gap={1}><VerifiedUserRounded color="primary" /><Box><Typography fontWeight={850}>入房审核</Typography><Typography variant="caption" color="text.secondary">会员申请通过后才能进入</Typography></Box></Stack>
              <Switch checked={settings.require_join_review} onChange={(_, checked) => setSettings(current => current ? { ...current, require_join_review: checked } : current)} />
            </Stack>
          </Box>
        </Box>
        <Divider />
        <Stack direction="row" alignItems="center" justifyContent="space-between"><Typography fontWeight={900}>房间游戏</Typography><Chip size="small" color="primary" label={`${activeGames}/${games.length} 开放`} /></Stack>
        <Box display="grid" gridTemplateColumns={{ xs: '1fr', sm: '1fr 1fr' }} gap={1}>
          {games.map(game => <Stack key={game.id} direction="row" alignItems="center" gap={1.2} sx={{ minWidth: 0, p: 1.1, border: 1, borderColor: 'divider', borderRadius: 2, opacity: game.platform_enabled ? 1 : .55 }}>
            <Avatar src={gameLogo(game.id)} sx={{ width: 38, height: 38, bgcolor: 'action.hover', color: 'text.secondary', fontSize: 10 }}>{game.name.slice(0, 2)}</Avatar>
            <Box minWidth={0} flex={1}><Typography fontWeight={800} fontSize={12} noWrap>{game.name}</Typography><Typography variant="caption" color="text.secondary">{game.platform_enabled ? (game.room_enabled ? '本房开放' : '本房关闭') : '平台已关闭'}</Typography></Box>
            <FormControlLabel sx={{ m: 0 }} control={<Switch size="small" checked={game.platform_enabled && game.room_enabled} disabled={!game.platform_enabled || changingGame === game.id} onChange={(_, checked) => void toggleGame(game, checked)} />} label="" />
          </Stack>)}
        </Box>
      </Stack>}
    </DialogContent>
    <DialogActions><Button onClick={onClose}>关闭</Button><Button variant="contained" disabled={!settings || saving} onClick={() => void save()}>{saving ? '保存中…' : '保存营业与审核'}</Button></DialogActions>
  </Dialog>
}
