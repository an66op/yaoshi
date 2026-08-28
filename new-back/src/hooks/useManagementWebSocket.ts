import { useEffect, useState } from 'react'
import { managementWebSocketURL, realtimeApi, type ManagementWsEvent } from '../api'

export const MANAGEMENT_WS_EVENT = 'wangzhe-management-ws'
export const MANAGEMENT_WS_STATUS_EVENT = 'wangzhe-management-ws-status'
export type ManagementWsStatus = { connected: boolean }

let managementWebSocketConnected = false
const recentManagementEvents = new Map<string, number>()

function duplicateManagementEvent(payload: ManagementWsEvent) {
  const now = Date.now()
  const keys: string[] = []
  if (payload.event_id) keys.push(`event:${payload.event_id}`)
  if (payload.type === 'chat_message' && payload.data?.message_id) {
    keys.push(`chat:${String(payload.data.operation || '')}:${String(payload.data.message_id)}:${String(payload.data.scope || '')}:${String(payload.data.room_scope || '')}:${String(payload.data.game_id || '')}`)
  }
  if (keys.some(key => recentManagementEvents.has(key))) return true
  keys.forEach(key => recentManagementEvents.set(key, now))
  if (recentManagementEvents.size > 256) {
    for (const [key, seenAt] of recentManagementEvents) {
      if (now - seenAt > 120_000 || recentManagementEvents.size > 256) recentManagementEvents.delete(key)
    }
  }
  return false
}

export function useManagementWebSocketConnected() {
  const [connected, setConnected] = useState(managementWebSocketConnected)
  useEffect(() => {
    const onStatus = (event: Event) => setConnected((event as CustomEvent<ManagementWsStatus>).detail.connected)
    window.addEventListener(MANAGEMENT_WS_STATUS_EVENT, onStatus)
    return () => window.removeEventListener(MANAGEMENT_WS_STATUS_EVENT, onStatus)
  }, [])
  return connected
}

export function useManagementWebSocket(role?: string, enabled = true) {
  useEffect(() => {
	if (!enabled || (role !== 'admin' && role !== 'agent' && role !== 'tenant')) return
    recentManagementEvents.clear()
    let socket: WebSocket | null = null
    let retryTimer = 0
    let closed = false

    const report = (connected: boolean) => {
      managementWebSocketConnected = connected
      window.dispatchEvent(new CustomEvent<ManagementWsStatus>(MANAGEMENT_WS_STATUS_EVENT, { detail: { connected } }))
    }
    const retry = () => {
      if (!closed) retryTimer = window.setTimeout(() => { void connect() }, 3000)
    }
    const connect = async () => {
	  if (closed) return
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
          if (duplicateManagementEvent(payload)) return
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
