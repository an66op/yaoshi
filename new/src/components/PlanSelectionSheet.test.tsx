import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PlanSelectionSheet, type PlanSelectionSheetProps } from './PlanSelectionSheet'
import { HookHarness } from '../test/hookHarness'
import { racingPlanOptions, racingPlanPositions } from '../test/racingPlanFixtures'
import { DEFAULT_RACING_PLAN } from '../utils/racingPlans'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useRef: <T,>(initial: T) => runtime.hooks!.useRef(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))

type ButtonProps = { children: ReactNode; onClick: () => void; disabled?: boolean; 'aria-pressed'?: boolean }
function buttons(node: ReactNode): Array<ReactElement<ButtonProps>> {
  if (Array.isArray(node)) return node.flatMap(buttons)
  if (!isValidElement<{ children?: ReactNode }>(node)) return []
  return [...(node.type === 'button' ? [node as ReactElement<ButtonProps>] : []), ...buttons(node.props.children)]
}

describe('plan selection confirmation sheet', () => {
  let props: PlanSelectionSheetProps
  beforeEach(() => {
    runtime.hooks = new HookHarness()
    props = { selection: { ...DEFAULT_RACING_PLAN }, positions: racingPlanPositions, options: racingPlanOptions,
      allowedPositions: racingPlanPositions.map(item => item.position), allowedPlanKeys: racingPlanOptions.map(item => item.key),
      submitting: false, error: '', onCancel: vi.fn(), onConfirm: vi.fn(), onEdit: vi.fn() }
  })
  afterEach(() => runtime.hooks?.unmount())
  const render = () => runtime.hooks!.render(() => PlanSelectionSheet(props))
  const button = (label: string) => buttons(render()).find(item => item.props.children === label)!

  it('renders ten positions in the position grid and all seventeen grouped plan types', () => {
    const html = runtime.hooks!.render(() => renderToStaticMarkup(<PlanSelectionSheet {...props} />))
    expect(html).toContain('role="dialog"')
    expect(html).toContain('plan-position-grid')
    expect(html).toContain('plan-type-grid')
    for (const position of racingPlanPositions) expect(html).toContain(position.label)
    for (const option of racingPlanOptions) expect(html).toContain(option.label)
    expect(button('冠军').props['aria-pressed']).toBe(true)
    expect(button('四期五码').props['aria-pressed']).toBe(true)
    expect(html).not.toContain('演示')
    expect(html).not.toContain('命中率')
  })
  it('keeps local edits isolated and cancel never submits or mutates the selected stream', () => {
    button('亚军').props.onClick()
    button('三期六码').props.onClick()
    expect(button('亚军').props['aria-pressed']).toBe(true)
    expect(button('三期六码').props['aria-pressed']).toBe(true)
    expect(props.selection).toEqual(DEFAULT_RACING_PLAN)
    expect(props.onConfirm).not.toHaveBeenCalled()
    button('取消').props.onClick()
    expect(props.onCancel).toHaveBeenCalledTimes(1)
    expect(props.onConfirm).not.toHaveBeenCalled()
  })
  it('submits the exact draft only when confirmed', () => {
    button('第十名').props.onClick()
    button('一期八码').props.onClick()
    button('确定').props.onClick()
    expect(props.onConfirm).toHaveBeenCalledWith({ position: 10, plan_key: 'one-period-eight-codes' })
    expect(props.onCancel).not.toHaveBeenCalled()
  })
  it('disables unallowed options and preserves server quota feedback', () => {
    props.allowedPositions = [1]
    props.allowedPlanKeys = ['four-period-five-codes']
    props.error = '活跃计划已达20组上限'
    expect(button('亚军').props.disabled).toBe(true)
    expect(button('三期六码').props.disabled).toBe(true)
    const html = runtime.hooks!.render(() => renderToStaticMarkup(<PlanSelectionSheet {...props} />))
    expect(html).toContain('role="alert"')
    expect(html).toContain(props.error)
  })
  it('prevents repeat submission or cancellation while confirmation is in flight', () => {
    props.submitting = true
    expect(button('正在切换…').props.disabled).toBe(true)
    expect(button('取消').props.disabled).toBe(true)
    expect(button('亚军').props.disabled).toBe(true)
  })
})
