import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatMessage } from '../api/chat'
import type { DrawResult } from '../api/lottery'
import type { Game } from '../types'
import { HookHarness } from '../test/hookHarness'
import { resolveLotteryTiming } from '../utils/lotteryTiming'
import { BetKeyboard, GameRoom, GameTimeline } from './GameRoom'

const runtime = vi.hoisted(() => ({
  hooks: null as HookHarness | null,
  draws: [] as DrawResult[], drawsLoading: false,
  chatMessages: vi.fn(), command: vi.fn(), send: vi.fn(), bets: vi.fn(), feed: vi.fn(), notifications: vi.fn(), sound: vi.fn(),
}))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useMemo: <T,>(factory: () => T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(factory, dependencies),
  useCallback: <T,>(callback: T, dependencies: readonly unknown[]) => runtime.hooks!.useMemo(() => callback, dependencies),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
  useLayoutEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../api/client', () => ({ apiBase: 'http://localhost:8080/api', request: vi.fn(), publicRequest: vi.fn() }))
vi.mock('../api/chat', () => ({ chatApi: { messages: runtime.chatMessages, command: runtime.command, send: runtime.send, availableRedPacket: vi.fn(async () => null) } }))
vi.mock('../api/bets', () => ({ betsApi: { list: runtime.bets, assistantHistory: vi.fn(async () => []), assistantStatus: vi.fn(async () => null) } }))
vi.mock('../api/portal', () => ({ portalApi: { gameFeed: runtime.feed, notifications: runtime.notifications, roomSettings: vi.fn(async () => ({})), gameOdds: vi.fn(async () => ({ show_odds: false, items: [] })) } }))
vi.mock('../api/member', () => ({ memberApi: { walletSummary: vi.fn(async () => null) } }))
vi.mock('../hooks/useGameDraws', () => ({ useGameDraws: () => ({ draws: runtime.draws, loading: runtime.drawsLoading }) }))
vi.mock('../hooks/useWebSocket', () => ({ WS_EVENT: 'test-room-ws', useWebSocketConnected: () => true }))
vi.mock('../hooks/useMemberPreferences', () => ({ useMemberPreferences: () => ({ drawHistoryLimit: 8, defaultBetMode: 'quick', fontScale: 'standard' }) }))
vi.mock('../utils/notificationAudio', () => ({ playNotificationSound: runtime.sound }))

type Props = ComponentProps<typeof GameRoom>
type KeyboardProps = ComponentProps<typeof BetKeyboard>
type TestNodeProps = { children?: ReactNode; className?: string; onClick?: () => void; onQuickBet?: () => void; data?: { amount: number } }
const timing = resolveLotteryTiming({ issue_status: 'accepting', source_healthy: true, next_draw_at: '2026-08-30T06:46:00Z', seal_seconds: 30 }, Date.parse('2026-08-30T06:45:00Z'))
const game: Game = { id: 'speed-racing', title: '极速赛车', tag: '', category: 'racing', lobbyCategory: 'lottery', online: '', period: '34137153', latestIssue: '34137152', due: timing.due, timing, balls: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10], color: '', issueStatus: 'accepting', sourceKind: '', sourceName: '', sourceHealthy: true, syncStatus: '', sourceError: '' }
const noop = () => undefined
const base: Props = { game, games: [game], theme: 'day', nickname: '王者玩家', balance: 1000, onBack: noop, onOpenGame: noop, onOpenService: noop, onOpenWallet: noop, onOpenResults: noop, onRefreshBalance: async () => undefined }

function find(node: ReactNode, predicate: (node: ReactElement<TestNodeProps>) => boolean): ReactElement<TestNodeProps> | undefined {
  if (Array.isArray(node)) return node.map(child => find(child, predicate)).find(Boolean)
  if (!isValidElement<TestNodeProps>(node)) return
  if (predicate(node)) return node
  return find(node.props.children, predicate)
}
function visibleText(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(visibleText).join('')
  return isValidElement<TestNodeProps>(node) ? visibleText(node.props.children) : ''
}

