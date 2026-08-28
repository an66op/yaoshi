import { describe, expect, it } from 'vitest'
import { buildRoomEntries } from './roomHistory'

describe('buildRoomEntries', () => {
  it('always includes the verified current room at the top', () => {
    expect(buildRoomEntries({ code: '88001', name: '永生' }, [])).toEqual([
      expect.objectContaining({ code: '88001', name: '永生', status: 'current' }),
    ])
  })

  it('deduplicates the current room and keeps other rooms in recent order', () => {
    const result = buildRoomEntries({ code: '88001', name: '新房间名' }, [
      { code: '88001', name: '旧房间名', status: 'available', lastUsedAt: 1 },
      { code: '10002', name: '待审核房', status: 'pending', lastUsedAt: 30 },
      { code: '10001', name: '历史房', status: 'available', lastUsedAt: 20 },
      { code: 'invalid', name: '无效数据', status: 'available', lastUsedAt: 99 },
    ])

    expect(result.map((item) => [item.code, item.status])).toEqual([
      ['88001', 'current'],
      ['10002', 'pending'],
      ['10001', 'available'],
    ])
    expect(result[0].name).toBe('新房间名')
  })
})
