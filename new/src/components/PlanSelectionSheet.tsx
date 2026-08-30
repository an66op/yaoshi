import { useEffect, useRef, useState } from 'react'
import type { RacingPlanOption, RacingPlanPosition, RacingPlanSelection } from '../api/plans'
import { RACING_PLAN_GROUPS, racingPlanPositionLabel } from '../utils/racingPlans'
import { managePlanSelectionFocus } from '../utils/planSelectionFocus'

export type PlanSelectionSheetProps = {
  selection: RacingPlanSelection
  positions: RacingPlanPosition[]
  options: RacingPlanOption[]
  allowedPositions: number[]
  allowedPlanKeys: string[]
  submitting: boolean
  error: string
  onCancel: () => void
  onConfirm: (selection: RacingPlanSelection) => void
  onEdit: () => void
}

export function PlanSelectionSheet({ selection, positions, options, allowedPositions, allowedPlanKeys, submitting, error, onCancel, onConfirm, onEdit }: PlanSelectionSheetProps) {
  const [draft, setDraft] = useState<RacingPlanSelection>(() => ({ ...selection }))
  const dialogRef = useRef<HTMLElement | null>(null)
  const closeRef = useRef(() => undefined)
  closeRef.current = () => { if (!submitting) onCancel() }
  useEffect(() => dialogRef.current ? managePlanSelectionFocus(dialogRef.current, () => closeRef.current()) : undefined, [])
  const pick = (patch: Partial<RacingPlanSelection>) => { setDraft(current => ({ ...current, ...patch })); onEdit() }
  const valid = allowedPositions.includes(draft.position) && allowedPlanKeys.includes(draft.plan_key)
  const position = positions.find(item => item.position === draft.position)
  const option = options.find(item => item.key === draft.plan_key)
  return <div className="plan-selection-overlay" role="presentation" onClick={() => closeRef.current()}>
    <section ref={dialogRef} className="plan-selection-sheet" role="dialog" aria-modal="true" aria-label="切换计划" onClick={event => event.stopPropagation()}>
      <header><div><b>切换计划</b><small>只切换当前页面，确认后读取独立计划数据</small></div><button type="button" aria-label="关闭切换计划" disabled={submitting} onClick={onCancel}>×</button></header>
      <div className="plan-selection-body">
        <fieldset><legend>选择名次</legend><div className="plan-position-grid">
          {positions.map(item => <button type="button" key={item.position} className={draft.position === item.position ? 'is-selected' : ''} aria-pressed={draft.position === item.position} disabled={submitting || !allowedPositions.includes(item.position)} title={allowedPositions.includes(item.position) ? item.label : '房间未开放该名次'} onClick={() => pick({ position: item.position })}>{item.label}</button>)}
        </div></fieldset>
        <fieldset><legend>选择计划类型</legend>
          {RACING_PLAN_GROUPS.map(group => <section className="plan-option-group" key={group.kind} aria-label={group.label}>
            <h3>{group.label}</h3><div className="plan-type-grid">{options.filter(item => item.kind === group.kind).map(item => <button type="button" key={item.key} className={draft.plan_key === item.key ? 'is-selected' : ''} aria-pressed={draft.plan_key === item.key} disabled={submitting || !allowedPlanKeys.includes(item.key)} title={allowedPlanKeys.includes(item.key) ? item.label : '房间未开放该计划'} onClick={() => pick({ plan_key: item.key })}>{item.label}</button>)}</div>
          </section>)}
        </fieldset>
        <p className="plan-selection-help">灰色选项尚未开放。不同名次与类型分别生成，切换不会修改房间全局偏好。</p>
        {option?.kind === 'size' && <p className="plan-selection-help">大小规则：1–5 为小，6–10 为大。</p>}
        {option?.kind === 'dragon_tiger' && <p className="plan-selection-help">龙虎比较：{position?.label ?? racingPlanPositionLabel(draft.position)}与{racingPlanPositionLabel(position?.opponent_position ?? 11 - draft.position)}，方向以当前所选名次为准。</p>}
      </div>
      <footer>
        <p aria-live="polite">已选：<b>{position?.label ?? racingPlanPositionLabel(draft.position)} · {option?.label ?? draft.plan_key}</b></p>
        {error && <p className="plan-selection-error" role="alert">{error}</p>}
        {!valid && <p className="plan-selection-error">请选择房间已开放的名次和计划类型。</p>}
        <div><button type="button" disabled={submitting} onClick={onCancel}>取消</button><button type="button" className="plan-selection-confirm" disabled={submitting || !valid} onClick={() => onConfirm(draft)}>{submitting ? '正在切换…' : '确定'}</button></div>
      </footer>
    </section>
  </div>
}
