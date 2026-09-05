import { readFileSync } from 'node:fs'
import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Icon } from '../components/Icon'
import { AnnouncementDialog } from '../components/Dialogs'
import { HookHarness } from '../test/hookHarness'
import { DEFAULT_ROOM_LOGO } from '../utils/roomHistory'
import { Lobby } from './Lobby'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, roomSettings: vi.fn(), portal: vi.fn() }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(factory, dependencies),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('react-dom', () => ({ createPortal: (children: ReactNode, target: unknown) => { runtime.portal(children, target); return children } }))
vi.mock('../api/portal', () => ({ portalApi: { roomSettings: runtime.roomSettings } }))

type Props = ComponentProps<typeof Lobby>
type ImageTarget = {
  src: string
  getAttribute: (name: string) => string | null
  setAttribute: (name: string, value: string) => void
  onerror: null | (() => void)
}
type ElementProps = {
  children?: ReactNode; className?: string; disabled?: boolean; role?: string
  src?: string; alt?: string; draggable?: boolean; name?: string
  'aria-current'?: string; 'aria-label'?: string
  onClick?: () => void
  onError?: (event: { currentTarget: ImageTarget }) => void
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
const withClass = (node: ReactNode, name: string) => elements(node).find(element => hasClass(element, name))!
const roomList = (node: ReactNode) => withClass(node, 'room-switcher-list')
const cards = (node: ReactNode) => elements(roomList(node)).filter(element => element.type === 'button')
const card = (node: ReactNode, code: string) => cards(node).find(element => text(element).includes(`ROOM · ${code}`))!
const image = (node: ReactNode) => elements(node).find(element => element.type === 'img')!
const dialog = (node: ReactNode) => elements(node).find(element => element.props.role === 'dialog')

/** The real DOM resolves .src to an absolute URL but keeps the HTML attribute
 * relative; simulate both so the fallback guard cannot accidentally loop. */
function imageTarget(initialSource: string) {
  let source = initialSource
  const writes: string[] = []
  const assign = (value: string) => { source = value; writes.push(value) }
  const target: ImageTarget = {
    get src() { return new URL(source, 'http://127.0.0.1:5173').href },
    set src(value: string) { assign(value) },
    getAttribute: name => name === 'src' ? source : null,
    setAttribute: (name, value) => { if (name === 'src') assign(value) },
    onerror: null,
  }
  return { target, writes }
}

const roomCodes = ['10001', '10002', '10003', '10004']
const states = ['current', 'available', 'pending', 'disabled'] as const

describe('lobby room switcher cards', () => {
  let props: Props
  const render = (updates: Partial<Props> = {}) => {
    props = { ...props, ...updates }
    const result = runtime.hooks!.render(() => Lobby(props))
    runtime.hooks!.flushEffects()
    return result
  }
  const settle = async () => { for (let index = 0; index < 8; index++) await Promise.resolve(); return render() }
  const open = async (updates: Partial<Props> = {}) => {
    render(updates)
    const root = await settle()
    withClass(root, 'room-cluster').props.onClick!()
    return render()
  }

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    runtime.portal.mockReset()
    runtime.roomSettings.mockReset().mockResolvedValue({ announcements: [], room_notice: '' })
    props = {
      room: roomCodes[0], roomName: '当前房间', roomLogo: undefined,
      roomHistory: [
        { code: roomCodes[1], name: '可用房间', status: 'available', logo: '', lastUsedAt: 30 },
        { code: roomCodes[2], name: '待审核房间', status: 'pending', logo: '  ', lastUsedAt: 20 },
        { code: roomCodes[3], name: '停用房间', status: 'disabled', lastUsedAt: 10 },
      ],
      games: [], gamesLive: true, theme: 'day', onOpenGame: vi.fn(), onToggleTheme: vi.fn(), onSwitchRoom: vi.fn().mockResolvedValue(undefined),
    }
    vi.stubGlobal('document', { body: { style: { overflow: 'auto' } } })
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), matchMedia: () => ({ matches: false }) })
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    vi.stubGlobal('sessionStorage', { getItem: vi.fn(() => null), setItem: vi.fn() })
  })
  afterEach(() => { runtime.hooks!.unmount(); vi.unstubAllGlobals() })

  it('shows login announcements only as a dialog and omits the lobby top banner', async () => {
    runtime.roomSettings.mockResolvedValue({
      room_notice: '房间公告内容',
      announcements: [{ id: 'notice-1', title: '欢迎公告', content: '房间公告内容', enabled: true, popup_on_login: true, sort_order: 1 }],
    })

    render()
    const root = await settle()

    expect(elements(root).some(element => hasClass(element, 'lobby-announcement'))).toBe(false)
    expect(elements(root).some(element => element.type === AnnouncementDialog)).toBe(true)
    expect(sessionStorage.setItem).toHaveBeenCalledWith(`wangzhe-login-announcements-shown:${roomCodes[0]}`, '1')
  })

  it('still shows the login announcement when session storage is unavailable', async () => {
    runtime.roomSettings.mockResolvedValue({
      room_notice: '',
      announcements: [{ id: 'notice-private', title: '公告', content: '隐私模式也应显示', enabled: true, popup_on_login: true, sort_order: 1 }],
    })
    vi.stubGlobal('sessionStorage', {
      getItem: vi.fn(() => { throw new DOMException('blocked', 'SecurityError') }),
      setItem: vi.fn(() => { throw new DOMException('blocked', 'SecurityError') }),
    })

    render()
    const root = await settle()

    expect(elements(root).some(element => element.type === AnnouncementDialog)).toBe(true)
    expect(elements(root).some(element => hasClass(element, 'lobby-announcement'))).toBe(false)
  })

  it('renders a real non-draggable default image for current, available, pending and disabled rooms', async () => {
    const root = await open()
    expect(DEFAULT_ROOM_LOGO).toBe('/images/wangzhe-header-logo.png')
    expect(cards(root)).toHaveLength(4)
    expect(runtime.roomSettings).toHaveBeenCalledTimes(1)
    expect(runtime.portal).toHaveBeenLastCalledWith(expect.anything(), document.body)
    expect(document.body.style.overflow).toBe('hidden')
    for (const [index, code] of roomCodes.entries()) {
      const room = card(root, code)
      expect(room.props.className).toBe(`room-switcher-list-${states[index]}`)
      const icon = withClass(room, 'room-switcher-list-icon')
      expect(hasClass(icon, 'has-image')).toBe(true)
      expect(image(icon).props).toMatchObject({ src: DEFAULT_ROOM_LOGO, draggable: false })
      expect(elements(icon).filter(element => element.type === 'img')).toHaveLength(1)
      expect(elements(icon).some(element => element.type === Icon)).toBe(false)
    }
  })

  it('preserves custom room logos in all four states', async () => {
    const logos = roomCodes.map(code => `/images/custom-${code}.png`)
    const root = await open({ roomLogo: logos[0], roomHistory: props.roomHistory.map((item, index) => ({ ...item, logo: logos[index + 1] })) })
    roomCodes.forEach((code, index) => expect(image(card(root, code)).props).toMatchObject({ src: logos[index], draggable: false }))
  })

  it('uses the same-room history logo when the current session has no logo field', async () => {
    const root = await open({ roomLogo: undefined, roomHistory: [{ code: roomCodes[0], name: '历史房间名称', status: 'available', logo: '/images/remembered-room.png', lastUsedAt: 40 }, ...props.roomHistory] })
    expect(image(card(root, roomCodes[0])).props.src).toBe('/images/remembered-room.png')
    expect(text(card(root, roomCodes[0]))).toContain('当前房间')
    expect(card(root, roomCodes[0]).props['aria-current']).toBe('true')
    expect(cards(root)).toHaveLength(4)
  })

  it.each(states)('falls back once when a %s room custom image fails, without retrying a broken default forever', async state => {
    const root = await open({ roomLogo: '/images/broken-current.png', roomHistory: props.roomHistory.map(item => ({ ...item, logo: `/images/broken-${item.code}.png` })) })
    const logo = image(cards(root).find(item => item.props.className === `room-switcher-list-${state}`)!)
    expect(logo.props.onError).toBeTypeOf('function')
    const failed = imageTarget(logo.props.src!)
    logo.props.onError!({ currentTarget: failed.target })
    expect(failed.target.getAttribute('src')).toBe(DEFAULT_ROOM_LOGO)
    expect(failed.writes).toEqual([DEFAULT_ROOM_LOGO])
    logo.props.onError!({ currentTarget: failed.target })
    logo.props.onError!({ currentTarget: failed.target })
    expect(failed.writes).toEqual([DEFAULT_ROOM_LOGO])
  })

  it('does not reassign a default image that itself fails to load', async () => {
    const root = await open()
    const logo = image(card(root, roomCodes[0]))
    const failed = imageTarget(DEFAULT_ROOM_LOGO)
    logo.props.onError!({ currentTarget: failed.target })
    expect(failed.writes).toEqual([])
  })

  it('keeps current, recent, pending and disabled labels and disabled-room protection', async () => {
    const root = await open()
    expect(text(card(root, roomCodes[0]))).toContain('当前')
    expect(text(card(root, roomCodes[1]))).toContain('最近')
    expect(text(card(root, roomCodes[2]))).toContain('待审核')
    expect(text(card(root, roomCodes[3]))).toContain('已停用')
    expect(card(root, roomCodes[0]).props['aria-current']).toBe('true')
    expect(card(root, roomCodes[0]).props.disabled).toBe(false)
    expect(card(root, roomCodes[1]).props.disabled).toBe(false)
    expect(card(root, roomCodes[2]).props.disabled).toBe(false)
    expect(card(root, roomCodes[3]).props.disabled).toBe(true)
    expect(props.onSwitchRoom).not.toHaveBeenCalled()
  })

  it('closes the switcher when selecting the current room without requesting a switch', async () => {
    const root = await open()
    card(root, roomCodes[0]).props.onClick!()
    expect(dialog(render())).toBeUndefined()
    expect(props.onSwitchRoom).not.toHaveBeenCalled()
    expect(document.body.style.overflow).toBe('auto')
  })

  it.each(['available', 'pending'] as const)('preserves %s room selection and disables all cards while switching', async state => {
    let finish!: () => void
    const switchRoom = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    const root = await open({ onSwitchRoom: switchRoom })
    const code = roomCodes[states.indexOf(state)]
    card(root, code).props.onClick!()
    const waiting = render()
    expect(switchRoom).toHaveBeenCalledExactlyOnceWith(code)
    expect(text(card(waiting, code))).toContain('切换中…')
    expect(cards(waiting).every(item => item.props.disabled)).toBe(true)
    for (const item of cards(waiting)) expect(image(item).props.src).toBeTruthy()
    finish()
    expect(dialog(await settle())).toBeUndefined()
  })

  it('keeps the room list and images available for a deliberate retry after switch failure', async () => {
    const switchRoom = vi.fn().mockRejectedValueOnce(new Error('房间仍在审核中')).mockResolvedValue(undefined)
    const root = await open({ onSwitchRoom: switchRoom })
    card(root, roomCodes[2]).props.onClick!()
    const failed = await settle()
    expect(dialog(failed)).toBeDefined()
    expect(text(withClass(failed, 'room-switcher-error'))).toBe('房间仍在审核中')
    expect(card(failed, roomCodes[2]).props.disabled).toBe(false)
    expect(image(card(failed, roomCodes[2])).props.src).toBe(DEFAULT_ROOM_LOGO)
    card(failed, roomCodes[2]).props.onClick!()
    expect(dialog(await settle())).toBeUndefined()
    expect(switchRoom).toHaveBeenCalledTimes(2)
  })

  it('keeps the selected lobby category and passes it into game navigation', async () => {
    const timing = {
      phase: 'accepting' as const,
      phaseLabel: '受理倒计时',
      statusLabel: '正在受理',
      accepting: true,
      due: '00:30',
      remainingSeconds: 30,
      drawAtMs: 60_000,
      sealAtMs: 55_000,
      acceptAtMs: 0,
      intervalSeconds: 60,
      sealSeconds: 5,
    }
    const game = (id: string, title: string, lobbyCategory: string): Props['games'][number] => ({
      id, title, lobbyCategory, timing,
      tag: title,
      category: lobbyCategory,
      online: '—',
      period: '100',
      latestIssue: '99',
      due: timing.due,
      color: '#4aa3b4',
      balls: [1, 2, 3],
      issueStatus: 'accepting',
      sourceKind: 'live',
      sourceName: 'test',
      sourceHealthy: true,
      syncStatus: 'ready',
      sourceError: '',
    })
    const onOpenGame = vi.fn()
    const onFilterChange = vi.fn()
    let root = render({
      games: [game('speed-racing', '极速赛车', '彩票'), game('bingo-mark-six', '宾果六合彩', '宾果')],
      initialFilter: '宾果',
      onFilterChange,
      onOpenGame,
    })

    const gameCards = () => elements(root).filter(element => hasClass(element, 'game-card'))
    expect(gameCards().map(element => text(element))).toEqual([expect.stringContaining('宾果六合彩')])
    gameCards()[0].props.onClick!()
    expect(onOpenGame).toHaveBeenCalledExactlyOnceWith('bingo-mark-six', '宾果')

    const lotteryFilter = elements(root).find(element => element.type === 'button' && text(element) === '彩票')!
    lotteryFilter.props.onClick!()
    root = render()
    expect(onFilterChange).toHaveBeenCalledExactlyOnceWith('彩票')
    expect(gameCards().map(element => text(element))).toEqual([expect.stringContaining('极速赛车')])
  })

  it('shows friendly results-only and draw-pause states without technical source details', () => {
    const timing = {
      phase: 'unavailable' as const, phaseLabel: '仅开奖', statusLabel: '仅展示已公布开奖 · 投注未开放', accepting: false,
      due: '--:--', remainingSeconds: null, drawAtMs: null, sealAtMs: null, acceptAtMs: null, intervalSeconds: null, sealSeconds: null,
    }
    const baseGame: Props['games'][number] = {
      id: 'hong-kong-mark-six', title: '香港六合彩', lobbyCategory: '彩票', timing, tag: '香港', category: '六合彩', online: '—',
      period: '100', latestIssue: '99', due: '--:--', color: '#d64155', balls: [1, 7, 18, 25, 30, 42, 49], issueStatus: 'pending',
      sourceKind: 'external', sourceName: 'internal-provider-name', sourceHealthy: true, syncStatus: 'error', sourceError: 'secret upstream timeout', rulesReady: false,
    }
    const root = render({ games: [baseGame, { ...baseGame, id: 'new-macau-mark-six', title: '新澳门六合彩', sourceHealthy: false, rulesReady: true }] })
    const gameCards = elements(root).filter(element => hasClass(element, 'game-card')).map(element => text(element))
    expect(gameCards).toEqual([expect.stringContaining('仅开奖 · 投注未开放'), expect.stringContaining('开奖暂停 · 投注暂停')])
    expect(gameCards.join('')).not.toContain('internal-provider-name')
    expect(gameCards.join('')).not.toContain('secret upstream timeout')
    expect(elements(root).filter(element => element.props.className?.includes('mark-six-special-ball'))).toHaveLength(2)
  })

  it('shows all three enabled PC catalogue entries as independent game cards', () => {
    const timing = {
      phase: 'accepting' as const, phaseLabel: '受理倒计时', statusLabel: '正在受理', accepting: true,
      due: '00:30', remainingSeconds: 30, drawAtMs: 60_000, sealAtMs: 55_000, acceptAtMs: 0,
      intervalSeconds: 210, sealSeconds: 5,
    }
    const games: Props['games'] = [
      ['pc-canada', 'PC加拿大', 'pc28-v1'],
      ['canada-28', '加拿大28', 'pc28-v2'],
      ['canada-20', '加拿大2.0', 'pc28-v3'],
    ].map(([id, title, ruleVersion]) => ({
      id, title, lobbyCategory: 'PC', timing, tag: title, category: 'PC', online: '—', period: '100', latestIssue: '99',
      due: timing.due, color: '#4aa3b4', balls: [9, 1, 9], issueStatus: 'accepting', sourceKind: 'external',
      sourceName: '163开奖', sourceHealthy: true, syncStatus: 'ready', sourceError: '', rulesReady: true, ruleVersion,
    }))
    const onOpenGame = vi.fn()
    const root = render({ games, initialFilter: 'PC', onOpenGame })
    const gameCards = elements(root).filter(element => hasClass(element, 'game-card'))
    expect(gameCards.map(element => text(element))).toEqual([
      expect.stringContaining('PC加拿大'), expect.stringContaining('加拿大28'), expect.stringContaining('加拿大2.0'),
    ])
    gameCards.forEach(card => card.props.onClick!())
    expect(onOpenGame.mock.calls).toEqual([
      ['pc-canada', 'PC'], ['canada-28', 'PC'], ['canada-20', 'PC'],
    ])
  })

  it('synchronizes a cleared category route back to the default lottery category', () => {
    const timing = {
      phase: 'accepting' as const, phaseLabel: '受理倒计时', statusLabel: '正在受理', accepting: true,
      due: '00:30', remainingSeconds: 30, drawAtMs: 60_000, sealAtMs: 55_000, acceptAtMs: 0,
      intervalSeconds: 60, sealSeconds: 5,
    }
    const game = (id: string, title: string, lobbyCategory: string): Props['games'][number] => ({
      id, title, lobbyCategory, timing, tag: title, category: lobbyCategory, online: '—', period: '100', latestIssue: '99',
      due: timing.due, color: '#4aa3b4', balls: [1, 2, 3], issueStatus: 'accepting', sourceKind: 'live',
      sourceName: 'test', sourceHealthy: true, syncStatus: 'ready', sourceError: '',
    })
    const games = [game('speed-racing', '极速赛车', '彩票'), game('bingo-mark-six', '宾果六合彩', '宾果')]
    let root = render({ games, initialFilter: '宾果' })
    expect(elements(root).filter(element => hasClass(element, 'game-card')).map(element => text(element))).toEqual([expect.stringContaining('宾果六合彩')])

    render({ initialFilter: '' })
    root = render()
    expect(elements(root).filter(element => hasClass(element, 'game-card')).map(element => text(element))).toEqual([expect.stringContaining('极速赛车')])
  })
})

