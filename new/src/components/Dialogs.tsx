import { Icon } from './Icon'
import { createPortal } from 'react-dom'
import type { ReactNode } from 'react'

export function AnnouncementDialog({ title, body, onClose }: { title?: string; body?: string; onClose: () => void }) {
  const heading = title ?? '本周系统维护安排与活动说明'
  const content = body ?? '为了提升服务稳定性，系统将进行例行维护。维护期间，部分页面可能暂时无法访问。如有疑问，请通过「聊天 - 在线客服」联系工作人员。'
  return createPortal(<div className="modal-layer" role="presentation" onClick={onClose}><section className="notice-modal" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}><header><span>系统公告</span><button onClick={onClose}>×</button></header><article><small>平台公告 · 来自后台配置</small><h2>{heading}</h2><p>{content}</p></article><footer><button onClick={onClose}>我知道了</button></footer></section></div>, document.body)
}

export function RedPacketDialog({ type, claimed, onOpen, onClose }: { type: 'daily' | 'lucky'; claimed: boolean; onOpen: () => void; onClose: () => void }) {
  const canOpen = type === 'lucky' && !claimed
  return createPortal(<div className="modal-layer red-layer" role="presentation" onClick={onClose}><section className="packet-modal" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}><button className="modal-close" onClick={onClose}>×</button><div className="packet-modal-top"><b>曜图 · 奖励包</b><small>{canOpen ? '给你发了一个奖励包' : '奖励已领取'}</small></div><div className="packet-gift"><Icon name="gift" /></div>{canOpen ? <><h2>好运相伴</h2><p>本期专属积分奖励，祝您一切顺利</p><button className="open-packet" onClick={onOpen}>开</button></> : <><h2>{type === 'daily' ? '18.00' : '8.00'} <small>积分</small></h2><p>奖励已存入积分账户</p><button className="packet-detail" onClick={onClose}>查看积分明细</button></>}</section></div>, document.body)
}

export function ActionDialog({ title, description, confirmLabel = '我知道了', onConfirm, onClose, children }: { title: string; description: string; confirmLabel?: string; onConfirm?: () => void; onClose: () => void; children?: ReactNode }) {
  const confirm = () => {
    onConfirm?.()
    onClose()
  }
  return createPortal(<div className="modal-layer" role="presentation" onClick={onClose}><section className="notice-modal action-modal" role="dialog" aria-modal="true" aria-label={title} onClick={(event) => event.stopPropagation()}><header><span>{title}</span><button aria-label="关闭" onClick={onClose}>×</button></header><article><p>{description}</p>{children}</article><footer><button onClick={confirm}>{confirmLabel}</button></footer></section></div>, document.body)
}
