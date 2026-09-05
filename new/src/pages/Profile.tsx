import { useEffect, useMemo, useRef, useState } from 'react'
import { BRAND_NAME } from '../data/brand'
import { createPortal } from 'react-dom'
import { Avatar } from '../components/Avatar'
import { Icon } from '../components/Icon'
import { avatarIndexFromSrc, avatarSrcForIndex, avatars } from '../data/avatars'
import { defaultNotificationSounds, notificationKinds, notificationSounds } from '../data/notificationSounds'
import type { NotificationKind } from '../data/notificationSounds'
import { isNotificationMuted, setNotificationMuted, startNotificationSound } from '../utils/notificationAudio'
import { memberApi } from '../api/member'
import { lotteryApi } from '../api/lottery'
import { useMemberPreferences, type BetModePreference, type FontScalePreference } from '../hooks/useMemberPreferences'
import { usePersistentState } from '../hooks/usePersistentState'
import { generateNickname } from '../data/nicknames'
import type { Theme } from '../types'
import type { GameGuideTab } from '../components/GameGuidePanel'
import { dispatchProfileNavigation, type ProfileNavigationTarget, type ProfileSheetPanel } from '../utils/profileNavigation'
import { passwordUTF8ByteLength } from '../utils/password'

type SettingRow = { icon: string; label: string; hint?: string; panel: ProfileNavigationTarget; tone: string }

type PreferenceSummary = { drawHistoryLimit: number; defaultBetMode: BetModePreference; fontScale: FontScalePreference }
type AvatarSelection = { index: number; src?: string }
export type ProfileProps = {
  account: string
  publicId?: number
  balance: number
  avatarUrl?: string
  publicTitle?: string
  badge?: string
  theme: Theme
  onLogout: () => Promise<void>
  logoutError?: string
  loggingOut?: boolean
  onResetDemo: () => void
  onToggleTheme: () => void
  onOpenGuide: (tab: GameGuideTab) => void
  onOpenService: () => void
  onPasswordChanged: () => Promise<void>
  onChangeNickname: (nickname: string) => Promise<void>
  onChangeAvatar: (avatar: string) => Promise<void>
}

const preferenceRowDefs: Array<Omit<SettingRow, 'hint'> & { hint?: (prefs: PreferenceSummary) => string }> = [
  { icon: '◷', label: '开奖统计期数', panel: 'history', tone: 'mint', hint: (p) => `最近 ${p.drawHistoryLimit} 期` },
  { icon: '▤', label: '默认投注面板', panel: 'betMode', tone: 'violet', hint: (p) => ({ chat: '聊天下注', detail: '详细网投' }[p.defaultBetMode]) },
  { icon: 'A', label: '字体大小', panel: 'fontSize', tone: 'blue', hint: (p) => ({ standard: '标准', large: '大一号', xlarge: '大两号' }[p.fontScale]) },
  { icon: '♪', label: '消息与通知声音', hint: () => '4 类提醒', panel: 'sounds', tone: 'coral' },
  { icon: '◉', label: '线路检测', hint: () => '连接良好', panel: 'line', tone: 'lime' },
]
const accountRowDefs: SettingRow[] = [
  { icon: '⚙', label: '账户与安全', panel: 'security', tone: 'blue' },
  { icon: '◐', label: '显示与主题', hint: '白天模式', panel: 'theme', tone: 'gold' },
  { icon: '?', label: '帮助与反馈', panel: 'help', tone: 'aqua' },
]
const gameRowDefs: SettingRow[] = [
  { icon: '书', label: '游戏玩法', hint: '规则与识别示例', panel: 'guideRules', tone: 'mint' },
  { icon: '赔', label: '赔率查看', hint: '当前房间实时赔率', panel: 'guideOdds', tone: 'gold' },
]

