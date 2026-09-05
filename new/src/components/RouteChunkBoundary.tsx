import { Component, Suspense, useEffect, type ErrorInfo, type ReactNode } from 'react'
import type { Theme } from '../types'
import { clearRouteChunkReloadMarker, tryReloadStaleRouteChunk } from '../utils/routeChunkRecovery'

type RouteChunkBoundaryProps = {
  children: ReactNode
  resetKey: string
  theme: Theme
}

type RouteChunkErrorBoundaryProps = Omit<RouteChunkBoundaryProps, 'resetKey'>

type RouteChunkBoundaryState = {
  failed: boolean
  reloading: boolean
}

function clearRouteRecoveryMarker() {
  try {
    clearRouteChunkReloadMarker(window.sessionStorage)
  } catch {
    // Storage can be disabled by the browser. A successful route needs no
    // recovery action, so there is nothing else to do.
  }
}

function RouteChunkReady() {
  useEffect(clearRouteRecoveryMarker, [])
  return null
}

function RouteLoading({ theme }: { theme: Theme }) {
  return (
    <div className={`session-check-notice route-loading theme-${theme}`} role="status" aria-live="polite">
      <span className="session-check-spinner" aria-hidden="true" />
      <p>页面加载中…</p>
    </div>
  )
}

class RouteChunkErrorBoundary extends Component<RouteChunkErrorBoundaryProps, RouteChunkBoundaryState> {
  state: RouteChunkBoundaryState = { failed: false, reloading: false }

  static getDerivedStateFromError(): RouteChunkBoundaryState {
    return { failed: true, reloading: false }
  }

  componentDidCatch(error: Error, _info: ErrorInfo) {
    const recovered = tryReloadStaleRouteChunk(error, {
      currentPath: `${window.location.pathname}${window.location.search}`,
      reload: () => window.location.reload(),
      storage: window.sessionStorage,
    })
    if (recovered) this.setState({ failed: true, reloading: true })
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <div className={`session-check-notice route-loading route-load-error theme-${this.props.theme}`} role="alert">
        <p>{this.state.reloading ? '检测到页面版本更新，正在重新加载…' : '页面资源加载失败，请重新加载后继续。'}</p>
        {!this.state.reloading && <button type="button" onClick={() => window.location.reload()}>重新加载</button>}
      </div>
    )
  }
}

export function RouteChunkBoundary({ children, resetKey, theme }: RouteChunkBoundaryProps) {
  return (
    <RouteChunkErrorBoundary key={resetKey} theme={theme}>
      <Suspense fallback={<RouteLoading theme={theme} />}>
        <RouteChunkReady />
        {children}
      </Suspense>
    </RouteChunkErrorBoundary>
  )
}
