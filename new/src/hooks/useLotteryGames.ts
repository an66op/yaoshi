import { useEffect, useMemo, useRef, useState } from 'react'
import { lotteryApi, type LotteryGame } from '../api/lottery'
import { WS_EVENT, type WsEvent, useWebSocketConnected } from './useWebSocket'
import { shouldReloadGameCatalog } from '../utils/gameCatalogEvents'
import { readServerClock, resolveLotteryBetting, resolveLotteryTiming, sampleServerClock, type ServerClockSample } from '../utils/lotteryTiming'
import { createRefreshLoop } from '../utils/refreshLoop'
import { gameCatalogRefreshDelay } from '../utils/gameCatalogRefresh'
import type { Game } from '../types'
import { gameRulesReady, rulesBlockedTiming, UNCONFIGURED_RULES_MESSAGE } from '../utils/lotteryRules'

export const gameLogoPaths: Partial<Record<string, string>> = {
  'speed-racing': '/images/game-logos/speed-racing.png',
  'speed-fly': '/images/game-logos/speed-fly.png',
  'speed-ssc': '/images/game-logos/speed-ssc.png',
  'sg-fly': '/images/game-logos/sg-fly.png',
  'sg-ssc': '/images/game-logos/sg-ssc.png',
  'fly-racing': '/images/game-logos/fly-racing.png',
  'au-lucky-5': '/images/game-logos/au-lucky-5.png',
  'au-lucky-10': '/images/game-logos/au-lucky-10.png',
  'pc-canada': '/images/game-logos/pc-canada.svg',
  'canada-28': '/images/game-logos/canada-28.png',
  'canada-20': '/images/game-logos/canada-20.svg',
  'bingo-mark-six': '/images/game-logos/bingo-mark-six.png',
  'bingo-racing-a': '/images/game-logos/bingo-racing.png',
  'bingo-racing-b': '/images/game-logos/bingo-racing.png',
  'bingo-ssc-1': '/images/game-logos/bingo-ssc-1.svg',
  'bingo-ssc-2': '/images/game-logos/bingo-ssc-2.svg',
  'bingo-ssc-3': '/images/game-logos/bingo-ssc-3.svg',
  'bingo-ssc-4': '/images/game-logos/bingo-ssc-4.svg',
  'hong-kong-mark-six': '/images/game-logos/hong-kong-mark-six.svg',
  'happy8-mark-six': '/images/game-logos/happy8-mark-six.png',
  'new-macau-mark-six': '/images/game-logos/new-macau-mark-six.svg',
  'old-macau-mark-six': '/images/game-logos/old-macau-mark-six.svg',
  'official-tw-bingo': '/images/game-logos/official-tw-bingo.svg',
  'official-tw-super-lotto': '/images/game-logos/official-tw-super-lotto.svg',
  'official-tw-daily539': '/images/game-logos/official-tw-daily539.svg',
  'official-tw-lotto649': '/images/game-logos/official-tw-lotto649.svg',
  'official-fc3d': '/images/game-logos/official-fc3d.svg',
  'official-kl8': '/images/game-logos/official-kl8.svg',
  'official-pl3': '/images/game-logos/official-pl3.svg',
  'official-qxc': '/images/game-logos/official-qxc.svg',
}

export function lotteryGameLogo(gameId?: string) {
  return gameId ? gameLogoPaths[gameId] : undefined
}

const resolvedBadgeColor = (item: LotteryGame) => {
  const color = item.badge_color?.trim().toLowerCase()
  // 白色徽标会与大厅卡片、六合彩号码底色融在一起；香港六合彩固定采用其红色主题。
  if (!color || color === 'white' || color === '#fff' || color === '#ffffff') {
    return item.id === 'hong-kong-mark-six' ? '#d64155' : '#3b83ec'
  }
  return item.badge_color
}

export const mapLotteryGame = (item: LotteryGame, nowMs: number): Game => {
  const rulesReady = gameRulesReady({ id: item.id, rulesReady: item.rules_ready, ruleVersion: item.rule_version })
  const resolvedTiming = resolveLotteryTiming(item, nowMs)
  const timing = rulesReady ? resolvedTiming : rulesBlockedTiming(resolvedTiming)
  return {
    id: item.id,
    title: item.name,
    tag: item.badge || item.code.toUpperCase(),
    category: item.category,
    lobbyCategory: item.lobby_category,
    online: item.bettor_count != null ? String(item.bettor_count) : '—',
    period: item.current_issue || '—',
    // The latest published draw is independent of the currently open period.
    latestIssue: item.issue || '—',
    due: timing.due,
    timing,
    betting: rulesReady ? resolveLotteryBetting(item, nowMs) : undefined,
    color: resolvedBadgeColor(item),
    logo: gameLogoPaths[item.id],
    // Missing draw data is an empty state. Zero-filled balls look like a real
    // result and would be recreated even after the database was reset.
    balls: item.latest_numbers?.length ? item.latest_numbers : [],
    issueStatus: item.issue_status || 'pending',
    sourceKind: item.source_kind || 'platform',
    sourceName: item.source_name || '王者开奖',
    sourceHealthy: item.source_healthy !== false,
    syncStatus: item.sync_status || 'idle',
    sourceError: item.last_sync_error || '',
    rulesReady,
    ruleVersion: item.rule_version || '',
    rulesMessage: rulesReady ? '' : item.rules_message || UNCONFIGURED_RULES_MESSAGE,
    lastSyncAt: item.last_sync_at || undefined,
  }
}

