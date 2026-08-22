export type Tab = 'lobby' | 'shop' | 'chat' | 'profile'
export type Theme = 'day' | 'night'

export type Game = {
  id: string
  title: string
  tag: string
  online: string
  period: string
  due: string
  color: string
  balls: number[]
}

export type IconName = 'game' | 'shop' | 'chat' | 'user' | 'back' | 'bell' | 'plus' | 'more' | 'gift' | 'arrow'
