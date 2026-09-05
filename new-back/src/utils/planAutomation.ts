import type { PlanAutomationPayload } from '../api'

export const canManagePlanAutomation = (role: string | undefined) => role === 'admin'

export const RACING_PLAN_GAME_IDS = [
  'speed-racing', 'speed-fly', 'sg-fly', 'fly-racing', 'au-lucky-10',
  'bingo-racing-a', 'bingo-racing-b',
] as const

export const hasRacingPlanGame = (gameIds: string[]) => gameIds.some(id => (RACING_PLAN_GAME_IDS as readonly string[]).includes(id))

type RacingSelection = { positions: number[]; plan_keys: string[] }

export function buildPlanAutomationPayload(workspaceId: number, enabled: boolean, gameIds: string[], racing?: RacingSelection): PlanAutomationPayload {
  if (!Number.isSafeInteger(workspaceId) || workspaceId <= 0) throw new Error('请先选择配置房间')
  const selectedGames = [...new Set(gameIds.map(id => id.trim()).filter(Boolean))]
  if (enabled && selectedGames.length === 0) throw new Error('开启自动推荐前，请至少选择一个彩种')
  const payload: PlanAutomationPayload = { workspace_id: workspaceId, enabled, mode: 'demo', game_ids: selectedGames }
  if (racing) {
    if (racing.positions.some(position => !Number.isSafeInteger(position) || position < 1 || position > 10)) throw new Error('名次必须为冠军至第十名')
    payload.positions = [...new Set(racing.positions)].sort((a, b) => a - b)
    payload.plan_keys = [...new Set(racing.plan_keys.map(key => key.trim()).filter(Boolean))]
    if (enabled && hasRacingPlanGame(selectedGames) && (!payload.positions.length || !payload.plan_keys.length)) throw new Error('赛车类彩种至少选择一个名次和一种计划')
  }
  return payload
}

export const hasPlanAutomationChanges = (
  saved: Pick<PlanAutomationPayload, 'enabled' | 'game_ids' | 'positions' | 'plan_keys'>,
  enabled: boolean,
  gameIds: string[],
  racing?: RacingSelection,
) => saved.enabled !== enabled || !sameSet(saved.game_ids, gameIds) || Boolean(racing && (
  !sameSet(saved.positions || [], racing.positions) || !sameSet(saved.plan_keys || [], racing.plan_keys)
))

function sameSet<T extends string | number>(left: T[], right: T[]): boolean {
  const a = new Set(left)
  const b = new Set(right)
  return a.size === b.size && [...a].every(value => b.has(value))
}
