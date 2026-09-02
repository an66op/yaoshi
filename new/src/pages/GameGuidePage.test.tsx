import { isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { Game } from '../types'
import { GameGuidePage } from './GameGuidePage'

vi.mock('../components/GameGuidePanel', () => ({
  GameGuidePanel: ({ initialTab }: { initialTab: string }) => <div data-guide-tab={initialTab}>手册内容</div>,
}))

type NodeProps = { children?: ReactNode; onClick?: () => void; 'aria-label'?: string }
type Node = ReactElement<NodeProps>
function elements(node: ReactNode): Node[] {
  if (Array.isArray(node)) return node.flatMap(elements)
  return isValidElement<NodeProps>(node) ? [node, ...elements(node.props.children)] : []
}

describe('GameGuidePage', () => {
  it('renders as a page, starts on the requested tab and exposes page back navigation', () => {
    const onBack = vi.fn()
    const page = GameGuidePage({ games: [] as Game[], initialTab: 'odds', onBack })
    const html = renderToStaticMarkup(page)

    expect(html).toContain('class="game-guide-page"')
    expect(html).toContain('data-guide-tab="odds"')
    expect(html).not.toContain('role="dialog"')
    elements(page).find(node => node.props['aria-label'] === '返回我的')!.props.onClick!()
    expect(onBack).toHaveBeenCalledOnce()
  })

  it('defaults to the gameplay rules tab', () => {
    const html = renderToStaticMarkup(<GameGuidePage games={[]} onBack={() => undefined} />)
    expect(html).toContain('data-guide-tab="rules"')
  })
})
