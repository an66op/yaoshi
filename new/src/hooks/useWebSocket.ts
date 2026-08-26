import { useEffect, useRef, useState } from 'react'
import { apiBase, getToken } from '../api/client'
import { memberApi } from '../api/member'

export type WsEvent = {
  event_id?: string
  type: string
  room_scope?: string
  game_id?: string
  issue?: string
  server_at?: string
  data: Record<string, unknown>
}

export const WS_EVENT = 'yaotu-ws'
export const WS_STATUS_EVENT = 'yaotu-ws-status'

export type WsConnectionStatus = { connected: boolean }

let websocketConnected = false

/** 当前会员实时通道状态，供业务查询决定是否启用断线补拉。 */
export function useWebSocketConnected() {
  const [connected, setConnected] = useState(websocketConnected)
  useEffect(() => {
    const onStatus = (event: Event) => setConnected((event as CustomEvent<WsConnectionStatus>).detail.connected)
    window.addEventListener(WS_STATUS_EVENT, onStatus)
    return () => window.removeEventListener(WS_STATUS_EVENT, onStatus)
  }, [])
  return connected
}

function wsURL(ticket: string) {
  const base = apiBase.startsWith('https')
    ? apiBase.replace(/^https/i, 'wss')
    : apiBase.replace(/^http/i, 'ws')
  return `${base}/ws?ticket=${encodeURIComponent(ticket)}`
}

/** 会员 WebSocket：实时推送；断线后 3 秒重连。 */
export function useWebSocket(onEvent: (event: WsEvent) => void, enabled = true) {
  const handlerRef = useRef(onEvent)
  handlerRef.current = onEvent

  useEffect(() => {
    if (!enabled || !getToken()) return
    let ws: WebSocket | null = null
    let retryTimer = 0
    let closed = false

    const reportStatus = (connected: boolean) => {
      websocketConnected = connected
      window.dispatchEvent(new CustomEvent<WsConnectionStatus>(WS_STATUS_EVENT, { detail: { connected } }))
    }

    const retry = () => {
      if (!closed) retryTimer = window.setTimeout(() => { void connect() }, 3000)
    }
    const connect = async () => {
      if (closed || !getToken()) return
      try {
        const { ticket } = await memberApi.wsTicket()
        if (closed) return
        ws = new WebSocket(wsURL(ticket))
      } catch {
        reportStatus(false)
        retry()
        return
      }
      ws.onopen = () => reportStatus(true)
      ws.onmessage = (event) => {
        try {
          const payload = JSON.parse(String(event.data)) as WsEvent
          window.dispatchEvent(new CustomEvent(WS_EVENT, { detail: payload }))
          handlerRef.current(payload)
        } catch {
          /* ignore malformed frames */
        }
      }
      ws.onclose = () => {
        reportStatus(false)
        retry()
      }
    }

    void connect()
    return () => {
      closed = true
      window.clearTimeout(retryTimer)
      ws?.close()
      reportStatus(false)
    }
  }, [enabled])
}
