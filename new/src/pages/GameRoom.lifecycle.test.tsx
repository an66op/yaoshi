import { isValidElement, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatMessage } from '../api/chat'
import type { DrawResult } from '../api/lottery'
import type { Game } from '../types'
import { HookHarness } from '../test/hookHarness'
import { MarkSixDrawBall } from '../components/MarkSixBall'
import { LotteryCountdown } from '../components/LotteryCountdown'
import { resolveLotteryTiming } from '../utils/lotteryTiming'
import { GAME_TIMELINE_LIMIT } from '../utils/gameTimelineBudget'
import { SG_SSC_LOTTERY_SOURCE_URL } from '../utils/lotterySourceURL'
import { BetKeyboard, GameRoom, GameTimeline, QuickActions } from './GameRoom'

const runtime = vi.hoisted(() => ({
  hooks: null as HookHarness | null,
  draws: [] as DrawResult[], drawsLoading: false,
  chatMessages: vi.fn(), command: vi.fn(), send: vi.fn(), bets: vi.fn(), assistantHistory: vi.fn(), assistantStatus: vi.fn(), assistantPlace: vi.fn(), webPlaceBatch: vi.fn(), feed: vi.fn(), notifications: vi.fn(), roomSettings: vi.fn(), gameOdds: vi.fn(), sound: vi.fn(),
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
vi.mock('../api/bets', () => ({ betsApi: { list: runtime.bets, assistantHistory: runtime.assistantHistory, assistantStatus: runtime.assistantStatus, assistantPlace: runtime.assistantPlace, webPlaceBatch: runtime.webPlaceBatch } }))
vi.mock('../api/portal', () => ({ portalApi: { gameFeed: runtime.feed, notifications: runtime.notifications, roomSettings: runtime.roomSettings, gameOdds: runtime.gameOdds } }))
vi.mock('../api/member', () => ({ memberApi: { walletSummary: vi.fn(async () => null) } }))
vi.mock('../hooks/useGameDraws', () => ({ useGameDraws: () => ({ draws: runtime.draws, loading: runtime.drawsLoading }) }))
vi.mock('../hooks/useWebSocket', () => ({ WS_EVENT: 'test-room-ws', useWebSocketConnected: () => true }))
vi.mock('../hooks/useMemberPreferences', () => ({ useMemberPreferences: () => ({ drawHistoryLimit: 8, defaultBetMode: 'quick', fontScale: 'standard' }) }))
vi.mock('../utils/notificationAudio', () => ({ playNotificationSound: runtime.sound }))

type Props = ComponentProps<typeof GameRoom>
type KeyboardProps = ComponentProps<typeof BetKeyboard>
type TestPointerEvent = { pointerId: number; clientX: number; clientY: number; isPrimary: boolean; currentTarget: { style: { setProperty: (name: string, value: string) => void; removeProperty: (name: string) => void }; setPointerCapture: (id: number) => void; hasPointerCapture: (id: number) => boolean; releasePointerCapture: (id: number) => void }; preventDefault: () => void }
type TestNodeProps = { children?: ReactNode; className?: string; description?: string; href?: string; embedded?: boolean; active?: boolean; rulesReady?: boolean; oddsInfo?: unknown; hidden?: boolean; 'aria-hidden'?: boolean; 'aria-label'?: string; onChange?: (event: { target: { value: string } }) => void; onClick?: () => void; onClose?: () => void; onQuickBet?: () => void; onConfirm?: (content: unknown) => unknown; onPointerDown?: (event: TestPointerEvent) => void; onPointerMove?: (event: TestPointerEvent) => void; onPointerUp?: (event: TestPointerEvent) => void; data?: { amount: number } }
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
function elements(node: ReactNode): ReactElement<TestNodeProps>[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  return isValidElement<TestNodeProps>(node) ? [node, ...elements(node.props.children)] : []
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
    runtime.assistantHistory.mockReset().mockResolvedValue([])
    runtime.assistantStatus.mockReset().mockResolvedValue(null)
    runtime.assistantPlace.mockReset()
    runtime.webPlaceBatch.mockReset()
    runtime.feed.mockReset().mockResolvedValue([])
    runtime.notifications.mockReset()
    runtime.roomSettings.mockReset().mockResolvedValue({})
    runtime.gameOdds.mockReset().mockResolvedValue({ game_id: game.id, game_name: game.title, show_odds: false, rules_ready: true, items: [] })
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

  it('updates the first quick action from the room settings API', async () => {
    runtime.roomSettings.mockResolvedValueOnce({ lottery_source_url: 'https://configured.example/results?room=88001' })
    render(); await settle()
    const actions = find(render(), node => node.type === QuickActions)!
    const source = find(QuickActions(actions.props as unknown as ComponentProps<typeof QuickActions>), node => node.props['aria-label'] === '查看开奖源（新窗口）')
    expect(source?.props.href).toBe('https://configured.example/results?room=88001')
  })

  it('keeps the SG quick action bound to its actual source when room settings load or the source pauses', async () => {
    const sg: Game = { ...game, id: 'sg-ssc', title: 'SG时时彩', sourceKind: 'external', sourceURL: SG_SSC_LOTTERY_SOURCE_URL,
      rulesReady: true, ruleVersion: 'digits5-v3', balls: [0, 9, 2, 7, 4] }
    runtime.roomSettings.mockResolvedValueOnce({ lottery_source_url: 'https://configured.example/results?room=88001' })
    const sourceLink = (current = sg) => {
      const actions = find(render({ game: current, games: [current] }), node => node.type === QuickActions)!
      return find(QuickActions(actions.props as unknown as ComponentProps<typeof QuickActions>), node => node.props['aria-label'] === '查看开奖源（新窗口）')?.props.href
    }
    expect(sourceLink()).toBe(SG_SSC_LOTTERY_SOURCE_URL)
    await settle()
    expect(sourceLink()).toBe(SG_SSC_LOTTERY_SOURCE_URL)
    expect(sourceLink({ ...sg, sourceHealthy: false, issueStatus: 'error' })).toBe(SG_SSC_LOTTERY_SOURCE_URL)
    expect(sourceLink({ ...sg, sourceURL: undefined })).toBe(SG_SSC_LOTTERY_SOURCE_URL)
  })

  it('describes an unavailable draw as paused without exposing technical source details', async () => {
    const sg: Game = { ...game, id: 'sg-ssc', title: 'SG时时彩', rulesReady: true, ruleVersion: 'digits5-v3', balls: [0, 9, 2, 7, 4] }
    const odds = { game_id: sg.id, game_name: sg.title, show_odds: false, rules_ready: true, rule_version: 'digits5-v3', items: [] }
    runtime.gameOdds.mockResolvedValue(odds)
    runtime.assistantStatus.mockResolvedValue({ game_id: sg.id, rules_ready: true, rule_version: 'digits5-v3' })
    render({ game: sg }); await settle(); render({ game: sg })
    runtime.gameOdds.mockRejectedValueOnce(new Error('offline'))
    runtime.assistantStatus.mockRejectedValueOnce(new Error('offline'))
    const unavailable: Game = { ...sg, period: '—', sourceHealthy: false, sourceError: 'offline', issueStatus: 'error',
      timing: { ...timing, phase: 'error', phaseLabel: '已停盘', statusLabel: '开奖源异常 · 已停盘', accepting: false, due: '--:--' } }
    render({ game: unavailable }); await settle()
    const root = render({ game: unavailable })
    expect(visibleText(root)).toContain('开奖同步暂时暂停，当前可查看已公布结果和聊天，投注已暂停。')
    expect(visibleText(root)).not.toContain('暂未配置完整玩法')
    expect(visibleText(root)).not.toContain('offline')
    expect(visibleText(root)).toContain(`上期 ${sg.latestIssue}`)
    const countdown = find(root, node => node.type === LotteryCountdown)!.props as unknown as ComponentProps<typeof LotteryCountdown>
    expect(countdown.timing).toMatchObject({ phase: 'unavailable', accepting: false, phaseLabel: '开奖暂停' })
    find(root, node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    expect(visibleText(render({ game: unavailable }))).toContain('SG时时彩详细投注暂停受理')
    expect(visibleText(render({ game: unavailable }))).not.toContain('SG时时彩详细投注待配置')
    expect(visibleText(render({ game: unavailable }))).not.toContain('接口恢复')
    expect(runtime.assistantPlace).not.toHaveBeenCalled()
    expect(unavailable).toMatchObject({ rulesReady: true, ruleVersion: 'digits5-v3' })
    runtime.gameOdds.mockResolvedValue(odds)
    render({ game: sg }); await settle()
    expect(visibleText(render({ game: sg }))).not.toContain('开奖同步暂时暂停')
  })

  it('keeps an unknown game readable and normal chat usable while blocking wagers and the detailed board', async () => {
    const unknown: Game = { ...game, id: 'unconfigured-game', title: '待配置彩种', category: '其他', rulesReady: false, balls: [1, 2, 3, 4, 5] }
    const updates = { game: unknown, games: [unknown] }
    const native = () => find(render(updates), node => node.type === 'input' && node.props.className?.includes('ticket-native-input') === true)!
    const send = () => find(render(updates), node => node.props['aria-label'] === '发送')!.props.onClick!()
    render(updates); await settle(); render(updates)
    const blockedRoom = render(updates)
    expect(visibleText(blockedRoom)).toContain('当前仅提供开奖查看和聊天，投注暂未开放。')
    expect(find(blockedRoom, node => node.type === 'main')?.props.className).toContain('rules-blocked')
    native().props.onChange!({ target: { value: '1/1/10' } })
    send(); await settle()
    expect(runtime.command).not.toHaveBeenCalled()
    expect(runtime.send).not.toHaveBeenCalled()
    find(render(updates), node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    expect(find(render(updates), node => typeof node.type === 'function' && ['FullBetBoard', 'DigitBetBoard'].includes(node.type.name))).toBeUndefined()
    runtime.send.mockResolvedValueOnce({ id: 1, game_id: unknown.id, content: '你好', created_at: '2026-08-30T06:45:10Z' })
    native().props.onChange!({ target: { value: '你好' } })
    send(); await settle()
    expect(runtime.send).toHaveBeenCalledWith('你好', 'group', 'unconfigured-game')
    expect(runtime.command).not.toHaveBeenCalled()
  })

  it.each([
    ['hong-kong-mark-six', '香港六合彩'],
    ['happy8-mark-six', '快乐8六合彩'],
    ['new-macau-mark-six', '新澳门六合彩'],
    ['old-macau-mark-six', '老澳门六合彩'],
  ])('renders %s as a seven-ball draw while keeping every betting board closed', async (id, title) => {
    const marksix: Game = { ...game, id, title, category: '六合彩', rulesReady: false, ruleVersion: '', balls: [1, 7, 18, 25, 30, 42, 49] }
    const updates = { game: marksix, games: [marksix] }
    render(updates); await settle()
    const tree = render(updates)
    expect(visibleText(tree)).toContain('当前仅提供开奖查看和聊天，投注暂未开放。')
    expect(visibleText(tree)).toContain('正1正2正3正4正5正6特')
    expect(elements(tree).filter(node => node.type === MarkSixDrawBall)).toHaveLength(7)
    expect(find(tree, node => typeof node.type === 'function' && ['MarkSixBetBoard', 'FullBetBoard', 'DigitBetBoard'].includes(node.type.name))).toBeUndefined()
    expect(find(tree, node => node.type === 'main')?.props.className).toContain('rules-blocked')
  })

  it('opens the independent digit board for a known five-ball game and uses its total keyboard', async () => {
    const digit: Game = { ...game, id: 'speed-ssc', title: '极速时时彩', rulesReady: true, ruleVersion: 'digits5-v3', balls: [9, 8, 1, 2, 3] }
    runtime.gameOdds.mockResolvedValue({ game_id: digit.id, game_name: digit.title, show_odds: false, rules_ready: true, rule_version: 'digits5-v3', items: [] })
    runtime.assistantStatus.mockResolvedValue({ game_id: digit.id, rules_ready: true, rule_version: 'digits5-v3' })
    const updates = { game: digit }
    render(updates); await settle(); render(updates)
    expect(visibleText(render(updates))).toContain('总和 23大单')
    find(render(updates), node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    expect(find(render(updates), node => typeof node.type === 'function' && node.type.name === 'DigitBetBoard')).toBeDefined()
    expect(find(render(updates), node => typeof node.type === 'function' && node.type.name === 'FullBetBoard')).toBeUndefined()
  })

  it.each([
    ['pc-canada', 'PC加拿大', 'pc28-v1'],
    ['canada-28', '加拿大28', 'pc28-v2'],
    ['canada-20', '加拿大2.0', 'pc28-v3'],
  ])('recognizes %s with its version, result summary and typed detail board', async (id, title, ruleVersion) => {
    const pc28: Game = { ...game, id, title, category: 'PC', rulesReady: true, ruleVersion, period: '20260902001', latestIssue: '20260902000', balls: [9, 1, 9] }
    const quotes = [
      { play_code: 'pc28_dragon_tiger', play_name: '龙虎', odds: 1.98, min_bet: 1, max_bet: 200, max_user_period: 1000 },
      { play_code: 'pc28_dragon_tiger_tie', play_name: '和', odds: 8.5, min_bet: 1, max_bet: 200, max_user_period: 1000 },
    ]
    runtime.gameOdds.mockResolvedValue({ game_id: pc28.id, game_name: pc28.title, show_odds: true, rules_ready: true, rule_version: ruleVersion, items: quotes })
    runtime.assistantStatus.mockResolvedValue({ game_id: pc28.id, rules_ready: true, rule_version: ruleVersion })
    runtime.webPlaceBatch.mockResolvedValue({ game_id: pc28.id, game_name: pc28.title, issue: pc28.period, content: '网投 1 注', lines: [], bet_count: 1, total: 20, balance: 980, accepted_at: '2026-09-02T06:45:02Z', rule_version: ruleVersion })
    const updates = { game: pc28, games: [pc28] }
    render(updates); await settle(); render(updates)
    expect(visibleText(render(updates))).toContain('和值 19大单')
    find(render(updates), node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    const board = find(render(updates), node => typeof node.type === 'function' && node.type.name === 'PC28BetBoard')!
    expect(board).toBeDefined()
    expect(board.props.active).toBe(true)
    const items = [{ play_code: 'pc28_dragon_tiger_tie', play_name: '和', position: 1, selection: '和', amount: 20 }]
    await board.props.onConfirm!(items)
    expect(runtime.webPlaceBatch).toHaveBeenCalledWith(pc28.id, expect.objectContaining({ issue: pc28.period, items, request_id: expect.stringMatching(/^web-board-/) }))
  })

  it('keeps the PC28 board closed when its draw source is unhealthy even if all version snapshots match', async () => {
    const pc28: Game = {
      ...game,
      id: 'pc-canada', title: 'PC加拿大', category: 'PC', rulesReady: true, ruleVersion: 'pc28-v1',
      sourceHealthy: false, sourceError: 'upstream timeout', balls: [9, 1, 9],
    }
    runtime.gameOdds.mockResolvedValue({ game_id: pc28.id, game_name: pc28.title, show_odds: true, rules_ready: true, rule_version: 'pc28-v1', items: [] })
    runtime.assistantStatus.mockResolvedValue({ game_id: pc28.id, rules_ready: true, rule_version: 'pc28-v1' })
    const updates = { game: pc28, games: [pc28] }
    render(updates); await settle()
    const tree = render(updates)
    expect(find(tree, node => node.type === 'main')?.props.className).toContain('rules-blocked')
    expect(find(tree, node => typeof node.type === 'function' && node.type.name === 'PC28BetBoard')).toBeUndefined()
    expect(visibleText(tree)).toContain('开奖同步暂时暂停')
    expect(visibleText(tree)).not.toContain('upstream timeout')
    find(tree, node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    expect(find(render(updates), node => node.props['aria-label'] === '详细投注暂停受理')).toBeDefined()
  })

  it('keeps PC28 wagering closed when an authoritative endpoint reports another variant', async () => {
    const pc28: Game = { ...game, id: 'canada-28', title: '加拿大28', category: 'PC28', rulesReady: true, ruleVersion: 'pc28-v2', balls: [9, 1, 9] }
    runtime.gameOdds.mockResolvedValue({ game_id: pc28.id, game_name: pc28.title, show_odds: true, rules_ready: true, rule_version: 'pc28-v1', items: [] })
    runtime.assistantStatus.mockResolvedValue({ game_id: pc28.id, rules_ready: true, rule_version: 'pc28-v2' })
    const updates = { game: pc28, games: [pc28] }
    render(updates); await settle()
    const tree = render(updates)
    expect(find(tree, node => node.type === 'main')?.props.className).toContain('rules-blocked')
    expect(find(tree, node => typeof node.type === 'function' && node.type.name === 'PC28BetBoard')).toBeUndefined()
    find(tree, node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    expect(find(render(updates), node => node.props['aria-label'] === '详细投注暂停受理')).toBeDefined()
  })

  it('does not mount the SSC1 v3 board while odds still advertise the previous contract', async () => {
    const digit: Game = { ...game, id: 'bingo-ssc-1', title: '宾果时时彩(一)', rulesReady: true, ruleVersion: 'digits5-v3', balls: [9, 8, 1, 2, 3] }
    runtime.gameOdds.mockResolvedValue({ game_id: digit.id, game_name: digit.title, show_odds: true, rules_ready: true, rule_version: 'digits5-v2', items: [] })
    runtime.assistantStatus.mockResolvedValue({ game_id: digit.id, rules_ready: true, rule_version: 'digits5-v3' })
    const updates = { game: digit, games: [digit] }
    render(updates); await settle()
    const blocked = render(updates)
    expect(find(blocked, node => node.type === 'main')?.props.className).toContain('rules-blocked')
    expect(find(blocked, node => typeof node.type === 'function' && node.type.name === 'DigitBetBoard')).toBeUndefined()
    find(blocked, node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    const detail = render(updates)
    expect(find(detail, node => typeof node.type === 'function' && node.type.name === 'DigitBetBoard')).toBeUndefined()
    expect(find(detail, node => node.props['aria-label'] === '详细投注暂停受理')).toBeDefined()
  })

  it('does not reuse a same-version odds response from another upgraded room', async () => {
    const digit: Game = { ...game, id: 'speed-ssc', title: '极速时时彩', rulesReady: true, ruleVersion: 'digits5-v3', balls: [9, 8, 1, 2, 3] }
    runtime.gameOdds.mockResolvedValue({ game_id: 'au-lucky-5', game_name: '澳洲幸运5', show_odds: true, rules_ready: true, rule_version: 'digits5-v3', items: [] })
    runtime.assistantStatus.mockResolvedValue({ game_id: digit.id, rules_ready: true, rule_version: 'digits5-v3' })
    const updates = { game: digit, games: [digit] }
    render(updates); await settle()
    const blocked = render(updates)
    expect(find(blocked, node => node.type === 'main')?.props.className).toContain('rules-blocked')
    find(blocked, node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    expect(find(render(updates), node => typeof node.type === 'function' && node.type.name === 'DigitBetBoard')).toBeUndefined()
  })

  it('does not mount the Bingo Racing A board while assistant status is on another contract', async () => {
    const racing: Game = { ...game, id: 'bingo-racing-a', title: '宾果赛车(A)', rulesReady: true, ruleVersion: 'racing-v2' }
    runtime.gameOdds.mockResolvedValue({ game_id: racing.id, game_name: racing.title, show_odds: true, rules_ready: true, rule_version: 'racing-v2', items: [] })
    runtime.assistantStatus.mockResolvedValue({ game_id: racing.id, rules_ready: true, rule_version: 'racing-v1' })
    const updates = { game: racing, games: [racing] }
    render(updates); await settle()
    const blocked = render(updates)
    expect(find(blocked, node => node.type === 'main')?.props.className).toContain('rules-blocked')
    find(blocked, node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    const detail = render(updates)
    expect(find(detail, node => typeof node.type === 'function' && node.type.name === 'FullBetBoard')).toBeUndefined()
    expect(find(detail, node => node.props['aria-label'] === '详细投注暂停受理')).toBeDefined()
  })

  it('hides mode labels and visible edge handles, then returns through the board collapse control', async () => {
    render(); await settle(); render()
    expect(find(render(), node => node.props['aria-label'] === '投注模式')).toBeUndefined()
    expect(visibleText(render())).not.toContain('模式1')
    expect(visibleText(render())).not.toContain('模式2')
    find(render(), node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    const detailed = render()
    const board = find(detailed, node => typeof node.type === 'function' && node.type.name === 'FullBetBoard')!
    expect(board.props.embedded).toBe(true)
    expect(board.props.active).toBe(true)
    expect(find(detailed, node => node.props.className === 'bet-chat')?.props.hidden).toBe(true)
    expect(find(detailed, node => node.props.className?.includes('detail-mode-open') === true)).toBeDefined()
    expect(find(detailed, node => node.props.className?.includes('room-mode-edge-switch') === true)).toBeUndefined()
    expect(find(detailed, node => ['打开详细投注', '返回聊天投注'].includes(node.props['aria-label'] || ''))).toBeUndefined()

    board.props.onClose!()
    const returned = render()
    expect(find(returned, node => node.props.className === 'bet-chat')?.props.hidden).toBe(false)
    expect(find(returned, node => typeof node.type === 'function' && node.type.name === 'FullBetBoard')?.props.active).toBe(false)
  })

  it('switches only on a deliberate horizontal edge swipe and ignores vertical movement', async () => {
    render(); await settle(); render()
    const target = { style: { setProperty: vi.fn(), removeProperty: vi.fn() }, setPointerCapture: vi.fn(), hasPointerCapture: vi.fn(() => true), releasePointerCapture: vi.fn() }
    const point = (clientX: number, clientY: number): TestPointerEvent => ({ pointerId: 7, clientX, clientY, isPrimary: true, currentTarget: target, preventDefault: vi.fn() })
    const vertical = find(render(), node => node.props.className?.includes('room-mode-edge-gesture from-right') === true)!
    expect(vertical.props['aria-hidden']).toBeTruthy()
    expect(vertical.props.onClick).toBeUndefined()
    vertical.props.onPointerDown!(point(390, 300))
    vertical.props.onPointerMove!(point(380, 380))
    vertical.props.onPointerUp!(point(380, 380))
    expect(find(render(), node => node.props.className?.includes('room-mode-edge-gesture from-right') === true)).toBeDefined()

    const open = find(render(), node => node.props.className?.includes('room-mode-edge-gesture from-right') === true)!
    open.props.onPointerDown!(point(390, 300))
    open.props.onPointerMove!(point(320, 304))
    open.props.onPointerUp!(point(320, 304))
    const close = find(render(), node => node.props.className?.includes('room-mode-edge-gesture from-left') === true)!
    expect(close).toBeDefined()
    close.props.onPointerDown!(point(8, 300))
    close.props.onPointerMove!(point(72, 303))
    close.props.onPointerUp!(point(72, 303))
    expect(find(render(), node => node.props.className?.includes('room-mode-edge-gesture from-right') === true)).toBeDefined()
  })

  it('opens verified Bingo Mark Six directly in its only typed web board and never exposes chat switching', async () => {
    const bingo: Game = { ...game, id: 'bingo-mark-six', title: '宾果六合彩', category: '六合彩', rulesReady: true, ruleVersion: 'mark6-v2', period: '115049456', latestIssue: '115049455', balls: [5, 36, 40, 47, 29, 2, 18] }
    const quote = { play_code: 'marksix_special_a_number', play_name: '特码A', odds: 48, min_bet: 1, max_bet: 200, max_user_period: 1000 }
    runtime.gameOdds.mockResolvedValue({ game_id: bingo.id, game_name: bingo.title, show_odds: true, rules_ready: true, rule_version: 'mark6-v2', items: [quote, { ...quote, play_code: 'marksix_color_wave', play_name: '色波' }] })
    runtime.assistantStatus.mockResolvedValue({ game_id: bingo.id, rules_ready: true, rule_version: 'mark6-v2' })
    runtime.webPlaceBatch.mockResolvedValue({ game_id: bingo.id, game_name: bingo.title, issue: bingo.period, content: '网投 1 注', lines: [], bet_count: 1, total: 20, balance: 980, accepted_at: '2026-09-01T06:45:02Z' })
    const updates = { game: bingo, games: [bingo] }
    const tree = render(updates)
    await settle()
    const settled = render(updates)
    expect(tree).toBeDefined()
    const board = find(settled, node => typeof node.type === 'function' && node.type.name === 'MarkSixBetBoard')!
    expect(board).toBeDefined()
    expect(board.props.active).toBe(true)
    expect(find(settled, node => node.props.className === 'bet-chat')).toBeUndefined()
    expect(find(settled, node => node.props.className?.includes('ticket-strip') === true)).toBeUndefined()
    expect(find(settled, node => node.type === QuickActions)).toBeUndefined()
    expect(find(settled, node => typeof node.type === 'function' && ['FullBetBoard', 'DigitBetBoard'].includes(node.type.name))).toBeUndefined()
    expect(find(settled, node => node.props['aria-label'] === '详细投注暂停受理')).toBeUndefined()
    expect(elements(settled).some(node => (node.props['aria-label'] ?? '').includes('返回聊天'))).toBe(false)
    expect(find(settled, node => node.props.className?.includes('room-mode-edge-gesture') === true)).toBeUndefined()
    expect(find(settled, node => node.props.className?.includes('room-mode-edge-switch') === true)).toBeUndefined()
    expect(visibleText(settled)).not.toContain('模式1')
    expect(visibleText(settled)).not.toContain('模式2')
    expect(visibleText(find(settled, node => node.props.className === 'last-draw')!)).toContain('特码 18 红波 小双')
    const drawBallCells = elements(find(settled, node => node.props.className === 'last-draw')!).filter(node => node.type === MarkSixDrawBall)
    expect(drawBallCells).toHaveLength(7)
    const renderedBalls = drawBallCells.map(node => MarkSixDrawBall(node.props as ComponentProps<typeof MarkSixDrawBall>))
    expect(elements(renderedBalls[6]).find(node => node.type === 'b')?.props.className).toContain('mark-six-special-ball')
    expect(renderedBalls.map(cell => elements(cell).find(node => node.type === 'b')?.props.className)).toEqual(expect.arrayContaining([expect.stringContaining('wave-red'), expect.stringContaining('wave-blue'), expect.stringContaining('wave-green')]))
    expect(renderedBalls.map(visibleText).join('')).toBe('05虎36羊40兔47猴29虎02蛇18牛')

    const items = [{ play_code: 'marksix_special_a_number', play_name: '特码A', position: 7, selection: '18', amount: 20 }]
    await board.props.onConfirm!(items)
    expect(runtime.webPlaceBatch).toHaveBeenCalledWith(bingo.id, expect.objectContaining({ issue: bingo.period, items, request_id: expect.stringMatching(/^web-board-/) }))
    await board.props.onConfirm!([{ play_code: 'marksix_color_wave', play_name: '色波', position: 7, selection: '红波', amount: 20 }])
    expect(runtime.webPlaceBatch).toHaveBeenCalledTimes(1)
    expect(runtime.assistantPlace).not.toHaveBeenCalled()
  })

  it('submits Mark Six linked and tiered parent tickets only when every private price row is current', async () => {
    const bingo: Game = { ...game, id: 'bingo-mark-six', title: '宾果六合彩', category: '六合彩', rulesReady: true, ruleVersion: 'mark6-v2', period: '115049456', latestIssue: '115049455', balls: [5, 36, 40, 47, 29, 2, 18] }
    const quote = { play_code: '', play_name: '组合定价', odds: 4.2, min_bet: 1, max_bet: 200, max_user_period: 1000 }
    runtime.gameOdds.mockResolvedValue({ game_id: bingo.id, game_name: bingo.title, show_odds: true, rules_ready: true, rule_version: 'mark6-v2', items: [
      { ...quote, play_code: 'marksix_link_zodiac_2_rat', odds: 4.2 },
      { ...quote, play_code: 'marksix_link_zodiac_2_horse', odds: 3.55 },
      { ...quote, play_code: 'marksix_combo_3_2_exact2', odds: 20.1 },
      { ...quote, play_code: 'marksix_combo_3_2_exact3', odds: 125 },
    ] })
    runtime.assistantStatus.mockResolvedValue({ game_id: bingo.id, rules_ready: true, rule_version: 'mark6-v2' })
    runtime.webPlaceBatch.mockImplementation(async (_gameID: string, payload: { items: unknown[] }) => ({ game_id: bingo.id, game_name: bingo.title, issue: bingo.period, content: `网投 ${payload.items.length} 注`, lines: [], bet_count: payload.items.length, total: 20, balance: 980, accepted_at: '2026-09-01T06:45:02Z', rule_version: 'mark6-v2' }))
    const updates = { game: bingo, games: [bingo] }
    render(updates); await settle()
    const board = find(render(updates), node => typeof node.type === 'function' && node.type.name === 'MarkSixBetBoard')!

    const linked = [{ play_code: 'marksix_link_zodiac_2', play_name: '2连肖', position: 0, selection: '鼠,马', amount: 20 }]
    await board.props.onConfirm!(linked)
    expect(runtime.webPlaceBatch).toHaveBeenLastCalledWith(bingo.id, expect.objectContaining({ issue: bingo.period, items: linked, request_id: expect.stringMatching(/^web-board-/) }))

    const tiered = [{ play_code: 'marksix_combo_3_2', play_name: '三中二', position: 0, selection: '1,2,3', amount: 20 }]
    await board.props.onConfirm!(tiered)
    expect(runtime.webPlaceBatch).toHaveBeenLastCalledWith(bingo.id, expect.objectContaining({ issue: bingo.period, items: tiered, request_id: expect.stringMatching(/^web-board-/) }))
    expect(runtime.webPlaceBatch).toHaveBeenCalledTimes(2)

    await board.props.onConfirm!([{ play_code: 'marksix_link_zodiac_2', play_name: '2连肖', position: 0, selection: '鼠,牛', amount: 20 }])
    expect(runtime.webPlaceBatch).toHaveBeenCalledTimes(2)
    const dialog = find(render(updates), node => typeof node.type === 'function' && node.type.name === 'ActionDialog')!
    expect(dialog.props.description).toContain('赔率或规则待配置')
  })

  it('reloads stale odds when the catalog publishes a new rule version', async () => {
    const bingo: Game = { ...game, id: 'bingo-mark-six', title: '宾果六合彩', category: '六合彩', rulesReady: true, ruleVersion: '', period: '115049500', latestIssue: '115049499', balls: [5, 7, 15, 19, 23, 25, 29] }
    const quote = { play_code: 'marksix_special_a_number', play_name: '特码A', odds: 48, min_bet: 1, max_bet: 200, max_user_period: 1000 }
    runtime.gameOdds
      .mockResolvedValueOnce({ game_id: bingo.id, game_name: bingo.title, show_odds: true, rules_ready: false, rules_message: '该彩种尚未配置完整玩法，暂不受理投注', items: [] })
      .mockResolvedValueOnce({ game_id: bingo.id, game_name: bingo.title, show_odds: true, rules_ready: true, rule_version: 'mark6-v2', items: [quote] })
    runtime.assistantStatus
      .mockResolvedValueOnce({ game_id: bingo.id, rules_ready: false, rule_version: 'mark6-v1', rules_message: '该彩种尚未配置完整玩法，暂不受理投注' })
      .mockResolvedValueOnce({ game_id: bingo.id, rules_ready: true, rule_version: 'mark6-v2' })

    const blocked = { game: bingo, games: [bingo] }
    render(blocked); await settle(); render(blocked)
    expect(runtime.gameOdds).toHaveBeenCalledTimes(1)
    expect(visibleText(render(blocked))).toContain('当前仅提供开奖查看和聊天，投注暂未开放')
    expect(visibleText(render(blocked))).not.toContain('该彩种尚未配置完整玩法')

    const verifiedGame = { ...bingo, ruleVersion: 'mark6-v2' }
    const verified = { game: verifiedGame, games: [verifiedGame] }
    render(verified); await settle()
    const tree = render(verified)
    const board = find(tree, node => typeof node.type === 'function' && node.type.name === 'MarkSixBetBoard')!
    expect(runtime.gameOdds).toHaveBeenCalledTimes(2)
    expect(runtime.assistantStatus).toHaveBeenCalledTimes(2)
    expect(board.props.rulesReady).toBe(true)
    expect(board.props.oddsInfo).toMatchObject({ rules_ready: true, rule_version: 'mark6-v2' })
    expect(visibleText(tree)).not.toContain('当前仅提供开奖查看和聊天，投注暂未开放')
  })

  it('rejects an invalid board command before the API call and labels it as a format error, not missing odds', async () => {
    render(); await settle(); render()
    find(render(), node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    const board = find(render(), node => typeof node.type === 'function' && node.type.name === 'FullBetBoard')!
    board.props.onConfirm!('1/1/20#6/龙/20')
    await settle()
    expect(runtime.assistantPlace).not.toHaveBeenCalled()
    const dialog = find(render(), node => typeof node.type === 'function' && node.type.name === 'ActionDialog')!
    expect(dialog.props.description).toContain('无法识别投注格式')
    expect(dialog.props.description).not.toContain('当前玩法赔率待配置')
  })

  it('retains an assistant rules denial across a rollover until the new status is verified', async () => {
    runtime.assistantStatus.mockResolvedValueOnce({ rules_ready: false, rules_message: '玩法核验未通过' })
    render(); await settle(); render()
    expect(visibleText(render())).toContain('当前仅提供开奖查看和聊天，投注暂未开放')
    expect(visibleText(render())).not.toContain('玩法核验未通过')
    runtime.assistantStatus.mockReturnValueOnce(new Promise(() => undefined))
    const next = { ...game, period: '34137154' }
    render({ game: next }); await settle()
    expect(runtime.assistantStatus).toHaveBeenCalledTimes(2)
    const tree = render({ game: next })
    expect(visibleText(tree)).toContain('当前仅提供开奖查看和聊天，投注暂未开放')
    expect(find(tree, node => node.props.className?.includes('ticket-native-input') === true)).toBeDefined()
    find(tree, node => typeof node.props.onQuickBet === 'function')!.props.onQuickBet!()
    expect(find(render({ game: next }), node => typeof node.type === 'function' && node.type.name === 'FullBetBoard')).toBeUndefined()
  })

  it('does not let a delayed older status overwrite a newer rules denial', async () => {
    let resolveOld: (value: { rules_ready: boolean }) => void = () => undefined
    runtime.assistantStatus.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
    render(); await settle()
    runtime.assistantStatus.mockResolvedValueOnce({ rules_ready: false, rules_message: '新状态暂停受理' })
    const next = { ...game, period: '34137154' }
    render({ game: next }); await settle(); render({ game: next })
    resolveOld({ rules_ready: true }); await settle()
    expect(visibleText(render({ game: next }))).toContain('当前仅提供开奖查看和聊天，投注暂未开放')
    expect(visibleText(render({ game: next }))).not.toContain('新状态暂停受理')
    expect(find(render({ game: next }), node => node.props.className?.includes('ticket-native-input') === true)).toBeDefined()
  })

  it('requires explicit verified readiness to clear an earlier assistant rules denial', async () => {
    runtime.assistantStatus.mockResolvedValueOnce({ rules_ready: false, rules_message: '玩法核验未通过' })
    render(); await settle(); render()
    runtime.assistantStatus.mockResolvedValueOnce({ accepting: true })
    const next = { ...game, period: '34137154' }
    render({ game: next }); await settle()
    expect(visibleText(render({ game: next }))).toContain('当前仅提供开奖查看和聊天，投注暂未开放')
    runtime.assistantStatus.mockResolvedValueOnce({ rules_ready: true, accepting: true })
    const verified = { ...game, period: '34137155' }
    render({ game: verified }); await settle()
    const tree = render({ game: verified })
    expect(visibleText(tree)).not.toContain('当前仅提供开奖查看和聊天，投注暂未开放')
    expect(find(tree, node => node.props.className?.includes('ticket-native-input') === true)).toBeUndefined()
  })

  it('never substitutes the current numbers for an empty older draw', async () => {
    runtime.draws = [{ id: 1, game_id: game.id, issue: '34137150', numbers: [], draw_at: '2026-08-30T06:43:00Z' }]
    render(); await settle()
    const recent = find(render(), node => node.props.className === 'recent-draws')!
    const row = find(recent, node => node.type === 'article')!
    expect(visibleText(row)).toBe('34137150')
    expect(find(row, node => node.type === 'b')).toBeUndefined()
  })

  it('rejects excessive decimal precision before sending a chat bet', async () => {
    render(); await settle()
    openKeyboard().onSelectOption('1/1/1.005')
    keyboard(render()).onConfirm()
    await settle()
    expect(runtime.command).not.toHaveBeenCalled()
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

  it('bounds live chat after a paged catch-up without rewinding the cursor or duplicating a sent command', async () => {
    const chat = (id: number, content = 'chat'): ChatMessage => ({ id, game_id: game.id, user_id: 8, nickname: '玩家', room_type: 'group', room_scope: 'agent:2', content, message_type: 'text', mine: false, created_at: new Date(Date.parse('2026-08-30T06:44:00Z') + id * 1000).toISOString() })
    const count = GAME_TIMELINE_LIMIT + 50
    const all = Array.from({ length: count }, (_, index) => chat(index + 1))
    for (let offset = 0; offset < count; offset += 50) runtime.chatMessages.mockResolvedValueOnce({ items: all.slice(offset, offset + 50), has_more: offset + 50 < count })
    render(); await settle(); render(); await settle()
    const timeline = () => find(render(), node => node.type === GameTimeline)!.props as unknown as ComponentProps<typeof GameTimeline>
    expect(timeline().messages).toHaveLength(GAME_TIMELINE_LIMIT)
    expect(timeline().messages[0].id).toBe(51)
    expect(timeline().messages.at(-1)?.id).toBe(count)

    // A stale duplicate page cannot repopulate the evicted cache or loop just
    // because has_more is incorrectly left on by a recovery response.
    runtime.chatMessages.mockResolvedValueOnce({ items: [chat(1), chat(count, 'updated')], has_more: true })
    const event = new Event('test-room-ws')
    Object.assign(event, { detail: { type: 'chat_message', data: { game_id: game.id } } })
    window.dispatchEvent(event); await settle()
    expect(runtime.chatMessages.mock.calls.at(-1)?.[3].after_id).toBe(count)
    expect(timeline().messages).toHaveLength(GAME_TIMELINE_LIMIT)
    expect(timeline().messages[0].id).toBe(51)
    expect(timeline().messages.at(-1)?.content).toBe('updated')

    const sent = { ...chat(count + 1, '4/88'), mine: true }
    runtime.command.mockResolvedValueOnce(sent)
    runtime.chatMessages.mockResolvedValueOnce({ items: [sent, chat(count + 2, '下单成功')], has_more: false })
    openKeyboard().onSelectOption(sent.content)
    keyboard(render()).onConfirm()
    await settle()
    expect(timeline().messages).toHaveLength(GAME_TIMELINE_LIMIT)
    expect(timeline().messages.filter(row => row.id === sent.id)).toEqual([sent])
    expect(timeline().messages.at(-1)?.id).toBe(count + 2)
    keyboard(render()).onShortcut('repeat')
    expect(visibleText(input(render()))).toBe(sent.content)
    window.dispatchEvent(event); await settle()
    expect(runtime.chatMessages.mock.calls.at(-1)?.[3].after_id).toBe(count + 2)
  })

  it('caps feed and restored detailed-receipt caches without mixing their issue or mutating API records', async () => {
    const count = GAME_TIMELINE_LIMIT + 30
    const at = (index: number) => new Date(Date.parse('2026-08-30T06:44:00Z') + index * 1000).toISOString()
    const feed = Array.from({ length: count }, (_, index) => ({ nickname: `会员${index}`, amount: 20, detail: `冠军 ${index}`, created_at: at(index) })).reverse()
    const history = Array.from({ length: count }, (_, index) => ({ game_id: game.id, issue: game.period, content: `1/${index + 1}`, lines: [], total: index + 1, balance: 100, accepted_at: at(index) }))
    runtime.feed.mockResolvedValueOnce(feed)
    runtime.assistantHistory.mockResolvedValueOnce(history)
    render(); await settle(); render(); await settle()
    const timeline = find(render(), node => node.type === GameTimeline)!.props as unknown as ComponentProps<typeof GameTimeline>
    expect(timeline.feed).toHaveLength(GAME_TIMELINE_LIMIT)
    expect(timeline.feed[0].nickname).toBe('会员30')
    expect(timeline.feed.at(-1)?.nickname).toBe(`会员${count - 1}`)
    expect(timeline.feed.every(row => row.issue === game.period)).toBe(true)
    expect(timeline.tickets).toHaveLength(GAME_TIMELINE_LIMIT)
    expect(timeline.tickets[0].content).toBe('1/31')
    expect(timeline.tickets.at(-1)?.content).toBe(`1/${count}`)
    expect(feed[0].nickname).toBe(`会员${count - 1}`)
    expect(history).toHaveLength(count)
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
