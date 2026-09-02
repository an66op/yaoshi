import type { Tab, Theme } from '../types'
import { controlSurfaceProps } from '../utils/controlSurface'

const items: Array<{ id: Tab; label: string; dayIconSrc: string; nightIconSrc: string }> = [
  { id: 'lobby', label: '游戏大厅', dayIconSrc: '/images/nav-day-lobby-3d.png', nightIconSrc: '/images/nav-lobby-3d.png' },
  { id: 'shop', label: '钱包', dayIconSrc: '/images/nav-day-shop-3d.png', nightIconSrc: '/images/nav-shop-3d.png' },
  { id: 'chat', label: '聊天', dayIconSrc: '/images/nav-day-chat-3d.png', nightIconSrc: '/images/nav-chat-3d.png' },
  { id: 'profile', label: '我的', dayIconSrc: '/images/nav-day-profile-3d.png', nightIconSrc: '/images/nav-profile-3d.png' },
]

export function BottomNav({ activeTab, theme, unreadCount, onSelect }: { activeTab: Tab; theme: Theme; unreadCount: number; onSelect: (tab: Tab) => void }) {
  return <nav className="bottom-nav" {...controlSurfaceProps}>
    {items.map((item) => <button className={activeTab === item.id ? 'nav-active' : ''} key={item.id} onClick={() => onSelect(item.id)}><span className="nav-icon"><img src={theme === 'day' ? item.dayIconSrc : item.nightIconSrc} alt="" draggable={false} />{item.id === 'chat' && unreadCount > 0 && <i>{unreadCount}</i>}</span><b>{item.label}</b></button>)}
  </nav>
}
