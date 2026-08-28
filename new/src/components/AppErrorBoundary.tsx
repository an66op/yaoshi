import { Component, type ErrorInfo, type ReactNode } from 'react'

type Props = { children: ReactNode }
type State = { error: Error | null }

export class AppErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State { return { error } }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('用户端页面渲染失败', error, info.componentStack)
  }

  render() {
    if (!this.state.error) return this.props.children
    return <main className="app-fatal-error" role="alert">
      <section>
        <b>页面暂时无法显示</b>
        <p>当前操作没有丢失，请重新加载后继续。</p>
        <button type="button" onClick={() => window.location.reload()}>重新加载</button>
      </section>
    </main>
  }
}
