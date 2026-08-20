import { useState } from 'react'
import { Icon } from '../components/Icon'
import { ActionDialog } from '../components/Dialogs'

type WalletAction = '上分申请' | '下分申请' | '收款方式' | '申请记录' | '娱乐额度' | '游戏记录' | '竞猜列表' | '娱乐记录' | '积分账变' | '福利报表' | '红包报表' | '邀请好友'

const actions: Array<{ icon: string; name: WalletAction; tone: string }> = [
  { icon: '/icons/duo/coin-stack.svg', name: '上分申请', tone: 'aqua' }, { icon: '/icons/duo/credit-card.svg', name: '下分申请', tone: 'coral' }, { icon: '/icons/duo/credit-card.svg', name: '收款方式', tone: 'gold' },
  { icon: '/icons/duo/clipboard.svg', name: '申请记录', tone: 'blue' }, { icon: '/icons/lucide/wallet.svg', name: '娱乐额度', tone: 'violet' }, { icon: '/icons/duo/clapperboard.svg', name: '游戏记录', tone: 'aqua' },
  { icon: '/icons/duo/chart-pie.svg', name: '竞猜列表', tone: 'blue' }, { icon: '/icons/duo/clock.svg', name: '娱乐记录', tone: 'violet' }, { icon: '/icons/duo/coin-stack.svg', name: '积分账变', tone: 'gold' },
  { icon: '/icons/duo/confetti.svg', name: '福利报表', tone: 'coral' }, { icon: '/icons/duo/discount.svg', name: '红包报表', tone: 'aqua' }, { icon: '/icons/lucide/gift.svg', name: '邀请好友', tone: 'blue' },
]

/** 钱包独立于“我的”：用于查看资产和发起各类前端演示操作。 */
export function Wallet({ points }: { points: number }) {
  const [activeAction, setActiveAction] = useState<WalletAction | null>(null)
  return <section className="wallet-page"><header className="wallet-header"><b>钱包中心</b><button aria-label="钱包说明" onClick={() => setActiveAction('娱乐额度')}><Icon name="more" /></button></header><section className="wallet-balance"><div><small>总资产</small><b>{points.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</b><span>积分账户 · 安全保障中</span></div><em>今日统计</em><footer><span>流水 <b>6,880.00</b></span><span>回水 <b>68.00</b></span><span>盈亏 <b>+128.00</b></span></footer></section><section className="wallet-primary-actions">{actions.slice(0, 3).map((action) => <button key={action.name} onClick={() => setActiveAction(action.name)}><span className={`wallet-action-icon ${action.tone}`}><i aria-hidden="true" style={{ maskImage: `url(${action.icon})`, WebkitMaskImage: `url(${action.icon})` }} /></span><b>{action.name}</b></button>)}</section><section className="wallet-tools"><header><b>钱包服务</b><small>所有数据仅用于前端演示</small></header><div>{actions.slice(3).map((action) => <button key={action.name} onClick={() => setActiveAction(action.name)}><span className={`wallet-action-icon ${action.tone}`}><i aria-hidden="true" style={{ maskImage: `url(${action.icon})`, WebkitMaskImage: `url(${action.icon})` }} /></span><b>{action.name}</b></button>)}</div></section>{activeAction && <ActionDialog title={activeAction} description={`${activeAction} 功能已准备就绪。目前为纯前端演示，提交后不会产生真实资金或账户变动。`} confirmLabel={activeAction.includes('申请') ? '模拟提交' : undefined} onClose={() => setActiveAction(null)} />}</section>
}
