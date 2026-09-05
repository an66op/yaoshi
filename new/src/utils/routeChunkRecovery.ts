const routeChunkReloadKey = 'seven-star-route-chunk-reload'

type RouteChunkRecoveryRuntime = {
  currentPath: string
  reload: () => void
  storage: Pick<Storage, 'getItem' | 'setItem'>
}

export function isRouteChunkLoadError(error: unknown) {
  if (!(error instanceof Error)) return false
  const text = `${error.name} ${error.message}`.toLowerCase()
  return text.includes('chunkloaderror') ||
    text.includes('loading chunk') ||
    text.includes('failed to fetch dynamically imported module') ||
    text.includes('error loading dynamically imported module') ||
    text.includes('importing a module script failed')
}

// A tab kept open across an atomic release may still reference chunk names
// from the previous asset manifest. Reload that route once, then stop and show
// a manual recovery control if the error is persistent or unrelated.
export function tryReloadStaleRouteChunk(error: unknown, runtime: RouteChunkRecoveryRuntime) {
  if (!isRouteChunkLoadError(error)) return false
  try {
    if (runtime.storage.getItem(routeChunkReloadKey) === runtime.currentPath) return false
    runtime.storage.setItem(routeChunkReloadKey, runtime.currentPath)
  } catch {
    return false
  }
  runtime.reload()
  return true
}

export function clearRouteChunkReloadMarker(storage: Pick<Storage, 'removeItem'>) {
  storage.removeItem(routeChunkReloadKey)
}
