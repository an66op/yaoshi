import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { passwordUTF8ByteLength } from '../utils/password'
import { BetModeSettings, HelpSettings, HistorySettings, SecuritySettings } from './Profile'

const runtime = vi.hoisted(() => ({
  drawHistoryLimit: 50,
  defaultBetMode: 'chat' as 'chat' | 'detail',
  setDrawHistoryLimit: vi.fn(),
  setDefaultBetMode: vi.fn(),
}))

vi.mock('../hooks/useMemberPreferences', () => ({
  useMemberPreferences: () => ({
    drawHistoryLimit: runtime.drawHistoryLimit,
    defaultBetMode: runtime.defaultBetMode,
    setDrawHistoryLimit: runtime.setDrawHistoryLimit,
    setDefaultBetMode: runtime.setDefaultBetMode,
  }),
}))
vi.mock('../api/member', () => ({ memberApi: { changePassword: vi.fn() } }))
vi.mock('../api/lottery', () => ({ lotteryApi: { clock: vi.fn() } }))

type NodeProps = { children?: ReactNode; onClick?: () => void }

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

function optionButton(tree: ReactNode, label: string) {
  const section = find(tree, node => node.type === 'section' && text(node).includes(label))
  return find(section, node => node.type === 'button')!
}

describe('profile preference and account pages', () => {
  beforeEach(() => {
    runtime.drawHistoryLimit = 50
    runtime.defaultBetMode = 'chat'
    runtime.setDrawHistoryLimit.mockReset()
    runtime.setDefaultBetMode.mockReset()
  })

  it('offers exactly chat and detail as persisted default room modes', () => {
    const tree = BetModeSettings()
    const html = renderToStaticMarkup(tree)
    expect(html).toContain('聊天下注')
    expect(html).toContain('详细网投')
    expect(html).not.toContain('两面盘')
    expect(html).not.toContain('号码面板')
    optionButton(tree, '详细网投').props.onClick!()
    expect(runtime.setDefaultBetMode).toHaveBeenCalledWith('detail')
  })

  it('keeps the history page open long enough to apply its real query limit', () => {
    const tree = HistorySettings()
    const html = renderToStaticMarkup(tree)
    expect(html).toContain('供开奖记录图片、长龙及走势统计使用')
    expect(html).toContain('不会改变聊天消息或时间线的保留条数')
    optionButton(tree, '最近 100 期').props.onClick!()
    expect(runtime.setDrawHistoryLimit).toHaveBeenCalledWith(100)
  })

  it('shows only real password controls without fabricated security status', () => {
    const html = renderToStaticMarkup(<SecuritySettings onPasswordChanged={async () => undefined} />)
    expect(html).toContain('修改登录密码')
    expect(html).toContain('确认新密码')
    expect(html).toContain('8–72 个 UTF-8 字节')
    expect(html).not.toContain('实名认证')
    expect(html).not.toContain('设备保护')
    expect(html).not.toContain('已认证')
  })

  it('counts the same UTF-8 bytes as the backend password validator', () => {
    expect(passwordUTF8ByteLength('12345678')).toBe(8)
    expect(passwordUTF8ByteLength('密码密')).toBe(9)
    expect(passwordUTF8ByteLength('密'.repeat(24))).toBe(72)
    expect(passwordUTF8ByteLength('密'.repeat(25))).toBe(75)
  })

  it('limits help to real contact methods and the service navigation action', () => {
    const html = renderToStaticMarkup(<HelpSettings onOpenService={() => undefined} />)
    expect(html).toContain('联系方式')
    expect(html).toContain('在线客服')
    expect(html).toContain('进入客服')
    expect(html).not.toContain('邀请码')
    expect(html).not.toContain('注册链接')
    expect(html).not.toContain('提交问题反馈')
    expect(html).not.toContain('全天候')
  })
})
