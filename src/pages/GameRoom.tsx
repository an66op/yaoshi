import { useEffect, useRef, useState } from 'react'
import type { Game, Theme } from '../types'
import { ballTone } from '../data/games'
import { Avatar } from '../components/Avatar'
import { Icon } from '../components/Icon'
import { ActionDialog } from '../components/Dialogs'
import { playNotificationSound } from '../utils/notificationAudio'

type Props = { game: Game; games: Game[]; theme: Theme; onBack: () => void; onOpenGame: (gameId: string) => void; onOpenService: () => void }
type Dialog = 'history' | 'orders' | 'assist' | 'required' | null
type BetMode = 'quick' | 'dual' | 'numbers'
type ParsedBet = { content: string; lines: string[]; total: number }

const quickKeys = ['大', '1', '2', '3', '←', '小', '4', '5', '6', '龙', '单', '7', '8', '9', '冠亚', '双', '0', '#', '/', '虎']
const quickOptions = new Set(['大', '小', '单', '双', '龙', '虎', '冠亚'])
const dualOptions = ['冠军大', '冠军小', '冠军单', '冠军双', '冠军龙', '冠军虎', '亚军大', '亚军小', '亚军单', '亚军双', '冠亚和大', '冠亚和小']
const modes: Array<{ id: BetMode; label: string }> = [{ id: 'quick', label: '快捷' }, { id: 'dual', label: '两面盘' }, { id: 'numbers', label: '号码 1~10' }]
const positionNames = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十']

function parseBetInput(content: string): ParsedBet {
  const lines: string[] = []
  let total = 0

  for (const segment of content.replace(/^买/, '').split('#').map((item) => item.trim()).filter(Boolean)) {
    const parts = segment.split('/').map((item) => item.trim()).filter(Boolean)
    const amountText = parts.at(-1) ?? ''
    const amount = /^\d+(?:\.\d+)?$/.test(amountText) ? Number(amountText) : 0
    const play = parts.slice(0, -1).join('/') || segment
    total += amount

    const rankedOption = play.match(/^(\d+)([大小单双龙虎])$/)
    const rankedNumbers = play.match(/^(\d+)\/(\d+)$/)
    if (rankedOption) {
      const position = positionNames[Number(rankedOption[1]) - 1] ?? rankedOption[1]
      lines.push(`第${position}名[${rankedOption[2]}/${amountText || '待定'}]`)
    } else if (rankedNumbers) {
      const position = positionNames[Number(rankedNumbers[1]) - 1] ?? rankedNumbers[1]
      lines.push(`第${position}名[${rankedNumbers[2]}/${amountText || '待定'}]`)
    } else {
      lines.push(`号码[${play}/${amountText || '待定'}]`)
    }
  }

  return { content, lines: lines.length ? lines : [`号码[${content}]`], total }
}

