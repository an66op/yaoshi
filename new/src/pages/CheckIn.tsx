import { useEffect, useState } from 'react'
import { Icon } from '../components/Icon'
import { portalApi, type ActivityStatus } from '../api/portal'

type Props = {
  onBack: () => void
  onComplete?: () => void
}

const defaultRewards = [5, 8, 12, 18, 25, 35, 50]

/** 每日签到：接后端活动 API */
export function CheckIn({ onBack, onComplete }: Props) {
  const [activityId, setActivityId] = useState<number | null>(null)
  const [status, setStatus] = useState<ActivityStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [message, setMessage] = useState('')

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const items = await portalApi.activities('checkin')
        const active = items.find((item) => item.type === 'checkin' && item.status === 'active')
        if (!active) {
          setMessage('暂无签到活动')
          return
        }
        if (cancelled) return
        setActivityId(active.id)
        setStatus(await portalApi.activityStatus(active.id))
      } catch (reason) {
        if (!cancelled) setMessage(reason instanceof Error ? reason.message : '加载失败')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => { cancelled = true }
  }, [])

  const claim = async () => {
    if (!activityId) return
    setSubmitting(true)
    setMessage('')
    try {
      const result = await portalApi.checkIn(activityId)
      setStatus(await portalApi.activityStatus(activityId))
      onComplete?.()
      setMessage(result.message)
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '签到失败')
    } finally {
      setSubmitting(false)
    }
  }

  const streak = status?.streak ?? 0
  const checkedIn = status?.checked_in ?? false
  const rewards = defaultRewards

  return (
    <main className="check-in-page">
      <header className="check-in-header">
        <button aria-label="返回投注页" onClick={onBack}><Icon name="back" /></button>
        <b>每日签到</b><span />
      </header>
      <section className="check-in-hero">
        <small>DAILY REWARD</small>
        <h1>{status?.title ?? '好运每日相伴'}</h1>
        <p>{loading ? '加载中…' : '连续签到可领取更多奖励'}</p>
        <div><span>已连续签到</span><b>{streak} <small>天</small></b></div>
      </section>
      <section className="check-in-card">
        <header>
          <div><small>本周签到</small><b>连续签到，奖励加倍</b></div>
          <em>{checkedIn ? '今日已签到' : '待签到'}</em>
        </header>
        <div className="check-in-days">
          {rewards.map((reward, index) => {
            const completed = index < streak || (checkedIn && index === streak - 1)
            const today = !checkedIn && index === streak
            return (
              <article className={`${completed ? 'completed' : ''} ${today ? 'today' : ''}`} key={reward}>
                <small>第 {index + 1} 天</small>
                <b>{completed ? '✓' : `+${reward}`}</b>
                <span>{completed ? '已领取' : `${reward} 奖励`}</span>
              </article>
            )
          })}
        </div>
        <button className={`check-in-claim ${checkedIn ? 'claimed' : ''}`} disabled={checkedIn || submitting || !activityId} onClick={() => void claim()}>
          {submitting ? '提交中…' : checkedIn ? '今日已签到，明日再来' : `立即签到领取奖励`}
        </button>
        {message && <p className="check-in-note">{message}</p>}
      </section>
      <p className="check-in-note">每日 00:00 刷新 · 奖励入账后可于钱包查看</p>
    </main>
  )
}