/** “我的”仅保留资料与设置；资产类服务全部收拢到钱包。 */
export function Profile({ account, publicId, balance, avatarUrl = '', publicTitle = '', badge = '', theme, onLogout, logoutError = '', loggingOut = false, onResetDemo, onToggleTheme, onOpenGuide, onOpenService, onPasswordChanged, onChangeNickname, onChangeAvatar }: ProfileProps) {
  const [panel, setPanel] = useState<ProfileSheetPanel | null>(null)
  const [avatar, setAvatar] = usePersistentState<AvatarSelection>('seven-star-avatar', { index: 0 })
  const { drawHistoryLimit, defaultBetMode, fontScale, displayStyle } = useMemberPreferences()
  const displayedAvatar = avatar.src?.trim() || avatarUrl.trim()
  const selectedAvatarIndex = avatarIndexFromSrc(displayedAvatar) ?? avatar.index
  const changeAvatar = async (index: number) => {
    const src = avatarSrcForIndex(index)
    // Keep an on-device selection even when an older backend cannot yet save
    // avatars. Once persistence succeeds, App refreshes the session profile.
    setAvatar({ index, src })
    await onChangeAvatar(src)
  }
  const preferenceRows = useMemo<SettingRow[]>(() => preferenceRowDefs.map((row) => ({
    icon: row.icon,
    label: row.label,
    panel: row.panel,
    tone: row.tone,
    hint: row.hint?.({ drawHistoryLimit, defaultBetMode, fontScale }),
  })), [drawHistoryLimit, defaultBetMode, fontScale])
  const accountRows = useMemo(() => accountRowDefs.map((row) => row.panel === 'theme' ? {
    ...row,
    hint: theme === 'night' ? '夜间模式' : displayStyle === 'simple' ? '简洁模式' : '白天模式',
  } : row), [displayStyle, theme])
  const selectProfileTarget = (target: ProfileNavigationTarget) => {
    dispatchProfileNavigation(target, onOpenGuide, setPanel)
  }
  return <section className="profile-simple-page"><header className="profile-simple-hero"><b>个人资料</b><button aria-label="打开显示设置" onClick={() => setPanel('theme')}><Icon name="more" /></button><section><button className="profile-simple-avatar" aria-label="修改头像" onClick={() => setPanel('avatar')}><Avatar index={selectedAvatarIndex} src={displayedAvatar} label="当前头像" /><i>编辑</i></button><div className="profile-name-block"><button className="profile-nickname-edit" aria-label="修改昵称" onClick={() => setPanel('nickname')}>修改</button><button className="profile-current-name" aria-label="修改昵称" onClick={() => setPanel('nickname')}><strong>{account}{badge && <i>{badge}</i>}</strong>{publicTitle && <em>{publicTitle}</em>}</button><small>{publicId ? `ID ${publicId} · ` : ''}余额 {balance.toFixed(2)} 元</small></div></section></header><ProfileGroup title="游戏服务" rows={gameRowDefs} onSelect={selectProfileTarget} /><ProfileGroup title="偏好设置" rows={preferenceRows} onSelect={selectProfileTarget} /><ProfileGroup title="账户设置" rows={accountRows} onSelect={selectProfileTarget} /><button className="profile-logout" disabled={loggingOut} onClick={() => void onLogout()}>{loggingOut ? '退出中…' : '退出登录'}</button>{logoutError && <p className="profile-logout-error" role="alert">{logoutError}</p>}<p className="profile-simple-version">{BRAND_NAME}</p>{panel && <ProfilePanel avatarIndex={selectedAvatarIndex} onAvatarChange={changeAvatar} account={account} onChangeNickname={onChangeNickname} onOpenService={onOpenService} onPasswordChanged={onPasswordChanged} panel={panel} theme={theme} onClose={() => setPanel(null)} onResetDemo={onResetDemo} onToggleTheme={onToggleTheme} />}</section>
}

function ProfileGroup({ title, rows, onSelect }: { title: string; rows: SettingRow[]; onSelect: (panel: ProfileNavigationTarget) => void }) {
  return <section className="profile-setting-group"><small>{title}</small><div>{rows.map((row) => <button key={row.label} onClick={() => onSelect(row.panel)}><span className={row.tone}>{row.icon}</span><b>{row.label}</b>{row.hint && <em>{row.hint}</em>}<Icon name="arrow" /></button>)}</div></section>
}

