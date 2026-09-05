import { useEffect, useMemo, useRef, useState } from 'react'
import { portalApi, type GameOdds } from '../api/portal'
import { gameManualOptions, type GameManual } from '../data/gamePlayManuals'
import type { Game } from '../types'
import { formatBetAmount } from '../utils/betAmount'
import { managePlanSelectionFocus } from '../utils/planSelectionFocus'

export type GameGuideTab = 'rules' | 'odds'

type Props = {
  games: Game[]
  initialTab: GameGuideTab
}

const oddsText = (value: number) => Number.isFinite(value) ? value.toFixed(3).replace(/0+$/, '').replace(/\.$/, '') : '—'
const statusLabel = (manual: GameManual) => manual.status === 'implemented' ? '已接入' : manual.status === 'partial' ? '部分接入' : '参考'

export function GameGuidePanel({ games, initialTab }: Props) {
  const manuals = useMemo(() => gameManualOptions(games), [games])
  const [tab, setTab] = useState<GameGuideTab>(initialTab)
  const [selectedID, setSelectedID] = useState(() => manuals[0]?.id ?? '')
  const selected = manuals.find(item => item.id === selectedID) ?? manuals[0]
  const [odds, setOdds] = useState<GameOdds | null>(null)
  const [oddsError, setOddsError] = useState('')
  const [loadingOdds, setLoadingOdds] = useState(false)
  const [pickerOpen, setPickerOpen] = useState(false)
  const requestIDRef = useRef(0)
  const pickerDialogRef = useRef<HTMLElement | null>(null)
  const activeOdds = selected?.gameId && odds?.game_id === selected.gameId ? odds : null
  const oddsPending = Boolean(selected?.gameId && !activeOdds && !oddsError)

  useEffect(() => {
    if (!selected?.gameId) {
      requestIDRef.current += 1
      setOdds(null)
      setOddsError('该手册尚未绑定本地彩种，因此没有可用赔率。')
      setLoadingOdds(false)
      return
    }
    const requestID = ++requestIDRef.current
    setLoadingOdds(true)
    setOdds(null)
    setOddsError('')
    void portalApi.gameOdds(selected.gameId).then(result => {
      if (requestID !== requestIDRef.current) return
      setOdds(result)
    }).catch(reason => {
      if (requestID !== requestIDRef.current) return
      setOddsError(reason instanceof Error ? reason.message : '赔率暂时无法读取')
    }).finally(() => {
      if (requestID === requestIDRef.current) setLoadingOdds(false)
    })
    return () => { requestIDRef.current += 1 }
  }, [selected?.gameId])

  useEffect(() => pickerOpen && pickerDialogRef.current
    ? managePlanSelectionFocus(pickerDialogRef.current, () => setPickerOpen(false))
    : undefined, [pickerOpen])

  if (!selected) return <p className="game-guide-empty">暂无玩法手册</p>

  const selectGame = (nextID: string) => {
    const next = manuals.find(item => item.id === nextID)
    requestIDRef.current += 1
    setOdds(null)
    setOddsError('')
    setLoadingOdds(Boolean(next?.gameId))
    setSelectedID(nextID)
    setPickerOpen(false)
  }

  const manualGroups = [
    { id: 'live', title: '当前房间彩种', items: manuals.filter(item => Boolean(item.gameId)) },
    { id: 'reference', title: '规则参考', items: manuals.filter(item => !item.gameId) },
  ].filter(group => group.items.length > 0)

  return <div className="game-guide-panel">
    <nav className="game-guide-tabs" aria-label="玩法与赔率切换">
      <button className={tab === 'rules' ? 'selected' : ''} onClick={() => setTab('rules')}>玩法说明</button>
      <button className={tab === 'odds' ? 'selected' : ''} onClick={() => setTab('odds')}>彩种赔率</button>
    </nav>
    <div className="game-guide-picker">
      <span className="game-guide-picker-label">选择彩种</span>
      <button type="button" className="game-guide-picker-trigger" aria-haspopup="dialog" aria-expanded={pickerOpen} aria-label={`选择彩种，当前${selected.title}`} onClick={() => setPickerOpen(true)}>
        <span><b>{selected.title}</b><small>{selected.statusText}</small></span>
        <em className={`status-${selected.status}`}>{statusLabel(selected)}</em>
        <i aria-hidden="true" />
      </button>
    </div>
    {pickerOpen && <div className="game-guide-picker-overlay" role="presentation" onClick={() => setPickerOpen(false)}>
      <section ref={pickerDialogRef} className="game-guide-picker-sheet" role="dialog" aria-modal="true" aria-label="选择彩种" onClick={event => event.stopPropagation()}>
        <header><div><b>选择彩种</b><small>按当前房间与参考手册分类展示</small></div><button type="button" aria-label="关闭彩种选择" onClick={() => setPickerOpen(false)}>×</button></header>
        <div className="game-guide-picker-body">
          {manualGroups.map(group => <section className="game-guide-picker-group" key={group.id} aria-label={group.title}>
            <header><b>{group.title}</b><span>{group.items.length} 个</span></header>
            <div>{group.items.map(item => {
              const current = item.id === selected.id
              return <button type="button" key={item.id} className={current ? 'selected' : ''} data-game-manual-id={item.id} aria-pressed={current} onClick={() => selectGame(item.id)}>
                <span><b>{item.title}</b><small>{item.statusText}</small></span>
                <em className={`status-${item.status}`}>{statusLabel(item)}</em>
                <i aria-hidden="true">{current ? '✓' : ''}</i>
              </button>
            })}</div>
          </section>)}
        </div>
      </section>
    </div>}
    <GuideHeader manual={selected} odds={activeOdds} />
    {tab === 'rules'
      ? <RulesContent manual={selected} />
      : <OddsContent manual={selected} odds={activeOdds} loading={loadingOdds || oddsPending} error={oddsError} />}
  </div>
}

