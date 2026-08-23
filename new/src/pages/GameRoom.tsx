import { useCallback, useEffect, useRef, useState } from 'react'
import type { Game, Theme } from '../types'
import { ballTone } from '../data/games'
import { Avatar } from '../components/Avatar'
import { Icon } from '../components/Icon'
import { ActionDialog } from '../components/Dialogs'
import { playNotificationSound } from '../utils/notificationAudio'
import { CheckIn } from './CheckIn'
import { parseBetInput, type ParsedBet } from '../utils/betParser'
import { betsApi, type AssistantDrawStatus, type MemberBet } from '../api/bets'
import { useGameDraws } from '../hooks/useGameDraws'
import type { DrawResult } from '../api/lottery'
import { useMemberPreferences } from '../hooks/useMemberPreferences'
import { WS_EVENT, type WsEvent } from '../hooks/useWebSocket'
import { portalApi, type GameFeedItem, type GameOdds } from '../api/portal'
import type { WalletActionSlug } from '../router'

type Props = {
  game: Game
  games: Game[]
  theme: Theme
  nickname: string
  balance: number
  onBack: () => void
  onOpenGame: (gameId: string) => void
  onOpenService: () => void
  onOpenWallet: (action?: WalletActionSlug) => void
  onOpenResults: () => void
  startWithQuickMenu?: boolean
  onRefreshBalance: () => Promise<void>
}
type Dialog = 'orders' | 'assist' | 'required' | 'bet-error' | null
type BetMode = 'quick' | 'dual' | 'numbers'
type AcceptedTicket = { gameId: string; content: string; lines: string[]; total: number; issue: string; acceptedAt: string }