function ProfilePanel({ panel, theme, onClose, onResetDemo, onToggleTheme, avatarIndex, onAvatarChange, account, onChangeNickname, onOpenService, onPasswordChanged }: { panel: ProfileSheetPanel; theme: Theme; onClose: () => void; onResetDemo: () => void; onToggleTheme: () => void; avatarIndex: number; onAvatarChange: (index: number) => Promise<void>; account: string; onChangeNickname: (nickname: string) => Promise<void>; onOpenService: () => void; onPasswordChanged: () => Promise<void> }) {
  const info = panelInfo[panel]
  return createPortal(<div className={`profile-sheet-layer theme-${theme}`} role="presentation" onClick={onClose}><section className={`profile-sheet${panel === 'security' ? ' profile-internal-page' : ''}`} role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}><header><button aria-label="返回个人资料" onClick={onClose}><Icon name="back" /></button><b>{info.title}</b><button onClick={onClose}>完成</button></header>{panel === 'avatar' ? <AvatarSettings selected={avatarIndex} onSelect={onAvatarChange} /> : panel === 'nickname' ? <NicknameSettings current={account} onSave={onChangeNickname} onSaved={onClose} /> : panel === 'sounds' ? <SoundSettings /> : panel === 'theme' ? <ThemeSettings theme={theme} onResetDemo={onResetDemo} onToggleTheme={onToggleTheme} /> : panel === 'security' ? <SecuritySettings onPasswordChanged={onPasswordChanged} /> : panel === 'line' ? <LineSettings /> : panel === 'help' ? <HelpSettings onOpenService={onOpenService} /> : panel === 'history' ? <HistorySettings /> : panel === 'betMode' ? <BetModeSettings /> : panel === 'fontSize' ? <FontSizeSettings /> : <SimplePanel content={info} />}</section></div>, document.body)
}

const panelInfo: Record<ProfileSheetPanel, { title: string; summary: string; rows: Array<{ icon: string; title: string; detail: string; value?: string }> }> = {
  avatar: { title: '选择头像', summary: '', rows: [] },
  nickname: { title: '显示昵称', summary: '', rows: [] },
  sounds: { title: '消息与通知声音', summary: '', rows: [] },
  theme: { title: '显示与主题', summary: '', rows: [] },
  history: { title: '开奖统计期数', summary: '', rows: [] },
  betMode: { title: '默认投注面板', summary: '', rows: [] },
  fontSize: { title: '字体大小', summary: '', rows: [] },
  line: { title: '线路检测', summary: '当前线路连接稳定。', rows: [{ icon: '✓', title: '主线路', detail: '延迟 26 ms', value: '良好' }, { icon: '✓', title: '备用线路', detail: '延迟 41 ms', value: '可用' }] },
  security: { title: '账户与安全', summary: '', rows: [] },
  help: { title: '帮助与反馈', summary: '', rows: [] },
}

function SimplePanel({ content }: { content: (typeof panelInfo)[ProfileSheetPanel] }) {
  return <><p className="sheet-subtitle">{content.summary}</p><div className="sheet-list">{content.rows.map((row) => <article key={row.title}><span>{row.icon}</span><div><b>{row.title}</b><small>{row.detail}</small></div>{row.value && <em>{row.value}</em>}</article>)}</div></>
}

function AvatarSettings({ selected, onSelect }: { selected: number; onSelect: (index: number) => Promise<void> }) {
  const [saving, setSaving] = useState<number | null>(null)
  const [message, setMessage] = useState('')
  const select = async (index: number) => {
    if (saving !== null) return
    setSaving(index)
    setMessage('')
    try {
      await onSelect(index)
      setMessage('头像已同步')
    } catch (reason) {
      setMessage(`${reason instanceof Error ? reason.message : '头像同步失败'}，已保留本机头像`)
    } finally {
      setSaving(null)
    }
  }
  return <div className="avatar-settings"><p>选择一个头像，保存后将同时用于你的个人资料与会话展示。</p><div>{avatars.map((name, index) => <button aria-label={`选择${name}头像`} className={selected === index ? 'selected' : ''} disabled={saving !== null} key={name} onClick={() => void select(index)}><Avatar index={index} label={`${name}头像`} />{selected === index && <i>{saving === index ? '…' : '✓'}</i>}</button>)}</div>{message && <small className="avatar-feedback">{message}</small>}</div>
}

