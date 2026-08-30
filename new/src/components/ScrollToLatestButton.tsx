type Props = { keyboardOpen: boolean; onScrollToLatest: () => void }

export function ScrollToLatestButton({ keyboardOpen, onScrollToLatest }: Props) {
  return <button
    className={`scroll-latest-button${keyboardOpen ? ' keyboard-open' : ''}`}
    type="button"
    aria-label="回到最新消息"
    onClick={(event) => { event.stopPropagation(); onScrollToLatest() }}
  >
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M12 4v12m-4-4 4 4 4-4M5 20h14" />
    </svg>
  </button>
}
