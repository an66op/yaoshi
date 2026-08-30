import { adminApi, type AgentItem, type TenantItem } from '../api'

export type PlanWorkspaceOption = { id: number; label: string; ownerId: number; type: 'tenant' | 'agent' }

async function allPages<T>(fetchPage: (params: { page: number; pageSize: number }) => Promise<{ items: T[]; total: number }>): Promise<T[]> {
  const items: T[] = []
  for (let page = 1; ; page += 1) {
    const response = await fetchPage({ page, pageSize: 100 })
    const rows = Array.isArray(response.items) ? response.items : []
    items.push(...rows)
    if (rows.length === 0 || items.length >= response.total) return items
  }
}

export async function loadPlanWorkspaces(): Promise<PlanWorkspaceOption[]> {
  const [tenants, agents] = await Promise.all([allPages<TenantItem>(adminApi.tenants), allPages<AgentItem>(adminApi.agents)])
  const options: PlanWorkspaceOption[] = [
    ...tenants.filter(item => item.workspace_id).map(item => ({ id: item.workspace_id, ownerId: item.id, type: 'tenant' as const, label: `租户直属 · ${item.room_code || '未分配'} · ${item.room_name || item.nickname || item.username}` })),
    ...agents.filter(item => item.workspace_id).map(item => ({ id: item.workspace_id, ownerId: item.id, type: 'agent' as const, label: `代理房间 · ${item.room_code || '未分配'} · ${item.room_name || item.nickname || item.username}` })),
  ]
  return [...new Map(options.map(item => [item.id, item])).values()]
}

export const loadPlanWorkspaceGames = (workspace: PlanWorkspaceOption) => workspace.type === 'tenant'
  ? adminApi.tenantRoomGames(workspace.ownerId)
  : adminApi.agentRoomGames(workspace.ownerId)
