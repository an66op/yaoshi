export type ChatScrollMetrics = {
  scrollHeight: number
  scrollTop: number
  clientHeight: number
}

export const CHAT_FOLLOW_DISTANCE = 96

/**
 * “自动跟随”和“显示最新按钮”使用不同阈值：
 * 轻微回弹或键盘开合不会立即出现按钮，真正翻阅历史后才提示返回最新。
 */
export function chatScrollState(metrics: ChatScrollMetrics) {
  const distance = Math.max(0, metrics.scrollHeight - metrics.scrollTop - metrics.clientHeight)
  const latestButtonDistance = Math.max(160, Math.min(260, metrics.clientHeight * 0.45))
  return {
    distance,
    following: distance <= CHAT_FOLLOW_DISTANCE,
    showLatest: distance >= latestButtonDistance,
  }
}