/** 独立彩种会话：快捷输入、两面盘和侧边操作共用同一份模拟投注数据。 */
export function GameRoom({ game, games, theme, onBack, onOpenGame, onOpenService }: Props) {
  const [betInput, setBetInput] = useState('')
  const [showKeyboard, setShowKeyboard] = useState(false)
  const [showQuickBet, setShowQuickBet] = useState(false)
  const [betMode, setBetMode] = useState<BetMode>('quick')
  const [dialog, setDialog] = useState<Dialog>(null)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [submittedBets, setSubmittedBets] = useState<ParsedBet[]>([])
  const [showAddMenu, setShowAddMenu] = useState(false)
  const [showGameSwitcher, setShowGameSwitcher] = useState(false)
  const chatRef = useRef<HTMLElement>(null)

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => chatRef.current?.scrollTo({ top: chatRef.current.scrollHeight, behavior: 'auto' }))
    return () => window.cancelAnimationFrame(frame)
  }, [submittedBets])

  const appendNumber = (number: number) => setBetInput((current) => `${current}${number}`)
  const appendOption = (option: string) => setBetInput((current) => `${current}${option}`)
  const clearSelection = () => setBetInput('')
  const removeNumber = () => setBetInput((current) => current.slice(0, -1))
  const submitBet = () => {
    const content = betInput.trim()
    if (!content) return setDialog('required')
    setSubmittedBets((bets) => [...bets, parseBetInput(content)])
    playNotificationSound('lottery')
    clearSelection()
    setShowKeyboard(false)
    setShowQuickBet(false)
  }

  const previousPeriod = Number(game.period) - 1
  const drawPositionNames = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十']
  const drawPositionLabel = drawPositionNames.slice(0, game.balls.length).join('　')
  const recentDraws = Array.from({ length: 8 }, (_, offset) => {
    const balls = game.balls.map((ball, index) => (ball + offset + index) % 10 || 10)
    const crownSum = balls[0] + balls[1]
    return {
      period: previousPeriod - offset,
      balls,
      crownResult: `${crownSum}${crownSum >= 12 ? '大' : '小'}${crownSum % 2 ? '单' : '双'}`,
      dragonTiger: balls.slice(0, 5).map((ball, index) => ball > balls[9 - index] ? '龙' : '虎').join(''),
    }
  })
  return <main className={`game-room theme-${theme}`} onClick={() => { setShowKeyboard(false); setShowAddMenu(false) }}>
    <header className="game-header"><button aria-label="返回大厅" onClick={onBack}><Icon name="back" /></button><b>{game.title}</b><div><small>积分: 2,468</small><small>回水: 0.00</small></div></header>
    <section className="game-info"><div><span>第 {game.period} 期</span><b>{game.due.split('').map((number, index) => <i key={index}>{number}</i>)}</b></div><button onClick={() => setDialog('history')}>开奖记录</button><button onClick={() => setDialog('orders')}>注单</button></section>
    <section className={`draw-history ${historyOpen ? 'open' : ''}`}><button className="last-draw" aria-expanded={historyOpen} onClick={() => setHistoryOpen((open) => !open)}><span>上期 {previousPeriod}</span><div>{game.balls.map((number, index) => <b className={ballTone(number)} key={index}>{number}</b>)}</div><small>冠军 <b>大 · 单</b></small></button><div className="recent-draws" aria-hidden={!historyOpen}><header><span>期数</span><b>{drawPositionLabel}</b><small>冠亚和 · 龙虎</small></header>{recentDraws.map((draw) => <article key={draw.period}><span>{draw.period}</span><div>{draw.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div><small><b>{draw.crownResult}</b><em>{draw.dragonTiger}</em></small></article>)}</div></section>
    <section className="bet-chat" ref={chatRef}><p>以上全接，以下无效。</p><div className="admin-message"><span className="service-logo">七</span><div><small>开奖助手 · 系统通知</small><article><b>【{game.title} - {previousPeriod}】</b><hr />当前期号正在受理中<br /><br />按“号码/金额#号码/金额”格式输入，例如 12345/1000#3大/2000</article></div></div><BetFeed game={game} />{submittedBets.map((bet, index) => <div className="submitted-ticket" key={`${bet.content}-${index}`}><div className="player-bet"><div><small>林先生 · 刚刚</small><article>{bet.content}</article></div><Avatar className="player-avatar" index={-1} label="我的头像" /></div><div className="admin-message parsed-ticket"><span className="service-logo">七</span><div><small>开奖助手 · 刚刚</small><article><b>@林先生</b><br /><strong>【{game.title} - {game.period}】下单成功</strong><br />{bet.lines.map((line) => <span className="parsed-line" key={line}>{line}</span>)}<footer>使用：{bet.total.toLocaleString('zh-CN')}</footer></article></div></div></div>)}</section>
    {showKeyboard ? <BetKeyboard mode={betMode} selectedCount={betInput.length} onBackspace={removeNumber} onClear={clearSelection} onConfirm={submitBet} onModeChange={setBetMode} onSelectNumber={appendNumber} onSelectOption={appendOption} showModes={false} /> : <QuickActions onCustomerService={onOpenService} onQuickBet={() => { setShowKeyboard(false); setShowQuickBet(true) }} onSwitchGame={() => setShowGameSwitcher(true)} />}
    <section className="ticket-strip" onClick={(event) => event.stopPropagation()}><button aria-label={showKeyboard ? '收起快捷键盘' : '打开快捷键盘'} className="ticket-ime" onClick={() => { setShowAddMenu(false); setShowKeyboard((visible) => !visible) }}><img alt="" src="/icons/lucide/keyboard.svg" /></button><button aria-label="打开投注键盘" className="ticket-selection" onClick={() => { setShowAddMenu(false); setShowKeyboard((visible) => !visible) }}>{betInput}</button>{betInput ? <button aria-label="发送投注" className="ticket-add ticket-send" onClick={submitBet}>发送</button> : <button aria-expanded={showAddMenu} aria-label="打开更多功能" className="ticket-add" onClick={() => { setShowKeyboard(false); setShowAddMenu((visible) => !visible) }}><Icon name="plus" /></button>}</section>
    {showAddMenu && <AddMenu onSelect={() => { setShowAddMenu(false); setDialog('assist') }} />}
    {showQuickBet && <FullBetBoard game={game} mode={betMode} selectedCount={betInput.length} onBackspace={removeNumber} onClear={clearSelection} onClose={() => setShowQuickBet(false)} onConfirm={submitBet} onModeChange={setBetMode} onSelectNumber={appendNumber} onSelectOption={appendOption} />}
    {showGameSwitcher && <GameSwitcher currentGame={game.id} games={games} onClose={() => setShowGameSwitcher(false)} onSelect={onOpenGame} />}
    {dialog === 'history' && <ActionDialog title="开奖记录" description={`第 ${previousPeriod} 期开奖号码：${game.balls.join('、')}。开奖记录为前端演示数据。`} onClose={() => setDialog(null)} />}
    {dialog === 'orders' && <ActionDialog title="我的注单" description={submittedBets.length ? `本期已发送 ${submittedBets.length} 条模拟注单。` : '当前期暂无已提交注单，可通过下方输入框开始投注。'} onClose={() => setDialog(null)} />}
    {dialog === 'assist' && <ActionDialog title="投注助手" description="选择快捷、两面盘或号码面板后可自由组合；数字可重复输入，确认后会生成一条模拟注单。" onClose={() => setDialog(null)} />}
    {dialog === 'required' && <ActionDialog title="请先选择投注内容" description="点击输入框或左侧输入法按钮打开投注面板，再选择号码或玩法。" onClose={() => setDialog(null)} />}
  </main>
}

