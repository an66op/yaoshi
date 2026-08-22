import { useEffect, useState } from 'react'
import { Icon } from '../components/Icon'
import { ActionDialog } from '../components/Dialogs'
import { betsApi, type MemberBet } from '../api/bets'
import { memberApi, type BalanceRecord, type EntertainmentLaunch, type EntertainmentPlatform, type InviteInfo, type MemberApplication, type RebatePreview, type WalletChannel, type WalletSummary } from '../api/member'

type WalletAction = '上分申请' | '下分申请' | '收款方式' | '申请记录' | '娱乐额度' | '游戏记录' | '竞猜列表' | '娱乐记录' | '积分账变' | '福利报表' | '红包报表' | '邀请好友'

const actions: Array<{ icon: string; name: WalletAction; tone: string }> = [
  { icon: '/icons/duo/coin-stack.svg', name: '上分申请', tone: 'aqua' }, { icon: '/icons/duo/credit-card.svg', name: '下分申请', tone: 'coral' }, { icon: '/icons/duo/credit-card.svg', name: '收款方式', tone: 'gold' },
  { icon: '/icons/duo/clipboard.svg', name: '申请记录', tone: 'blue' }, { icon: '/icons/lucide/wallet.svg', name: '娱乐额度', tone: 'violet' }, { icon: '/icons/duo/clapperboard.svg', name: '游戏记录', tone: 'aqua' },
  { icon: '/icons/duo/chart-pie.svg', name: '竞猜列表', tone: 'blue' }, { icon: '/icons/duo/clock.svg', name: '娱乐记录', tone: 'violet' }, { icon: '/icons/duo/coin-stack.svg', name: '积分账变', tone: 'gold' },
  { icon: '/icons/duo/confetti.svg', name: '福利报表', tone: 'coral' }, { icon: '/icons/duo/discount.svg', name: '红包报表', tone: 'aqua' }, { icon: '/icons/lucide/gift.svg', name: '邀请好友', tone: 'blue' },
]

const wiredActions = new Set<WalletAction>(['上分申请', '下分申请', '申请记录', '积分账变', '收款方式', '娱乐额度', '游戏记录', '竞猜列表', '娱乐记录', '福利报表', '红包报表', '邀请好友'])

const betStatusLabel: Record<string, string> = {
  pending: '待开奖', won: '已中奖', lost: '未中奖', cancelled: '已撤销',
}

