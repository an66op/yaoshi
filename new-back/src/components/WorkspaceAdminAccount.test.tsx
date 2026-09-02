import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { WorkspaceAdminAccountFields, WorkspaceAdminCreatedDialog, WorkspaceAdminLoginHint } from './WorkspaceAdminAccount'

type ElementProps = {
  children?: ReactNode
  label?: string
  value?: string
  type?: string
  required?: boolean
  disabled?: boolean
  helperText?: string
  autoComplete?: string
  href?: string
  target?: string
  rel?: string
  severity?: string
  open?: boolean
  onChange?: (event: { target: { value: string } }) => void
  onClick?: () => void
  onClose?: () => void
}
type Element = ReactElement<ElementProps>

function elements(node: ReactNode): Element[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  if (!isValidElement<ElementProps>(node)) return []
  return [node, ...elements(node.props.children)]
}

function content(node: ReactNode): string {
  if (Array.isArray(node)) return node.map(content).join('')
  if (isValidElement<ElementProps>(node)) return content(node.props.children)
  return typeof node === 'string' || typeof node === 'number' ? String(node) : ''
}

describe('workspace administrator fields and login guidance', () => {
  afterEach(() => vi.unstubAllGlobals())

  it.each(['tenant', 'agent'] as const)('identifies the %s account as a management login with its current host', role => {
    vi.stubGlobal('window', { location: { origin: 'https://admin.example.test' } })
    const hint = WorkspaceAdminLoginHint({ role })
    const text = content(hint)
    expect(text).toContain(role === 'tenant' ? '租户' : '代理')
    expect(text).toContain('账号本身即可登录管理后台')
    expect(text).toContain('无需另外创建管理员账号')
    expect(text).toContain('管理后台')
    const link = elements(hint).find(element => element.props.href)!
    expect(link.props.href).toBe('https://admin.example.test/login')
    expect(link.props.target).toBe('_blank')
    expect(link.props.rel).toContain('noopener')
  })

  it('requires an explicit new administrator account and masked initial password', () => {
    const onUsernameChange = vi.fn()
    const onPasswordChange = vi.fn()
    const root = WorkspaceAdminAccountFields({ role: 'agent', username: 'agent-account', password: 'initial-secret', onUsernameChange, onPasswordChange })
    const fields = elements(root).filter(element => element.props.label)
    const username = fields.find(element => element.props.label === '代理登录账号')!
    const password = fields.find(element => element.props.type === 'password')!
    expect(username.props).toMatchObject({ required: true, value: 'agent-account', disabled: false })
    expect(username.props.helperText).toContain('不是房间号')
    expect(password.props).toMatchObject({ required: true, value: 'initial-secret', autoComplete: 'new-password', disabled: false })
    expect(password.props.helperText).toContain('字节')
    username.props.onChange!({ target: { value: 'edited-account' } })
    password.props.onChange!({ target: { value: 'edited-secret' } })
    expect(onUsernameChange).toHaveBeenCalledWith('edited-account')
    expect(onPasswordChange).toHaveBeenCalledWith('edited-secret')
  })

  it('does not create or expose an initial password while editing an existing account', () => {
    const root = WorkspaceAdminAccountFields({ role: 'tenant', username: 'existing-tenant', password: '', onUsernameChange: vi.fn(), onPasswordChange: vi.fn(), editing: true })
    expect(elements(root).find(element => element.props.label === '租户登录账号')?.props.disabled).toBe(true)
    expect(elements(root).find(element => element.props.label === '租户登录账号')?.props.helperText).toContain('创建后不可修改')
    expect(elements(root).some(element => element.props.type === 'password')).toBe(false)
    expect(elements(root).some(element => element.type === WorkspaceAdminLoginHint)).toBe(false)
  })

  it('locks both credentials while creation is pending', () => {
    const root = WorkspaceAdminAccountFields({ role: 'agent', username: 'new-agent', password: 'initial-secret', onUsernameChange: vi.fn(), onPasswordChange: vi.fn(), disabled: true })
    const fields = elements(root).filter(element => element.props.label)
    expect(fields).toHaveLength(2)
    expect(fields.every(element => element.props.disabled)).toBe(true)
  })
})

describe('workspace administrator success dialog', () => {
  afterEach(() => vi.unstubAllGlobals())

  it.each(['tenant', 'agent'] as const)('shows the returned %s identity and login details without the initial secret', role => {
    vi.stubGlobal('window', { location: { origin: 'http://127.0.0.1:5174' } })
    const account = { role, username: 'api-returned-login', roomCode: '998877', status: 1, password: 'never-display-this-secret' }
    const root = WorkspaceAdminCreatedDialog({ account, onClose: vi.fn() })
    const text = content(root)
    expect(root.props.open).toBe(true)
    expect(text).toContain('api-returned-login')
    expect(text).toContain('998877')
    expect(text).toContain(`账号身份：${role === 'tenant' ? '租户' : '代理'}`)
    expect(text).toContain(`${role === 'tenant' ? '租户' : '代理'}账号已创建`)
    expect(text).toContain('身份由系统识别')
    expect(text).toContain(`账号资料在${role === 'tenant' ? '租户' : '代理'}管理中维护`)
    expect(text).toContain('重置密码')
    expect(text).not.toContain(account.password)
    expect(elements(root).find(element => element.props.href)?.props.href).toBe('http://127.0.0.1:5174/login')
    expect(elements(root).find(element => element.props.severity)?.props.severity).toBe('success')
  })

  it('does not claim disabled accounts can sign in before being enabled', () => {
    const root = WorkspaceAdminCreatedDialog({ account: { role: 'agent', username: 'disabled-agent', roomCode: '998877', status: 0 }, onClose: vi.fn() })
    expect(elements(root).find(element => element.props.severity)?.props.severity).toBe('warning')
    expect(content(root)).toContain('已停用')
    expect(content(root)).toContain('启用后才能登录管理后台')
    expect(content(root)).not.toContain('可使用创建时设置的密码登录管理后台')
  })

  it('stays closed until an account was actually created and dismisses through the owner', () => {
    const onClose = vi.fn()
    const root = WorkspaceAdminCreatedDialog({ account: null, onClose })
    expect(root.props.open).toBe(false)
    expect(elements(root).some(element => element.props.severity)).toBe(false)
    root.props.onClose()
    elements(root).find(element => element.props.onClick)?.props.onClick!()
    expect(onClose).toHaveBeenCalledTimes(2)
  })
})
