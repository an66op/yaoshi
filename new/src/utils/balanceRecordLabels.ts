const labels: Record<string, string> = {
  manual: '人工调整', credit: '上分到账', debit: '下分扣除',
  bet: '投注扣款', bet_cancel: '撤单退款', settlement: '注单结算',
  checkin: '每日签到', activity: '活动奖励', invite: '邀请奖励',
  redpacket: '红包奖励', rebate: '回水返利',
}

export function balanceRecordLabel(type: string) {
  return labels[type] ?? '其他账变'
}
