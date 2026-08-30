import { useState } from 'react'
import type { Game } from '../types'
import { ballTone } from '../data/games'
import { formatBetAmount } from '../utils/betAmount'
import { roomBettingTarget } from '../utils/gameRoomBetting'
import { oddsForPlayCode, oddsLabel, type PlayOdds } from '../utils/gameRoomSafety'
import { boardAmountCents, boardChoiceCode, boardChoiceState, boardRankLabel, boardSelectionCommand, boardSelectionGroups, boardSelectionKey, racingRanks, toggleBoardChoice, type BoardSelection } from '../utils/fullBetSelection'
import { Icon } from './Icon'

type BetMode = 'quick' | 'dual' | 'numbers'
type Props = {
  game: Game; mode: BetMode; submitting?: boolean; odds: PlayOdds; oddsHidden: boolean; oddsResponseReady: boolean
  onModeChange: (mode: BetMode) => void; onConfirm: (content: string) => void; onClose: () => void
}
const tabs: Array<{ id: BetMode | 'sum'; label: string }> = [
  { id: 'quick', label: '快捷' }, { id: 'dual', label: '两面盘' }, { id: 'numbers', label: '车号 1–10' }, { id: 'sum', label: '冠亚和' },
]

export function FullBetBoard({ game, mode, submitting, odds, oddsHidden, oddsResponseReady, onModeChange, onConfirm, onClose }: Props) {
  // 快捷是批量编辑；车号/两面盘的名次按钮只是切换独立面板。
  // 两套编辑目标不能共用，否则访问过但未选号的名次也会被一起下单。
  const [quickPositions, setQuickPositions] = useState([1])
  const [rankPosition, setRankPosition] = useState(1)
  const [selections, setSelections] = useState<BoardSelection[]>([])
  const [amount, setAmount] = useState('20')
  const [sumTab, setSumTab] = useState(false)
  const [selectionOpen, setSelectionOpen] = useState(false)
  const activeTab = sumTab ? 'sum' : mode
  const batchEditing = activeTab === 'quick'
  const positions = batchEditing ? quickPositions : [rankPosition]
  const { issue, timing } = roomBettingTarget(game)
  const groups = boardSelectionGroups(selections)
  const cents = boardAmountCents(amount)
  const totalCents = cents === null ? null : cents * selections.length
  const validAmount = totalCents !== null && Number.isSafeInteger(totalCents)
  const total = validAmount ? formatBetAmount(totalCents / 100) : '—'
  const command = boardSelectionCommand(selections, amount)
  const hasOdds = oddsResponseReady && selections.every(item => oddsHidden || oddsForPlayCode(item.playCode, odds) !== null)
  const canSubmit = !submitting && selections.length > 0 && command !== '' && timing.accepting && hasOdds
  const targetPositions = sumTab ? [6] : positions
  const canChoose = (choice: string) => !submitting && targetPositions.length > 0 && oddsResponseReady
    && (oddsHidden || oddsForPlayCode(sumTab ? 'sum' : boardChoiceCode(choice), odds) !== null)
    && (sumTab || !/[龙虎]/.test(choice) || positions.every(position => position <= 5))
  const choose = (choice: string) => {
    if (canChoose(choice)) setSelections(previous => toggleBoardChoice(previous, targetPositions, choice, sumTab))
  }
  const changeRank = (position: number) => {
    if (submitting) return
    if (batchEditing) {
      setQuickPositions(previous => previous.includes(position) ? previous.filter(item => item !== position) : [...previous, position].sort((a, b) => a - b))
    } else {
      setRankPosition(position)
    }
  }
  const option = (choice: string) => {
    const state = boardChoiceState(selections, targetPositions, choice, sumTab)
    const numeric = /^\d+$/.test(choice)
    // 部分名次已选时保持普通外观；各名次的选择仍保存在清单中。
    return <button type="button" key={choice} data-choice={choice} aria-label={`${sumTab ? '冠亚和' : '选择'}${choice}`} aria-pressed={state}
      className={`board-choice${state === true ? ' selected' : ''}`} disabled={!canChoose(choice)} onClick={() => choose(choice)}>
      <b className={numeric && !sumTab ? `board-number ${ballTone(Number(choice))}` : ''}>{choice}</b>
      <small>{oddsLabel(oddsForPlayCode(sumTab ? 'sum' : boardChoiceCode(choice), odds), 3, oddsHidden)}</small>
    </button>
  }
  return <div className="full-bet-layer" onClick={submitting ? undefined : onClose}>
    <section className="full-bet-board" aria-label="详细投注面板" onClick={event => event.stopPropagation()}>
      <header className="full-bet-header">
        <button type="button" aria-label="返回游戏聊天室" disabled={submitting} onClick={onClose}><Icon name="back" /></button>
        <div><b>{game.title}</b><small>第 {issue} 期 · {timing.statusLabel}</small></div>
        <button type="button" className="full-bet-close" aria-label="关闭投注面板" disabled={submitting} onClick={onClose}>×</button>
      </header>
      <div className="full-bet-current"><span>{timing.due}</span><i className={`full-bet-acceptance ${timing.accepting ? 'open' : 'closed'}`}>{timing.statusLabel}</i><div>{game.balls.map((ball, index) => <b className={ballTone(ball)} key={index}>{ball}</b>)}</div></div>
      <div className="full-bet-workspace">
        <aside aria-label="投注玩法">{tabs.map(tab => <button type="button" className={activeTab === tab.id ? 'active' : ''} aria-pressed={activeTab === tab.id} disabled={submitting} key={tab.id} onClick={() => { setSumTab(tab.id === 'sum'); if (tab.id !== 'sum') onModeChange(tab.id) }}>{tab.label}</button>)}</aside>
        <section className="full-bet-content">
          <header><b>{tabs.find(tab => tab.id === activeTab)?.label}</b><small>{sumTab ? '前两名号码之和' : batchEditing ? '名次可多选' : '按名次独立选择'}</small></header>
          {!sumTab && <>
            <div className="rank-selector" aria-label={batchEditing ? '批量选择名次（可多选）' : '切换名次（独立编辑）'}>{racingRanks.map((label, index) => {
              const position = index + 1
              const count = selections.filter(item => item.position === position && item.playCode !== 'sum').length
              return <button type="button" aria-label={`编辑${label}`} aria-pressed={positions.includes(position)} className={positions.includes(position) ? 'active' : ''} disabled={submitting} key={label} onClick={() => changeRank(position)}>{label}{count > 0 && <small className="rank-count">{count}</small>}</button>
            })}</div>
            <p className="board-section-title">{positions.length ? `${positions.map(boardRankLabel).join('、')} · ${batchEditing ? '批量选择投注项' : '选择投注项'}` : '请先选择名次'}</p>
          </>}
          {activeTab !== 'numbers' && <div className="full-bet-options">{(activeTab === 'dual' ? ['大', '小', '单', '双', '龙', '虎'] : ['大', '小', '单', '双']).map(option)}</div>}
          {activeTab !== 'dual' && <>
            {activeTab === 'quick' && <p className="board-section-title">车号 1–10</p>}
            <div className="full-bet-numbers">{Array.from({ length: sumTab ? 17 : 10 }, (_, index) => String(index + (sumTab ? 3 : 1))).map(option)}</div>
          </>}
        </section>
      </div>
      <footer className="full-bet-footer">
        <div className="full-bet-summary"><button type="button" disabled={submitting || !selections.length} onClick={() => setSelections([])}>清空选择</button><button type="button" className="full-bet-selection-toggle" aria-expanded={selectionOpen} onClick={() => setSelectionOpen(previous => !previous)}><span aria-live="polite">已选 <b>{groups.length}</b> 组 · <b>{selections.length}</b> 注</span><i>{selectionOpen ? '收起' : '查看清单'}</i></button></div>
        {selections.length > 0 && <div className="board-selected-preview" aria-label="已选投注">{groups.map(group => <span key={group.rank}>{group.label} <b>{group.choices.map(item => item.selection).join('、')}</b></span>)}</div>}
        {selectionOpen && <div className="full-bet-selection-list"><header><b>本次投注清单</b><span>合计 ¥ {total}</span></header>{groups.length ? groups.map(group => <article key={group.rank}><div><b>{group.label}</b><div className="board-selection-chips">{group.choices.map(item => <button type="button" key={boardSelectionKey(item)} disabled={submitting} aria-label={`移除${group.label}${item.selection}`} onClick={() => setSelections(previous => previous.filter(choice => boardSelectionKey(choice) !== boardSelectionKey(item)))}>{item.selection}<span aria-hidden="true"> ×</span></button>)}</div></div></article>) : <p>暂未选择玩法或号码</p>}</div>}
        <div className="amount-pills" aria-label="单注金额">{[20, 50, 100, 200].map(value => <button type="button" className={cents === value * 100 ? 'active' : ''} aria-pressed={cents === value * 100} disabled={submitting} key={value} onClick={() => setAmount(String(value))}>{value}</button>)}</div>
        <div className="board-submit-row"><label className="board-custom-amount">单注<input aria-label="自定义单注金额" aria-invalid={!validAmount} inputMode="decimal" autoComplete="off" placeholder="金额" disabled={submitting} value={amount} onChange={event => setAmount(event.target.value)} /></label><button type="button" className="full-bet-confirm" disabled={!canSubmit} onClick={() => { if (canSubmit) onConfirm(command) }}>{submitting ? '提交中…' : !timing.accepting ? timing.statusLabel : !validAmount ? '金额须大于0，最多2位小数' : !hasOdds && selections.length ? '赔率待配置' : '立即投注'} <small>¥ {total}</small></button></div>
      </footer>
    </section>
  </div>
}