function QuickActions({ onSwitchGame, onCustomerService, onQuickBet }: { onSwitchGame: () => void; onCustomerService: () => void; onQuickBet: () => void }) {
  return <div className="quick-actions"><button aria-label="切换游戏" onClick={onSwitchGame}>⇄</button><button aria-label="联系客服" onClick={onCustomerService}>🎧</button><button aria-label="快捷投注" onClick={onQuickBet}>☷</button></div>
}

function BetFeed({ game }: { game: Game }) {
  const robotMessages = [
    { name: '周先生', time: '11:27', avatar: 8, detail: '冠军：大 · 单 · 2 注' }, { name: '艾米', time: '11:28', avatar: 1, detail: '冠亚：龙 · 5 注' },
    { name: '陈先生', time: '11:29', avatar: 16, detail: '号码：3、7、9 · 1 注' }, { name: '小北', time: '11:30', avatar: 11, detail: '冠军：小 · 双 · 3 注' },
    { name: '程野', time: '11:31', avatar: 6, detail: '号码：5、8 · 2 注' }, { name: '安然', time: '11:32', avatar: 14, detail: '冠军：小 · 单 · 1 注' },
    { name: '阿南', time: '11:33', avatar: 2, detail: '冠亚：虎 · 3 注' }, { name: '乐乐', time: '11:34', avatar: 9, detail: '号码：1、4、6 · 2 注' },
    { name: '清欢', time: '11:35', avatar: 17, detail: '冠军：大 · 双 · 1 注' }, { name: '夏先生', time: '11:36', avatar: 19, detail: '号码：2、5、10 · 2 注' },
  ]
  const messages = game.id === 'lucky' ? robotMessages : robotMessages.slice(0, 4)
  return <div className="bet-feed">{messages.map((message) => <article className="market-bet" key={message.name}><Avatar index={message.avatar} label={`${message.name}的头像`} /><div><small>{message.name} · {message.time}</small><p><b>【{game.title} · 第 {game.period} 期】</b><br />{message.detail}<em>已受理</em></p></div></article>)}</div>
}

function BetKeyboard({ mode, selectedCount, onModeChange, onBackspace, onClear, onSelectNumber, onSelectOption, onConfirm, showModes }: { mode: BetMode; selectedCount: number; onModeChange: (mode: BetMode) => void; onBackspace: () => void; onClear: () => void; onSelectNumber: (number: number) => void; onSelectOption: (option: string) => void; onConfirm: () => void; showModes: boolean }) {
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
  return <section className={`bet-keyboard ${showModes ? 'complex-bet-keyboard' : 'input-bet-keyboard'}`} onClick={(event) => event.stopPropagation()}>{showModes && <header><div className="bet-mode-tabs">{modes.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}</button>)}</div><span>已选 <b>{selectedCount}</b> 项</span>{selectedCount > 0 && <button className="clear-selection" onClick={onClear}>清空</button>}</header>}{activeMode === 'quick' ? <div>{quickKeys.map((key) => <button className={keyClass(key)} key={key} onClick={() => key === '←' ? deleteOne() : selectQuick(key)} onPointerDown={key === '←' ? startDelete : undefined} onPointerLeave={key === '←' ? endDelete : undefined} onPointerUp={key === '←' ? endDelete : undefined}>{key === '确认' ? '确认投注' : key}</button>)}</div> : activeMode === 'dual' ? <div className="dual-board">{dualOptions.map((option) => <button key={option} onClick={() => onSelectOption(option)}><b>{option}</b><small>1.92</small></button>)}</div> : <div className="number-board">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button key={number} onClick={() => onSelectNumber(number)}>{number}</button>)}</div>}</section>
}

