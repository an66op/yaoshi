import { useEffect, useMemo, useState } from 'react'
import { lotteryApi, type LotteryGame } from '../api/lottery'
import { WS_EVENT, type WsEvent, useWebSocketConnected } from './useWebSocket'
import { shouldReloadGameCatalog } from '../utils/gameCatalogEvents'
import type { Game } from '../types'
import { SPEED_RACING_TRIO_SRC } from '../data/gameArtwork'

export const gameLogoPaths: Partial<Record<string, string>> = {
  'speed-racing': SPEED_RACING_TRIO_SRC,
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

const toClock = (seconds: number) => {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const resolvedBadgeColor = (item: LotteryGame) => {
  const color = item.badge_color?.trim().toLowerCase()
  // 白色徽标会与大厅卡片、六合彩号码底色融在一起；香港六合彩固定采用其红色主题。
  if (!color || color === 'white' || color === '#fff' || color === '#ffffff') {
    return item.id === 'hong-kong-mark-six' ? '#d64155' : '#3b83ec'
  }
  return item.badge_color
}

const mapGame = (item: LotteryGame, nowMs: number): Game => {
  const drawMs = new Date(item.next_draw_at).getTime()
  const sealMs = item.seal_at ? new Date(item.seal_at).getTime() : Number.NaN
  // “截止倒计时”应以封盘时间为准；旧数据没有封盘时间时才退回开奖时间。
  const deadlineMs = Number.isFinite(sealMs) ? sealMs : drawMs
  const remaining = Math.max(0, Math.ceil((deadlineMs - nowMs) / 1000))
  return {
    id: item.id,
    title: item.name,
    tag: item.badge || item.code.toUpperCase(),
    category: item.category,
    lobbyCategory: item.lobby_category,
    online: item.bettor_count != null ? String(item.bettor_count) : '—',
    period: item.current_issue || item.issue || '—',
    due: toClock(remaining),
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
    lastSyncAt: item.last_sync_at || undefined,
  }
}

/** 从后端拉取彩种列表；接口异常时不伪造业务数据。 */
export function useLotteryGames(enabled = true, roomKey = '') {
  const realtimeConnected = useWebSocketConnected()
  const [remote, setRemote] = useState<LotteryGame[] | null>(null)
  const [loadedRoomKey, setLoadedRoomKey] = useState('')
  const [serverOffsetMs, setServerOffsetMs] = useState(0)
  const [clientNowMs, setClientNowMs] = useState(() => Date.now())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let timer = 0
    const tick = () => {
      const now = Date.now()
      setClientNowMs(now)
      // 对齐真实秒边界，避免 setInterval 累积漂移后出现停顿、跨秒跳动。
      timer = window.setTimeout(tick, Math.max(80, 1020 - (now % 1000)))
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
    let cancelled = false
    let loadSequence = 0
    const load = async () => {
      const requestSequence = ++loadSequence
      try {
        const [games, clock] = await Promise.all([lotteryApi.enabledGames(), lotteryApi.clock()])
        if (cancelled || requestSequence !== loadSequence) return
        setRemote(games)
        setLoadedRoomKey(roomKey)
        setServerOffsetMs(clock.server_time_ms - Date.now())
        setError('')
      } catch (reason) {
        if (!cancelled && requestSequence === loadSequence) {
          // 实时开奖后会立即补拉一次目录。弱网或瞬时超时不能清空上一份
          // 已成功的数据，否则正在打开的游戏页会被卸载，开奖/中奖弹窗也
          // 会随之消失。首次加载失败时 remote 本来就是 null，仍会正常
          // 呈现错误状态；已有数据时则保留页面并等待下一次恢复。
          setError(reason instanceof Error ? reason.message : '网络连接失败，请稍后重试')
        }
      } finally {
        if (!cancelled && requestSequence === loadSequence) setLoading(false)
      }
    }
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail
      if (detail && shouldReloadGameCatalog(detail, roomKey)) void load()
    }
    void load()
    window.addEventListener(WS_EVENT, onWs)
    // WebSocket 是主链路；只有实时连接中断时才用 HTTP 做恢复性补拉。
    const recovery = realtimeConnected ? 0 : window.setInterval(() => void load(), 10_000)
    return () => {
      cancelled = true
      window.removeEventListener(WS_EVENT, onWs)
      if (recovery) window.clearInterval(recovery)
    }
  }, [enabled, realtimeConnected, roomKey])

  return useMemo(() => {
    const nowMs = clientNowMs + serverOffsetMs
    // `remote !== null` means the request succeeded. Preserve an intentionally
    // 启用列表为空时保持空状态，不使用本地业务数据补位。
    if (remote !== null && loadedRoomKey === roomKey) {
      return { games: remote.map((item) => mapGame(item, nowMs)), loading, error, live: true }
    }
    return { games: [], loading, error, live: false }
  }, [remote, loadedRoomKey, roomKey, serverOffsetMs, clientNowMs, loading, error])
}