describe('room keyboard and issue lifecycle', () => {
  const render = (updates: Partial<Props> = {}) => {
    const tree = runtime.hooks!.render(() => GameRoom({ ...base, ...updates }))
    runtime.hooks!.flushEffects()
    return tree
  }
  const settle = async () => { for (let pass = 0; pass < 20; pass += 1) await Promise.resolve() }
  const input = (tree: ReactNode) => find(tree, node => node.props.className === 'ticket-selection')!
  const keyboard = (tree: ReactNode) => find(tree, node => node.type === BetKeyboard)!.props as unknown as KeyboardProps
  const openKeyboard = () => { input(render()).props.onClick!(); return keyboard(render()) }

  beforeEach(() => {
    runtime.hooks = new HookHarness()
    runtime.draws = []
    runtime.drawsLoading = false
    runtime.chatMessages.mockReset().mockResolvedValue({ items: [] })
    runtime.command.mockReset().mockImplementation(async (content: string, gameId: string): Promise<ChatMessage> => ({ id: 10, user_id: 8, nickname: '王者玩家', room_type: 'group', room_scope: 'agent:2', game_id: gameId, content, message_type: 'text', mine: true, created_at: '2026-08-30T06:45:10Z' }))
    runtime.send.mockReset()
    runtime.bets.mockReset().mockResolvedValue({ items: [] })
    runtime.feed.mockReset().mockResolvedValue([])
    runtime.notifications.mockReset()
    runtime.sound.mockReset()
    vi.stubGlobal('window', Object.assign(new EventTarget(), { requestAnimationFrame: vi.fn(() => 1), cancelAnimationFrame: vi.fn(), setInterval, clearInterval, setTimeout, clearTimeout }))
    vi.stubGlobal('document', Object.assign(new EventTarget(), { visibilityState: 'visible' }))
  })
  afterEach(() => { runtime.hooks?.unmount(); vi.unstubAllGlobals() })

  it('removes the multi-lane tutorial from the opening assistant message', async () => {
    render(); await settle(); render(); await settle()
    const text = visibleText(render())
    expect(text).toContain('本期正在受理。')
    expect(text).not.toContain('多车道示例')
    expect(text).not.toContain('每组用 #')
  })

  it('refills the last sent bet for editing without another command request', async () => {
    render(); await settle()
    openKeyboard().onSelectOption('4444/88')
    keyboard(render()).onConfirm()
    await settle()
    expect(runtime.command).toHaveBeenCalledOnce()
    keyboard(render()).onShortcut('credit')
    expect(visibleText(input(render()))).toBe('上分 ')
    keyboard(render()).onShortcut('repeat')
    expect(visibleText(input(render()))).toBe('4444/88')
    expect(runtime.command).toHaveBeenCalledOnce()
    expect(runtime.send).not.toHaveBeenCalled()
  })

  it('keeps integer stakes unchanged when sending and repeating keyboard commands', async () => {
    const content = '3/2/20#4/2/20#5/2/20'
    render(); await settle()
    openKeyboard().onSelectOption(content)
    keyboard(render()).onConfirm()
    await settle()
    expect(runtime.command).toHaveBeenCalledWith(content, 'speed-racing', expect.objectContaining({ issue: game.period }))
    keyboard(render()).onShortcut('repeat')
    expect(visibleText(input(render()))).toBe(content)
    expect(runtime.command).toHaveBeenCalledOnce()
  })

  it('does not unmount the ready timeline while next-issue recovery requests are pending', async () => {
    render(); await settle(); render(); await settle()
    expect(find(render(), node => node.type === GameTimeline)).toBeDefined()
    runtime.bets.mockReturnValue(new Promise(() => undefined))
    runtime.feed.mockReturnValue(new Promise(() => undefined))
    const next = { ...game, period: '34137154' }
    render({ game: next })
    expect(find(render({ game: next }), node => node.type === GameTimeline)).toBeDefined()
  })

  it('waits for the draw boundary and requests only messages since it, then follows the ID cursor', async () => {
    runtime.drawsLoading = true
    render(); await settle()
    expect(runtime.chatMessages).not.toHaveBeenCalled()
    const anchor: DrawResult = { id: 34, issue: '34', game_id: game.id, numbers: game.balls, draw_at: '2026-08-30T06:44:00Z' }
    runtime.draws = [anchor, { ...anchor, id: 33, issue: '33', draw_at: '2026-08-30T06:42:45Z' }]
    runtime.drawsLoading = false
    const chat: ChatMessage = { id: 50, game_id: game.id, user_id: 8, nickname: '玩家', room_type: 'group', room_scope: 'agent:2', content: 'hello', message_type: 'text', mine: false, created_at: '2026-08-30T06:44:10Z' }
    runtime.chatMessages.mockResolvedValueOnce({ items: [chat], has_more: true }).mockResolvedValueOnce({ items: [{ ...chat, id: 51 }], has_more: false })
    render(); render(); await settle()
    const tree = render()
    expect(runtime.chatMessages.mock.calls[0]).toEqual(['group', game.id, 50, { since: '2026-08-30T06:44:00.000Z', after_id: undefined }])
    expect(runtime.chatMessages.mock.calls[1]).toEqual(['group', game.id, 50, { since: '2026-08-30T06:44:00.000Z', after_id: 50 }])
    const timeline = find(tree, node => node.type === GameTimeline)!.props as unknown as ComponentProps<typeof GameTimeline>
    expect(timeline.draws.map(draw => draw.issue)).toEqual(['34'])
    expect(timeline.drawHistory).toHaveLength(2)
    expect(timeline.messages.map(row => row.id)).toEqual([50, 51])
    expect(visibleText(tree)).not.toContain('本期正在受理。')
    const event = new Event('test-room-ws')
    Object.assign(event, { detail: { type: 'chat_message', data: { game_id: game.id } } })
    window.dispatchEvent(event); await settle()
    expect(runtime.chatMessages.mock.calls.at(-1)?.[3]).toEqual({ since: '2026-08-30T06:44:00.000Z', after_id: 51 })
  })

  it('preserves already displayed betting activity and its original issue while staying across rounds', async () => {
    runtime.feed.mockResolvedValueOnce([{ nickname: '旧期会员', amount: 5, detail: '冠军 1', created_at: '2026-08-30T06:45:00Z' }])
    render(); await settle(); render(); await settle()
    const next = { ...game, period: '34137154' }
    runtime.feed.mockResolvedValueOnce([{ nickname: '新期会员', amount: 6, detail: '冠军 2', created_at: '2026-08-30T06:46:00Z' }])
    render({ game: next }); await settle()
    const timeline = find(render({ game: next }), node => node.type === GameTimeline)!.props as unknown as ComponentProps<typeof GameTimeline>
    expect(timeline.feed.map(row => row.issue)).toEqual(['34137153', '34137154'])
  })

  it('labels and submits the server-confirmed next issue, never the old drawing issue', async () => {
    const drawing = { ...game, timing: { ...timing, phase: 'awaiting_draw' as const, accepting: false, phaseLabel: '开奖中', statusLabel: '开奖中' }, betting: { issue: '34137154', timing } }
    render({ game: drawing }); await settle()
    input(render({ game: drawing })).props.onClick!()
    keyboard(render({ game: drawing })).onSelectOption('6/9')
    const tree = render({ game: drawing })
    expect(visibleText(input(tree))).toContain('下期 34137154')
    keyboard(tree).onConfirm()
    await settle()
    expect(runtime.command).toHaveBeenCalledWith('6/9', 'speed-racing', expect.objectContaining({ issue: '34137154' }))
  })

  it('preserves the next-issue draft and open keyboard when the official issue catches up', async () => {
    const drawing = { ...game, timing: { ...timing, phase: 'awaiting_draw' as const, accepting: false }, betting: { issue: '34137154', timing } }
    render({ game: drawing }); await settle()
    input(render({ game: drawing })).props.onClick!()
    keyboard(render({ game: drawing })).onSelectOption('1/12345/100#6/大/200')
    const next = { ...game, period: '34137154' }
    render({ game: next })
    const continued = render({ game: next })
    expect(visibleText(input(continued))).toBe('1/12345/100#6/大/200')
    expect(find(continued, node => node.type === BetKeyboard)).toBeDefined()
    expect(runtime.command).not.toHaveBeenCalled()
    keyboard(continued).onConfirm()
    await settle()
    expect(runtime.command).toHaveBeenCalledWith('1/12345/100#6/大/200', 'speed-racing', expect.objectContaining({ issue: '34137154' }))
  })

  it('keeps the detailed betting panel mounted across an issue change', async () => {
    render(); await settle()
    const quickActions = find(render(), node => typeof node.type === 'function' && node.type.name === 'QuickActions')!
    quickActions.props.onQuickBet!()
    const original = find(render(), node => typeof node.type === 'function' && node.type.name === 'FullBetBoard')!
    expect(original).toBeDefined()
    const next = { ...game, period: '34137154' }
    render({ game: next })
    const continued = find(render({ game: next }), node => node.type === original.type)
    // Same component and key retain its own draft/selection hook state.
    expect(continued).toBeDefined()
    expect(continued?.key).toBe(original.key)
  })

  it('still clears the draft and closes the keyboard when switching games', async () => {
    render(); await settle()
    openKeyboard().onSelectOption('4444/88')
    expect(visibleText(input(render()))).toBe('4444/88')
    const other = { ...game, id: 'speed-fly', title: '极速飞艇' }
    render({ game: other })
    const switched = render({ game: other })
    expect(visibleText(input(switched))).toBe('输入玩法/金额或聊天内容')
    expect(find(switched, node => node.type === BetKeyboard)).toBeUndefined()
  })

  it('keeps winning popups and balance events but does not request a personal settlement timeline', async () => {
    const refresh = vi.fn(async () => undefined)
    render({ onRefreshBalance: refresh }); await settle()
    const event = new Event('test-room-ws')
    Object.assign(event, { detail: { type: 'notification', game_id: game.id, data: { category: 'winning', won_count: 1, payout_amount: 267.3, issue: game.period }, event_id: 'winning-1' } })
    window.dispatchEvent(event)
    await settle()
    const popup = find(render({ onRefreshBalance: refresh }), node => typeof node.type === 'function' && node.type.name === 'WinningPopup')
    expect(popup?.props.data?.amount).toBe(267.3)
    expect(refresh).toHaveBeenCalledOnce()
    expect(runtime.sound).toHaveBeenCalledWith('reward')
    expect(runtime.notifications).not.toHaveBeenCalled()
  })
})