function GuideHeader({ manual, odds }: { manual: GameManual; odds: GameOdds | null }) {
  const chat = odds?.bet_modes?.chat ?? manual.betModes.chat
  const web = odds?.bet_modes?.web ?? manual.betModes.web
  return <section className={`game-guide-summary status-${manual.status}`}>
    <header><div><b>{manual.title}</b><small>{manual.summary}</small></div><em>{manual.statusText}</em></header>
    <div className="game-guide-modes">
      <span><i className={chat ? 'on' : 'off'} />聊天下注 {chat ? '可用' : '暂停'}</span>
      <span><i className={web ? 'on' : 'off'} />详细网投 {web ? '可用' : '暂停'}</span>
    </div>
    {manual.sourceURL && <a href={manual.sourceURL} target="_blank" rel="noreferrer">查看开奖来源</a>}
  </section>
}

function RulesContent({ manual }: { manual: GameManual }) {
  return <div className="game-guide-rules">
    {manual.sections.map((section, index) => <article key={`${section.title}-${index}`}>
      <header><span>{index + 1}</span><b>{section.title}</b></header>
      <p>{section.body}</p>
      {section.examples?.map(example => <code key={example}>{example}</code>)}
    </article>)}
  </div>
}

function OddsContent({ manual, odds, loading, error }: { manual: GameManual; odds: GameOdds | null; loading: boolean; error: string }) {
  if (loading) return <p className="game-guide-empty">正在读取当前房间赔率…</p>
  if (error) return <p className="game-guide-empty" role="status">{error}</p>
  if (!odds?.rules_ready) return <p className="game-guide-empty">{odds?.rules_message || '该彩种规则尚未配置，暂不展示赔率。'}</p>
  if (!odds.show_odds) return <p className="game-guide-empty">当前房间已关闭赔率展示。</p>
  if (!odds.items.length) return <p className="game-guide-empty">{manual.title} 暂无已配置赔率。</p>
  return <div className="game-guide-odds">
    <header><span>玩法</span><span>赔率</span></header>
    {odds.items.map(item => <article key={item.play_code}>
      <div><b>{item.play_name}</b>{item.category && <em>{item.category}</em>}<small>{item.description || '以当前房间规则为准'}</small>{item.example && <code>{item.example}</code>}</div>
      <div><strong>{oddsText(item.odds)}</strong><small>单注 {formatBetAmount(item.min_bet)}–{formatBetAmount(item.max_bet)}</small><small>每期上限 {formatBetAmount(item.max_user_period)}</small></div>
    </article>)}
    <p>这里展示的是当前会员、当前房间最终生效赔率；切换彩种后会重新读取，不使用演示或默认值。</p>
  </div>
}
