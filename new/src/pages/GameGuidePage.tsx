import { GameGuidePanel, type GameGuideTab } from '../components/GameGuidePanel'
import { Icon } from '../components/Icon'
import type { Game } from '../types'
import './game-guide-page.css'

export type GameGuidePageProps = {
  games: Game[]
  initialTab?: GameGuideTab
  onBack: () => void
}

/** A routed page for the complete game manual and effective member odds. */
export function GameGuidePage({ games, initialTab = 'rules', onBack }: GameGuidePageProps) {
  return <section className="game-guide-page" aria-label="游戏玩法">
    <header className="game-guide-page-header">
      <button type="button" aria-label="返回我的" onClick={onBack}><Icon name="back" /></button>
      <div><b>游戏玩法</b><small>玩法说明与当前赔率</small></div>
      <span aria-hidden="true" />
    </header>
    <main className="game-guide-page-content">
      <GameGuidePanel key={initialTab} games={games} initialTab={initialTab} />
    </main>
  </section>
}
