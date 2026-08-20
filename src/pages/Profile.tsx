import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Avatar } from '../components/Avatar'
import { Icon } from '../components/Icon'
import { avatars } from '../data/avatars'
import { defaultNotificationSounds, notificationKinds, notificationSounds } from '../data/notificationSounds'
import type { NotificationKind } from '../data/notificationSounds'
import { usePersistentState } from '../hooks/usePersistentState'
import type { Theme } from '../types'

type Panel = 'avatar' | 'security' | 'history' | 'betMode' | 'sounds' | 'line' | 'theme' | 'help' | null
type SettingRow = { icon: string; label: string; hint?: string; panel: Exclude<Panel, null>; tone: string }

const preferenceRows: SettingRow[] = [
  { icon: '◷', label: '聊天室历史开奖期数', hint: '最近 50 期', panel: 'history', tone: 'mint' },
  { icon: '▤', label: '投注模式', hint: '快捷输入', panel: 'betMode', tone: 'violet' },
  { icon: '♪', label: '消息与通知声音', hint: '4 类提醒', panel: 'sounds', tone: 'coral' },
  { icon: '◉', label: '线路检测', hint: '连接良好', panel: 'line', tone: 'lime' },
]
const accountRows: SettingRow[] = [
  { icon: '⚙', label: '账户与安全', hint: '已认证', panel: 'security', tone: 'blue' },
  { icon: '◐', label: '显示与主题', hint: '白天模式', panel: 'theme', tone: 'gold' },
  { icon: '?', label: '帮助与反馈', panel: 'help', tone: 'aqua' },
]

/** “我的”仅保留资料与设置；资产类服务全部收拢到钱包。 */
export function Profile({ account, theme, onLogout, onResetDemo, onToggleTheme }: { account: string; theme: Theme; onLogout: () => void; onResetDemo: () => void; onToggleTheme: () => void }) {
  const [panel, setPanel] = useState<Panel>(null)
  const [avatar, setAvatar] = usePersistentState('seven-star-avatar', { index: 0 })
  return <section className="profile-simple-page"><header className="profile-simple-hero"><b>个人资料</b><button aria-label="打开显示设置" onClick={() => setPanel('theme')}><Icon name="more" /></button><section><button className="profile-simple-avatar" aria-label="修改头像" onClick={() => setPanel('avatar')}><Avatar index={avatar.index} label="当前头像" /><i>编辑</i></button><div><strong>{account}</strong><small>帐号 {account} · 已实名认证</small></div></section></header><ProfileGroup title="偏好设置" rows={preferenceRows} onSelect={setPanel} /><ProfileGroup title="账户设置" rows={accountRows} onSelect={setPanel} /><button className="profile-logout" onClick={onLogout}>退出登录</button><p className="profile-simple-version">曜图 · 安全服务已开启</p>{panel && <ProfilePanel avatarIndex={avatar.index} onAvatarChange={(index) => setAvatar({ index })} panel={panel} theme={theme} onClose={() => setPanel(null)} onResetDemo={onResetDemo} onToggleTheme={onToggleTheme} />}</section>
}

function ProfileGroup({ title, rows, onSelect }: { title: string; rows: SettingRow[]; onSelect: (panel: Panel) => void }) {
  return <section className="profile-setting-group"><small>{title}</small><div>{rows.map((row) => <button key={row.label} onClick={() => onSelect(row.panel)}><span className={row.tone}>{row.icon}</span><b>{row.label}</b>{row.hint && <em>{row.hint}</em>}<Icon name="arrow" /></button>)}</div></section>
}

function ProfilePanel({ panel, theme, onClose, onResetDemo, onToggleTheme, avatarIndex, onAvatarChange }: { panel: Exclude<Panel, null>; theme: Theme; onClose: () => void; onResetDemo: () => void; onToggleTheme: () => void; avatarIndex: number; onAvatarChange: (index: number) => void }) {
  const info = panelInfo[panel]
  return createPortal(<div className="profile-sheet-layer" role="presentation" onClick={onClose}><section className="profile-sheet" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}><header><button onClick={onClose}><Icon name="back" /></button><b>{info.title}</b><button onClick={onClose}>完成</button></header>{panel === 'avatar' ? <AvatarSettings selected={avatarIndex} onSelect={onAvatarChange} /> : panel === 'sounds' ? <SoundSettings /> : panel === 'theme' ? <ThemeSettings theme={theme} onResetDemo={onResetDemo} onToggleTheme={onToggleTheme} /> : <SimplePanel content={info} />}</section></div>, document.body)
}

