import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from '../test/hookHarness'
import { SecuritySettings } from './Profile'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null, changePassword: vi.fn(), onPasswordChanged: vi.fn() }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
}))
vi.mock('../api/member', () => ({ memberApi: { changePassword: runtime.changePassword } }))
vi.mock('../api/lottery', () => ({ lotteryApi: { clock: vi.fn() } }))

type NodeProps = { children?: ReactNode; disabled?: boolean; onClick?: () => void; onChange?: (event: { target: { value: string } }) => void }

function text(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(text).join('')
  return isValidElement<NodeProps>(node) ? text(node.props.children) : ''
}

function find(node: ReactNode, predicate: (node: ReactElement<NodeProps>) => boolean): ReactElement<NodeProps> | undefined {
  if (Array.isArray(node)) return node.map(child => find(child, predicate)).find(Boolean)
  if (!isValidElement<NodeProps>(node)) return
  return predicate(node) ? node : find(node.props.children, predicate)
}

describe('profile password confirmation', () => {
  beforeEach(() => {
    runtime.hooks = new HookHarness()
    runtime.changePassword.mockReset().mockResolvedValue(null)
    runtime.onPasswordChanged.mockReset().mockResolvedValue(undefined)
  })
  afterEach(() => runtime.hooks?.unmount())

  const render = () => runtime.hooks!.render(() => SecuritySettings({ onPasswordChanged: runtime.onPasswordChanged }))
  const input = (label: string) => find(find(render(), node => node.type === 'label' && text(node).startsWith(label)), node => node.type === 'input')!
  const fill = (label: string, value: string) => input(label).props.onChange!({ target: { value } })
  const save = () => find(render(), node => node.type === 'button' && text(node).includes('保存新密码'))!

  it('blocks mismatched confirmation and submits only an exact valid pair', async () => {
    fill('原密码', 'OldPassword#1')
    fill('新密码', 'NewPassword#2')
    fill('确认新密码', 'different')
    expect(text(render())).toContain('两次输入的新密码不一致')
    expect(save().props.disabled).toBe(true)
    expect(runtime.changePassword).not.toHaveBeenCalled()

    fill('确认新密码', 'NewPassword#2')
    expect(save().props.disabled).toBe(false)
    save().props.onClick!()
    await Promise.resolve()
    expect(runtime.changePassword).toHaveBeenCalledWith('OldPassword#1', 'NewPassword#2')
    expect(runtime.onPasswordChanged).toHaveBeenCalledOnce()
    expect(runtime.changePassword.mock.invocationCallOrder[0]).toBeLessThan(runtime.onPasswordChanged.mock.invocationCallOrder[0])
  })

  it('uses UTF-8 bytes at both password boundaries and never exits on an API failure', async () => {
    fill('原密码', 'OldPassword#1')
    fill('新密码', '密码')
    fill('确认新密码', '密码')
    expect(save().props.disabled).toBe(true)

    fill('新密码', '密码密')
    fill('确认新密码', '密码密')
    expect(save().props.disabled).toBe(false)

    fill('新密码', '密'.repeat(25))
    fill('确认新密码', '密'.repeat(25))
    expect(save().props.disabled).toBe(true)

    runtime.changePassword.mockRejectedValueOnce(new Error('原密码不正确'))
    fill('新密码', '密'.repeat(24))
    fill('确认新密码', '密'.repeat(24))
    save().props.onClick!()
    await Promise.resolve()
    expect(runtime.onPasswordChanged).not.toHaveBeenCalled()
    expect(text(render())).toContain('原密码不正确')
  })
})