/** 从后端拉取彩种列表；接口异常时不伪造业务数据。 */
export function useLotteryGames(enabled = true, roomKey = '', activeGameId: string | null = null) {
  const realtimeConnected = useWebSocketConnected()
  const refreshContextRef = useRef({ realtimeConnected, activeGameId })
  refreshContextRef.current = { realtimeConnected, activeGameId }
  const catalogLoopRef = useRef<ReturnType<typeof createRefreshLoop<LotteryGame[]>> | null>(null)
  const [remote, setRemote] = useState<LotteryGame[] | null>(null)
  const [loadedRoomKey, setLoadedRoomKey] = useState('')
  const clockSampleRef = useRef<ServerClockSample | null>(null)
  const [serverNowMs, setServerNowMs] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let timer = 0
    const tick = () => {
      const now = readServerClock(clockSampleRef.current, performance.now())
      if (now !== null) setServerNowMs((current) => Math.max(current, now))
      // 对齐真实秒边界，避免 setInterval 累积漂移后出现停顿、跨秒跳动。
      timer = window.setTimeout(tick, now === null ? 250 : Math.max(25, 1020 - (now % 1000)))
    }
    tick()
    return () => window.clearTimeout(timer)
  }, [])

  useEffect(() => {
    if (!enabled) {
      setRemote(null)
      setLoadedRoomKey('')
      setLoading(false)
      setError('')
      return
    }
    setLoading(true)
    const catalogLoop = createRefreshLoop({
      request: lotteryApi.enabledGames,
      onData: (games) => {
        setRemote(games)
        setLoadedRoomKey(roomKey)
        setError('')
        setLoading(false)
      },
      onError: (reason) => {
        // Preserve the last confirmed room snapshot during transient failures.
        setError(reason instanceof Error ? reason.message : '网络连接失败，请稍后重试')
        setLoading(false)
      },
      delay: (games, failures) => gameCatalogRefreshDelay(games, readServerClock(clockSampleRef.current, performance.now()), refreshContextRef.current.realtimeConnected, refreshContextRef.current.activeGameId, failures),
    })
    catalogLoopRef.current = catalogLoop
    // Clock failures must not discard a successful new-period response. A
    // verified monotonic sample keeps running while clock calibration retries.
    const clockLoop = createRefreshLoop({
      request: async (signal) => {
        const sentAt = performance.now()
        const clock = await lotteryApi.clock(signal)
        const sample = sampleServerClock(clock.server_time_ms, sentAt, performance.now())
        if (!sample) throw new Error('服务器时间无效，请稍后重试')
        return sample
      },
      onData: (sample) => {
        clockSampleRef.current = sample
        setServerNowMs((current) => Math.max(current, sample.serverTimeMs))
        catalogLoop.reschedule()
      },
      onError: () => { /* timing stays unavailable until a valid first sample */ },
      delay: (_sample, failures) => failures ? Math.min(30_000, 5000 * 2 ** Math.min(failures - 1, 3)) : 60_000,
    })
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail && shouldReloadGameCatalog(detail, roomKey)) catalogLoop.refresh()
    }
    const onVisible = () => {
      if (document.visibilityState === 'hidden') {
        catalogLoop.pause()
        clockLoop.pause()
      } else {
        // Resume from the server immediately; never restart a local period.
        const now = readServerClock(clockSampleRef.current, performance.now())
        if (now !== null) setServerNowMs((current) => Math.max(current, now))
        catalogLoop.resume()
        clockLoop.resume()
      }
    }
    window.addEventListener(WS_EVENT, onWs)
    document.addEventListener('visibilitychange', onVisible)
    onVisible()
    return () => {
      window.removeEventListener(WS_EVENT, onWs)
      document.removeEventListener('visibilitychange', onVisible)
      catalogLoop.dispose()
      clockLoop.dispose()
      if (catalogLoopRef.current === catalogLoop) catalogLoopRef.current = null
    }
  }, [enabled, roomKey])

  useEffect(() => {
    // Reconnects and entering another game get a new snapshot without tearing
    // down an in-flight request or clearing the currently rendered room.
    catalogLoopRef.current?.refresh()
  }, [realtimeConnected, activeGameId])

  return useMemo(() => {
    // `remote !== null` means the request succeeded. Preserve an intentionally
    // 启用列表为空时保持空状态，不使用本地业务数据补位。
    if (remote !== null && loadedRoomKey === roomKey) {
      return { games: remote.map((item) => mapLotteryGame(item, serverNowMs)), loading, error, live: true }
    }
    return { games: [], loading, error, live: false }
  }, [remote, loadedRoomKey, roomKey, serverNowMs, loading, error])
}