describe('room switcher horizontal-scroll CSS contract', () => {
  const css = readFileSync(new URL('../lobby-polish.css', import.meta.url), 'utf8')
  const rules = [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)].map(match => ({ selector: match[1].trim(), declarations: match[2] }))
  const declarations = (selector: string) => rules.filter(rule => rule.selector === selector).map(rule => rule.declarations).join('\n')

  it('lays room cards out in one horizontally scrollable row', () => {
    const list = declarations('.room-switcher-list')
    expect(list).toMatch(/display\s*:\s*flex\s*;/)
    expect(list).toMatch(/flex-wrap\s*:\s*nowrap\s*;/)
    expect(list).toMatch(/overflow-x\s*:\s*auto\s*;/)
    expect(list).not.toMatch(/display\s*:\s*grid|grid-template-columns/)
    const card = declarations('.room-switcher-list > button')
    expect(card).toMatch(/(?:flex-basis\s*:\s*104px|flex\s*:\s*0\s+0\s+104px)\s*;/)
    expect(card).toMatch(/(?:^|[;\s])height\s*:\s*104px\s*;/)
  })

  it('does not block touch gestures on the switcher or its cards', () => {
    const switcherRules = rules.filter(rule => rule.selector.includes('.room-switcher')).map(rule => rule.declarations).join('\n')
    expect(switcherRules).not.toMatch(/touch-action\s*:\s*none(?:\s*!important)?(?:\s*;|\s*$)/m)
  })
})
