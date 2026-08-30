import type { MouseEvent } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { ScrollToLatestButton } from './ScrollToLatestButton'

describe('compact scroll-to-latest control', () => {
  it('shows only an icon while preserving its accessible purpose', () => {
    const html = renderToStaticMarkup(<ScrollToLatestButton keyboardOpen={false} onScrollToLatest={() => {}} />)
    expect(html.replace(/<[^>]*>/g, '').trim()).toBe('')
    expect(html).toContain('aria-label="回到最新消息"')
    expect(html).toContain('aria-hidden="true"')
    expect(html).toContain('<svg')
    expect(html).not.toContain('<small')
    expect(html).not.toContain('keyboard-open')
  })

  it('retains the keyboard offset and existing scroll action without bubbling', () => {
    const onScrollToLatest = vi.fn()
    const stopPropagation = vi.fn()
    const button = ScrollToLatestButton({ keyboardOpen: true, onScrollToLatest })
    expect(button.props.className).toBe('scroll-latest-button keyboard-open')
    button.props.onClick({ stopPropagation } as unknown as MouseEvent<HTMLButtonElement>)
    expect(stopPropagation).toHaveBeenCalledOnce()
    expect(onScrollToLatest).toHaveBeenCalledOnce()
  })
})
