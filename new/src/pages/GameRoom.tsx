import { useCallback, useEffect, useRef, useState } from 'react'
import type { Game, Theme } from '../types'
import { ballTone } from '../data/games'
import { Avatar } from '../components/Avatar'
import { Icon } from '../components/Icon'
import { ActionDialog } from '../components/Dialogs'
import { playNotificationSound } from '../utils/notificationAudio'
import { CheckIn } from './CheckIn'
import { parseBetInput, type ParsedBet } from '../utils/betParser'
import { betsApi, type MemberBet } from '../api/bets'
import { useGameDraws } from '../hooks/useGameDraws'
import { useMemberPreferences } from '../hooks/useMemberPreferences'
import { WS_EVENT, type WsEvent } from '../hooks/useWebSocket'
import { portalApi, type GameFeedItem, type GameOdds } from '../api/portal'

type Props = {
  game: Game
  games: Game[]
  theme: Theme
  nickname: string
  balance: number
  onBack: () => void
  onOpenGame: (gameId: string) => void
  onOpenService: () => void
  onRefreshBalance: () => Promise<void>
}
type Dialog = 'history' | 'orders' | 'assist' | 'required' | 'bet-error' | null
type BetMode = 'quick' | 'dual' | 'numbers'

const quickKeys = ['大', '1', '2', '3', '←', '小', '4', '5', '6', '龙', '单', '7', '8', '9', '冠亚', '双', '0', '#', '/', '虎']
const quickOptions = new Set(['大', '小', '单', '双', '龙', '虎', '冠亚'])
const dualOptions = ['冠军大', '冠军小', '冠军单', '冠军双', '冠军龙', '冠军虎', '亚军大', '亚军小', '亚军单', '亚军双', '冠亚和大', '冠亚和小']
const modes: Array<{ id: BetMode; label: string }> = [{ id: 'quick', label: '快捷' }, { id: 'dual', label: '两面盘' }, { id: 'numbers', label: '号码 1~10' }]
const drawPositionNames = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十']

function crownMeta(balls: number[]) {
  if (balls.length < 2) return { crownResult: '—', dragonTiger: '—' }
  const crownSum = balls[0] + balls[1]
  const crownResult = `${crownSum}${crownSum >= 12 ? '大' : '小'}${crownSum % 2 ? '单' : '双'}`
  const dragonTiger = balls.slice(0, 5).map((ball, index) => (balls[9 - index] !== undefined && ball > balls[9 - index] ? '龙' : '虎')).join('')
  return { crownResult, dragonTiger }
}

