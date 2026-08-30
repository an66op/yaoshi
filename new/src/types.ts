import type { LotteryTiming } from './utils/lotteryTiming'

export type Tab = 'lobby' | 'shop' | 'chat' | 'profile'
export type Theme = 'day' | 'night'

export type Game = {
  id: string
  title: string
  tag: string
  category: string
  lobbyCategory: string
  online: string
  period: string
  latestIssue: string
  due: string
  timing: LotteryTiming
  color: string
  logo?: string
  balls: number[]
  issueStatus: string
  sourceKind: string
  sourceName: string
  sourceHealthy: boolean
  syncStatus: string
  sourceError: string
  lastSyncAt?: string
}

export type IconName = 'game' | 'shop' | 'chat' | 'user' | 'back' | 'bell' | 'plus' | 'more' | 'gift' | 'arrow' | 'switch' | 'room'
