import { Alert, Avatar, Box, Button, Card, CardContent, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, Stack, Switch, TextField, Typography } from '@mui/material'
import PlayArrowRounded from '@mui/icons-material/PlayArrowRounded'
import SmartToyRounded from '@mui/icons-material/SmartToyRounded'
import EditRounded from '@mui/icons-material/EditRounded'
import RestartAltRounded from '@mui/icons-material/RestartAltRounded'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { agentApi, tenantApi, type AdminUser, type RobotResetInput, type RobotSetting, type WorkspaceGame } from '../api'
import { getStoredUser } from '../auth'
import { PageHeader } from '../components/PageHeader'
import { RobotResetDialog } from '../components/RobotResetDialog'
import { useFeedback } from '../components/feedback'

export function WorkspaceRobotsPage() {
  const role = getStoredUser()?.role ?? 'agent'
  const api = useMemo(() => role === 'tenant' ? tenantApi : agentApi, [role])
  const { showMessage } = useFeedback()
  const [setting, setSetting] = useState<RobotSetting | null>(null)
	const [robots, setRobots] = useState<AdminUser[]>([])
	const [games, setGames] = useState<WorkspaceGame[]>([])
	const [editing, setEditing] = useState<AdminUser | null>(null)
	const [nickname, setNickname] = useState('')
	const [enabled, setEnabled] = useState(true)
	const [activeStart, setActiveStart] = useState('')
	const [activeEnd, setActiveEnd] = useState('')
	const [minBet, setMinBet] = useState('')
	const [maxBet, setMaxBet] = useState('')
	const [gameIDs, setGameIDs] = useState<string[]>([])
  const [resetOpen, setResetOpen] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
		const [nextSetting, nextRobots, nextGames] = await Promise.all([api.robotSetting(), api.robots(), api.games()])
		setSetting(nextSetting)
		setRobots(nextRobots.items)
		setGames(nextGames.filter(game => game.enabled))
	}
    catch (reason) { setError(reason instanceof Error ? reason.message : '读取机器人设置失败') }
    finally { setLoading(false) }
  }, [api])

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => window.clearTimeout(timer)
  }, [load])
  const save = async () => {
    if (!setting) return
    setSaving(true); setError('')
    try {
      setSetting(await api.updateRobotSetting({
        enabled: setting.enabled, interval_secs: setting.interval_secs, bets_per_cycle: setting.bets_per_cycle,
        daily_bet_limit: setting.daily_bet_limit, max_pending_bets: setting.max_pending_bets,
      }))
      showMessage('本房间机器人设置已保存')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '保存失败') }
    finally { setSaving(false) }
  }
  const runOnce = async () => {
    setSaving(true); setError('')
    try { await api.runRobotOnce(); showMessage('已执行一轮房间机器人任务'); await load() }
    catch (reason) { setError(reason instanceof Error ? reason.message : '执行失败') }
    finally { setSaving(false) }
  }
	const openEdit = (robot: AdminUser) => {
		setEditing(robot)
		setNickname(robot.nickname || robot.username)
		setEnabled(robot.status === 1)
		setActiveStart(robot.robot_active_start ?? '')
		setActiveEnd(robot.robot_active_end ?? '')
		setMinBet(robot.robot_min_bet ? String(robot.robot_min_bet) : '')
		setMaxBet(robot.robot_max_bet ? String(robot.robot_max_bet) : '')
		const enabledGameIDs = new Set(games.map(game => game.id))
		setGameIDs((robot.robot_game_ids ?? []).filter(id => enabledGameIDs.has(id)))
	}
	const saveRobot = async () => {
		if (!editing || !nickname.trim()) return
		setSaving(true); setError('')
		try {
			const updated = await api.updateRobot(editing.id, {
				nickname: nickname.trim(), avatar: editing.robot_avatar, status: enabled ? 1 : 0,
				game_ids: gameIDs, active_start: activeStart, active_end: activeEnd,
				min_bet: Number(minBet) || 0, max_bet: Number(maxBet) || 0,
			})
			setRobots(current => current.map(item => item.id === updated.id ? updated : item))
			setEditing(null)
			showMessage('本房间机器人已更新')
		} catch (reason) { setError(reason instanceof Error ? reason.message : '保存机器人失败') }
		finally { setSaving(false) }
	}
	const resetRobots = async (payload: RobotResetInput) => {
		setSaving(true); setError('')
		try {
			const result = await api.resetRobots(payload)
			setRobots(result.items)
			setResetOpen(false)
			showMessage(result.duplicate ? '该次重置已处理，无需重复执行' : `已重置当前房间 ${result.count} 个机器人`)
		} catch (reason) {
			const message = reason instanceof Error ? reason.message : '批量重置机器人失败'
			setError(message); showMessage(message, 'error')
		}
		finally { setSaving(false) }
	}

  return <Box p={{ xs: 1.5, md: 2.5 }}>
    <PageHeader eyebrow="会员与自动化" title="机器人管理" description="" />
    {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
    <Card sx={{ mt: 2 }}><CardContent sx={{ p: { xs: 2, md: 2.5 } }}>
      <Stack direction="row" alignItems="center" gap={1.25} mb={2} flexWrap="wrap">
        <Box sx={{ width: 44, height: 44, borderRadius: 2.5, display: 'grid', placeItems: 'center', color: 'white', bgcolor: 'primary.main' }}><SmartToyRounded /></Box>
		<Box flex={1}><Typography fontWeight={850}>房间自动下注</Typography><Typography variant="caption" color="text.secondary">账号、下注和运行记录只属于当前房间 · 今日 {setting?.today_bets ?? 0}/{setting?.daily_bet_limit ?? 200} · 待结 {setting?.pending_bets ?? 0}/{setting?.max_pending_bets ?? 50}</Typography></Box>
        <Button color="warning" size="small" variant="outlined" startIcon={<RestartAltRounded />} disabled={loading || saving || !robots.length} onClick={() => setResetOpen(true)}>一键重置昵称与余额</Button>
        <Chip label={setting?.enabled ? '运行中' : '已停用'} color={setting?.enabled ? 'success' : 'default'} />
      </Stack>
      {loading || !setting ? <CircularProgress size={22} /> : <Stack gap={2}>
        <FormControlLabel control={<Switch checked={setting.enabled} onChange={event => setSetting(current => current ? { ...current, enabled: event.target.checked } : current)} />} label="开启本房间机器人" />
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}>
          <TextField fullWidth type="number" label="执行间隔（秒）" value={setting.interval_secs} onChange={event => setSetting(current => current ? { ...current, interval_secs: Number(event.target.value) } : current)} slotProps={{ htmlInput: { min: 30, max: 3600 } }} helperText="范围 30–3600 秒" />
          <TextField fullWidth type="number" label="每轮下注数量" value={setting.bets_per_cycle} onChange={event => setSetting(current => current ? { ...current, bets_per_cycle: Number(event.target.value) } : current)} slotProps={{ htmlInput: { min: 1, max: 20 } }} helperText="范围 1–20 笔" />
        </Stack>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}>
          <TextField fullWidth type="number" label="每日下注上限" value={setting.daily_bet_limit} onChange={event => setSetting(current => current ? { ...current, daily_bet_limit: Number(event.target.value) } : current)} slotProps={{ htmlInput: { min: 1, max: 10000 } }} helperText="达到上限后当天自动暂停" />
          <TextField fullWidth type="number" label="待结注单保护线" value={setting.max_pending_bets} onChange={event => setSetting(current => current ? { ...current, max_pending_bets: Number(event.target.value) } : current)} slotProps={{ htmlInput: { min: 1, max: 5000 } }} helperText="超过后等待结算恢复" />
        </Stack>
        <Stack direction="row" gap={1} flexWrap="wrap">
          <Button variant="contained" disabled={saving} onClick={() => void save()}>{saving ? '保存中…' : '保存设置'}</Button>
          <Button variant="outlined" startIcon={<PlayArrowRounded />} disabled={saving} onClick={() => void runOnce()}>立即执行一轮</Button>
        </Stack>
        <Typography variant="caption" color={setting.last_error ? 'error.main' : 'text.secondary'}>最近执行：{setting.last_run_at ? new Date(setting.last_run_at).toLocaleString('zh-CN', { hour12: false }) : '尚未执行'}{setting.pause_reason ? ` · ${setting.pause_reason}` : setting.last_error ? ` · ${setting.last_error}` : ''}</Typography>
      </Stack>}
    </CardContent></Card>
	<Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2,1fr)', xl: 'repeat(3,1fr)' }, gap: 1.25, mt: 1.5 }}>
		{robots.map(robot => <Card key={robot.id} variant="outlined"><CardContent sx={{ p: '15px !important' }}><Stack direction="row" gap={1.2} alignItems="center"><Avatar src={robot.robot_avatar} sx={{ width: 44, height: 44, bgcolor: 'primary.main' }}>{(robot.nickname || robot.username).slice(0, 1)}</Avatar><Box flex={1} minWidth={0}><Stack direction="row" gap={.7} alignItems="center"><Typography fontWeight={850} noWrap>{robot.nickname || robot.username}</Typography><Chip size="small" color={robot.status === 1 ? 'success' : 'default'} label={robot.status === 1 ? '启用' : '停用'} sx={{ height: 20, fontSize: 10 }} /></Stack><Typography variant="caption" color="text.secondary" noWrap>ID {robot.public_id} · 积分 {robot.balance.toFixed(2)}</Typography></Box><Button size="small" startIcon={<EditRounded />} onClick={() => openEdit(robot)}>配置</Button></Stack><Typography display="block" mt={1.2} variant="caption" color="text.secondary">{robot.robot_active_start && robot.robot_active_end ? `${robot.robot_active_start}–${robot.robot_active_end}` : '全天运行'} · {robot.robot_min_bet && robot.robot_max_bet ? `${robot.robot_min_bet}–${robot.robot_max_bet}/注` : '默认金额'} · {robot.robot_game_ids?.length ? `${robot.robot_game_ids.length} 个彩种` : '跟随全部开放彩种'}</Typography></CardContent></Card>)}
		{!loading && !robots.length && <Alert severity="info">当前房间暂无机器人账号；重启后端完成增量预置后即可配置。</Alert>}
	</Box>
	<Dialog open={Boolean(editing)} onClose={() => !saving && setEditing(null)} fullWidth maxWidth="sm"><DialogTitle>配置房间机器人</DialogTitle><DialogContent><Stack gap={1.5} pt={1}><Stack direction="row" gap={1.2} alignItems="center"><Avatar src={editing?.robot_avatar} sx={{ width: 48, height: 48 }} /><TextField fullWidth label="显示昵称" value={nickname} onChange={event => setNickname(event.target.value)} inputProps={{ maxLength: 50 }} /></Stack><FormControlLabel control={<Switch checked={enabled} onChange={event => setEnabled(event.target.checked)} />} label={enabled ? '允许参与本房间自动下注' : '已停用该机器人'} /><Stack direction={{ xs: 'column', sm: 'row' }} gap={1}><TextField fullWidth type="time" label="运行开始" value={activeStart} onChange={event => setActiveStart(event.target.value)} slotProps={{ inputLabel: { shrink: true } }} helperText="留空为全天" /><TextField fullWidth type="time" label="运行结束" value={activeEnd} onChange={event => setActiveEnd(event.target.value)} slotProps={{ inputLabel: { shrink: true } }} helperText="支持跨午夜" /></Stack><Stack direction={{ xs: 'column', sm: 'row' }} gap={1}><TextField fullWidth type="number" label="最小单注" value={minBet} onChange={event => setMinBet(event.target.value)} slotProps={{ htmlInput: { min: 0, step: .01 } }} /><TextField fullWidth type="number" label="最大单注" value={maxBet} onChange={event => setMaxBet(event.target.value)} slotProps={{ htmlInput: { min: 0, step: .01 } }} /></Stack><Box><Stack direction="row" alignItems="center" justifyContent="space-between" mb={1}><Box><Typography fontSize={13} fontWeight={850}>参与彩种</Typography><Typography fontSize={10} color="text.secondary">仅显示本房间已开放彩种；不选则跟随全部开放彩种</Typography></Box><Button size="small" onClick={() => setGameIDs([])}>跟随全部</Button></Stack><Stack direction="row" gap={.7} flexWrap="wrap" useFlexGap>{games.map(game => { const selected = gameIDs.includes(game.id); return <Chip key={game.id} clickable color={selected ? 'primary' : 'default'} variant={selected ? 'filled' : 'outlined'} label={game.name} onClick={() => setGameIDs(current => current.includes(game.id) ? current.filter(id => id !== game.id) : [...current, game.id])} /> })}{!games.length && <Typography variant="caption" color="text.secondary">当前房间暂无开放彩种</Typography>}</Stack></Box></Stack></DialogContent><DialogActions><Button onClick={() => setEditing(null)}>取消</Button><Button variant="contained" disabled={saving || !nickname.trim()} onClick={() => void saveRobot()}>保存</Button></DialogActions></Dialog>
	{resetOpen && <RobotResetDialog open robotCount={robots.length} scopeLabel="当前房间" submitting={saving} onClose={() => setResetOpen(false)} onSubmit={resetRobots} />}
  </Box>
}
