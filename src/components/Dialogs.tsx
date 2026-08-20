import { Icon } from './Icon'
import { createPortal } from 'react-dom'

export function AnnouncementDialog({ onClose }: { onClose: () => void }) {
  return createPortal(<div className="modal-layer" role="presentation" onClick={onClose}><section className="notice-modal" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}><header><span>系统公告</span><button onClick={onClose}>×</button></header><article><small>08-16 10:00 · 平台公告</small><h2>本周系统维护安排与活动说明</h2><p>为了提升服务稳定性，系统将于本周日 02:00 至 03:30 进行例行维护。维护期间，部分页面可能暂时无法访问。</p><p>维护完成后，连续签到奖励将自动补发；如有疑问，请通过“我的 - 在线客服”联系工作人员。</p></article><footer><button onClick={onClose}>我知道了</button></footer></section></div>, document.body)
}

export function RedPacketDialog({ type, claimed, onOpen, onClose }: { type: 'daily' | 'lucky'; claimed: boolean; onOpen: () => void; onClose: () => void }) {
  const canOpen = type === 'lucky' && !claimed
  return createPortal(<div className="modal-layer red-layer" role="presentation" onClick={onClose}><section className="packet-modal" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}><button className="modal-close" onClick={onClose}>×</button><div className="packet-modal-top"><span>曜</span><b>曜图</b><small>{canOpen ? '给你发了一个奖励包' : '奖励已领取'}</small></div><div className="packet-seal"><Icon name="gift" /></div>{canOpen ? <><h2>好运相伴</h2><p>本期专属积分奖励，祝您一切顺利</p><button className="open-packet" onClick={onOpen}>开</button></> : <><h2>{type === 'daily' ? '18.00' : '8.00'} <small>积分</small></h2><p>奖励已存入积分账户</p><button className="packet-detail" onClick={onClose}>查看积分明细</button></>}</section></div>, document.body)
}

export function ActionDialog({ title, description, confirmLabel = '我知道了', onConfirm, onClose }: { title: string; description: string; confirmLabel?: string; onConfirm?: () => void; onClose: () => void }) {
  const confirm = () => {
    onConfirm?.()
    onClose()
  }
  return createPortal(<div className="modal-layer" role="presentation" onClick={onClose}><section className="notice-modal action-modal" role="dialog" aria-modal="true" aria-label={title} onClick={(event) => event.stopPropagation()}><header><span>{title}</span><button aria-label="关闭" onClick={onClose}>×</button></header><article><p>{description}</p></article><footer><button onClick={confirm}>{confirmLabel}</button></footer></section></div>, document.body)
}