function FullBetBoard({ game, mode, selectedCount, onModeChange, onBackspace, onClear, onSelectNumber, onSelectOption, onConfirm, onClose }: { game: Game; mode: BetMode; selectedCount: number; onModeChange: (mode: BetMode) => void; onBackspace: () => void; onClear: () => void; onSelectNumber: (number: number) => void; onSelectOption: (option: string) => void; onConfirm: () => void; onClose: () => void }) {
  const [rank, setRank] = useState('冠军')
  const [amount, setAmount] = useState(20)
  const ranks = ['冠军', '亚军', '第三名', '第四名', '第五名', '第六名', '第七名', '第八名', '第九名', '第十名']
  const modeItems: Array<{ id: BetMode; label: string; helper: string }> = [{ id: 'quick', label: '快捷', helper: '常用玩法' }, { id: 'dual', label: '两面盘', helper: '大小单双' }, { id: 'numbers', label: '号码', helper: '1 ~ 10' }]
  const quickOptions = ['大', '小', '单', '双', '龙', '虎']
  const closeAndClearOne = () => { onBackspace() }
  return <div className="full-bet-layer" onClick={onClose}><section className="full-bet-board" onClick={(event) => event.stopPropagation()}><header className="full-bet-header"><button aria-label="返回游戏聊天室" onClick={onClose}><Icon name="back" /></button><div><b>{game.title}</b><small>第 {game.period} 期 · 正在受理</small></div><button className="full-bet-close" aria-label="关闭投注面板" onClick={onClose}>×</button></header><div className="full-bet-current"><span>距离截止 {game.due}</span><div>{game.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div></div><div className="full-bet-tabs">{modeItems.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}><b>{item.label}</b><small>{item.helper}</small></button>)}</div><div className="full-bet-workspace"><aside>{modeItems.map((item) => <button className={mode === item.id ? 'active' : ''} key={item.id} onClick={() => onModeChange(item.id)}>{item.label}<small>{item.helper}</small></button>)}</aside><section className="full-bet-content"><header><div><b>{mode === 'quick' ? '快捷投注' : mode === 'dual' ? '两面盘' : '号码投注'}</b><small>可自由叠加选择，不设互斥</small></div><span>赔率 <b>1.92</b></span></header>{mode === 'quick' && <><div className="rank-selector">{ranks.map((item) => <button className={rank === item ? 'active' : ''} key={item} onClick={() => setRank(item)}>{item}</button>)}</div><p className="board-section-title">{rank} · 选择玩法</p><div className="full-bet-options">{quickOptions.map((item) => <button key={item} onClick={() => onSelectOption(`${rank}${item}`)}><b>{item}</b><small>1.92</small></button>)}</div></>}{mode === 'dual' && <div className="full-bet-options">{dualOptions.map((item) => <button key={item} onClick={() => onSelectOption(item)}><b>{item}</b><small>1.92</small></button>)}</div>}{mode === 'numbers' && <><p className="board-section-title">选择号码 · 可重复选择</p><div className="full-bet-numbers">{Array.from({ length: 10 }, (_, index) => index + 1).map((number) => <button key={number} onClick={() => onSelectNumber(number)}>{number}<small>9.75</small></button>)}</div></>}</section></div><footer className="full-bet-footer"><div className="full-bet-summary"><button onClick={onClear}>清空选择</button><span>已选 <b>{selectedCount}</b> 项</span>{selectedCount > 0 && <button aria-label="删除最后一项" onClick={closeAndClearOne}>⌫</button>}</div><div className="amount-pills">{[20, 50, 100, 200].map((value) => <button className={amount === value ? 'active' : ''} key={value} onClick={() => setAmount(value)}>{value}</button>)}</div><button className="full-bet-confirm" onClick={onConfirm}>确认投注 <small>¥ {selectedCount * amount || 0}</small></button></footer></section></div>
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
