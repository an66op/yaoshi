import { readFileSync } from 'node:fs'
import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from '../test/hookHarness'
import { DEFAULT_ROOM_LOGO } from '../utils/roomHistory'
import { RoomEntry } from './RoomEntry'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, joinRoom: vi.fn() }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
}))
vi.mock('../api/member', () => ({ memberApi: { joinRoom: runtime.joinRoom } }))

type ElementProps = {
  children?: ReactNode
  className?: string
  disabled?: boolean
  id?: string
  role?: string
  src?: string
  value?: string
  'aria-current'?: string
  'aria-label'?: string
  onClick?: () => void
}
type Element = ReactElement<ElementProps>

function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<ElementProps>(node)) return []
  return [node, ...elements(node.props.children)]
}

function text(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(text).join('')
  if (isValidElement<ElementProps>(node)) return text(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}

const hasClass = (element: Element, name: string) => element.props.className?.split(/\s+/).includes(name)
const withClass = (node: ReactNode, name: string) => elements(node).find(element => hasClass(element, name))
const cards = (node: ReactNode) => elements(node).filter(element => element.type === 'button' && hasClass(element, 'room-entry-history-card'))
const card = (node: ReactNode, code: string) => cards(node).find(element => text(element).includes(`ROOM · ${code}`))!

describe('room entry history lifecycle', () => {
  let props: ComponentProps<typeof RoomEntry>
  const render = (updates: Partial<ComponentProps<typeof RoomEntry>> = {}) => {
    props = { ...props, ...updates }
    return runtime.hooks!.render(() => RoomEntry(props))
  }
  const settle = async () => {
    for (let index = 0; index < 6; index++) await Promise.resolve()
    return render()
  }

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    runtime.joinRoom.mockReset().mockResolvedValue({ status: 'joined', room_code: '10002', room_name: '最近房间' })
    props = {
      onBack: vi.fn(),
      onEnter: vi.fn(),
      theme: 'day',
      roomHistory: [
        { code: '10001', name: '较早房间', status: 'available', lastUsedAt: 10 },
        { code: '10004', name: '当前房间', status: 'current', lastUsedAt: 1 },
        { code: '10002', name: '最近房间', status: 'available', logo: '/images/recent-room.png', lastUsedAt: 40 },
        { code: '10003', name: '审核房间', status: 'pending', lastUsedAt: 30 },
        { code: '10005', name: '停用房间', status: 'disabled', lastUsedAt: 20 },
        { code: 'not-a-room', name: '无效房间', status: 'available', lastUsedAt: 99 },
      ],
    }
  })
  afterEach(() => runtime.hooks?.unmount())

  it('renders only verified server history in current-first and recent-first order', () => {
    const root = render()
    expect(cards(root).map(item => text(item).match(/ROOM · (\d+)/)?.[1])).toEqual(['10004', '10002', '10003', '10005', '10001'])
    expect(text(card(root, '10004'))).toContain('当前')
    expect(text(card(root, '10002'))).toContain('最近')
    expect(text(card(root, '10003'))).toContain('待审核')
    expect(text(card(root, '10005'))).toContain('已停用')
    expect(card(root, '10004').props['aria-current']).toBe('true')
    expect(card(root, '10005').props.disabled).toBe(true)
    expect(elements(card(root, '10002')).find(item => item.type === 'img')?.props.src).toBe('/images/recent-room.png')
    expect(elements(card(root, '10001')).find(item => item.type === 'img')?.props.src).toBe(DEFAULT_ROOM_LOGO)
  })

  it('does not invent a local history section when the authenticated API has no rows', () => {
    const root = render({ roomHistory: [] })
    expect(withClass(root, 'room-entry-history')).toBeUndefined()
    expect(cards(root)).toEqual([])
  })

  it('enters a history room with the same guarded join flow and locks duplicate taps', async () => {
    const root = render()
    card(root, '10002').props.onClick!()
    card(root, '10002').props.onClick!()
    expect(runtime.joinRoom).toHaveBeenCalledExactlyOnceWith('10002')
    expect(cards(render()).every(item => item.props.disabled)).toBe(true)
    await settle()
    expect(props.onEnter).toHaveBeenCalledExactlyOnceWith('10002', '最近房间')
    expect(elements(render()).find(item => item.type === 'input' && item.props.id === 'room-entry-code')?.props.value).toBe('10002')
  })

  it('keeps a pending history room on the entry page and reports the real application status', async () => {
    runtime.joinRoom.mockResolvedValue({ status: 'pending', room_code: '10003', room_name: '审核房间', application_id: 76 })
    const root = render()
    card(root, '10003').props.onClick!()
    const pending = await settle()
    expect(props.onEnter).not.toHaveBeenCalled()
    expect(text(withClass(pending, 'room-entry-success'))).toContain('入房申请已提交（编号 76）')
    expect(text(withClass(pending, 'room-entry-success'))).toContain('10003')
    expect(card(pending, '10003').props.disabled).toBe(false)
  })
})

describe('room entry compact-history CSS contract', () => {
  const css = readFileSync(new URL('../room-entry.css', import.meta.url), 'utf8')
  const rules = [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)].map(match => ({ selector: match[1].trim(), declarations: match[2] }))
  const declarations = (selector: string) => rules.filter(rule => rule.selector === selector).map(rule => rule.declarations).join('\n')

  it('uses one compact horizontal row of 104px history cards', () => {
    const list = declarations('.room-entry-history-list')
    expect(list).toMatch(/display\s*:\s*flex\s*;/)
    expect(list).toMatch(/flex-wrap\s*:\s*nowrap\s*;/)
    expect(list).toMatch(/overflow-x\s*:\s*auto\s*;/)
    const cardRule = declarations('.room-entry-history-card')
    expect(cardRule).toMatch(/flex\s*:\s*0\s+0\s+104px\s*;/)
    expect(cardRule).toMatch(/height\s*:\s*104px\s*;/)
  })

  it('keeps the code-only surface above center and tempers the history rail natural recentering', () => {
    expect(declarations('.room-entry-shell')).toMatch(/translateY\(clamp\(-48px,\s*-5\.5dvh,\s*-28px\)\)/)
    expect(declarations('.room-entry-shell:has(.room-entry-history)')).toMatch(/translateY\(clamp\(12px,\s*2\.2dvh,\s*20px\)\)/)
  })
})
