import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HookHarness } from '../test/hookHarness'
import { Wallet } from './Wallet'

const runtime = vi.hoisted(() => ({ hooks: null as HookHarness | null }))
vi.mock('react', async importOriginal => ({
  ...await importOriginal<typeof import('react')>(),
  useState: <T,>(initial: T | (() => T)) => runtime.hooks!.useState(initial),
  useEffect: (effect: () => void | (() => void), dependencies?: readonly unknown[]) => runtime.hooks!.useEffect(effect, dependencies),
}))
vi.mock('../api/member', () => ({
  memberApi: {
    walletSummary: vi.fn(), paymentAccounts: vi.fn(), paymentAccountQRCodeURL: (id: number) => `/api/member/payment-accounts/${id}/qr-code`,
  },
}))
vi.mock('../api/bets', () => ({ betsApi: {} }))

type ButtonProps = { children?: ReactNode; onClick?: () => void; 'aria-label'?: string }
function buttons(node: ReactNode): Array<ReactElement<ButtonProps>> {
  if (Array.isArray(node)) return node.flatMap(buttons)
  if (!isValidElement<{ children?: ReactNode }>(node)) return []
  return [...(node.type === 'button' ? [node as ReactElement<ButtonProps>] : []), ...buttons(node.props.children)]
}

function textContent(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textContent).join('')
  if (!isValidElement<{ children?: ReactNode }>(node)) return ''
  return textContent(node.props.children)
}

describe('member payment-account add flow', () => {
  beforeEach(() => { runtime.hooks = new HookHarness() })
  afterEach(() => runtime.hooks?.unmount())

  const render = () => runtime.hooks!.render(() => Wallet({ balance: 100, walletAction: 'channels', onNavigate: () => undefined }))

  it('opens a dedicated internal page with a secure QR image picker and clear return', () => {
    const list = render()
    const addAlipay = buttons(list).find(button => textContent(button.props.children).includes('支付宝'))
    expect(addAlipay).toBeDefined()
    addAlipay!.props.onClick?.()

    const editor = render()
    const html = renderToStaticMarkup(editor)
    expect(html).toContain('新增支付宝')
    expect(html).toContain('账户资料')
    expect(html).toContain('type="file"')
    expect(html).toContain('accept="image/png,image/jpeg,image/webp"')
    expect(html).toContain('安全重编码')
    expect(html).toContain('aria-label="返回收款方式"')
    expect(html).not.toContain('wallet-payment-account-list')

    ;(editor as ReactElement<{ onBack: () => void }>).props.onBack()
    expect(renderToStaticMarkup(render())).toContain('wallet-payment-account-list')
  })
})
