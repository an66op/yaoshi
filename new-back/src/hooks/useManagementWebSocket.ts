import { useEffect } from 'react'
import { managementWebSocketURL, realtimeApi, type ManagementWsEvent } from '../api'
import { getToken } from '../auth'

export const MANAGEMENT_WS_EVENT = 'wangzhe-management-ws'
export const MANAGEMENT_WS_STATUS_EVENT = 'wangzhe-management-ws-status'

export function useManagementWebSocket(role?: string, enabled = true) {
  useEffect(() => {
    if (!enabled || !getToken() || (role !== 'admin' && role !== 'agent' && role !== 'tenant')) return
    let socket: WebSocket | null = null
    let retryTimer = 0
    let closed = false

    const report = (connected: boolean) => {
      window.dispatchEvent(new CustomEvent(MANAGEMENT_WS_STATUS_EVENT, { detail: { connected } }))
    }
    const retry = () => {
      if (!closed) retryTimer = window.setTimeout(() => { void connect() }, 3000)
    }
    const connect = async () => {
      if (closed || !getToken()) return
      try {
        const { ticket } = await realtimeApi.ticket(role)
        if (closed) return
        socket = new WebSocket(managementWebSocketURL(ticket))
      } catch {
        report(false)
        retry()
        return
      }
      socket.onopen = () => report(true)
      socket.onmessage = (event) => {
        try {
          const payload = JSON.parse(String(event.data)) as ManagementWsEvent
          window.dispatchEvent(new CustomEvent<ManagementWsEvent>(MANAGEMENT_WS_EVENT, { detail: payload }))
        } catch {
          // Ignore malformed frames and keep the live channel connected.
        }
      }
      socket.onclose = () => {
        report(false)
        retry()
      }
    }

    void connect()
    return () => {
      closed = true
      window.clearTimeout(retryTimer)
      socket?.close()
      report(false)
    }
  }, [enabled, role])
}