const panelInfo: Record<Exclude<Panel, null>, { title: string; summary: string; rows: Array<{ icon: string; title: string; detail: string; value?: string }> }> = {
  avatar: { title: '选择头像', summary: '', rows: [] },
  sounds: { title: '消息与通知声音', summary: '', rows: [] },
  theme: { title: '显示与主题', summary: '', rows: [] },
  history: { title: '聊天室历史开奖期数', summary: '默认展示最近 50 期开奖记录，可随时调整。', rows: [{ icon: '20', title: '最近 20 期', detail: '节省聊天内容空间' }, { icon: '50', title: '最近 50 期', detail: '当前使用中', value: '已选择' }, { icon: '100', title: '最近 100 期', detail: '保留更多开奖信息' }] },
  betMode: { title: '投注模式', summary: '当前房间使用可自由组合的快捷输入模式。', rows: [{ icon: '⌨', title: '快捷输入', detail: '数字与玩法可重复点击', value: '使用中' }, { icon: '▦', title: '两面盘', detail: '进入彩种页后可切换使用' }, { icon: '1~10', title: '号码面板', detail: '快速连续输入号码' }] },
  line: { title: '线路检测', summary: '当前线路连接稳定。', rows: [{ icon: '✓', title: '主线路', detail: '延迟 26 ms', value: '良好' }, { icon: '✓', title: '备用线路', detail: '延迟 41 ms', value: '可用' }] },
  security: { title: '账户与安全', summary: '请妥善保管帐号和房间号。', rows: [{ icon: '✓', title: '实名认证', detail: '帐号认证状态正常', value: '已认证' }, { icon: '✓', title: '登录设备保护', detail: '当前设备安全', value: '已开启' }, { icon: '△', title: '修改登录密码', detail: '建议定期更新密码' }] },
  help: { title: '帮助与反馈', summary: '遇到问题可联系在线客服。', rows: [{ icon: '七', title: '专属客服小七', detail: '全天候在线支持', value: '在线' }, { icon: '!', title: '提交问题反馈', detail: '描述遇到的问题，我们会尽快处理' }] },
}

function SimplePanel({ content }: { content: (typeof panelInfo)[Exclude<Panel, null>] }) {
  return <><p className="sheet-subtitle">{content.summary}</p><div className="sheet-list">{content.rows.map((row) => <article key={row.title}><span>{row.icon}</span><div><b>{row.title}</b><small>{row.detail}</small></div>{row.value && <em>{row.value}</em>}</article>)}</div></>
}

function AvatarSettings({ selected, onSelect }: { selected: number; onSelect: (index: number) => void }) {
  return <div className="avatar-settings"><p>选择一个头像，保存后将同时用于你的个人资料与会话展示。</p><div>{avatars.map((name, index) => <button aria-label={`选择${name}头像`} className={selected === index ? 'selected' : ''} key={name} onClick={() => onSelect(index)}><Avatar index={index} label={`${name}头像`} />{selected === index && <i>✓</i>}</button>)}</div></div>
}

function ThemeSettings({ theme, onResetDemo, onToggleTheme }: { theme: Theme; onResetDemo: () => void; onToggleTheme: () => void }) {
  return <div className="theme-setting"><section><span className="theme-preview day-preview">☀</span><div><b>白天模式</b><small>晴空背景与清晰卡片</small></div><button className={theme === 'day' ? 'selected' : ''} onClick={() => theme === 'night' && onToggleTheme()}>{theme === 'day' ? '使用中' : '使用'}</button></section><section><span className="theme-preview night-preview">☾</span><div><b>夜间模式</b><small>深海背景与沉浸界面</small></div><button className={theme === 'night' ? 'selected' : ''} onClick={() => theme === 'day' && onToggleTheme()}>{theme === 'night' ? '使用中' : '使用'}</button></section><button className="demo-reset" onClick={onResetDemo}>重置前端演示数据</button></div>
}

function SoundSettings() {
  const [activeKind, setActiveKind] = useState<NotificationKind>('lottery')
  const [selectedSounds, setSelectedSounds] = useState<Record<NotificationKind, string>>(() => {
    try { return { ...defaultNotificationSounds, ...JSON.parse(window.localStorage.getItem('seven-star-notification-sounds') ?? '{}') } } catch { return defaultNotificationSounds }
  })
  const [playing, setPlaying] = useState<string | null>(null)
  const audioRef = useRef<HTMLAudioElement | null>(null)
  useEffect(() => () => audioRef.current?.pause(), [])
  const preview = (soundId: string) => {
    const sound = notificationSounds.find((item) => item.id === soundId)
    if (!sound) return
    if (playing === soundId) { audioRef.current?.pause(); return setPlaying(null) }
    audioRef.current?.pause()
    const audio = new Audio(sound.src)
    audioRef.current = audio
    audio.onended = () => setPlaying(null)
    audio.play().then(() => setPlaying(soundId)).catch(() => setPlaying(null))
  }
  const choose = (soundId: string) => {
    const next = { ...selectedSounds, [activeKind]: soundId }
    setSelectedSounds(next)
    window.localStorage.setItem('seven-star-notification-sounds', JSON.stringify(next))
  }
  return <div className="sound-settings"><div className="sound-category-list">{notificationKinds.map((kind) => { const current = notificationSounds.find((sound) => sound.id === selectedSounds[kind.id]); return <button className={`sound-category ${activeKind === kind.id ? 'active' : ''}`} key={kind.id} onClick={() => setActiveKind(kind.id)}><span>{kind.id === 'lottery' ? '开奖号' : kind.id === 'message' ? '消息' : kind.id === 'reward' ? '奖励' : '公告'}</span><div><b>{kind.label}</b><small>{kind.description}</small></div><em>{current?.name}</em></button> })}</div><div className="sound-library-title"><b>铃声库</b><small>选择后自动保存 · 点右侧试听</small></div><div className="sound-library">{notificationSounds.map((sound) => <div className="sound-row" key={sound.id}><button className={`sound-select ${selectedSounds[activeKind] === sound.id ? 'selected' : ''}`} onClick={() => choose(sound.id)}><span>♪</span><div><b>{sound.name}</b><small>{sound.description}</small></div></button><button className="sound-preview" aria-label={`试听${sound.name}`} onClick={() => preview(sound.id)}>{playing === sound.id ? '■' : '▶'}</button></div>)}</div><p className="sound-note">声音仅在本设备保存。页面需要完成一次点击后，浏览器才允许播放试听音效。</p></div>
}