function NicknameSettings({ current, onSave, onSaved }: { current: string; onSave: (nickname: string) => Promise<void>; onSaved: () => void }) {
  const [value, setValue] = useState(current)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')
  const cleanValue = value.trim().replace(/\s+/g, ' ')
  const save = async () => {
    if (cleanValue.length < 2 || cleanValue.length > 16) return
    setSaving(true)
    setMessage('')
    try {
      await onSave(cleanValue)
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '昵称保存失败')
      setSaving(false)
      return
    }
    onSaved()
  }
  const randomize = () => {
    setValue(generateNickname())
    setMessage('')
  }
  return <div className="nickname-settings"><p>输入你想使用的显示昵称，保存后会同步更新个人资料与会话名称。</p><label><span>当前显示昵称</span><div><input maxLength={16} value={value} onChange={(event) => setValue(event.target.value)} placeholder="请输入 2–16 个字符" /><button type="button" aria-label="换一个随机昵称" title="随机昵称" onClick={randomize}>↻</button></div><small>{cleanValue.length}/16</small></label><button className="nickname-save" disabled={saving || cleanValue.length < 2 || cleanValue.length > 16} onClick={() => void save()}>{saving ? '保存中…' : '保存'}</button>{message && <small className="nickname-feedback">{message}</small>}</div>
}

function ThemeSettings({ theme, onResetDemo, onToggleTheme }: { theme: Theme; onResetDemo: () => void; onToggleTheme: () => void }) {
  const { displayStyle, setDisplayStyle } = useMemberPreferences()
  const activeMode = theme === 'night' ? 'night' : displayStyle === 'simple' ? 'simple' : 'day'
  const selectDay = () => {
    setDisplayStyle('scenic')
    if (theme === 'night') onToggleTheme()
  }
  const selectSimple = () => {
    setDisplayStyle('simple')
    if (theme === 'night') onToggleTheme()
  }
  const selectNight = () => {
    if (theme === 'day') onToggleTheme()
  }
  const reset = () => {
    setDisplayStyle('scenic')
    onResetDemo()
  }
  return <div className="theme-setting"><section><span className="theme-preview day-preview">☀</span><div><b>白天模式</b><small>晴空背景与清晰卡片</small></div><button className={activeMode === 'day' ? 'selected' : ''} onClick={selectDay}>{activeMode === 'day' ? '使用中' : '使用'}</button></section><section><span className="theme-preview simple-preview">◇</span><div><b>简洁模式</b><small>隐藏背景图片，使用纯净底色</small></div><button className={activeMode === 'simple' ? 'selected' : ''} onClick={selectSimple}>{activeMode === 'simple' ? '使用中' : '使用'}</button></section><section><span className="theme-preview night-preview">☾</span><div><b>夜间模式</b><small>深海背景与沉浸界面</small></div><button className={activeMode === 'night' ? 'selected' : ''} onClick={selectNight}>{activeMode === 'night' ? '使用中' : '使用'}</button></section><button className="demo-reset" onClick={reset}>恢复默认偏好</button></div>
}

function FontSizeSettings() {
  const { fontScale, setFontScale } = useMemberPreferences()
  const options: Array<{ id: FontScalePreference; label: string; helper: string }> = [
    { id: 'standard', label: '标准', helper: '默认显示大小' },
    { id: 'large', label: '大一号', helper: '阅读更舒适' },
    { id: 'xlarge', label: '大两号', helper: '适合小屏或长时间阅读' },
  ]
  return <div className="theme-setting font-size-setting"><p className="sheet-subtitle">仅调整界面阅读大小，不影响下注金额、号码与开奖结果。</p>{options.map((item) => <section key={item.id}><span className={`theme-preview font-size-preview ${item.id}`}>Aa</span><div><b>{item.label}</b><small>{item.helper}</small></div><button className={fontScale === item.id ? 'selected' : ''} onClick={() => setFontScale(item.id)}>{fontScale === item.id ? '使用中' : '使用'}</button></section>)}</div>
}

