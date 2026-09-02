import { useEffect, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Icon } from '../components/Icon'
import { betsApi, type MemberBet } from '../api/bets'
import { memberApi, type BalanceRecord, type InviteInfo, type MemberApplication, type MemberPaymentAccount, type RebatePreview, type WalletChannel, type WalletSummary } from '../api/member'
import { pathForWallet, type WalletActionSlug } from '../router'
import { balanceRecordLabel } from '../utils/balanceRecordLabels'
import { betStatusText, betStatusTone } from '../utils/betStatus'

type WalletAction = '上分申请' | '下分申请' | '收款方式' | '申请记录' | '娱乐额度' | '游戏记录' | '竞猜列表' | '积分账变' | '自助回水' | '福利报表' | '红包报表' | '邀请好友'

const actionSlugs: Record<WalletAction, WalletActionSlug> = {
  '上分申请': 'credit',
  '下分申请': 'debit',
  '收款方式': 'channels',
  '申请记录': 'applications',
  '娱乐额度': 'quota',
  '游戏记录': 'bets',
  '竞猜列表': 'pending-bets',
  '积分账变': 'ledger',
  '自助回水': 'rebate',
  '福利报表': 'welfare',
  '红包报表': 'redpacket',
  '邀请好友': 'invite',
}

const slugActions = Object.fromEntries(Object.entries(actionSlugs).map(([label, slug]) => [slug, label])) as Record<WalletActionSlug, WalletAction>

const actions: Array<{ icon: string; name: WalletAction; tone: string }> = [
  { icon: '/icons/duo/coin-stack.svg', name: '上分申请', tone: 'aqua' }, { icon: '/icons/duo/credit-card.svg', name: '下分申请', tone: 'coral' }, { icon: '/icons/duo/credit-card.svg', name: '收款方式', tone: 'gold' },
  { icon: '/icons/duo/clipboard.svg', name: '申请记录', tone: 'blue' }, { icon: '/icons/lucide/wallet.svg', name: '娱乐额度', tone: 'violet' }, { icon: '/icons/duo/clapperboard.svg', name: '游戏记录', tone: 'aqua' },
  { icon: '/icons/duo/chart-pie.svg', name: '竞猜列表', tone: 'blue' }, { icon: '/icons/duo/coin-stack.svg', name: '积分账变', tone: 'gold' }, { icon: '/icons/duo/clock.svg', name: '自助回水', tone: 'aqua' },
  { icon: '/icons/duo/confetti.svg', name: '福利报表', tone: 'coral' }, { icon: '/icons/duo/discount.svg', name: '红包报表', tone: 'aqua' }, { icon: '/icons/lucide/gift.svg', name: '邀请好友', tone: 'blue' },
]

const applicationStatus: Record<string, { label: string; tone: string }> = {
  pending: { label: '审核中', tone: 'pending' },
  approved: { label: '已通过', tone: 'success' },
  rejected: { label: '未通过', tone: 'danger' },
}

const quickAmounts = [100, 500, 1000, 2000]

const paymentAccountTypes = [
  { id: 'wechat', label: '微信', mark: '微', hint: '填写微信收款账号' },
  { id: 'alipay', label: '支付宝', mark: '支', hint: '填写支付宝账号' },
  { id: 'bank', label: '银行卡', mark: '卡', hint: '填写银行卡号' },
  { id: 'usdt', label: 'USDT', mark: '₮', hint: '填写 USDT 收款地址' },
] as const

const featuredPaymentAccountTypes = [paymentAccountTypes[1], paymentAccountTypes[0], paymentAccountTypes[2]] as const

const paymentAccountTypeLabel: Record<string, string> = Object.fromEntries(paymentAccountTypes.map((item) => [item.id, item.label]))

