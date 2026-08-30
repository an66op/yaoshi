type RefreshLoopOptions<T> = {
  request: (signal: AbortSignal) => Promise<T>
  onData: (data: T) => void
  onError: (reason: unknown) => void
  delay: (data: T | undefined, failures: number) => number
  timeoutMs?: number
}

/** One request at a time, with one trailing refresh for any in-flight events.
 * A late response is never allowed to update an unmounted/previous room. */
export function createRefreshLoop<T>(options: RefreshLoopOptions<T>) {
  let disposed = false
  let paused = false
  let queued = false
  let failures = 0
  let latest: T | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  let deadline: ReturnType<typeof setTimeout> | undefined
  let scheduledAt = Infinity
  let flight: AbortController | null = null

  const clearTimer = () => {
    if (timer !== undefined) clearTimeout(timer)
    timer = undefined
    scheduledAt = Infinity
  }
  const schedule = (delayMs: number) => {
    if (disposed || paused) return
    const delay = Math.max(0, Math.min(2_147_483_647, delayMs))
    if (timer !== undefined && scheduledAt <= Date.now() + delay) return
    clearTimer()
    scheduledAt = Date.now() + delay
    timer = setTimeout(() => { clearTimer(); void run() }, delay)
  }
  const run = async () => {
    if (disposed || paused) return
    if (flight) { queued = true; return }
    const controller = new AbortController()
    flight = controller
    deadline = setTimeout(() => controller.abort(), options.timeoutMs ?? 15_000)
    try {
      const data = await options.request(controller.signal)
      if (disposed) return
      latest = data
      failures = 0
      options.onData(data)
    } catch (reason) {
      if (disposed) return
      failures += 1
      options.onError(reason)
    } finally {
      if (deadline !== undefined) clearTimeout(deadline)
      deadline = undefined
      flight = null
      if (!disposed) {
        const followUp = queued
        queued = false
        // HTTP failures back off even if WebSocket events arrived meanwhile.
        schedule(followUp && failures === 0 ? 100 : options.delay(latest, failures))
      }
    }
  }

  return {
    refresh(immediate = false) {
      if (disposed || paused) return
      if (flight) { queued = true; return }
      // Burst events coalesce before starting a request, not just in flight.
      schedule(immediate ? 0 : failures ? options.delay(latest, failures) : 100)
    },
    reschedule() {
      if (!flight) schedule(options.delay(latest, failures))
    },
    pause() { paused = true; queued = false; clearTimer() },
    resume() { paused = false; this.refresh(true) },
    dispose() {
      disposed = true
      clearTimer()
      if (deadline !== undefined) clearTimeout(deadline)
      flight?.abort()
    },
  }
}