/** 钱包：余额、上下分申请、账变与注单接后端 */
export function Wallet({ balance, onRefresh }: { balance: number; onRefresh?: () => void }) {
  const [activeAction, setActiveAction] = useState<WalletAction | null>(null)
  const [applications, setApplications] = useState<MemberApplication[]>([])
  const [history, setHistory] = useState<BalanceRecord[]>([])
  const [channels, setChannels] = useState<WalletChannel[]>([])
  const [bets, setBets] = useState<MemberBet[]>([])
  const [summary, setSummary] = useState<WalletSummary | null>(null)
  const [rebate, setRebate] = useState<RebatePreview | null>(null)
  const [platforms, setPlatforms] = useState<EntertainmentPlatform[]>([])
  const [launchInfo, setLaunchInfo] = useState<EntertainmentLaunch | null>(null)
  const [launchingCode, setLaunchingCode] = useState<string | null>(null)
  const [invite, setInvite] = useState<InviteInfo | null>(null)
  const [amount, setAmount] = useState('100')
  const [remark, setRemark] = useState('')
  const [message, setMessage] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    void memberApi.walletSummary().then(setSummary).catch(() => setSummary(null))
  }, [balance])

  useEffect(() => {
    if (!activeAction) return
    void (async () => {
      try {
        if (activeAction === '申请记录') {
          setApplications((await memberApi.applications()).items)
        } else if (activeAction === '积分账变') {
          setHistory(await memberApi.balanceHistory())
        } else if (activeAction === '收款方式') {
          setChannels(await memberApi.walletChannels())
        } else if (activeAction === '游戏记录') {
          setBets((await betsApi.list({ page_size: 30 })).items)
        } else if (activeAction === '竞猜列表') {
          setBets((await betsApi.list({ status: 'pending', page_size: 30 })).items)
        } else if (activeAction === '娱乐额度') {
          setSummary(await memberApi.walletSummary())
        } else if (activeAction === '福利报表') {
          const rows = await memberApi.balanceHistory(50)
          setHistory(rows.filter((item) => item.type.includes('checkin') || item.type.includes('activity') || item.type.includes('invite')))
          setRebate(await memberApi.rebatePreview())
        } else if (activeAction === '红包报表') {
          setHistory((await memberApi.balanceHistory(50)).filter((item) => item.type.includes('redpacket')))
        } else if (activeAction === '娱乐记录') {
          setPlatforms(await memberApi.entertainment())
        } else if (activeAction === '邀请好友') {
          setInvite(await memberApi.inviteInfo())
        }
      } catch {
        /* ignore */
      }
    })()
  }, [activeAction])

  const launchPlatform = async (code: string) => {
    setLaunchingCode(code)
    setLaunchInfo(null)
    try {
      const result = await memberApi.launchEntertainment(code)
      setLaunchInfo(result)
      if (result.launch_url) {
        window.open(result.launch_url, '_blank', 'noopener,noreferrer')
      }
    } catch (reason) {
      setLaunchInfo({ code, name: code, status: 'disabled', message: reason instanceof Error ? reason.message : '进入失败', ready: false })
    } finally {
      setLaunchingCode(null)
    }
  }

  const submitApplication = async (request_type: 'credit' | 'debit') => {
    setLoading(true)
    setMessage('')
    try {
      await memberApi.createApplication({ request_type, amount: Number(amount), remark })
      setMessage('申请已提交，请等待审核')
      onRefresh?.()
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '提交失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <section className="wallet-page">
      <header className="wallet-header"><b>钱包中心</b><button aria-label="刷新余额" onClick={onRefresh}><Icon name="more" /></button></header>
      <section className="wallet-balance"><div><small>账户余额</small><b>{balance.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</b><span>元 · 来自后端账户</span></div><em>今日统计</em><footer><span>流水 <b>{summary ? summary.today_turnover.toFixed(2) : '—'}</b></span><span>回水 <b>{summary ? summary.today_rebate.toFixed(2) : '—'}</b></span><span>盈亏 <b>{summary ? summary.today_profit.toFixed(2) : '—'}</b></span></footer></section>
      <section className="wallet-primary-actions">{actions.slice(0, 3).map((action) => <button key={action.name} onClick={() => setActiveAction(action.name)}><span className={`wallet-action-icon ${action.tone}`}><i aria-hidden="true" style={{ maskImage: `url(${action.icon})`, WebkitMaskImage: `url(${action.icon})` }} /></span><b>{action.name}</b></button>)}</section>
      <section className="wallet-tools"><header><b>钱包服务</b><small>注单与账变已接后端</small></header><div>{actions.slice(3).map((action) => <button key={action.name} onClick={() => setActiveAction(action.name)}><span className={`wallet-action-icon ${action.tone}`}><i aria-hidden="true" style={{ maskImage: `url(${action.icon})`, WebkitMaskImage: `url(${action.icon})` }} /></span><b>{action.name}</b></button>)}</div></section>
      {activeAction === '上分申请' && (
        <ActionDialog title="上分申请" description="提交后由后台审核并入账。" confirmLabel={loading ? '提交中…' : '提交申请'} onConfirm={() => void submitApplication('credit')} onClose={() => setActiveAction(null)}>
          <label style={{ display: 'block', marginTop: 12 }}>金额<input style={{ width: '100%', marginTop: 4 }} inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} /></label>
          <label style={{ display: 'block', marginTop: 8 }}>备注<input style={{ width: '100%', marginTop: 4 }} value={remark} onChange={(e) => setRemark(e.target.value)} /></label>
          {message && <p style={{ marginTop: 8 }}>{message}</p>}
        </ActionDialog>
      )}
      {activeAction === '下分申请' && (
        <ActionDialog title="下分申请" description="提交后由后台审核并扣款。" confirmLabel={loading ? '提交中…' : '提交申请'} onConfirm={() => void submitApplication('debit')} onClose={() => setActiveAction(null)}>
          <label style={{ display: 'block', marginTop: 12 }}>金额<input style={{ width: '100%', marginTop: 4 }} inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} /></label>
          <label style={{ display: 'block', marginTop: 8 }}>备注<input style={{ width: '100%', marginTop: 4 }} value={remark} onChange={(e) => setRemark(e.target.value)} /></label>
          {message && <p style={{ marginTop: 8 }}>{message}</p>}
        </ActionDialog>
      )}
      {activeAction === '申请记录' && (
        <ActionDialog title="申请记录" description={applications.length ? `共 ${applications.length} 条记录` : '暂无申请记录'} onClose={() => setActiveAction(null)}>
          {applications.map((item) => <div key={item.id} style={{ padding: '6px 0', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>{item.request_type === 'credit' ? '上分' : '下分'} {item.requested_amount} · {item.status}</div>)}
        </ActionDialog>
      )}
      {activeAction === '积分账变' && (
        <ActionDialog title="积分账变" description={history.length ? `最近 ${history.length} 条账变` : '暂无账变记录'} onClose={() => setActiveAction(null)}>
          {history.map((item) => <div key={item.id} style={{ padding: '6px 0', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>{item.type} {item.amount >= 0 ? '+' : ''}{item.amount.toFixed(2)} → {item.after.toFixed(2)}<small style={{ display: 'block' }}>{item.remark}</small></div>)}
        </ActionDialog>
      )}
      {activeAction === '收款方式' && (
        <ActionDialog title="收款方式" description={channels.length ? '后台配置的收款渠道' : '暂无可用渠道'} onClose={() => setActiveAction(null)}>
          {channels.map((ch) => <div key={ch.id} style={{ padding: '6px 0' }}><b>{ch.name}</b> · {ch.credit_type} · {ch.min_amount}-{ch.max_amount}</div>)}
        </ActionDialog>
      )}
      {activeAction === '娱乐额度' && (
        <ActionDialog title="娱乐额度" description="当前待结算注单占用额度" onClose={() => setActiveAction(null)}>
          <div style={{ marginTop: 12 }}>待结算金额：<b>{summary?.pending_amount.toFixed(2) ?? '—'}</b> 元</div>
          <div style={{ marginTop: 8 }}>待结算注单：<b>{summary?.pending_count ?? '—'}</b> 条</div>
          <div style={{ marginTop: 8 }}>历史注单总数：<b>{summary?.total_bet_count ?? '—'}</b> 条</div>
        </ActionDialog>
      )}
      {(activeAction === '游戏记录' || activeAction === '竞猜列表') && (
        <ActionDialog title={activeAction} description={bets.length ? `最近 ${bets.length} 条注单` : '暂无注单'} onClose={() => setActiveAction(null)}>
          {bets.map((bet) => (
            <div key={bet.id} style={{ padding: '8px 0', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
              <b>{bet.play_name || bet.selection}</b> · {bet.amount} 元 · {betStatusLabel[bet.status] ?? bet.status}
              <small style={{ display: 'block' }}>{bet.game_id} 第 {bet.issue} 期</small>
            </div>
          ))}
        </ActionDialog>
      )}
      {activeAction === '福利报表' && (
        <ActionDialog title="福利报表" description="签到奖励与今日回水预估" onClose={() => setActiveAction(null)}>
          {rebate && (
            <div style={{ marginTop: 12, paddingBottom: 8, borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
              今日有效流水 {rebate.today_turnover.toFixed(2)} · 回水比例 {rebate.rate_percent}%<br />
              预估回水 {rebate.estimated.toFixed(2)} · 已到账 {rebate.credited.toFixed(2)} · 待入账 {rebate.pending_credit.toFixed(2)}
            </div>
          )}
          {history.map((item) => <div key={item.id} style={{ padding: '6px 0', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>{item.type} +{item.amount.toFixed(2)}<small style={{ display: 'block' }}>{item.remark}</small></div>)}
          {!history.length && !rebate && <p>暂无福利记录</p>}
        </ActionDialog>
      )}
      {activeAction === '红包报表' && (
        <ActionDialog title="红包报表" description={history.length ? `最近 ${history.length} 条红包账变` : '暂无红包记录'} onClose={() => setActiveAction(null)}>
          {history.map((item) => <div key={item.id} style={{ padding: '6px 0', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>红包 +{item.amount.toFixed(2)}<small style={{ display: 'block' }}>{item.remark}</small></div>)}
        </ActionDialog>
      )}
      {activeAction === '娱乐记录' && (
        <ActionDialog title="娱乐记录" description={platforms.length ? '后台配置的娱乐平台' : '暂无可用平台'} onClose={() => { setActiveAction(null); setLaunchInfo(null) }}>
          {platforms.map((item) => (
            <div key={item.id} style={{ padding: '8px 0', borderBottom: '1px solid rgba(0,0,0,0.06)', display: 'flex', justifyContent: 'space-between', gap: 8 }}>
              <div><b>{item.name}</b> · {item.category} · {item.status === 'enabled' ? '可进入' : '维护中'}{item.remark && <small style={{ display: 'block' }}>{item.remark}</small>}</div>
              <button disabled={item.status !== 'enabled' || launchingCode === item.code} onClick={() => void launchPlatform(item.code)}>
                {launchingCode === item.code ? '跳转中…' : '进入'}
              </button>
            </div>
          ))}
          {launchInfo && <p style={{ marginTop: 12 }}>{launchInfo.message}</p>}
        </ActionDialog>
      )}
      {activeAction === '邀请好友' && (
        <ActionDialog title={invite?.title ?? '邀请好友'} description={invite?.share_text ?? '加载邀请信息…'} onClose={() => setActiveAction(null)}>
          {invite && (
            <div style={{ marginTop: 12 }}>
              <div>邀请码：<b>{invite.invite_code}</b></div>
              {invite.room_code && <div style={{ marginTop: 8 }}>房间号：<b>{invite.room_code}</b></div>}
              {invite.reward > 0 && <div style={{ marginTop: 8 }}>活动奖励：{invite.reward.toFixed(2)} 元</div>}
            </div>
          )}
        </ActionDialog>
      )}
      {activeAction && !wiredActions.has(activeAction) && null}
    </section>
  )
}