function formatMoney(value: number) {
  return Math.abs(value).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function formatDate(value?: string) {
  if (!value) return '时间待确认'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function StatusPill({ status }: { status: string }) {
  const meta = applicationStatus[status] ?? { label: status || '处理中', tone: 'pending' }
  return <span className={`wallet-status wallet-status-${meta.tone}`}>{meta.label}</span>
}

function SubpageNotice({ title, children }: { title: string; children: ReactNode }) {
  return <div className="wallet-subpage-notice"><b>{title}</b><span>{children}</span></div>
}

function WalletPage({ title, hint, sectionLabel = '资金服务', onBack, children, footer }: { title: string; hint?: string; sectionLabel?: string; onBack: () => void; children: ReactNode; footer?: ReactNode }) {
  return (
    <section className="wallet-page wallet-subpage">
      <header className="wallet-header wallet-header-back">
        <button type="button" className="wallet-panel-back" aria-label="返回钱包" onClick={onBack}>←</button>
        <div><b>{title}</b>{hint && <small>{hint}</small>}</div>
        <span aria-hidden="true" />
      </header>
      <div className="wallet-subpage-body">
        <div className="wallet-subpage-heading"><span>{sectionLabel}</span><i /></div>
        {children}
      </div>
      {footer && <footer className="wallet-panel-foot wallet-subpage-foot">{footer}</footer>}
    </section>
  )
}

function EmptyHint({ text }: { text: string }) {
  return <p className="wallet-empty"><span>—</span>{text}</p>
}

function BetDetail({ bet, onClose, onCancel }: { bet: MemberBet; onClose: () => void; onCancel: () => void }) {
  const isPending = bet.status === 'pending'
  return createPortal(
    <div
      className="wallet-bet-detail-layer"
      role="presentation"
      onClick={(event) => {
        // 只允许点击遮罩空白区关闭。详情卡片重新渲染时不能误触发关闭。
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <section className="wallet-bet-detail" role="dialog" aria-modal="true" aria-label="注单详情">
        <header><div><small>注单详情</small><b>{bet.play_name || bet.selection}</b></div><button type="button" aria-label="关闭" onClick={onClose}>×</button></header>
        <dl><div><dt>彩种</dt><dd>{bet.game_id}</dd></div><div><dt>期号</dt><dd>{bet.issue}</dd></div><div><dt>投注内容</dt><dd>{bet.play_name || '选号'} · {bet.selection}</dd></div><div><dt>投注金额</dt><dd>¥ {formatMoney(bet.amount)}</dd></div><div><dt>赔率</dt><dd>{bet.odds.toFixed(3)}</dd></div><div><dt>当前状态</dt><dd><span className={`wallet-bet-status ${betStatusTone(bet.status, bet.remark)}`}>{betStatusText(bet.status, bet.remark)}</span></dd></div><div><dt>派彩</dt><dd className={bet.payout > 0 ? 'is-income' : ''}>¥ {formatMoney(bet.payout)}</dd></div><div><dt>投注时间</dt><dd>{formatDate(bet.created_at)}</dd></div></dl>
        {isPending && <p>开奖前可撤销本注，撤销后金额将退回余额。</p>}
        <footer>{isPending && <button type="button" className="wallet-bet-cancel" onClick={onCancel}>撤销本注</button>}<button type="button" className="wallet-submit" onClick={onClose}>关闭</button></footer>
      </section>
    </div>,
    document.body,
  )
}

/** 钱包：余额、上下分、娱乐平台直接进入、其余服务独立子页 */
export function Wallet({ balance, walletAction, returnGameId, onBackToGame, onRefresh, onNavigate }: {
  balance: number
  walletAction?: WalletActionSlug
  returnGameId?: string
  onBackToGame?: () => void
  onRefresh?: () => void
  onNavigate: (path: string) => void
}) {
  const activeAction = walletAction ? slugActions[walletAction] : null
  const [applications, setApplications] = useState<MemberApplication[]>([])
  const [history, setHistory] = useState<BalanceRecord[]>([])
  const [historyHasMore, setHistoryHasMore] = useState(false)
  const [historyLoadingMore, setHistoryLoadingMore] = useState(false)
  const [channels, setChannels] = useState<WalletChannel[]>([])
  const [paymentAccounts, setPaymentAccounts] = useState<MemberPaymentAccount[]>([])
  const [bets, setBets] = useState<MemberBet[]>([])
  const [summary, setSummary] = useState<WalletSummary | null>(null)
  const [rebate, setRebate] = useState<RebatePreview | null>(null)
  const [invite, setInvite] = useState<InviteInfo | null>(null)
  const [amount, setAmount] = useState('100')
  const [remark, setRemark] = useState('')
  const [message, setMessage] = useState('')
  const [loading, setLoading] = useState(false)
  const [subpageLoading, setSubpageLoading] = useState(false)
  const [paymentType, setPaymentType] = useState('manual')
  const [paymentPickerOpen, setPaymentPickerOpen] = useState(false)
  const [paymentAccountPickerOpen, setPaymentAccountPickerOpen] = useState(false)
  const [paymentAccountId, setPaymentAccountId] = useState<number | null>(null)
  const [paymentAccountEditorOpen, setPaymentAccountEditorOpen] = useState(false)
  const [paymentAccountType, setPaymentAccountType] = useState<(typeof paymentAccountTypes)[number]['id']>('wechat')
  const [paymentAccountLabel, setPaymentAccountLabel] = useState('')
  const [paymentAccountName, setPaymentAccountName] = useState('')
  const [paymentAccountNo, setPaymentAccountNo] = useState('')
  const [paymentAccountHolder, setPaymentAccountHolder] = useState('')
  const [paymentAccountSaving, setPaymentAccountSaving] = useState(false)
  const [betFilter, setBetFilter] = useState<'all' | 'pending' | 'settled'>('all')
  const [betHasMore, setBetHasMore] = useState(false)
  const [betLoadingMore, setBetLoadingMore] = useState(false)
  // 详情保留一份快照，列表刷新、筛选或余额更新时不会把刚点开的记录清空。
  const [selectedBet, setSelectedBet] = useState<MemberBet | null>(null)
  const [copied, setCopied] = useState<'code' | 'link' | 'share' | null>(null)
  const [inviteReloadKey, setInviteReloadKey] = useState(0)

  const paymentChannels = channels.length ? channels : [{ id: 'manual', credit_type: 'manual', name: '人工处理', min_amount: 0, max_amount: 0 }]
  const selectedPaymentChannel = paymentChannels.find((channel) => channel.credit_type === paymentType) ?? paymentChannels[0]
  const selectedPaymentAccount = paymentAccounts.find((account) => account.id === paymentAccountId) ?? paymentAccounts.find((account) => account.is_default) ?? paymentAccounts[0]

  const goHome = () => {
    if (onBackToGame) {
      onBackToGame()
      return
    }
    onNavigate(pathForWallet())
  }
  const goAction = (action: WalletActionSlug) => onNavigate(pathForWallet(action, returnGameId))

  useEffect(() => {
    void memberApi.walletSummary().then(setSummary).catch(() => setSummary(null))
  }, [balance])

  useEffect(() => {
    if (!activeAction) return
    let cancelled = false
    setSubpageLoading(true)
    setMessage('')
    if (activeAction === '游戏记录' || activeAction === '竞猜列表') setBetHasMore(false)
    void (async () => {
      try {
        if (activeAction === '申请记录') {
          const result = await memberApi.applications()
          if (!cancelled) setApplications(result.items)
        } else if (activeAction === '积分账变') {
          const result = await memberApi.balanceHistory(20)
          if (!cancelled) {
            setHistory(result.items)
            setHistoryHasMore(result.has_more)
          }
        } else if (activeAction === '收款方式') {
          const result = await memberApi.paymentAccounts()
          if (!cancelled) {
            setPaymentAccounts(result)
            setPaymentAccountId((current) => current ?? result.find((item) => item.is_default)?.id ?? result[0]?.id ?? null)
          }
        } else if (activeAction === '上分申请') {
          const result = await memberApi.walletChannels()
          if (!cancelled) {
            setChannels(result)
            setPaymentType(result[0]?.credit_type ?? 'manual')
          }
        } else if (activeAction === '下分申请') {
          const result = await memberApi.paymentAccounts()
          if (!cancelled) {
            setPaymentAccounts(result)
            setPaymentAccountId((current) => current ?? result.find((item) => item.is_default)?.id ?? result[0]?.id ?? null)
          }
        } else if (activeAction === '游戏记录') {
          const result = await betsApi.list({ page_size: 30 })
          if (!cancelled) {
            setBets(result.items)
            setBetHasMore(result.has_more)
          }
        } else if (activeAction === '竞猜列表') {
          const result = await betsApi.list({ status: betFilter, page_size: 30 })
          if (!cancelled) {
            setBets(result.items)
            setBetHasMore(result.has_more)
          }
        } else if (activeAction === '娱乐额度') {
          const result = await memberApi.walletSummary()
          if (!cancelled) setSummary(result)
        } else if (activeAction === '自助回水') {
          const [rows, preview] = await Promise.all([memberApi.balanceHistory(50), memberApi.rebatePreview()])
          if (!cancelled) {
            setHistory(rows.items.filter((item) => item.type.includes('rebate')))
            setRebate(preview)
          }
        } else if (activeAction === '福利报表') {
          const [rows, preview] = await Promise.all([memberApi.balanceHistory(50), memberApi.rebatePreview()])
          if (!cancelled) {
            setHistory(rows.items.filter((item) => item.type.includes('checkin') || item.type.includes('activity') || item.type.includes('invite')))
            setRebate(preview)
          }
        } else if (activeAction === '红包报表') {
          const result = await memberApi.balanceHistory(50)
          if (!cancelled) setHistory(result.items.filter((item) => item.type.includes('redpacket')))
        } else if (activeAction === '邀请好友') {
          if (!cancelled) setInvite(null)
          const result = await memberApi.inviteInfo()
          if (!cancelled) setInvite(result)
        }
      } catch {
        if (!cancelled) setMessage('暂时无法读取数据，请稍后重试')
      } finally {
        if (!cancelled) setSubpageLoading(false)
      }
    })()
    return () => { cancelled = true }
  }, [activeAction, betFilter, inviteReloadKey])

  const loadMoreHistory = async () => {
    const beforeID = history.at(-1)?.id
    if (!beforeID || !historyHasMore || historyLoadingMore) return
    setHistoryLoadingMore(true)
    try {
      const result = await memberApi.balanceHistory(20, beforeID)
      setHistory((current) => [...current, ...result.items.filter((item) => !current.some((existing) => existing.id === item.id))])
      setHistoryHasMore(result.has_more)
    } catch {
      setMessage('暂时无法读取更多账变记录，请稍后重试')
    } finally {
      setHistoryLoadingMore(false)
    }
  }

  const loadMoreBets = async () => {
    const beforeID = bets.at(-1)?.id
    if (!beforeID || !betHasMore || betLoadingMore) return
    setBetLoadingMore(true)
    try {
      const result = await betsApi.list({
        status: activeAction === '竞猜列表' ? betFilter : 'all',
        page_size: 30,
        before_id: beforeID,
      })
      setBets((current) => [...current, ...result.items.filter((item) => !current.some((existing) => existing.id === item.id))])
      setBetHasMore(result.has_more)
    } catch {
      setMessage('暂时无法读取更多注单，请稍后重试')
    } finally {
      setBetLoadingMore(false)
    }
  }

  useEffect(() => {
    setPaymentPickerOpen(false)
    setPaymentAccountPickerOpen(false)
    setPaymentAccountEditorOpen(false)
    setSelectedBet(null)
  }, [activeAction])

  const submitApplication = async (request_type: 'credit' | 'debit') => {
    const numericAmount = Number(amount)
    if (!Number.isFinite(numericAmount) || numericAmount <= 0) {
      setMessage('请输入大于 0 的申请金额')
      return
    }
    if (request_type === 'debit' && !selectedPaymentAccount) {
      setMessage('请先新增并选择收款方式')
      return
    }
    setLoading(true)
    setMessage('')
    try {
      await memberApi.createApplication({ request_type, amount: numericAmount, payment_type: request_type === 'debit' ? selectedPaymentAccount?.account_type : paymentType, payment_account_id: request_type === 'debit' ? selectedPaymentAccount?.id : undefined, remark })
      setMessage('申请已提交，请等待审核')
      onRefresh?.()
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '提交失败')
    } finally {
      setLoading(false)
    }
  }

  const inviteLink = invite ? `${window.location.origin}/register?invite=${encodeURIComponent(invite.invite_code)}` : ''

  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 局域网 HTTP 和部分 Safari 不开放 Clipboard API，保留兼容复制。
      const input = document.createElement('textarea')
      try {
        input.value = text
        input.style.position = 'fixed'
        input.style.opacity = '0'
        document.body.appendChild(input)
        input.focus()
        input.select()
        return document.execCommand('copy')
      } catch {
        return false
      } finally {
        input.remove()
      }
    }
  }

  const markInviteCopied = (target: 'code' | 'link' | 'share') => {
    setCopied(target)
    window.setTimeout(() => setCopied((current) => current === target ? null : current), 1800)
  }

  const copyInvite = async (target: 'code' | 'link') => {
    if (!invite) return
    const successful = await copyText(target === 'code' ? invite.invite_code : inviteLink)
    if (successful) {
      setMessage('')
      markInviteCopied(target)
    } else {
      setMessage('复制失败，请长按邀请码手动复制')
    }
  }

  const shareInvite = async () => {
    if (!invite) return
    const shareText = `${invite.share_text}\n注册链接：${inviteLink}`
    if (typeof navigator.share === 'function') {
      try {
        await navigator.share({ title: invite.title || '邀请有礼', text: invite.share_text, url: inviteLink })
        markInviteCopied('share')
        return
      } catch (reason) {
        if (reason instanceof DOMException && reason.name === 'AbortError') return
      }
    }
    const successful = await copyText(shareText)
    if (successful) {
      setMessage('')
      markInviteCopied('share')
    } else {
      setMessage('暂时无法分享，请复制邀请码后发送给好友')
    }
  }

  const savePaymentAccount = async () => {
    if (!paymentAccountName.trim() || !paymentAccountNo.trim()) {
      setMessage('请填写收款账号和账户名称')
      return
    }
    setPaymentAccountSaving(true)
    setMessage('')
    try {
      const created = await memberApi.createPaymentAccount({
        account_type: paymentAccountType,
        label: paymentAccountLabel.trim(),
        account_name: paymentAccountName.trim(),
        account_no: paymentAccountNo.trim(),
        holder_name: paymentAccountHolder.trim(),
        is_default: paymentAccounts.length === 0,
      })
      setPaymentAccounts((current) => [created, ...current.map((item) => ({ ...item, is_default: created.is_default ? false : item.is_default }))])
      setPaymentAccountId(created.id)
      setPaymentAccountLabel('')
      setPaymentAccountName('')
      setPaymentAccountNo('')
      setPaymentAccountHolder('')
      setPaymentAccountEditorOpen(false)
      setMessage('收款方式已保存')
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '新增收款方式失败')
    } finally {
      setPaymentAccountSaving(false)
    }
  }

  const removePaymentAccount = async (account: MemberPaymentAccount) => {
    if (!window.confirm(`删除“${account.label}”吗？`)) return
    try {
      await memberApi.deletePaymentAccount(account.id)
      const next = paymentAccounts.filter((item) => item.id !== account.id)
      setPaymentAccounts(next)
      if (paymentAccountId === account.id) setPaymentAccountId(next.find((item) => item.is_default)?.id ?? next[0]?.id ?? null)
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '删除收款方式失败')
    }
  }

  const cancelBet = async (bet: MemberBet) => {
    try {
      const updated = await betsApi.cancel(bet.id)
      setBets((current) => current.map((item) => item.id === updated.id ? updated : item))
      setSelectedBet(updated)
      onRefresh?.()
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '撤单失败')
    }
  }

  const renderSubpage = () => {
    if (!activeAction) return null

    if (activeAction === '上分申请' || activeAction === '下分申请') {
      const isCredit = activeAction === '上分申请'
      return (
        <WalletPage
          title={activeAction}
          hint={isCredit ? '提交后由后台审核并入账' : '提交后由后台审核并扣款'}
          onBack={goHome}
          footer={<button type="button" className="wallet-submit" disabled={loading} onClick={() => void submitApplication(isCredit ? 'credit' : 'debit')}>{loading ? '提交中…' : '提交申请'}</button>}
        >
          <SubpageNotice title={isCredit ? '上分说明' : '下分说明'}>{isCredit ? '选择收款方式后提交，到账金额以审核结果为准。' : `当前可用余额 ${formatMoney(balance)} 元，审核通过后将扣除对应金额。`}</SubpageNotice>
          <label className="wallet-field wallet-amount-field"><span>申请金额（元）</span><div><i>¥</i><input inputMode="decimal" value={amount} onChange={(e) => { setAmount(e.target.value.replace(/[^\d.]/g, '')); setMessage('') }} placeholder="请输入金额" /></div></label>
          <div className="wallet-quick-amounts">{quickAmounts.map((value) => <button type="button" className={amount === String(value) ? 'is-selected' : ''} key={value} onClick={() => setAmount(String(value))}>¥ {value.toLocaleString()}</button>)}</div>
          {isCredit && <div className="wallet-field wallet-payment-picker">
            <span id="payment-channel-label">收款方式</span>
            <button type="button" className="wallet-payment-trigger" aria-expanded={paymentPickerOpen} aria-haspopup="listbox" aria-labelledby="payment-channel-label" onClick={() => setPaymentPickerOpen((open) => !open)}>
              <div><b>{selectedPaymentChannel.name}</b><small>{selectedPaymentChannel.min_amount > 0 ? `单笔 ${selectedPaymentChannel.min_amount}–${selectedPaymentChannel.max_amount} 元` : '由客服人工处理'}</small></div><i aria-hidden="true" />
            </button>
            {paymentPickerOpen && <div className="wallet-payment-menu" role="listbox" aria-label="选择收款方式">{paymentChannels.map((channel) => <button type="button" aria-selected={channel.credit_type === selectedPaymentChannel.credit_type} className={channel.credit_type === selectedPaymentChannel.credit_type ? 'is-selected' : ''} key={channel.id} onClick={() => { setPaymentType(channel.credit_type); setPaymentPickerOpen(false) }}><span>{channel.name.slice(0, 1)}</span><div><b>{channel.name}</b><small>{channel.min_amount > 0 ? `单笔 ${channel.min_amount}–${channel.max_amount} 元` : '由客服人工处理'}</small></div><i>{channel.credit_type === selectedPaymentChannel.credit_type ? '✓' : ''}</i></button>)}</div>}
          </div>}
          {!isCredit && <div className="wallet-field wallet-payment-picker">
            <span id="payment-account-label">收款账户</span>
            {selectedPaymentAccount ? <button type="button" className="wallet-payment-trigger" aria-expanded={paymentAccountPickerOpen} aria-haspopup="listbox" aria-labelledby="payment-account-label" onClick={() => setPaymentAccountPickerOpen((open) => !open)}><div><b>{selectedPaymentAccount.label}</b><small>{paymentAccountTypeLabel[selectedPaymentAccount.account_type] ?? selectedPaymentAccount.account_type} · {selectedPaymentAccount.account_name} · {selectedPaymentAccount.account_no}</small></div><i aria-hidden="true" /></button> : <button type="button" className="wallet-payment-trigger wallet-payment-empty" onClick={() => goAction('channels')}><div><b>暂无收款账户</b><small>请先新增收款方式</small></div><i aria-hidden="true" /></button>}
            {paymentAccountPickerOpen && <div className="wallet-payment-menu" role="listbox" aria-label="选择收款账户">{paymentAccounts.map((account) => <button type="button" aria-selected={account.id === selectedPaymentAccount?.id} className={account.id === selectedPaymentAccount?.id ? 'is-selected' : ''} key={account.id} onClick={() => { setPaymentAccountId(account.id); setPaymentAccountPickerOpen(false) }}><span>{paymentAccountTypeLabel[account.account_type]?.slice(0, 1) ?? '收'}</span><div><b>{account.label}</b><small>{account.account_name} · {account.account_no}</small></div><i>{account.id === selectedPaymentAccount?.id ? '✓' : ''}</i></button>)}<button type="button" className="wallet-add-account-option" onClick={() => goAction('channels')}>＋ 新增收款方式</button></div>}
          </div>}
          <label className="wallet-field"><span>备注 <em>选填</em></span><input value={remark} maxLength={120} onChange={(e) => setRemark(e.target.value)} placeholder={isCredit ? '例如付款账号或付款说明' : '例如收款备注'} /></label>
          {message && <p className="wallet-message">{message}</p>}
        </WalletPage>
      )
    }

    if (activeAction === '申请记录') {
      return (
        <WalletPage title="申请记录" hint={applications.length ? `共 ${applications.length} 条` : '暂无记录'} onBack={goHome}>
          {subpageLoading ? <EmptyHint text="正在读取申请记录…" /> : applications.length ? applications.map((item) => (
            <article className="wallet-row wallet-application-row" key={item.id}>
              <div><span className={`wallet-record-icon ${item.request_type}`}>{item.request_type === 'credit' ? '入' : '出'}</span><section><b>{item.request_type === 'credit' ? '上分申请' : '下分申请'}</b><small>{formatDate(item.created_at)} · {item.payment_type || '人工处理'}</small></section></div>
              <aside><strong>{item.request_type === 'credit' ? '+' : '-'}{formatMoney(item.requested_amount)}</strong><StatusPill status={item.status} /></aside>
              {(item.payment_account_label || item.remark || item.review_remark) && <p>{item.payment_account_label || item.review_remark || item.remark}</p>}
            </article>
          )) : <EmptyHint text="暂无上下分申请" />}
        </WalletPage>
      )
    }

    if (activeAction === '积分账变') {
      return (
        <WalletPage title="积分账变" hint={history.length ? `已加载 ${history.length} 条` : undefined} onBack={goHome}>
          {subpageLoading ? <EmptyHint text="正在读取账变记录…" /> : history.length ? history.map((item) => (
            <article className="wallet-row wallet-ledger-row" key={item.id}>
              <div><b>{balanceRecordLabel(item.type)}</b><small>{item.remark || formatDate(item.created_at)}</small></div>
              <aside><strong className={item.amount >= 0 ? 'is-income' : 'is-expense'}>{item.amount >= 0 ? '+' : '-'}{formatMoney(item.amount)}</strong><small>结余 {formatMoney(item.after)}</small></aside>
            </article>
          )) : <EmptyHint text="暂无账变记录" />}
          {historyHasMore && <button className="wallet-load-more" disabled={historyLoadingMore} onClick={() => void loadMoreHistory()}>{historyLoadingMore ? '正在加载…' : '加载更早账变记录'}</button>}
        </WalletPage>
      )
    }

    if (activeAction === '收款方式') {
      const selectedAccountType = paymentAccountTypes.find((item) => item.id === paymentAccountType) ?? paymentAccountTypes[0]
      return (
        <WalletPage title="收款方式" hint="下分到账账户，可随时新增或删除" onBack={goHome}>
          <SubpageNotice title="安全提示">仅保存用于下分审核的账户信息；展示时会自动隐藏账号。</SubpageNotice>
          {subpageLoading ? <EmptyHint text="正在读取收款方式…" /> : <>
            <div className="wallet-payment-type-actions" aria-label="新增收款方式">
              {featuredPaymentAccountTypes.map((item) => {
                const count = paymentAccounts.filter((account) => account.account_type === item.id).length
                return <button type="button" className={item.id} key={item.id} onClick={() => { setMessage(''); setPaymentAccountType(item.id); setPaymentAccountEditorOpen(true) }}><i>{item.mark}</i><span><b>{item.label}</b><small>{count > 0 ? `已添加 ${count} 个账户` : '尚未添加账户'}</small></span><em>＋ 添加</em></button>
              })}
            </div>
            <div className="wallet-payment-account-list">
              {paymentAccounts.map((account) => <article className="wallet-payment-account" key={account.id}><span className={`account-mark ${account.account_type}`}>{paymentAccountTypeLabel[account.account_type]?.slice(0, 1) ?? '收'}</span><div><b>{account.label}{account.is_default && <i>默认</i>}</b><small>{paymentAccountTypeLabel[account.account_type] ?? account.account_type} · {account.account_name}</small><em>{account.account_no}</em></div><button type="button" aria-label={`删除${account.label}`} onClick={() => void removePaymentAccount(account)}>删除</button></article>)}
              {paymentAccounts.length === 0 && <EmptyHint text="还没有收款方式，新增后可用于下分申请" />}
            </div>
            {paymentAccountEditorOpen && <section className="wallet-payment-account-editor"><header><b>新增{selectedAccountType.label}收款方式</b><button type="button" onClick={() => setPaymentAccountEditorOpen(false)}>收起</button></header><div className="wallet-account-type-grid">{paymentAccountTypes.map((item) => <button type="button" className={paymentAccountType === item.id ? 'is-selected' : ''} key={item.id} onClick={() => setPaymentAccountType(item.id)}><i>{item.mark}</i>{item.label}</button>)}</div><label className="wallet-field"><span>显示名称 <em>选填</em></span><input value={paymentAccountLabel} maxLength={80} onChange={(event) => setPaymentAccountLabel(event.target.value)} placeholder={`例如常用${selectedAccountType.label}`} /></label><label className="wallet-field"><span>收款账号 / 地址</span><input value={paymentAccountNo} maxLength={180} onChange={(event) => setPaymentAccountNo(event.target.value)} placeholder={selectedAccountType.hint} /></label><label className="wallet-field"><span>账户名称</span><input value={paymentAccountName} maxLength={100} onChange={(event) => setPaymentAccountName(event.target.value)} placeholder="例如张三或账户昵称" /></label><label className="wallet-field"><span>收款人姓名 <em>选填</em></span><input value={paymentAccountHolder} maxLength={80} onChange={(event) => setPaymentAccountHolder(event.target.value)} placeholder="与账户实名一致" /></label><button type="button" className="wallet-submit" disabled={paymentAccountSaving} onClick={() => void savePaymentAccount()}>{paymentAccountSaving ? '保存中…' : `保存${selectedAccountType.label}`}</button></section>}
          </>}
          {message && <p className="wallet-message">{message}</p>}
        </WalletPage>
      )
    }

    if (activeAction === '娱乐额度') {
      return (
        <WalletPage title="娱乐额度" hint="待结算注单占用" onBack={goHome}>
          <SubpageNotice title="当前余额">可用余额 {formatMoney(balance)} 元；待开奖注单将在开奖结算后更新。</SubpageNotice>
          <div className="wallet-stat-grid">
            <div><small>待结算金额</small><b>{summary ? formatMoney(summary.pending_amount) : '—'}</b></div>
            <div><small>待结算注单</small><b>{summary?.pending_count ?? '—'}</b></div>
            <div><small>历史注单</small><b>{summary?.total_bet_count ?? '—'}</b></div>
          </div>
        </WalletPage>
      )
    }

    if (activeAction === '游戏记录' || activeAction === '竞猜列表') {
      return (
        <WalletPage title={activeAction} hint={bets.length ? `${bets.length} 条注单` : undefined} onBack={goHome}>
          {activeAction === '竞猜列表' && <div className="wallet-bet-filter" role="tablist"><button type="button" className={betFilter === 'all' ? 'is-active' : ''} onClick={() => setBetFilter('all')}>全部</button><button type="button" className={betFilter === 'pending' ? 'is-active' : ''} onClick={() => setBetFilter('pending')}>待开奖</button><button type="button" className={betFilter === 'settled' ? 'is-active' : ''} onClick={() => setBetFilter('settled')}>已结算</button></div>}
          {subpageLoading ? <EmptyHint text="正在读取注单…" /> : bets.length ? bets.map((bet) => (
            <button type="button" className="wallet-row wallet-bet-row" key={bet.id} onClick={() => setSelectedBet({ ...bet })}>
              <div><b>{bet.play_name || bet.selection}</b><small>{bet.game_id} · 第 {bet.issue} 期</small></div><aside><strong>¥ {formatMoney(bet.amount)}</strong><span className={`wallet-bet-status ${betStatusTone(bet.status, bet.remark)}`}>{betStatusText(bet.status, bet.remark)}</span></aside><Icon name="arrow" />
            </button>
          )) : <EmptyHint text={activeAction === '竞猜列表' && betFilter === 'pending' ? '暂无待开奖注单' : '暂无注单'} />}
          {betHasMore && <button className="wallet-load-more" disabled={betLoadingMore} onClick={() => void loadMoreBets()}>{betLoadingMore ? '正在加载…' : '加载更早注单'}</button>}
          {selectedBet && <BetDetail bet={selectedBet} onClose={() => setSelectedBet(null)} onCancel={() => void cancelBet(selectedBet)} />}
        </WalletPage>
      )
    }

    if (activeAction === '福利报表') {
      return (
        <WalletPage title="福利报表" hint="签到与回水" onBack={goHome}>
          {rebate && (
            <div className="wallet-rebate-card">
              <span>今日回水预估</span><strong>¥ {formatMoney(rebate.estimated)}</strong><small>当前回水比例 {rebate.rate_percent}% · 流水 ¥ {formatMoney(rebate.today_turnover)}</small>
              <footer><b>已到账 <i>{formatMoney(rebate.credited)}</i></b><b>待入账 <i>{formatMoney(rebate.pending_credit)}</i></b></footer>
            </div>
          )}
          {subpageLoading ? <EmptyHint text="正在读取福利记录…" /> : history.length ? history.map((item) => (
            <article className="wallet-row" key={item.id}>
              <b>{balanceRecordLabel(item.type)} +{formatMoney(item.amount)}</b>
              <small>{item.remark || formatDate(item.created_at)}</small>
            </article>
          )) : !rebate && <EmptyHint text="暂无福利记录" />}
        </WalletPage>
      )
    }

    if (activeAction === '自助回水') {
      return (
        <WalletPage title="自助回水" hint="按平台设置自动结算" onBack={goHome}>
          {rebate && <div className="wallet-rebate-card"><span>今日预计回水</span><strong>¥ {formatMoney(rebate.estimated)}</strong><small>当前比例 {rebate.rate_percent}% · 有效流水 ¥ {formatMoney(rebate.today_turnover)}</small><footer><b>已到账 <i>{formatMoney(rebate.credited)}</i></b><b>待结算 <i>{formatMoney(rebate.pending_credit)}</i></b></footer></div>}
          <SubpageNotice title="结算说明">回水按后台设定比例自动核算并入账，无需手动申请。</SubpageNotice>
          {subpageLoading ? <EmptyHint text="正在读取回水记录…" /> : history.length ? history.map((item) => <article className="wallet-row" key={item.id}><b>回水返利 +{formatMoney(item.amount)}</b><small>{item.remark || formatDate(item.created_at)}</small></article>) : <EmptyHint text="暂无已到账回水" />}
        </WalletPage>
      )
    }

    if (activeAction === '红包报表') {
      return (
        <WalletPage title="红包报表" onBack={goHome}>
          {subpageLoading ? <EmptyHint text="正在读取红包记录…" /> : history.length ? history.map((item) => (
            <article className="wallet-row" key={item.id}>
              <b>红包奖励 +{formatMoney(item.amount)}</b>
              <small>{item.remark || formatDate(item.created_at)}</small>
            </article>
          )) : <EmptyHint text="暂无红包记录" />}
        </WalletPage>
      )
    }

    if (activeAction === '邀请好友') {
      return (
        <WalletPage title="邀请有礼" hint={invite?.room_code ? `房间 ${invite.room_code}` : undefined} sectionLabel="邀请中心" onBack={goHome}>
          {subpageLoading ? <EmptyHint text="正在读取专属邀请信息…" /> : invite ? (
            <>
              <section className="wallet-invite-hero">
                <div>
                  <span>INVITE BENEFITS</span>
                  <h2>{invite.title || '邀请好友'}</h2>
                  <p>{invite.reward > 0 ? `好友完成注册，双方各得 ${formatMoney(invite.reward)} 元奖励` : '分享您的专属链接，邀请好友一起加入'}</p>
                </div>
                <div className="wallet-invite-gift" aria-hidden="true"><i>礼</i><span /><span /></div>
              </section>

              <section className="wallet-invite-code-card">
                <header><div><small>专属邀请码</small><b>{invite.username}</b></div><em>长期有效</em></header>
                <div className="wallet-invite-code-row"><strong>{invite.invite_code}</strong><button type="button" onClick={() => void copyInvite('code')}>{copied === 'code' ? '已复制' : '复制'}</button></div>
                <div className="wallet-invite-link"><span>{inviteLink}</span><button type="button" aria-label="复制注册链接" onClick={() => void copyInvite('link')}>{copied === 'link' ? '已复制' : '复制链接'}</button></div>
                <button type="button" className="wallet-invite-share" onClick={() => void shareInvite()}><span aria-hidden="true">↗</span>{copied === 'share' ? '邀请内容已复制' : '立即邀请好友'}</button>
              </section>

              <div className="wallet-invite-stats">
                <div><small>成功邀请</small><b>{Math.max(0, Number(invite.invited_count) || 0)}<i> 人</i></b></div>
                <div><small>累计奖励</small><b>{formatMoney(Math.max(0, Number(invite.total_reward) || 0))}<i> 元</i></b></div>
                <div><small>每位好友奖励</small><b>{invite.reward > 0 ? formatMoney(invite.reward) : '按活动规则'}{invite.reward > 0 && <i> 元</i>}</b></div>
                <div><small>邀请归属</small><b>{invite.room_code || '当前房间'}</b></div>
              </div>

              <section className="wallet-invite-steps">
                <header><b>邀请流程</b><small>三步完成</small></header>
                <ol>
                  <li><i>1</i><div><b>分享邀请</b><small>发送专属链接或邀请码</small></div></li>
                  <li><i>2</i><div><b>好友注册</b><small>好友使用邀请码创建帐号</small></div></li>
                  <li><i>3</i><div><b>奖励到账</b><small>符合活动条件后自动发放</small></div></li>
                </ol>
              </section>

              <section className="wallet-invite-rules">
                <header><b>活动规则</b><span>RULES</span></header>
                <p><i>•</i>好友首次使用您的邀请码完成注册后，邀请关系生效。</p>
                <p><i>•</i>邀请关系归属于当前房间，好友无需另外填写房间号。</p>
                <p><i>•</i>奖励金额及发放条件以当前房间活动配置为准。</p>
              </section>
              {message && <p className="wallet-message" aria-live="polite">{message}</p>}
            </>
          ) : <section className="wallet-invite-error"><i>!</i><b>邀请信息暂未加载</b><p>{message || '请检查网络后重新获取专属邀请码。'}</p><button type="button" onClick={() => setInviteReloadKey((current) => current + 1)}>重新加载</button></section>}
        </WalletPage>
      )
    }

    return null
  }

  if (activeAction) return renderSubpage()

  return (
    <section className="wallet-page">
      <header className="wallet-header"><b>钱包中心</b></header>
      <section className="wallet-balance">
        <div><small>账户余额</small><b>{balance.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</b><span>元</span></div>
        <em>今日统计</em>
        <footer>
          <span>流水 <b>{summary ? summary.today_turnover.toFixed(2) : '—'}</b></span>
          <span>回水 <b>{summary ? summary.today_rebate.toFixed(2) : '—'}</b></span>
          <span>盈亏 <b>{summary ? summary.today_profit.toFixed(2) : '—'}</b></span>
        </footer>
      </section>

      <section className="wallet-primary-actions">
        {actions.slice(0, 3).map((action) => (
          <button type="button" key={action.name} onClick={() => goAction(actionSlugs[action.name])}>
            <span className={`wallet-action-icon ${action.tone}`}><i aria-hidden="true" style={{ maskImage: `url(${action.icon})`, WebkitMaskImage: `url(${action.icon})` }} /></span>
            <b>{action.name}</b>
          </button>
        ))}
      </section>
      <section className="wallet-tools">
        <header><b>钱包服务</b><small>点击进入详情页</small></header>
        <div>{actions.slice(3).map((action) => (
          <button type="button" key={action.name} onClick={() => goAction(actionSlugs[action.name])}>
            <span className={`wallet-action-icon ${action.tone}`}><i aria-hidden="true" style={{ maskImage: `url(${action.icon})`, WebkitMaskImage: `url(${action.icon})` }} /></span>
            <b>{action.name}</b>
          </button>
        ))}</div>
      </section>
    </section>
  )
}
