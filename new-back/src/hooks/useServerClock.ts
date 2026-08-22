import { useCallback, useEffect, useState } from 'react'
import { adminApi } from '../api'

type ClockState = {
  now: number
  offset: number
  latency: number
  synced: boolean
}

// Calibrates browser time against the backend using the midpoint of the HTTP
// round trip. The local clock only drives animation between calibrations.
export function useServerClock() {
  const [clock, setClock] = useState<ClockState>({ now: 0, offset: 0, latency: 0, synced: false })
  const calibrate = useCallback(async () => {
    const sentAt = Date.now()
    const response = await adminApi.clock()
    const receivedAt = Date.now()
    const offset = response.server_time_ms - (sentAt + receivedAt) / 2
    setClock({ now: receivedAt + offset, offset, latency: receivedAt - sentAt, synced: true })
  }, [])

  useEffect(() => {
    const kickoff = window.setTimeout(() => void calibrate().catch(() => undefined), 0)
    const tick = window.setInterval(() => setClock(current => ({ ...current, now: Date.now() + current.offset })), 250)
    const resync = window.setInterval(() => void calibrate().catch(() => undefined), 60_000)
    return () => { window.clearTimeout(kickoff); window.clearInterval(tick); window.clearInterval(resync) }
  }, [calibrate])

  return { ...clock, calibrate }
}
