type Effect = () => void | (() => void)
type Slot = { value?: unknown; dependencies?: readonly unknown[]; effect?: Effect; cleanup?: void | (() => void) }

/** Small effect/state driver for Node-only hook tests. It deliberately does not
 * simulate a browser renderer; callers attach test refs before flushing effects. */
export class HookHarness {
  private slots: Slot[] = []
  private cursor = 0
  private pendingEffects = new Set<number>()

  render<T>(render: () => T): T {
    this.cursor = 0
    return render()
  }

  useState<T>(initial: T | (() => T)): [T, (next: T | ((previous: T) => T)) => void] {
    const index = this.cursor++
    const slot = this.slots[index] ??= { value: typeof initial === 'function' ? (initial as () => T)() : initial }
    return [slot.value as T, (next) => {
      slot.value = typeof next === 'function' ? (next as (previous: T) => T)(slot.value as T) : next
    }]
  }

  useRef<T>(initial: T) {
    const slot = this.slots[this.cursor++] ??= { value: { current: initial } }
    return slot.value as { current: T }
  }

  useMemo<T>(factory: () => T, dependencies: readonly unknown[]): T {
    const index = this.cursor++
    const previous = this.slots[index]
    if (!previous || !this.sameDependencies(previous.dependencies, dependencies)) {
      this.slots[index] = { value: factory(), dependencies }
    }
    return this.slots[index].value as T
  }

  useEffect(effect: Effect, dependencies?: readonly unknown[]) {
    const index = this.cursor++
    const previous = this.slots[index]
    if (!previous || !this.sameDependencies(previous.dependencies, dependencies)) {
      this.slots[index] = { ...previous, effect, dependencies }
      this.pendingEffects.add(index)
    }
  }

  flushEffects() {
    const pending = [...this.pendingEffects]
    this.pendingEffects.clear()
    for (const index of pending) {
      const slot = this.slots[index]
      slot.cleanup?.()
      slot.cleanup = slot.effect?.()
    }
  }

  unmount() {
    this.pendingEffects.clear()
    for (const slot of this.slots) slot.cleanup?.()
  }

  private sameDependencies(previous?: readonly unknown[], next?: readonly unknown[]) {
    return previous !== undefined && next !== undefined && previous.length === next.length
      && previous.every((value, index) => Object.is(value, next[index]))
  }
}