function mergeAcceptedTickets(...groups: AcceptedTicket[][]) {
  const seen = new Set<string>()
  return groups.flat().filter((ticket) => {
    const key = `${ticket.gameId}:${ticket.issue}:${ticket.acceptedAt}:${ticket.content}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }).sort((left, right) => {
    const leftTime = new Date(left.acceptedAt).getTime()
    const rightTime = new Date(right.acceptedAt).getTime()
    if (!Number.isFinite(leftTime) || !Number.isFinite(rightTime)) return 0
    return leftTime - rightTime
  })
}

const quickKeys = ['大', '1', '2', '3', '←', '小', '4', '5', '6', '龙', '单', '7', '8', '9', '冠亚', '双', '0', '#', '/', '虎']
const quickOptions = new Set(['大', '小', '单', '双', '龙', '虎', '冠亚'])
const dualOptions = ['冠军大', '冠军小', '冠军单', '冠军双', '冠军龙', '冠军虎', '亚军大', '亚军小', '亚军单', '亚军双', '冠亚和大', '冠亚和小']
const modes: Array<{ id: BetMode; label: string }> = [{ id: 'quick', label: '快捷' }, { id: 'dual', label: '两面盘' }, { id: 'numbers', label: '号码 1~10' }]
const drawPositionNames = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十']

function shortIssue(issue: string) {
  return issue.match(/^(\d{8})-/)?.[1] ?? issue
}

function dueState(due: string) {
  const units = due.split(':').map(Number)
  const seconds = units.length === 3
    ? units[0] * 3600 + units[1] * 60 + units[2]
    : units.length === 2
      ? units[0] * 60 + units[1]
      : Number.NaN
  if (!Number.isFinite(seconds)) return { label: '状态同步中', tone: 'syncing' }
  if (seconds <= 0) return { label: '封盘中', tone: 'closed' }
  if (seconds <= 30) return { label: `${seconds} 秒后封盘`, tone: 'closing' }
  return { label: '正在受理', tone: 'open' }
}

function payloadLabel(payload: ParsedBet['payloads'][number]) {
  return payload.play_name || `第${drawPositionNames[payload.position - 1] ?? payload.position}名${payload.selection}`
}

function crownMeta(balls: number[]) {
  if (balls.length < 2) return { crownResult: '—', dragonTiger: '—' }
  const crownSum = balls[0] + balls[1]
  const crownResult = `${crownSum}${crownSum >= 12 ? '大' : '小'}${crownSum % 2 ? '单' : '双'}`
  const dragonTiger = balls.slice(0, 5).map((ball, index) => (balls[9 - index] !== undefined && ball > balls[9 - index] ? '龙' : '虎')).join('')
  return { crownResult, dragonTiger }
}

/** 彩种会话：快捷输入、两面盘和注单提交接后端 API。 */
export function GameRoom({ game, games, theme, nickname, balance, onBack, onOpenGame, onOpenService, onOpenWallet, onOpenResults, startWithQuickMenu = false, onRefreshBalance }: Props) {
  const [betInput, setBetInput] = useState('')
  const [showKeyboard, setShowKeyboard] = useState(false)
  const [showQuickBet, setShowQuickBet] = useState(false)
  const [betMode, setBetMode] = useState<BetMode>('quick')
  const { drawHistoryLimit, defaultBetMode, fontScale } = useMemberPreferences()
  const [dialog, setDialog] = useState<Dialog>(null)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [submittedBets, setSubmittedBets] = useState<AcceptedTicket[]>([])
  const [memberBets, setMemberBets] = useState<MemberBet[]>([])
  const [betError, setBetError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [showAddMenu, setShowAddMenu] = useState(startWithQuickMenu)
  const [showGameSwitcher, setShowGameSwitcher] = useState(false)
  const [showCheckIn, setShowCheckIn] = useState(false)
  const [feedReady, setFeedReady] = useState(false)
  const chatRef = useRef<HTMLElement>(null)
  const gameSessionRef = useRef(`${game.id}:${game.period}`)
  const markFeedReady = useCallback(() => setFeedReady(true), [])
  const { draws, loading: drawsLoading } = useGameDraws(game.id, drawHistoryLimit)
  const [oddsInfo, setOddsInfo] = useState<GameOdds | null>(null)
  const [assistantStatus, setAssistantStatus] = useState<AssistantDrawStatus | null>(null)

  // 游戏和期号共同构成一段独立会话。即使组件未来被其他入口复用，
  // 也不能把上一局的输入、订单回执或弹层带到下一局。
  useEffect(() => {
    gameSessionRef.current = `${game.id}:${game.period}`
    setBetInput('')
    setShowKeyboard(false)
    setShowQuickBet(false)
    setShowAddMenu(false)
    setShowGameSwitcher(false)
    setHistoryOpen(false)
    setDialog(null)
    setBetError('')
    setSubmittedBets([])
    setMemberBets([])
    setOddsInfo(null)
    setAssistantStatus(null)
    setFeedReady(false)
  }, [game.id, game.period])

  useEffect(() => {
    void portalApi.gameOdds(game.id).then(setOddsInfo).catch(() => setOddsInfo(null))
  }, [game.id])

  useEffect(() => {
    let cancelled = false
    const loadAssistant = () => {
      void betsApi.assistantStatus(game.id).then((result) => {
        if (!cancelled) setAssistantStatus(result)
      }).catch(() => {
        if (!cancelled) setAssistantStatus(null)
      })
    }
    loadAssistant()
    const timer = window.setInterval(loadAssistant, 12_000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [game.id])

  useEffect(() => {
    if (startWithQuickMenu) setShowAddMenu(true)
  }, [startWithQuickMenu])

  const defaultOdds = oddsInfo?.items.find((item) => item.play_code === 'ball_1_5')?.odds
    ?? oddsInfo?.items[0]?.odds
    ?? 1.92

  const loadBets = useCallback(async () => {
    const requestSession = `${game.id}:${game.period}`
    try {
      // Load recent bets for this game, not only the issue currently displayed.
      // The issue may advance while the member is outside the room; filtering
      // to the new issue would make their just-placed ticket appear to vanish.
      const result = await betsApi.list({ game_id: game.id, page_size: 50 })
      if (gameSessionRef.current === requestSession) setMemberBets(result.items)
    } catch {
      if (gameSessionRef.current === requestSession) setMemberBets([])
    }
  }, [game.id, game.period])

  const loadAssistantHistory = useCallback(async () => {
    const requestSession = `${game.id}:${game.period}`
    try {
      const history = await betsApi.assistantHistory(game.id, 20)
      if (gameSessionRef.current !== requestSession) return
      const restored = history.map((ticket) => ({
        gameId: ticket.game_id,
        content: ticket.content,
        lines: ticket.lines.map((line) => line.label),
        total: ticket.total,
        issue: ticket.issue,
        acceptedAt: ticket.accepted_at,
      }))
      setSubmittedBets((current) => mergeAcceptedTickets(current, restored))
    } catch {
      // Keep an already-rendered local acknowledgement if history recovery is
      // temporarily unavailable. A later room entry will retry from server.
    }
  }, [game.id, game.period])

  useEffect(() => {
    setBetMode(defaultBetMode)
  }, [game.id, defaultBetMode])

  useEffect(() => {
    void loadBets()
    void loadAssistantHistory()
  }, [loadAssistantHistory, loadBets])

  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadBets()
      void onRefreshBalance()
    }, 10_000)
    return () => window.clearInterval(timer)
  }, [loadBets, onRefreshBalance])

  useEffect(() => {
    setFeedReady(false)
  }, [game.id, game.period])

  // 开奖提示属于当前正在观看的彩种，不应在大厅、钱包或消息页响起。
  useEffect(() => {
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail?.type === 'draw_update' && detail.data.game_id === game.id) {
        playNotificationSound('lottery')
        void loadBets()
        void onRefreshBalance()
      }
    }
    window.addEventListener(WS_EVENT, onWs)
    return () => window.removeEventListener(WS_EVENT, onWs)
  }, [game.id, loadBets, onRefreshBalance])

  useEffect(() => {
    const scrollToLatest = () => chatRef.current?.scrollTo({ top: chatRef.current.scrollHeight, behavior: 'auto' })
    const frame = window.requestAnimationFrame(scrollToLatest)
    const timer = window.setTimeout(scrollToLatest, 80)
    return () => {
      window.cancelAnimationFrame(frame)
      window.clearTimeout(timer)
    }
  }, [game.id, game.period, feedReady, submittedBets, draws[0]?.id])

  const appendNumber = (number: number) => setBetInput((current) => `${current}${number}`)
  const appendOption = (option: string) => setBetInput((current) => `${current}${option}`)
  const clearSelection = () => setBetInput('')
  const removeNumber = () => setBetInput((current) => current.slice(0, -1))
  const submitBet = async (rawInput?: string, fallbackAmount?: number) => {
    if (dueState(game.due).tone === 'closed') {
      setBetError('本期已封盘，不能继续下注；请等待下一期开始受理。')
      setDialog('bet-error')
      return
    }
    let content = (rawInput ?? betInput).trim()
    if (!content) return setDialog('required')
    if (fallbackAmount && !content.includes('/')) content = `${content}/${fallbackAmount}`
    setSubmitting(true)
    setBetError('')
    const requestSession = `${game.id}:${game.period}`
    try {
      // The assistant resolves the live issue on the server, preventing a
      // countdown refresh from submitting a stale period captured by the UI.
      const requestId = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
      const accepted = await betsApi.assistantPlace(game.id, { content, request_id: requestId })
      // 请求返回时彩种或期号可能已切换；余额可以刷新，但旧会话不能继续写入 UI。
      if (gameSessionRef.current !== requestSession) {
        await onRefreshBalance()
        return
      }
      setSubmittedBets((bets) => mergeAcceptedTickets(bets, [{
        gameId: game.id,
        content: accepted.content,
        lines: accepted.lines.map((line) => line.label),
        total: accepted.total,
        issue: accepted.issue,
        acceptedAt: accepted.accepted_at,
      }]))
      setAssistantStatus((current) => current ? { ...current, issue: accepted.issue, accepting: true } : current)
      await onRefreshBalance()
      await loadBets()
      playNotificationSound('lottery')
      clearSelection()
      setShowKeyboard(false)
      setShowQuickBet(false)
    } catch (reason) {
      setBetError(reason instanceof Error ? reason.message : '投注失败')
      setDialog('bet-error')
    } finally {
      setSubmitting(false)
    }
  }

  const cancelBet = async (id: number) => {
    try {
      await betsApi.cancel(id)
      await onRefreshBalance()
      await loadBets()
    } catch (reason) {
      setBetError(reason instanceof Error ? reason.message : '撤单失败')
      setDialog('bet-error')
    }
  }

  const drawPositionLabels = drawPositionNames.slice(0, Math.max(game.balls.length, 5))
  const recentDraws = draws.slice(0, 8).map((draw) => {
    const balls = draw.numbers.length ? draw.numbers : game.balls
    const meta = crownMeta(balls)
    return { period: draw.issue, balls, ...meta }
  })
  const latestMeta = crownMeta(game.balls)
  const acceptance = dueState(game.due)
  const assistantAcceptance = assistantStatus
    ? assistantStatus.accepting ? '本期正在受理，识别后将由服务端统一扣分。' : '本期已封盘，请等待下一期开始受理。'
    : acceptance.tone === 'closed' ? '本期已封盘，请等待下一期开始受理。' : `本期${acceptance.label}。`
  if (showCheckIn) {
    return (
      <div className={`check-in-shell theme-${theme}`}>
        <CheckIn onBack={() => setShowCheckIn(false)} onComplete={() => void onRefreshBalance()} />
      </div>
    )
  }
  return <main className={`game-room theme-${theme} font-scale-${fontScale}`} onClick={() => { setShowKeyboard(false); setShowAddMenu(false) }}>
    <header className="game-header"><button aria-label="返回大厅" onClick={onBack}><Icon name="back" /></button><b>{game.title}</b><div className="game-header-meta"><small>余额: {balance.toFixed(2)}</small><small>第 {shortIssue(game.period)} 期</small></div></header>
    <section className="game-info"><div><span>截止倒计时</span><b>{game.due.split('').map((number, index) => <i key={index}>{number}</i>)}</b><small className={`game-acceptance ${acceptance.tone}`}>{acceptance.label}</small></div><button onClick={onOpenResults}>开奖记录</button><button onClick={() => setDialog('orders')}>注单</button></section>
    <section className={`draw-history ${historyOpen ? 'open' : ''}`}><button className="last-draw" aria-expanded={historyOpen} onClick={() => setHistoryOpen((open) => !open)}><span>上期 {shortIssue(recentDraws[0]?.period ?? game.period)}</span><div>{game.balls.map((number, index) => <b className={ballTone(number)} key={index}>{number}</b>)}</div><small>冠亚 <b>{latestMeta.crownResult}</b></small></button><div className="recent-draws" aria-hidden={!historyOpen}><header><span>期数</span><b>{drawPositionLabels.map((label) => <i key={label}>{label}</i>)}</b><small>冠亚和 · 龙虎</small></header>{drawsLoading && <p className="recent-draws-loading">加载开奖…</p>}{recentDraws.slice(0, 5).map((draw) => <article key={draw.period}><span>{shortIssue(draw.period)}</span><div>{draw.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div><small><b>{draw.crownResult}</b><em>{draw.dragonTiger}</em></small></article>)}<button className="more-draws" onClick={onOpenResults}>查看更多开奖</button></div></section>
    <section className="bet-chat" ref={chatRef}>
      <p>以上全接，以下无效。</p>
      <div className="admin-message assistant-notice">
        <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
        <div><small>开奖助手 · 24小时在线</small><article><b>【{game.title} - {shortIssue(assistantStatus?.issue ?? game.period)}】</b><hr />{assistantAcceptance}<br /><br />按“玩法/金额#玩法/金额”输入，例如 3大/200#12345/1000。系统会先完整识别，再一次性受理与扣分。</article></div>
      </div>
      {draws[0] && <DrawAssistantMessage game={game} draw={draws[0]} bets={memberBets} />}
      <BetFeed key={`${game.id}:${game.period}`} game={game} refreshKey={submittedBets.length} onInitialLoad={markFeedReady} />
      {submittedBets.filter((bet) => bet.gameId === game.id).map((bet, index) => <div className="submitted-ticket" key={`${bet.issue}-${bet.acceptedAt}-${index}`}>
        <div className="player-bet"><div><small>{nickname}</small><article><span>{bet.content}</span><time className="game-message-time mine">{formatFeedTime(bet.acceptedAt)}</time></article></div><Avatar className="player-avatar" index={-1} label="我的头像" /></div>
        <div className="admin-message parsed-ticket"><span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span><div><small>开奖助手 · 24小时在线</small><article><span className="assistant-mention">@{nickname}</span><strong>【{game.title} - {shortIssue(bet.issue)}】下单成功</strong><br />{bet.lines.map((line) => <span className="parsed-line" key={line}>{line}</span>)}<footer>使用：{bet.total.toLocaleString('zh-CN')}</footer><time className="game-message-time">{formatFeedTime(bet.acceptedAt)}</time></article></div></div>
      </div>)}
      {submittedBets.length === 0 && memberBets.length > 0 && <PersistedBetSummary game={game} bets={memberBets} nickname={nickname} onOpenOrders={() => setDialog('orders')} />}
    </section>
    {showKeyboard ? <BetKeyboard mode={betMode} selectedCount={betInput.length} submitting={submitting} onBackspace={removeNumber} onClear={clearSelection} onConfirm={() => void submitBet()} onModeChange={setBetMode} onSelectNumber={appendNumber} onSelectOption={appendOption} showModes={false} /> : <QuickActions onCheckIn={() => setShowCheckIn(true)} onCustomerService={onOpenService} onQuickBet={() => { setShowKeyboard(false); setShowQuickBet(true) }} onSwitchGame={() => setShowGameSwitcher(true)} />}
    <section className="ticket-strip" onClick={(event) => event.stopPropagation()}><button aria-label={showKeyboard ? '收起快捷键盘' : '打开快捷键盘'} className="ticket-ime" onClick={() => { setShowAddMenu(false); setShowKeyboard((visible) => !visible) }}><img alt="" src="/icons/lucide/keyboard.svg" /></button><button aria-label="打开投注键盘" className="ticket-selection" onClick={() => { setShowAddMenu(false); setShowKeyboard((visible) => !visible) }}>{betInput || '输入玩法/金额'}</button>{betInput ? <button aria-label="发送投注" className="ticket-add ticket-send" disabled={submitting} onClick={() => void submitBet()}>{submitting ? '…' : '发送'}</button> : <button aria-expanded={showAddMenu} aria-label="打开更多功能" className="ticket-add" onClick={() => { setShowKeyboard(false); setShowAddMenu((visible) => !visible) }}><Icon name="plus" /></button>}</section>
    {showAddMenu && <AddMenu onSelect={(action) => onOpenWallet(action)} />}
    {showQuickBet && <FullBetBoard game={game} mode={betMode} draft={betInput} submitting={submitting} defaultOdds={defaultOdds} onClear={clearSelection} onClose={() => setShowQuickBet(false)} onConfirm={(content) => void submitBet(content)} onModeChange={setBetMode} onSetDraft={setBetInput} />}
    {showGameSwitcher && <GameSwitcher currentGame={game.id} games={games} onClose={() => setShowGameSwitcher(false)} onSelect={onOpenGame} />}
    {dialog === 'orders' && <OrdersDialog bets={memberBets} onCancel={(id) => void cancelBet(id)} onClose={() => setDialog(null)} />}
    {dialog === 'assist' && <ActionDialog title="投注助手" description="选择快捷、两面盘或号码面板后可自由组合；确认格式为 玩法/金额，多条用 # 分隔。" onClose={() => setDialog(null)} />}
    {dialog === 'required' && <ActionDialog title="请先选择投注内容" description="点击输入框或左侧输入法按钮打开投注面板，再选择号码或玩法并加上金额。" onClose={() => setDialog(null)} />}
    {dialog === 'bet-error' && <ActionDialog title="投注未成功" description={betError || '请检查余额、格式或封盘状态后重试。'} onClose={() => setDialog(null)} />}
  </main>
}

function QuickActions({ onSwitchGame, onCustomerService, onQuickBet, onCheckIn }: { onSwitchGame: () => void; onCustomerService: () => void; onQuickBet: () => void; onCheckIn: () => void }) {
  return <div className="quick-actions"><button aria-label="切换游戏" onClick={onSwitchGame}>⇄</button><button aria-label="联系客服" onClick={onCustomerService}>🎧</button><button aria-label="快捷投注" onClick={onQuickBet}>☷</button><button aria-label="每日签到" className="quick-check-in" onClick={onCheckIn}>签</button></div>
}

function BetFeed({ game, refreshKey, onInitialLoad }: { game: Game; refreshKey: number; onInitialLoad: () => void }) {
  const [items, setItems] = useState<GameFeedItem[]>([])
  const [wsTick, setWsTick] = useState(0)
  const loadedKeyRef = useRef('')

  useEffect(() => {
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail?.type === 'bet_feed' && detail.data.game_id === game.id) {
        setWsTick((value) => value + 1)
      }
    }
    window.addEventListener(WS_EVENT, onWs)
    return () => window.removeEventListener(WS_EVENT, onWs)
  }, [game.id])

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const feed = await portalApi.gameFeed(game.id, game.period)
        if (!cancelled) {
          setItems(feed)
          const key = `${game.id}:${game.period}`
          if (loadedKeyRef.current !== key) {
            loadedKeyRef.current = key
            onInitialLoad()
          }
        }
      } catch {
        if (!cancelled) {
          setItems([])
          const key = `${game.id}:${game.period}`
          if (loadedKeyRef.current !== key) {
            loadedKeyRef.current = key
            onInitialLoad()
          }
        }
      }
    }
    // 先清空旧彩种动态，再读取新彩种；不能让网络请求完成前的旧内容
    // 短暂显示在新的游戏会话中。
    setItems([])
    void load()
    const timer = window.setInterval(() => void load(), 8000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [game.id, game.period, onInitialLoad, refreshKey, wsTick])

  if (!items.length) return null
  return (
    <div className="bet-feed">
      {items.map((item, index) => (
        <article className="market-bet" key={`${item.nickname}-${item.created_at}-${index}`}>
          <Avatar index={index} label={`${item.nickname}的头像`} />
          <div>
            <small>{item.nickname}</small>
            <p><b>【{game.title} · 第 {shortIssue(game.period)} 期】</b><br />{item.detail} · {item.amount} 元<em>已受理</em><time className="game-message-time">{formatFeedTime(item.created_at)}</time></p>
          </div>
        </article>
      ))}
    </div>
  )
}

function PersistedBetSummary({ game, bets, nickname, onOpenOrders }: { game: Game; bets: MemberBet[]; nickname: string; onOpenOrders: () => void }) {
  const latestIssue = bets[0]?.issue ?? game.period
  const issueBets = bets.filter((bet) => bet.issue === latestIssue)
  const visible = issueBets.slice(0, 8)
  const total = issueBets.filter((bet) => bet.status !== 'cancelled').reduce((sum, bet) => sum + bet.amount, 0)
  const isCurrent = latestIssue === game.period
  return <div className="admin-message parsed-ticket persisted-ticket"><span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span><div><small>开奖助手 · 24小时在线</small><article><span className="assistant-mention">@{nickname}</span><strong>【{game.title} - {shortIssue(latestIssue)}】{isCurrent ? '我的本期注单' : '我的最近注单'}</strong><i className="persisted-badge">已从服务器恢复</i>{visible.map((bet) => <span className="parsed-line persisted-line" key={bet.id}><span>{bet.play_name || `第${bet.position}球`} [{bet.selection}/{bet.amount.toFixed(2)}]</span><em className={bet.status}>{betStatusText(bet.status)}</em></span>)}{(issueBets.length > visible.length || bets.length > issueBets.length) && <button className="persisted-more" onClick={onOpenOrders}>查看该彩种全部注单</button>}<footer>共 {issueBets.length} 注 · 使用：{total.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</footer><time className="game-message-time">{formatFeedTime(issueBets[0]?.created_at ?? '')}</time></article></div></div>
}

function DrawAssistantMessage({ game, draw, bets }: { game: Game; draw: DrawResult; bets: MemberBet[] }) {
  const issueBets = bets.filter((bet) => bet.issue === draw.issue && bet.status !== 'cancelled')
  const pending = issueBets.filter((bet) => bet.status === 'pending').length
  const won = issueBets.filter((bet) => bet.status === 'won')
  const lost = issueBets.filter((bet) => bet.status === 'lost').length
  const stake = issueBets.reduce((sum, bet) => sum + bet.amount, 0)
  const payout = won.reduce((sum, bet) => sum + bet.payout, 0)
  const meta = crownMeta(draw.numbers)
  const settlement = issueBets.length === 0
    ? '本期开奖号码已同步，下一期已经开始受理。'
    : pending > 0
      ? `您的 ${issueBets.length} 注正在结算，结果将自动更新。`
      : won.length > 0
        ? `结算完成：${won.length} 注中奖，派彩 ${payout.toFixed(2)} 元。`
        : `结算完成：${lost} 注未中奖，本期使用 ${stake.toFixed(2)} 元。`
  return <div className="admin-message draw-announcement">
    <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
    <div><small>开奖助手 · 24小时在线</small><article>
      <strong>【{game.title} - {shortIssue(draw.issue)}】已开奖</strong>
      <div className="draw-announcement-balls">{draw.numbers.map((number, index) => <b className={ballTone(number)} key={`${draw.id}-${index}`}>{number}</b>)}</div>
      <span className="draw-announcement-meta">冠亚和：{meta.crownResult}{meta.dragonTiger ? ` · 龙虎：${meta.dragonTiger}` : ''}</span>
      <p>{settlement}</p>
      <time className="game-message-time">{formatFeedTime(draw.draw_at)}</time>
    </article></div>
  </div>
}

function formatFeedTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '刚刚'
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

function BetKeyboard({ mode, selectedCount, submitting, onModeChange, onBackspace, onClear, onSelectNumber, onSelectOption, onConfirm, showModes }: { mode: BetMode; selectedCount: number; submitting?: boolean; onModeChange: (mode: BetMode) => void; onBackspace: () => void; onClear: () => void; onSelectNumber: (number: number) => void; onSelectOption: (option: string) => void; onConfirm: () => void; showModes: boolean }) {
  const deleteTimerRef = useRef<number | null>(null)
  const didClearRef = useRef(false)
  const selectQuick = (key: string) => {
    if (key === '确认') return onConfirm()
    if (/^\d+$/.test(key)) return onSelectNumber(Number(key))
    if (quickOptions.has(key) || key === '/' || key === '#') onSelectOption(key)
  }
  const startDelete = () => {
    didClearRef.current = false
    deleteTimerRef.current = window.setTimeout(() => { onClear(); didClearRef.current = true }, 480)
  }
  const endDelete = () => {
    if (deleteTimerRef.current !== null) window.clearTimeout(deleteTimerRef.current)
    deleteTimerRef.current = null
  }
  const deleteOne = () => {
    if (didClearRef.current) { didClearRef.current = false; return }
    onBackspace()
  }
  const activeMode = showModes ? mode : 'quick'
  const keyClass = (key: string) => `bet-key ${key === '确认' ? 'confirm' : key === '←' ? 'command' : quickOptions.has(key) ? 'option' : 'number'}`
  return <section className={`bet-keyboard ${showModes ? 'complex-bet-keyboard' : 'input-bet-keyboard'}`} onClick={(event) => event.stopPropagation()}>{showModes && <header><div className="bet-mode-tabs">{modes.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}</button>)}</div><span>已选 <b>{selectedCount}</b> 项</span>{selectedCount > 0 && <button className="clear-selection" onClick={onClear}>清空</button>}</header>}{activeMode === 'quick' ? <div>{quickKeys.map((key) => <button className={keyClass(key)} disabled={submitting && key === '确认'} key={key} onClick={() => key === '←' ? deleteOne() : selectQuick(key)} onPointerDown={key === '←' ? startDelete : undefined} onPointerLeave={key === '←' ? endDelete : undefined} onPointerUp={key === '←' ? endDelete : undefined}>{key === '确认' ? (submitting ? '提交中' : '确认投注') : key}</button>)}</div> : activeMode === 'dual' ? <div className="dual-board">{dualOptions.map((option) => <button key={option} onClick={() => onSelectOption(option)}><b>{option}</b><small>1.92</small></button>)}</div> : <div className="number-board">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button key={number} onClick={() => onSelectNumber(number)}>{number}</button>)}</div>}</section>
}

type FullBetSelection = { label: string; play: string }

function previewFullBetSelections(draft: string): FullBetSelection[] {
  const segments = draft.replace(/^买/, '').split('#').map((part) => part.trim()).filter(Boolean)
  return segments.flatMap((segment) => {
    const play = segment.split('/')[0].trim()
    if (!play) return []
    if (/^\d+$/.test(play)) return [{ label: `号码组合 · ${play.split('').join(' ')}`, play }]
    const matched = play.match(/冠亚和[大小单双]|冠军[大小单双龙虎]|亚军[大小单双龙虎]|第[三四五六七八九十]名[大小单双龙虎]/g)
    if (matched?.length) return matched.map((item) => ({ label: item, play: item }))
    return [{ label: play, play }]
  })
}

function FullBetBoard({ game, mode, draft, submitting, defaultOdds, onModeChange, onClear, onSetDraft, onConfirm, onClose }: { game: Game; mode: BetMode; draft: string; submitting?: boolean; defaultOdds: number; onModeChange: (mode: BetMode) => void; onClear: () => void; onSetDraft: (value: string) => void; onConfirm: (content: string) => void; onClose: () => void }) {
  const [rank, setRank] = useState('冠军')
  const [amount, setAmount] = useState(20)
  const [selectionOpen, setSelectionOpen] = useState(false)
  const ranks = ['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名']
  const modeItems: Array<{ id: BetMode; label: string; helper: string }> = [{ id: 'quick', label: '快捷', helper: '常用玩法' }, { id: 'dual', label: '两面盘', helper: '大小单双' }, { id: 'numbers', label: '号码', helper: '1 ~ 10' }]
  const quickOptions = ['大', '小', '单', '双', '龙', '虎']
  const selections = previewFullBetSelections(draft)
  const preparedContent = selections.map((selection) => `${selection.play}/${amount}`).join('#')
  const preparedBet = parseBetInput(preparedContent)
  const acceptance = dueState(game.due)
  const isPlaySelected = (play: string) => selections.some((selection) => selection.play === play)
  const numericPlay = selections.find((selection) => /^\d+$/.test(selection.play))?.play ?? ''
  const isNumberSelected = (number: number) => numericPlay.includes(String(number))
  const togglePlay = (play: string) => {
    const next = isPlaySelected(play)
      ? selections.filter((selection) => selection.play !== play)
      : [...selections, { label: play, play }]
    onSetDraft(next.map((selection) => selection.play).join('#'))
  }
  const toggleNumber = (number: number) => {
    const token = String(number)
    const otherPlays = selections.filter((selection) => !/^\d+$/.test(selection.play)).map((selection) => selection.play)
    const nextNumbers = isNumberSelected(number) ? numericPlay.replace(token, '') : `${numericPlay}${token}`
    onSetDraft([...otherPlays, nextNumbers].filter(Boolean).join('#'))
  }
  const removeSelection = (play: string) => onSetDraft(selections.filter((selection) => selection.play !== play).map((selection) => selection.play).join('#'))
  return <div className="full-bet-layer" onClick={onClose}><section className="full-bet-board" onClick={(event) => event.stopPropagation()}><header className="full-bet-header"><button aria-label="返回游戏聊天室" onClick={onClose}><Icon name="back" /></button><div><b>{game.title}</b><small>第 {shortIssue(game.period)} 期 · {acceptance.label}</small></div><button className="full-bet-close" aria-label="关闭投注面板" onClick={onClose}>×</button></header><div className="full-bet-current"><span>距离截止 {game.due}</span><i className={`full-bet-acceptance ${acceptance.tone}`}>{acceptance.label}</i><div>{game.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div></div><div className="full-bet-workspace"><aside>{modeItems.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}<small>{item.helper}</small></button>)}</aside><section className="full-bet-content"><header><div><b>{mode === 'quick' ? '快捷投注' : mode === 'dual' ? '两面盘' : '号码投注'}</b><small>选择后高亮；再次点击可取消。</small></div><span>赔率 <b>{defaultOdds.toFixed(3)}</b></span></header>{mode === 'quick' && <><div className="rank-selector">{ranks.map((item) => <button className={rank === item ? 'active' : ''} key={item} onClick={() => setRank(item)}>{item}</button>)}</div><p className="board-section-title">{rank} · 选择玩法</p><div className="full-bet-options">{quickOptions.map((item) => { const play = `${rank}${item}`; return <button className={isPlaySelected(play) ? 'selected' : ''} key={item} onClick={() => togglePlay(play)}><b>{item}</b><small>{defaultOdds.toFixed(2)}</small></button> })}</div></>}{mode === 'dual' && <div className="full-bet-options">{dualOptions.map((item) => <button className={isPlaySelected(item) ? 'selected' : ''} key={item} onClick={() => togglePlay(item)}><b>{item}</b><small>{defaultOdds.toFixed(2)}</small></button>)}</div>}{mode === 'numbers' && <><p className="board-section-title">选择号码 · 已选会同步显示在投注清单，可再次点击取消</p><div className="full-bet-numbers">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button className={isNumberSelected(number) ? 'selected' : ''} key={number} onClick={() => toggleNumber(number)}><b>{number}</b><small>{(defaultOdds * 5).toFixed(2)}</small></button>)}</div></>}</section></div><footer className="full-bet-footer"><div className="full-bet-summary"><button onClick={onClear}>清空选择</button><button className="full-bet-selection-toggle" onClick={() => setSelectionOpen((open) => !open)}><span>已选 <b>{selections.length}</b> 组 · {preparedBet.payloads.length} 注</span><i>{selectionOpen ? '收起' : '查看清单'}</i></button>{selections.length > 0 && <button aria-label="删除最后一组选择" onClick={() => removeSelection(selections.at(-1)?.play ?? '')}>⌫</button>}</div>{selectionOpen && <div className="full-bet-selection-list"><header><b>本次投注清单</b><span>合计 ¥ {preparedBet.total.toFixed(2)}</span></header>{selections.length ? <div>{selections.map((selection, index) => { const selectionBet = parseBetInput(`${selection.play}/${amount}`); return <article key={`${selection.play}-${index}`}><div><b>{selection.label}</b><small>{selectionBet.payloads.map(payloadLabel).join('、')}</small></div><strong>¥ {selectionBet.total.toFixed(2)}</strong><button aria-label={`删除${selection.label}`} onClick={() => removeSelection(selection.play)}>×</button></article> })}</div> : <p>暂未选择玩法或号码</p>}</div>}<div className="amount-pills">{[20, 50, 100, 200].map((value) => <button className={amount === value ? 'active' : ''} key={value} onClick={() => setAmount(value)}>{value}</button>)}</div><button className="full-bet-confirm" disabled={submitting || !selections.length || acceptance.tone === 'closed'} onClick={() => onConfirm(preparedContent)}>{submitting ? '提交中…' : acceptance.tone === 'closed' ? '本期封盘' : '立即投注'} <small>¥ {preparedBet.total.toFixed(2)}</small></button></footer></section></div>
}

function OrdersDialog({ bets, onCancel, onClose }: { bets: MemberBet[]; onCancel: (id: number) => void; onClose: () => void }) {
  return <ActionDialog title="我的注单" description={bets.length ? `当前彩种最近 ${bets.length} 条个人注单` : '当前彩种暂无我的注单。'} onClose={onClose}>
    {bets.length > 0 && <div className="my-orders-list">{bets.map((bet) => <article key={bet.id}><header><b>{bet.play_name || bet.selection}</b><span className={`my-order-status ${bet.status}`}>{betStatusText(bet.status)}</span></header><p>第 {shortIssue(bet.issue)} 期 · 赔率 {bet.odds.toFixed(2)}</p><footer><strong>¥ {bet.amount.toFixed(2)}</strong>{bet.status === 'pending' && <button onClick={() => onCancel(bet.id)}>撤单</button>}</footer></article>)}</div>}
  </ActionDialog>
}

function betStatusText(status: string) {
  return ({ pending: '待开奖', won: '已中奖', lost: '未中奖', cancelled: '已撤销' } as Record<string, string>)[status] ?? status
}

function AddMenu({ onSelect }: { onSelect: (action?: WalletActionSlug) => void }) {
  const items: Array<{ icon: string; label: string; color: string; action?: WalletActionSlug }> = [
    { icon: '/icons/duo/coin-stack.svg', label: '上下分', color: '#4c8bf5', action: undefined }, { icon: '/icons/duo/clipboard.svg', label: '申请记录', color: '#f39a4b', action: 'applications' },
    { icon: '/icons/duo/clapperboard.svg', label: '游戏记录', color: '#42b99a', action: 'bets' }, { icon: '/icons/duo/chart-pie.svg', label: '竞猜报表', color: '#7b83ef', action: 'pending-bets' },
    { icon: '/icons/duo/credit-card.svg', label: '积分账变', color: '#e79b4b', action: 'ledger' }, { icon: '/icons/duo/clock.svg', label: '自助回水', color: '#42a8c2', action: 'rebate' },
    { icon: '/icons/duo/confetti.svg', label: '福利报表', color: '#e8799a', action: 'welfare' }, { icon: '/icons/duo/discount.svg', label: '红包报表', color: '#ef6b62', action: 'redpacket' },
  ]
  return <section className="add-menu add-menu-inline" onClick={(event) => event.stopPropagation()}><i className="add-menu-handle" /><div>{items.map((item) => <button key={item.label} onClick={() => onSelect(item.action)}><span className="duo-menu-icon" style={{ backgroundColor: item.color, maskImage: `url(${item.icon})`, WebkitMaskImage: `url(${item.icon})` }} /><b>{item.label}</b></button>)}</div></section>
}

function GameSwitcher({ currentGame, games, onClose, onSelect }: { currentGame: string; games: Game[]; onClose: () => void; onSelect: (id: string) => void }) {
  return <div className="game-menu-layer game-switch-layer" onClick={onClose}><aside className="game-switch-sheet" onClick={(event) => event.stopPropagation()}><header><b>⇄ 切换游戏</b><button onClick={onClose}>×</button></header>{games.map((item) => <button className={item.id === currentGame ? 'current' : ''} key={item.id} onClick={() => { onClose(); if (item.id !== currentGame) onSelect(item.id) }}><span style={{ background: item.color }}>{item.tag.slice(0, 2)}</span><div><b>{item.title}</b><small>第 {item.period} 期</small></div><em>{item.id === currentGame ? '当前游戏' : `剩余 ${item.due}`}</em></button>)}</aside></div>
}
