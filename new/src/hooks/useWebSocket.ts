import { useEffect, useRef } from 'react'
import { apiBase, getToken } from '../api/client'
import { memberApi } from '../api/member'

export type WsEvent = {
  type: string
  data: Record<string, unknown>
}

export const WS_EVENT = 'yaotu-ws'

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
        retry()
        return
      }
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
        retry()
      }
    }

    void connect()
    return () => {
      closed = true
      window.clearTimeout(retryTimer)
      ws?.close()
    }
  }, [enabled])
}
