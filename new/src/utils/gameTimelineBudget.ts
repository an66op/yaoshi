/** A game-room visit keeps only a bounded display cache. This never changes
 * the durable chat history, bet records or the incremental server cursor. */
export const GAME_TIMELINE_LIMIT = 200

/** Input must already be deduplicated and ordered oldest to newest. */
export function recentGameTimelineItems<T>(items: T[]): T[] {
  return items.length > GAME_TIMELINE_LIMIT ? items.slice(-GAME_TIMELINE_LIMIT) : items
}