export function SecuritySettings({ onPasswordChanged }: { onPasswordChanged: () => Promise<void> }) {
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [message, setMessage] = useState('')
  const [loading, setLoading] = useState(false)
  const passwordBytes = passwordUTF8ByteLength(newPassword)
  const passwordValid = passwordBytes >= 8 && passwordBytes <= 72
  const passwordsMatch = newPassword === confirmPassword
  const submit = async () => {
    if (!oldPassword || !passwordValid || !passwordsMatch || oldPassword === newPassword) return
    setLoading(true)
    setMessage('')
    try {
      await memberApi.changePassword(oldPassword, newPassword)
      setOldPassword('')
      setNewPassword('')
      setConfirmPassword('')
      await onPasswordChanged()
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '修改失败')
      setLoading(false)
    }
  }
  return (
    <div className="theme-setting">
      <p className="sheet-subtitle">修改登录密码</p>
      <div className="security-fields"><label>原密码<input type="password" autoComplete="current-password" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} /></label><label>新密码<input type="password" autoComplete="new-password" maxLength={72} value={newPassword} onChange={(e) => setNewPassword(e.target.value)} /><small>{passwordBytes}/72 字节 · 密码需为 8–72 个 UTF-8 字节，中文通常占 3 字节</small></label><label>确认新密码<input type="password" autoComplete="new-password" maxLength={72} value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} />{confirmPassword && !passwordsMatch && <small className="security-field-error">两次输入的新密码不一致</small>}</label></div>
      <button className="demo-reset security-submit" disabled={loading || !oldPassword || !passwordValid || !passwordsMatch || oldPassword === newPassword} onClick={() => void submit()}>{loading ? '提交中…' : '保存新密码'}</button>
      {message && <p className="security-message">{message}</p>}
    </div>
  )
}

export function HelpSettings({ onOpenService }: { onOpenService: () => void }) {
  const supportEmail = String(import.meta.env.VITE_SUPPORT_EMAIL ?? '').trim()
  return (
    <div className="theme-setting help-settings">
      <p className="sheet-subtitle">联系方式</p>
      <section className="help-setting-block"><div><b>在线客服</b><small>站内客服会话</small></div><button className="help-contact-link" type="button" onClick={onOpenService}>进入客服</button></section>
      {supportEmail && <section className="help-setting-block"><div><b>客服邮箱</b><small>{supportEmail}</small></div><a className="help-contact-link" href={`mailto:${supportEmail}`}>发邮件</a></section>}
    </div>
  )
}

export function HistorySettings() {
  const { drawHistoryLimit, setDrawHistoryLimit } = useMemberPreferences()
  const options = [20, 50, 100]
  return (
    <div className="theme-setting">
      <p className="sheet-subtitle">控制房间读取的开奖查询范围，供开奖记录图片、长龙及走势统计使用；不会改变聊天消息或时间线的保留条数。</p>
      {options.map((value) => (
        <section key={value}>
          <span className="theme-preview day-preview">{value}</span>
          <div><b>最近 {value} 期</b><small>{value === drawHistoryLimit ? '当前使用中' : '点击切换'}</small></div>
          <button className={drawHistoryLimit === value ? 'selected' : ''} onClick={() => setDrawHistoryLimit(value)}>{drawHistoryLimit === value ? '使用中' : '使用'}</button>
        </section>
      ))}
    </div>
  )
}

export function BetModeSettings() {
  const { defaultBetMode, setDefaultBetMode } = useMemberPreferences()
  const options: Array<{ id: BetModePreference; label: string; helper: string }> = [
    { id: 'chat', label: '聊天下注', helper: '进入彩种时先打开聊天室' },
    { id: 'detail', label: '详细网投', helper: '进入彩种时先打开详细投注面板' },
  ]
  return (
    <div className="theme-setting">
      <p className="sheet-subtitle">进入彩种时默认打开的投注方式；若该彩种只支持一种方式，会自动进入可用页面。</p>
      {options.map((item) => (
        <section key={item.id}>
          <span className="theme-preview day-preview">▤</span>
          <div><b>{item.label}</b><small>{item.helper}</small></div>
          <button className={defaultBetMode === item.id ? 'selected' : ''} onClick={() => setDefaultBetMode(item.id)}>{defaultBetMode === item.id ? '使用中' : '使用'}</button>
        </section>
      ))}
    </div>
  )
}

function LineSettings() {
  const [latency, setLatency] = useState<number | null>(null)
  const [checking, setChecking] = useState(false)
  const check = async () => {
    setChecking(true)
    const start = performance.now()
    try {
      await lotteryApi.clock()
      setLatency(Math.round(performance.now() - start))
    } catch {
      setLatency(null)
    } finally {
      setChecking(false)
    }
  }
  useEffect(() => { void check() }, [])
  const label = latency == null ? '不可用' : latency <= 80 ? '良好' : latency <= 200 ? '可用' : '较慢'
  return (
    <div className="theme-setting">
      <p className="sheet-subtitle">检测当前设备的网络连接质量。</p>
      <section><span className="theme-preview day-preview">✓</span><div><b>主线路</b><small>{checking ? '检测中…' : latency != null ? `延迟 ${latency} ms` : '连接失败'}</small></div><em>{label}</em></section>
      <button className="demo-reset" disabled={checking} onClick={() => void check()}>{checking ? '检测中…' : '重新检测'}</button>
    </div>
  )
}

