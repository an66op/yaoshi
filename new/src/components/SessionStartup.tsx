import { useEffect, useState } from 'react'
import { BRAND_NAME } from '../data/brand'
import type { Theme } from '../types'

/** A visible wait state, not an authenticated view or a cached account preview. */
export function SessionCheckNotice({ className = '' }: { className?: string }) {
  const [slow, setSlow] = useState(false)
  useEffect(() => {
    const timer = window.setTimeout(() => setSlow(true), 4000)
    return () => window.clearTimeout(timer)
  }, [])

  return (
    <div className={`session-check-notice ${className}`} role="status" aria-live="polite">
      <span className="session-check-spinner" aria-hidden="true" />
      <p>{slow ? '连接较慢，请稍候…' : '正在确认登录状态…'}</p>
    </div>
  )
}

type Props = {
  theme: Theme
  error?: string
  onRetry?: () => void
  onLogout?: () => void
  loggingOut?: boolean
}

export function SessionStartup({ theme, error, onRetry, onLogout, loggingOut = false }: Props) {
  return (
    <main className={`login-page session-startup theme-${theme}`}>
      <section className="login-card session-startup-card" aria-busy={!error}>
        <header className="login-brand">
          <img alt="" src="/images/king-racing-mark.jpg" width="50" height="50" />
          <div><b>{BRAND_NAME}</b><small>安全连接</small></div>
        </header>
        {error ? (
          <>
            <div className="login-copy session-startup-copy">
              <h1>暂时无法连接</h1>
              <p role="alert">{error}</p>
            </div>
            <div className="session-startup-actions">
              <button className="login-primary" disabled={loggingOut} onClick={onRetry}>重新连接</button>
              {onLogout && <button className="room-entry-back" disabled={loggingOut} onClick={onLogout}>{loggingOut ? '退出中…' : '退出登录'}</button>}
            </div>
          </>
        ) : <SessionCheckNotice className="session-startup-notice" />}
      </section>
    </main>
  )
}
