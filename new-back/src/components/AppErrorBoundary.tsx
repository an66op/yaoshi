import { Component, type ErrorInfo, type ReactNode } from 'react'

type Props = { children: ReactNode }
type State = { error: Error | null }

export class AppErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State { return { error } }

  componentDidCatch(error: Error, info: ErrorInfo) {
    if (import.meta.env.DEV) console.error('管理端页面渲染失败', error, info.componentStack)
  }

  render() {
    if (!this.state.error) return this.props.children
    return <main className="app-fatal-error" role="alert">
      <section>
        <b>页面暂时无法显示</b>
        <p>数据没有丢失，请重新加载管理中心后继续。</p>
        <button type="button" onClick={() => window.location.reload()}>重新加载</button>
      </section>
    </main>
  }
}
