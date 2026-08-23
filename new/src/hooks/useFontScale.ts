import { useLayoutEffect } from 'react'
import type { FontScalePreference } from './useMemberPreferences'

const scaleByPreference: Record<FontScalePreference, number> = {
  standard: 1,
  large: 1.1,
  xlarge: 1.2,
}

const textSelector = [
  'a', 'b', 'button', 'em', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
  'input', 'label', 'li', 'option', 'p', 'select', 'small', 'span',
  'strong', 'td', 'textarea', 'th',
].join(',')

/**
 * Applies the member's readability preference to text only.  Layout stays at
 * its native width, which avoids the clipping caused by whole-page scaling.
 */
export function useFontScale(preference: FontScalePreference, viewKey = '') {
  useLayoutEffect(() => {
    const scale = scaleByPreference[preference]
    const apply = (scope: ParentNode) => {
      scope.querySelectorAll<HTMLElement>(textSelector).forEach((element) => {
        const stored = Number(element.dataset.memberFontBase)
        const base = Number.isFinite(stored) && stored > 0
          ? stored
          // A fresh page has no saved element baseline yet: computed CSS is
          // always its unscaled design size, even if the chosen preference was
          // restored from local storage. Do not divide here, otherwise a
          // refresh silently shrinks the selected large text back to standard.
          : Number.parseFloat(window.getComputedStyle(element).fontSize)
        if (!Number.isFinite(base) || base <= 0) return
        element.dataset.memberFontBase = String(base)
        element.style.fontSize = `${Math.round(base * scale * 100) / 100}px`
      })
    }

    const roots = Array.from(document.querySelectorAll<HTMLElement>('.mobile-app, .game-room'))
    roots.forEach(apply)
    const observers = roots.map((root) => {
      const observer = new MutationObserver((records) => {
        records.forEach((record) => record.addedNodes.forEach((node) => {
          if (node instanceof HTMLElement) {
            if (node.matches(textSelector)) apply(node.parentElement ?? root)
            apply(node)
          }
        }))
      })
      observer.observe(root, { childList: true, subtree: true })
      return observer
    })
    return () => observers.forEach((observer) => observer.disconnect())
  }, [preference, viewKey])
}
