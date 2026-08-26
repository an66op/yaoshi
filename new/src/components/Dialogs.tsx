import { Icon } from './Icon'
import { createPortal } from 'react-dom'
import type { ReactNode } from 'react'
import { useEffect, useState } from 'react'
import { BRAND_NAME } from '../data/brand'
import type { AnnouncementItem } from '../api/portal'

export function AnnouncementDialog({ items, onClose }: { items: AnnouncementItem[]; onClose: () => void }) {
  const [index, setIndex] = useState(0)
  useEffect(() => setIndex(current => Math.min(current, Math.max(0, items.length - 1))), [items.length])
  const current = items[index]
  if (!current) return null
  const multiple = items.length > 1
  return createPortal(<div className="modal-layer" role="presentation" onClick={onClose}><section className="notice-modal multi-notice-modal" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}><header><span>大厅公告</span>{multiple && <em>{index + 1} / {items.length}</em>}<button aria-label="关闭公告" onClick={onClose}>×</button></header><article><small>IMPORTANT NOTICE</small><h2>{current.title}</h2><p>{current.content}</p></article><footer className={multiple ? 'notice-footer-multiple' : ''}>{multiple && <button className="notice-secondary" disabled={index === 0} onClick={() => setIndex(currentIndex => Math.max(0, currentIndex - 1))}>上一条</button>}<button onClick={() => index < items.length - 1 ? setIndex(currentIndex => currentIndex + 1) : onClose()}>{index < items.length - 1 ? '下一条' : '我知道了'}</button></footer></section></div>, document.body)
}

export function RedPacketDialog({ type, claimed, reward, greeting = '恭喜发财', cover = 'classic', minTurnover = 0, opening = false, error = '', onOpen, onClose }: { type: 'daily' | 'lucky'; claimed: boolean; reward?: number | null; greeting?: string; cover?: string; minTurnover?: number; opening?: boolean; error?: string; onOpen: () => void; onClose: () => void }) {
  const canOpen = type === 'lucky' && !claimed
  const amount = reward != null ? reward.toFixed(2) : type === 'daily' ? '18.00' : '--'
  return createPortal(<div className="modal-layer red-layer" role="presentation" onClick={() => !opening && onClose()}><section className={`packet-modal packet-cover-${cover} ${canOpen ? '' : 'packet-result-modal'}`} role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}><button aria-label="关闭红包" className="modal-close" disabled={opening} onClick={onClose}>×</button><div className="packet-modal-top"><b>{BRAND_NAME} · 红包</b><small>{canOpen ? '给你发了一个红包' : '领取成功'}</small></div><div className="packet-gift"><Icon name="gift" /></div>{canOpen ? <><h2>{greeting}</h2><p>{opening ? '正在开启红包…' : minTurnover > 0 ? `今日有效流水满 ${minTurnover.toFixed(2)} 可领取` : '点击领取本次红包'}</p>{error && <div className="packet-error" role="alert">{error}</div>}<button aria-label={opening ? '正在开启红包' : '开启红包'} className={`open-packet ${opening ? 'opening' : ''}`} disabled={opening} onClick={onOpen}>{opening ? <span /> : '开'}</button></> : <div className="packet-result"><small>本次领取</small><h2><strong>{amount}</strong><em>积分</em></h2><p>已存入积分账户</p></div>}</section></div>, document.body)
}

export function ActionDialog({ title, description, confirmLabel = '我知道了', onConfirm, onClose, children }: { title: string; description: string; confirmLabel?: string; onConfirm?: () => void; onClose: () => void; children?: ReactNode }) {
  const confirm = () => {
    onConfirm?.()
    onClose()
  }
  return createPortal(<div className="modal-layer" role="presentation" onClick={onClose}><section className="notice-modal action-modal" role="dialog" aria-modal="true" aria-label={title} onClick={(event) => event.stopPropagation()}><header><span>{title}</span><button aria-label="关闭" onClick={onClose}>×</button></header><article><p>{description}</p>{children}</article><footer><button onClick={confirm}>{confirmLabel}</button></footer></section></div>, document.body)
}
