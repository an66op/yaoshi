import { describe, expect, it } from 'vitest'
import { buildRecentRoomEntries, buildRoomEntries, DEFAULT_ROOM_LOGO } from './roomHistory'

describe('buildRecentRoomEntries', () => {
  it('keeps only real room codes, deduplicates them and sorts server history by recency', () => {
    const result = buildRecentRoomEntries([
      { code: '10001', name: '较早房间', status: 'available', lastUsedAt: 10 },
      { code: '10002', name: '当前房间', status: 'current', lastUsedAt: 1 },
      { code: 'bad-room', name: '无效房间', status: 'available', lastUsedAt: 99 },
      { code: '10001', name: '重复房间', status: 'available', lastUsedAt: 50 },
      { code: '10003', name: '最近房间', status: 'pending', lastUsedAt: 30 },
    ])

    expect(result.map(item => [item.code, item.name])).toEqual([
      ['10002', '当前房间'],
      ['10003', '最近房间'],
      ['10001', '较早房间'],
    ])
    expect(result.every(item => item.logo === DEFAULT_ROOM_LOGO)).toBe(true)
  })

  it('honors an empty or compact display limit without changing the source array', () => {
    const history = [
      { code: '10001', name: '房间一', status: 'available' as const, lastUsedAt: 10 },
      { code: '10002', name: '房间二', status: 'available' as const, lastUsedAt: 20 },
    ]
    expect(buildRecentRoomEntries(history, 0)).toEqual([])
    expect(buildRecentRoomEntries(history, 1).map(item => item.code)).toEqual(['10002'])
    expect(history).toEqual([
      { code: '10001', name: '房间一', status: 'available', lastUsedAt: 10 },
      { code: '10002', name: '房间二', status: 'available', lastUsedAt: 20 },
    ])
  })
})

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

  it('uses the classic room Logo for every empty history or current-room Logo', () => {
    const result = buildRoomEntries({ code: '88001', name: '当前房', logo: '' }, [
      { code: '10001', name: '待审核房', logo: '  ', status: 'pending' },
      { code: '10002', name: '停用房', status: 'disabled' },
      { code: '10003', name: '可进入房', logo: '', status: 'available' },
    ])
    expect(DEFAULT_ROOM_LOGO).toBe('/images/wangzhe-header-logo.png')
    expect(result).toHaveLength(4)
    expect(result.every(item => item.logo === DEFAULT_ROOM_LOGO)).toBe(true)
  })

  it('retains custom and built-in Logos without mutating server history', () => {
    const history = [
      { code: '10001', name: '自定义房', logo: 'data:image/png;base64,example', status: 'available' as const },
      { code: '10002', name: '预设房', logo: '/images/room-logos/crown-crystal.webp', status: 'pending' as const },
      { code: '10003', name: '默认房', logo: '', status: 'available' as const },
    ]
    const result = buildRoomEntries({ code: '88001', name: '当前房', logo: '/images/room-logos/crown-shield.webp' }, history)
    expect(result.map(item => item.logo)).toEqual(['/images/room-logos/crown-shield.webp', history[0].logo, history[1].logo, DEFAULT_ROOM_LOGO])
    expect(history[2].logo).toBe('')
  })

  it('uses the current room history Logo when the session omits it, but honors an explicit reset', () => {
    const history = [{ code: '88001', name: '当前房', logo: '/images/room-logos/crown-laurel.webp', status: 'available' as const }]
    expect(buildRoomEntries({ code: '88001', name: '当前房' }, history)[0].logo).toBe(history[0].logo)
    expect(buildRoomEntries({ code: '88001', name: '当前房', logo: '' }, history)[0].logo).toBe(DEFAULT_ROOM_LOGO)
  })
})
