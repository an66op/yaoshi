import { beforeEach, describe, expect, it, vi } from 'vitest'
import { adminApi, type AgentItem, type AgentListResponse, type TenantItem, type TenantListResponse } from '../api'
import { loadPlanWorkspaces, loadPlanWorkspaceGames } from './planWorkspaces'

vi.mock('../api', () => ({ adminApi: { tenants: vi.fn(), agents: vi.fn(), tenantRoomGames: vi.fn(), agentRoomGames: vi.fn() } }))

const tenant = (id: number, workspaceId = id + 1000) => ({ id, workspace_id: workspaceId, room_code: String(id), room_name: `房间 ${id}` }) as TenantItem
const tenantList = (items: TenantItem[], total = items.length): TenantListResponse => ({ items, total, page: 1, page_size: 100, active: 0, agents: 0, members: 0 })
const agentList = (items: AgentItem[], total = items.length): AgentListResponse => ({ items, total, page: 1, page_size: 100, summary: { total, active: 0, disabled: 0, members: 0 } })

beforeEach(() => vi.resetAllMocks())

describe('plan management workspace scope', () => {
  it('loads all room pages instead of silently stopping at the first 100 accounts', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => tenant(index + 1))
    vi.mocked(adminApi.tenants)
      .mockResolvedValueOnce(tenantList(firstPage, 101))
      .mockResolvedValueOnce(tenantList([tenant(101)], 101))
    vi.mocked(adminApi.agents).mockResolvedValue(agentList([]))

    const rooms = await loadPlanWorkspaces()

    expect(rooms).toHaveLength(101)
    expect(rooms.at(-1)).toMatchObject({ id: 1101, ownerId: 101, type: 'tenant' })
    expect(adminApi.tenants).toHaveBeenNthCalledWith(2, { page: 2, pageSize: 100 })
  })

  it('excludes accounts without a workspace and deduplicates repeated workspace IDs', async () => {
    vi.mocked(adminApi.tenants).mockResolvedValue(tenantList([tenant(1, 0), tenant(2, 72)]))
    const agent = { id: 3, workspace_id: 72, room_code: 'AG', room_name: '代理房间' } as AgentItem
    vi.mocked(adminApi.agents).mockResolvedValue(agentList([agent]))

    const rooms = await loadPlanWorkspaces()

    expect(rooms).toEqual([{ id: 72, ownerId: 3, type: 'agent', label: '代理房间 · AG · 代理房间' }])
  })

  it.each(['tenant', 'agent'] as const)('fetches %s game candidates using the room owner, not the workspace ID', async type => {
    await loadPlanWorkspaceGames({ id: 901, ownerId: 37, type, label: '测试房间' })
    expect(type === 'tenant' ? adminApi.tenantRoomGames : adminApi.agentRoomGames).toHaveBeenCalledWith(37)
    expect(type === 'tenant' ? adminApi.agentRoomGames : adminApi.tenantRoomGames).not.toHaveBeenCalled()
  })
})