function SoundSettings() {
  const [activeKind, setActiveKind] = useState<NotificationKind>('lottery')
  const [selectedSounds, setSelectedSounds] = useState<Record<NotificationKind, string>>(() => {
    try {
      const stored = { ...defaultNotificationSounds, ...JSON.parse(window.localStorage.getItem('seven-star-notification-sounds') ?? '{}') } as Record<NotificationKind, string>
      for (const kind of notificationKinds) {
        if (!notificationSounds.some((sound) => sound.id === stored[kind.id])) stored[kind.id] = defaultNotificationSounds[kind.id]
      }
      return stored
    } catch { return defaultNotificationSounds }
  })
  const [muted, setMuted] = useState(() => isNotificationMuted())
  const [playing, setPlaying] = useState<string | null>(null)
  const stopPreviewRef = useRef<(() => void) | null>(null)
  const previewTimerRef = useRef<number | null>(null)
  useEffect(() => () => { stopPreviewRef.current?.(); if (previewTimerRef.current) window.clearTimeout(previewTimerRef.current) }, [])
  const preview = (soundId: string) => {
    const sound = notificationSounds.find((item) => item.id === soundId)
    if (!sound) return
    if (playing === soundId) { stopPreviewRef.current?.(); return setPlaying(null) }
    stopPreviewRef.current?.()
    if (previewTimerRef.current) window.clearTimeout(previewTimerRef.current)
    const playback = startNotificationSound(sound, 0.78)
    if (!playback) return
    stopPreviewRef.current = playback.stop
    setPlaying(soundId)
    previewTimerRef.current = window.setTimeout(() => setPlaying(null), playback.durationMs)
  }
  const choose = (soundId: string) => {
    const next = { ...selectedSounds, [activeKind]: soundId }
    setSelectedSounds(next)
    window.localStorage.setItem('seven-star-notification-sounds', JSON.stringify(next))
  }
  const toggleMuted = () => { const next = !muted; setMuted(next); setNotificationMuted(next); if (next) { stopPreviewRef.current?.(); setPlaying(null) } }
  return <div className="sound-settings sound-settings-v2"><header className="sound-master"><div><b>通知声音</b><small>{muted ? '已静音，不会播放任何提醒音' : '已开启，开奖会即时播放提醒'}</small></div><button className={muted ? 'muted' : 'enabled'} aria-pressed={!muted} onClick={toggleMuted}>{muted ? '静音中' : '声音已开'}</button></header><div className="sound-browser"><aside className="sound-category-list">{notificationKinds.map((kind) => { const current = notificationSounds.find((sound) => sound.id === selectedSounds[kind.id]); return <button className={`sound-category ${activeKind === kind.id ? 'active' : ''}`} key={kind.id} onClick={() => setActiveKind(kind.id)}><span>{kind.id === 'lottery' ? '开' : kind.id === 'message' ? '消' : kind.id === 'reward' ? '奖' : '告'}</span><div><b>{kind.label.replace('通知', '')}</b><small>{current?.name ?? '未选择'}</small></div></button> })}</aside><section className="sound-library-panel"><div className="sound-library-title"><b>{notificationKinds.find((kind) => kind.id === activeKind)?.label}</b><small>{notificationSounds.length} 首 · 点右侧试听</small></div><div className="sound-library">{notificationSounds.map((sound) => <div className="sound-row" key={sound.id}><button className={`sound-select ${selectedSounds[activeKind] === sound.id ? 'selected' : ''}`} onClick={() => choose(sound.id)}><span>♪</span><div><b>{sound.name}</b><small>{sound.description}</small></div></button><button className="sound-preview" aria-label={`试听${sound.name}`} onClick={() => preview(sound.id)}>{playing === sound.id ? '■' : '▶'}</button></div>)}</div></section></div><p className="sound-note">声音仅保存在本设备。浏览器需先完成一次点击，才允许自动播放开奖提醒。</p></div>
}
