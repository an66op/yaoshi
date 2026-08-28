import { describe, expect, it } from 'vitest'
import { chatScrollState } from './chatScroll'

describe('chat scroll state', () => {
  it('keeps following after a small mobile scroll bounce', () => {
    expect(chatScrollState({ scrollHeight: 1000, scrollTop: 424, clientHeight: 500 })).toEqual({
      distance: 76,
      following: true,
      showLatest: false,
    })
  })

  it('does not show the latest button after only a slight upward move', () => {
    const state = chatScrollState({ scrollHeight: 1000, scrollTop: 370, clientHeight: 500 })
    expect(state.following).toBe(false)
    expect(state.showLatest).toBe(false)
  })

  it('shows the latest button when the reader is actually browsing history', () => {
    const state = chatScrollState({ scrollHeight: 1200, scrollTop: 400, clientHeight: 500 })
    expect(state.distance).toBe(300)
    expect(state.following).toBe(false)
    expect(state.showLatest).toBe(true)
  })
})