/** 彩种会话：快捷输入、两面盘和注单提交接后端 API。 */
export function GameRoom({ game, games, theme, nickname, balance, onBack, onOpenGame, onOpenService, onRefreshBalance }: Props) {
  const [betInput, setBetInput] = useState('')
  const [showKeyboard, setShowKeyboard] = useState(false)
  const [showQuickBet, setShowQuickBet] = useState(false)
  const [betMode, setBetMode] = useState<BetMode>('quick')
  const { drawHistoryLimit, defaultBetMode } = useMemberPreferences()
  const [dialog, setDialog] = useState<Dialog>(null)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [submittedBets, setSubmittedBets] = useState<ParsedBet[]>([])
  const [memberBets, setMemberBets] = useState<MemberBet[]>([])
  const [betError, setBetError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [showAddMenu, setShowAddMenu] = useState(false)
  const [showGameSwitcher, setShowGameSwitcher] = useState(false)
  const [showCheckIn, setShowCheckIn] = useState(false)
  const chatRef = useRef<HTMLElement>(null)
  const { draws, loading: drawsLoading } = useGameDraws(game.id, drawHistoryLimit)
  const [oddsInfo, setOddsInfo] = useState<GameOdds | null>(null)

  useEffect(() => {
    void portalApi.gameOdds(game.id).then(setOddsInfo).catch(() => setOddsInfo(null))
  }, [game.id])

  const defaultOdds = oddsInfo?.items.find((item) => item.play_code === 'ball_1_5')?.odds
    ?? oddsInfo?.items[0]?.odds
    ?? 1.92

  const resolveOdds = (payload: ParsedBet['payloads'][number]) => {
    const code = payload.play_code ?? 'ball_1_5'
    return payload.odds ?? oddsInfo?.items.find((item) => item.play_code === code)?.odds ?? defaultOdds
  }

  const loadBets = useCallback(async () => {
    try {
      const result = await betsApi.list({ game_id: game.id, issue: game.period, page_size: 50 })
      setMemberBets(result.items)
    } catch {
      setMemberBets([])
    }
  }, [game.id, game.period])

  useEffect(() => {
    setBetMode(defaultBetMode)
  }, [game.id, defaultBetMode])

  useEffect(() => { void loadBets() }, [loadBets])

  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadBets()
      void onRefreshBalance()
    }, 10_000)
    return () => window.clearInterval(timer)
  }, [loadBets, onRefreshBalance])

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => chatRef.current?.scrollTo({ top: chatRef.current.scrollHeight, behavior: 'auto' }))
    return () => window.cancelAnimationFrame(frame)
  }, [submittedBets])

  const appendNumber = (number: number) => setBetInput((current) => `${current}${number}`)
  const appendOption = (option: string) => setBetInput((current) => `${current}${option}`)
  const clearSelection = () => setBetInput('')
  const removeNumber = () => setBetInput((current) => current.slice(0, -1))
  const submitBet = async (rawInput?: string, fallbackAmount?: number) => {
    let content = (rawInput ?? betInput).trim()
    if (!content) return setDialog('required')
    if (fallbackAmount && !content.includes('/')) content = `${content}/${fallbackAmount}`
    const parsed = parseBetInput(content)
    if (!parsed.payloads.length) {
      setBetError('请在玩法后输入金额，例如 3大/200 或 12345/1000')
      return setDialog('bet-error')
    }
    setSubmitting(true)
    setBetError('')
    try {
      for (const payload of parsed.payloads) {
        await betsApi.place({
          game_id: game.id,
          issue: game.period,
          position: payload.position,
          selection: payload.selection,
          amount: payload.amount,
          play_code: payload.play_code ?? 'ball_1_5',
          play_name: payload.play_name,
          odds: resolveOdds(payload),
        })
      }
      setSubmittedBets((bets) => [...bets, parsed])
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

  const drawPositionLabel = drawPositionNames.slice(0, Math.max(game.balls.length, 5)).join('　')
  const recentDraws = draws.slice(0, 8).map((draw) => {
    const balls = draw.numbers.length ? draw.numbers : game.balls
    const meta = crownMeta(balls)
    return { period: draw.issue, balls, ...meta }
  })
  const latestMeta = crownMeta(game.balls)
  if (showCheckIn) {
    return (
      <div className={`check-in-shell theme-${theme}`}>
        <CheckIn onBack={() => setShowCheckIn(false)} onComplete={() => void onRefreshBalance()} />
      </div>
    )
  }
  return <main className={`game-room theme-${theme}`} onClick={() => { setShowKeyboard(false); setShowAddMenu(false) }}>
    <header className="game-header"><button aria-label="返回大厅" onClick={onBack}><Icon name="back" /></button><b>{game.title}</b><div><small>余额: {balance.toFixed(2)}</small><small>第 {game.period} 期</small></div></header>
    <section className="game-info"><div><span>截止倒计时</span><b>{game.due.split('').map((number, index) => <i key={index}>{number}</i>)}</b></div><button onClick={() => setDialog('history')}>开奖记录</button><button onClick={() => setDialog('orders')}>注单</button></section>
    <section className={`draw-history ${historyOpen ? 'open' : ''}`}><button className="last-draw" aria-expanded={historyOpen} onClick={() => setHistoryOpen((open) => !open)}><span>上期 {recentDraws[0]?.period ?? game.period}</span><div>{game.balls.map((number, index) => <b className={ballTone(number)} key={index}>{number}</b>)}</div><small>冠亚 <b>{latestMeta.crownResult}</b></small></button><div className="recent-draws" aria-hidden={!historyOpen}><header><span>期数</span><b>{drawPositionLabel}</b><small>冠亚和 · 龙虎</small></header>{drawsLoading && <p style={{ padding: 8 }}>加载开奖…</p>}{recentDraws.map((draw) => <article key={draw.period}><span>{draw.period}</span><div>{draw.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div><small><b>{draw.crownResult}</b><em>{draw.dragonTiger}</em></small></article>)}</div></section>
    <section className="bet-chat" ref={chatRef}><p>以上全接，以下无效。</p><div className="admin-message"><span className="service-logo">七</span><div><small>开奖助手 · 系统通知</small><article><b>【{game.title} - {game.period}】</b><hr />当前期号正在受理中<br /><br />按“玩法/金额#玩法/金额”格式输入，例如 3大/200#12345/1000</article></div></div><BetFeed game={game} refreshKey={submittedBets.length} />{submittedBets.map((bet, index) => <div className="submitted-ticket" key={`${bet.content}-${index}`}><div className="player-bet"><div><small>{nickname} · 刚刚</small><article>{bet.content}</article></div><Avatar className="player-avatar" index={-1} label="我的头像" /></div><div className="admin-message parsed-ticket"><span className="service-logo">七</span><div><small>开奖助手 · 刚刚</small><article><b>@{nickname}</b><br /><strong>【{game.title} - {game.period}】下单成功</strong><br />{bet.lines.map((line) => <span className="parsed-line" key={line}>{line}</span>)}<footer>使用：{bet.total.toLocaleString('zh-CN')}</footer></article></div></div></div>)}</section>
    {showKeyboard ? <BetKeyboard mode={betMode} selectedCount={betInput.length} submitting={submitting} onBackspace={removeNumber} onClear={clearSelection} onConfirm={() => void submitBet()} onModeChange={setBetMode} onSelectNumber={appendNumber} onSelectOption={appendOption} showModes={false} /> : <QuickActions onCheckIn={() => setShowCheckIn(true)} onCustomerService={onOpenService} onQuickBet={() => { setShowKeyboard(false); setShowQuickBet(true) }} onSwitchGame={() => setShowGameSwitcher(true)} />}
    <section className="ticket-strip" onClick={(event) => event.stopPropagation()}><button aria-label={showKeyboard ? '收起快捷键盘' : '打开快捷键盘'} className="ticket-ime" onClick={() => { setShowAddMenu(false); setShowKeyboard((visible) => !visible) }}><img alt="" src="/icons/lucide/keyboard.svg" /></button><button aria-label="打开投注键盘" className="ticket-selection" onClick={() => { setShowAddMenu(false); setShowKeyboard((visible) => !visible) }}>{betInput || '输入玩法/金额'}</button>{betInput ? <button aria-label="发送投注" className="ticket-add ticket-send" disabled={submitting} onClick={() => void submitBet()}>{submitting ? '…' : '发送'}</button> : <button aria-expanded={showAddMenu} aria-label="打开更多功能" className="ticket-add" onClick={() => { setShowKeyboard(false); setShowAddMenu((visible) => !visible) }}><Icon name="plus" /></button>}</section>
    {showAddMenu && <AddMenu onSelect={() => { setShowAddMenu(false); setDialog('orders') }} />}
    {showQuickBet && <FullBetBoard game={game} mode={betMode} selectedCount={betInput.length} submitting={submitting} defaultOdds={defaultOdds} onBackspace={removeNumber} onClear={clearSelection} onClose={() => setShowQuickBet(false)} onConfirm={(amount) => void submitBet(undefined, amount)} onModeChange={setBetMode} onSelectNumber={appendNumber} onSelectOption={appendOption} />}
    {showGameSwitcher && <GameSwitcher currentGame={game.id} games={games} onClose={() => setShowGameSwitcher(false)} onSelect={onOpenGame} />}
    {dialog === 'history' && <ActionDialog title="开奖记录" description={recentDraws.length ? `最近 ${recentDraws.length} 期数据来自后端。最新一期 ${recentDraws[0]?.period}：${recentDraws[0]?.balls.join('、')}` : '暂无开奖记录'} onClose={() => setDialog(null)} />}
    {dialog === 'orders' && <OrdersDialog bets={memberBets} onCancel={(id) => void cancelBet(id)} onClose={() => setDialog(null)} />}
    {dialog === 'assist' && <ActionDialog title="投注助手" description="选择快捷、两面盘或号码面板后可自由组合；确认格式为 玩法/金额，多条用 # 分隔。" onClose={() => setDialog(null)} />}
    {dialog === 'required' && <ActionDialog title="请先选择投注内容" description="点击输入框或左侧输入法按钮打开投注面板，再选择号码或玩法并加上金额。" onClose={() => setDialog(null)} />}
    {dialog === 'bet-error' && <ActionDialog title="投注未成功" description={betError || '请检查余额、格式或封盘状态后重试。'} onClose={() => setDialog(null)} />}
  </main>
}

function QuickActions({ onSwitchGame, onCustomerService, onQuickBet, onCheckIn }: { onSwitchGame: () => void; onCustomerService: () => void; onQuickBet: () => void; onCheckIn: () => void }) {
  return <div className="quick-actions"><button aria-label="切换游戏" onClick={onSwitchGame}>⇄</button><button aria-label="联系客服" onClick={onCustomerService}>🎧</button><button aria-label="快捷投注" onClick={onQuickBet}>☷</button><button aria-label="每日签到" className="quick-check-in" onClick={onCheckIn}>签</button></div>
}

function BetFeed({ game, refreshKey }: { game: Game; refreshKey: number }) {
  const [items, setItems] = useState<GameFeedItem[]>([])
  const [wsTick, setWsTick] = useState(0)

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
        if (!cancelled) setItems(feed)
      } catch {
        if (!cancelled) setItems([])
      }
    }
    void load()
    const timer = window.setInterval(() => void load(), 8000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [game.id, game.period, refreshKey, wsTick])

  if (!items.length) return null
  return (
    <div className="bet-feed">
      {items.map((item, index) => (
        <article className="market-bet" key={`${item.nickname}-${item.created_at}-${index}`}>
          <Avatar index={index} label={`${item.nickname}的头像`} />
          <div>
            <small>{item.nickname} · {formatFeedTime(item.created_at)}</small>
            <p><b>【{game.title} · 第 {game.period} 期】</b><br />{item.detail} · {item.amount} 元<em>已受理</em></p>
          </div>
        </article>
      ))}
    </div>
  )
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

function FullBetBoard({ game, mode, selectedCount, submitting, defaultOdds, onModeChange, onBackspace, onClear, onSelectNumber, onSelectOption, onConfirm, onClose }: { game: Game; mode: BetMode; selectedCount: number; submitting?: boolean; defaultOdds: number; onModeChange: (mode: BetMode) => void; onBackspace: () => void; onClear: () => void; onSelectNumber: (number: number) => void; onSelectOption: (option: string) => void; onConfirm: (amount: number) => void; onClose: () => void }) {
  const [rank, setRank] = useState('冠军')
  const [amount, setAmount] = useState(20)
  const ranks = ['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名']
  const modeItems: Array<{ id: BetMode; label: string; helper: string }> = [{ id: 'quick', label: '快捷', helper: '常用玩法' }, { id: 'dual', label: '两面盘', helper: '大小单双' }, { id: 'numbers', label: '号码', helper: '1 ~ 10' }]
  const quickOptions = ['大', '小', '单', '双', '龙', '虎']
  const closeAndClearOne = () => { onBackspace() }
  return <div className="full-bet-layer" onClick={onClose}><section className="full-bet-board" onClick={(event) => event.stopPropagation()}><header className="full-bet-header"><button aria-label="返回游戏聊天室" onClick={onClose}><Icon name="back" /></button><div><b>{game.title}</b><small>第 {game.period} 期 · 正在受理</small></div><button className="full-bet-close" aria-label="关闭投注面板" onClick={onClose}>×</button></header><div className="full-bet-current"><span>距离截止 {game.due}</span><div>{game.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div></div><div className="full-bet-tabs">{modeItems.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}><b>{item.label}</b><small>{item.helper}</small></button>)}</div><div className="full-bet-workspace"><aside>{modeItems.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}<small>{item.helper}</small></button>)}</aside><section className="full-bet-content"><header><div><b>{mode === 'quick' ? '快捷投注' : mode === 'dual' ? '两面盘' : '号码投注'}</b><small>可自由叠加选择，不设互斥</small></div><span>赔率 <b>{defaultOdds.toFixed(3)}</b></span></header>{mode === 'quick' && <><div className="rank-selector">{ranks.map((item) => <button className={rank === item ? 'active' : ''} key={item} onClick={() => setRank(item)}>{item}</button>)}</div><p className="board-section-title">{rank} · 选择玩法</p><div className="full-bet-options">{quickOptions.map((item) => <button key={item} onClick={() => onSelectOption(`${rank}${item}`)}><b>{item}</b><small>{defaultOdds.toFixed(2)}</small></button>)}</div></>}{mode === 'dual' && <div className="full-bet-options">{dualOptions.map((item) => <button key={item} onClick={() => onSelectOption(item)}><b>{item}</b><small>{defaultOdds.toFixed(2)}</small></button>)}</div>}{mode === 'numbers' && <><p className="board-section-title">选择号码 · 可重复选择</p><div className="full-bet-numbers">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button key={number} onClick={() => onSelectNumber(number)}>{number}<small>{(defaultOdds * 5).toFixed(2)}</small></button>)}</div></>}</section></div><footer className="full-bet-footer"><div className="full-bet-summary"><button onClick={onClear}>清空选择</button><span>已选 <b>{selectedCount}</b> 项</span>{selectedCount > 0 && <button aria-label="删除最后一项" onClick={closeAndClearOne}>⌫</button>}</div><div className="amount-pills">{[20, 50, 100, 200].map((value) => <button className={amount === value ? 'active' : ''} key={value} onClick={() => setAmount(value)}>{value}</button>)}</div><button className="full-bet-confirm" disabled={submitting} onClick={() => onConfirm(amount)}>{submitting ? '提交中…' : '确认投注'} <small>¥ {selectedCount > 0 ? amount : 0}</small></button></footer></section></div>
}

function OrdersDialog({ bets, onCancel, onClose }: { bets: MemberBet[]; onCancel: (id: number) => void; onClose: () => void }) {
  return <ActionDialog title="我的注单" description={bets.length ? `本期共 ${bets.length} 条注单（来自后端）。` : '当前期暂无注单。'} onClose={onClose}>
    {bets.length > 0 && <div style={{ marginTop: 12, maxHeight: 240, overflow: 'auto' }}>{bets.map((bet) => <div key={bet.id} style={{ padding: '8px 0', borderBottom: '1px solid rgba(255,255,255,0.08)' }}><b>{bet.play_name || bet.selection}</b> · {bet.amount} 元 · {bet.status}{bet.status === 'pending' && <button style={{ marginLeft: 8 }} onClick={() => onCancel(bet.id)}>撤单</button>}</div>)}</div>}
  </ActionDialog>
}

function AddMenu({ onSelect }: { onSelect: () => void }) {
  const items = [
    { icon: '/icons/duo/coin-stack.svg', label: '上下分', color: '#4c8bf5' }, { icon: '/icons/duo/clipboard.svg', label: '申请记录', color: '#f39a4b' },
    { icon: '/icons/duo/clapperboard.svg', label: '游戏记录', color: '#42b99a' }, { icon: '/icons/duo/chart-pie.svg', label: '竞猜报表', color: '#7b83ef' },
    { icon: '/icons/duo/credit-card.svg', label: '积分账变', color: '#e79b4b' }, { icon: '/icons/duo/clock.svg', label: '自助回水', color: '#42a8c2' },
    { icon: '/icons/duo/confetti.svg', label: '福利报表', color: '#e8799a' }, { icon: '/icons/duo/discount.svg', label: '红包报表', color: '#ef6b62' },
  ]
  return <section className="add-menu add-menu-inline" onClick={(event) => event.stopPropagation()}><i className="add-menu-handle" /><div>{items.map((item) => <button key={item.label} onClick={onSelect}><span className="duo-menu-icon" style={{ backgroundColor: item.color, maskImage: `url(${item.icon})`, WebkitMaskImage: `url(${item.icon})` }} /><b>{item.label}</b></button>)}</div></section>
}

function GameSwitcher({ currentGame, games, onClose, onSelect }: { currentGame: string; games: Game[]; onClose: () => void; onSelect: (id: string) => void }) {
  return <div className="game-menu-layer game-switch-layer" onClick={onClose}><aside className="game-switch-sheet" onClick={(event) => event.stopPropagation()}><header><b>⇄ 切换游戏</b><button onClick={onClose}>×</button></header>{games.map((item) => <button className={item.id === currentGame ? 'current' : ''} key={item.id} onClick={() => { onClose(); if (item.id !== currentGame) onSelect(item.id) }}><span style={{ background: item.color }}>{item.tag.slice(0, 2)}</span><div><b>{item.title}</b><small>第 {item.period} 期</small></div><em>{item.id === currentGame ? '当前游戏' : `剩余 ${item.due}`}</em></button>)}</aside></div>
}
