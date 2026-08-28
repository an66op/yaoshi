import { broadcastAdminLogout, clearLegacyAdminSession } from './auth'
import { createRequestId } from './utils/requestId'

export type AdminGame = {
  id: string
  code: string
  name: string
  category: string
  lobby_category: string
  lobby_sort_order: number
  badge: string
  badge_color: string
  enabled: boolean
  issue: string
	current_issue?: string
	issue_status?: string
	seal_at?: string
	latest_numbers?: number[]
	source_healthy?: boolean
  next_draw_at: string
  turnover: number
  profit: number
  source_kind: 'official' | 'simulated' | string
  source_name: string
  source_url: string
  sync_status: 'idle' | 'syncing' | 'ok' | 'error' | string
  last_sync_at: string | null
  last_sync_error: string
  schedule_mode: 'official-feed' | 'interval' | string
}

export type WorkspaceGame = AdminGame & {
  platform_enabled: boolean
  room_enabled: boolean
}

export type GameCategory = {
  id: number
  name: string
  sort_order: number
  game_count: number
  enabled_game_count: number
}

export type DashboardData = {
  overview: Record<string, number>
  stats: Record<string, number>
  games: AdminGame[]
}

export type DrawResult = {
  id: number
  game_id: string
  issue: string
  numbers: number[]
  draw_at: string
}

export type SourceSyncResult = {
  game_id: string
  source_name: string
  status: string
  imported: number
  latest_issue: string
  error?: string
}

export type OfficialSyncResponse = { results: SourceSyncResult[]; failed: number }

export type OfficialSourceTestResponse = OfficialSyncResponse & { group: string }

export type ServerClock = {
  server_time: string
  server_time_ms: number
  timezone: string
}

export type FeedJobStatus = {
  id: string
  name: string
  group: string
  game_ids: string[]
  timezone: string
  mode: 'draw-window' | 'normal' | 'waiting' | string
  running: boolean
  last_started_at?: string
  last_finished_at?: string
  last_success_at?: string
  next_run_at?: string
  imported: number
  latest_issue: string
  consecutive_errors: number
  last_error?: string
}

export type FeedStatus = {
  running: boolean
  started_at?: string
  server_time: string
  server_time_ms: number
  timezone: string
  jobs: FeedJobStatus[]
}

export type AuditLog = {
  id: number
  actor_id: number
  actor_name: string
  actor_role: 'admin' | 'agent' | string
  room_scope?: string
  method: string
  path: string
  status_code: number
  request_id?: string
  ip?: string
  created_at: string
}

export type AuditLogPage = { items: AuditLog[]; has_more: boolean; next_before_id?: number }

export type ReconciliationSummary = {
  generated_at: string
  issue_errors: Array<{ id: number; game_id: string; issue: string; status: string; last_error?: string; updated_at: string }>
  abnormal_bets: Array<{ id: number; workspace_id: number; game_id: string; issue: string; user_id: number; username: string; status: string; reconciliation_status: string; reconciliation_note?: string; amount_cents: number; refundable: boolean; created_at: string }>
  issue_error_count: number
  abnormal_bet_count: number
  historical_abnormal_bet_count: number
  pending_on_closed_count: number
  unresolved_bet_count: number
  recoverable_bet_count: number
  unrecoverable_bet_count: number
  missing_issue_bet_count: number
  disabled_game_pending_count: number
  stale_issue_count: number
  source_error_game_count: number
  negative_balance_count: number
  orphan_ledger_count: number
  duplicate_ledger_reference_count: number
  ledger_chain_gap_count: number
  ledger_arithmetic_error_count: number
  latest_balance_gap_count: number
  untracked_balance_user_count: number
  payment_account_error_count: number
  payment_channel_error_count: number
  notification_financial_error_count: number
  rebate_financial_error_count: number
  profit_share_financial_error_count: number
}

export type ReconciliationRefundResult = {
  bet_id: number
  workspace_id: number
  user_id: number
  amount_cents: number
  before_cents: number
  after_cents: number
  ledger_reference: string
  bet_status: string
  reconciliation_status: string
  already_refunded: boolean
}

export type LifecycleDataClass = 'chat_messages' | 'robot_chat_messages' | 'notifications' | 'audit_logs' | 'robot_test_data'

export type LifecycleAction = 'soft_delete' | 'hard_delete' | 'archive_then_purge_hot' | 'cold_archive'

export type LifecycleDeleteMode = 'soft' | 'hard'

export type LifecycleArchiveKind = 'bets' | 'ledger' | 'audit'

export type RetentionPolicyView = {
  id: number
  workspace_id: number
  data_class: LifecycleDataClass
  enabled: boolean
  retention_days: number
  action: LifecycleAction
  updated_by_id: number
  updated_by_name: string
  created_at: string | null
  updated_at: string | null
  inherited: boolean
  description: string
}

export type UpdateRetentionPolicyInput = {
  workspace_id: number
  enabled: boolean
  retention_days: number
}

export type CleanupPreviewInput = {
  request_id: string
  workspace_id?: number | null
  all_workspaces: boolean
  data_classes?: LifecycleDataClass[] | null
  batch_limit?: number
  delete_mode?: LifecycleDeleteMode
}

export type CleanupPreviewItem = {
  data_class: LifecycleDataClass
  action: LifecycleAction
  description: string
  enabled: boolean
  retention_days: number
  cutoff_at: string | null
  eligible_count: number
  planned_count: number
  protected_from_deletion: number
  candidate_fingerprint?: string | null
}

export type CleanupPreview = {
  request_id: string
  workspace_id: number
  all_workspaces: boolean
  batch_limit: number
  delete_mode: LifecycleDeleteMode
  status: string
  items: CleanupPreviewItem[] | null
  created_at: string | null
}

export type CleanupResultItem = {
  data_class: LifecycleDataClass
  action: LifecycleAction
  affected_count: number
  note?: string | null
}

export type CleanupExecution = {
  request_id: string
  workspace_id: number
  all_workspaces: boolean
  delete_mode: LifecycleDeleteMode
  status: string
  items: CleanupResultItem[] | null
  completed_at?: string | null
}

export type CleanupRunView = {
  id: number
  request_id: string
  workspace_id: number
  all_workspaces: boolean
  delete_mode: LifecycleDeleteMode
  actor_id: number
  actor_name: string
  executed_by_id?: number
  executed_by_name?: string | null
  status: string
  batch_limit: number
  preview: CleanupPreviewItem[] | null
  result: CleanupResultItem[] | null
  soft_restore_result: CleanupResultItem[] | null
  financial_restore_result: CleanupResultItem[] | null
  last_error?: string | null
  started_at?: string | null
  completed_at?: string | null
  soft_restored_at?: string | null
  financial_restored_at?: string | null
  soft_restored_by_id?: number
  soft_restored_by_name?: string | null
  financial_restored_by_id?: number
  financial_restored_by_name?: string | null
  content_purged_at?: string | null
  content_purge_count: number
  last_content_purge_request_id?: string | null
  created_at: string | null
}

export type CleanupRunPage = {
  items: CleanupRunView[] | null
  has_more: boolean
  next_before_id?: number | null
}

export type DataMaintenanceSummary = {
  soft_deleted_chat_count: number
  soft_deleted_robot_chat_count: number
  soft_deleted_notification_count: number
  stale_idempotency_count: number
  delivered_session_receipt_count: number
  orphan_chat_cursor_count: number
  protected_bet_count: number
  protected_ledger_count: number
  protected_audit_count: number
  generated_at: string | null
}

export type LifecycleRestoreResult = {
  request_id: string
  workspace_id: number
  all_workspaces: boolean
  kind: string
  items: CleanupResultItem[] | null
  restored_at?: string | null
}

export type LifecycleArchiveRecord = {
  id: number
  workspace_id: number
  user_id: number
  kind: string
  game_id?: string | null
  issue?: string | null
  status?: string | null
  reference?: string | null
  type?: string | null
  amount_cents: number
  created_at?: string | null
  archived_at: string | null
  row_hash: string
}

export type LifecycleArchivePage = {
  items: LifecycleArchiveRecord[] | null
  has_more: boolean
  next_before_id?: number | null
}

export type AdminUser = {
  id: number
  public_id: number
  username: string
  email: string
  nickname: string
  avatar?: string
  public_title?: string
  badge?: string
  phone: string
  role: 'member' | 'agent' | 'tenant' | 'admin'
  remark: string
  risk_level: 'normal' | 'watch' | 'restricted'
  balance: number
  fly_mode?: 'inherit' | 'custom' | 'off' | string
  fly_rate?: number
  agent_room_code?: string
  room_code?: string
  parent_agent_id?: number | null
  parent_tenant_id?: number | null
  agent_name?: string
  tenant_name?: string
  login_identity?: string
  status: 0 | 1
  last_login_at: string | null
  login_count: number
  created_at: string
  updated_at: string
  is_robot?: boolean
	robot_game_ids?: string[]
	robot_active_start?: string
	robot_active_end?: string
	robot_min_bet?: number
	robot_max_bet?: number
	robot_avatar?: string
	workspace_id?: number
}

export type RobotSetting = {
  workspace_id: number
  enabled: boolean
  interval_secs: number
  bets_per_cycle: number
  daily_bet_limit: number
  max_pending_bets: number
  today_bets: number
  pending_bets: number
  pause_reason?: string
  last_run_at?: string | null
  last_error?: string
}

export type RobotResetInput = {
  workspace_id?: number
  request_id: string
  mode: 'random' | 'custom'
  nickname_prefix?: string
  balance?: number
  balance_min?: number
  balance_max?: number
}

export type RobotResetResult = {
  request_id: string
  mode: 'random' | 'custom'
  count: number
  duplicate: boolean
  items: AdminUser[]
}

export type RobotWorkspaceOption = {
  workspace_id: number
  type: 'platform' | 'tenant' | 'agent' | string
  name: string
  room_code: string
  status: number
  robot_count: number
}

export type AgentItem = {
  id: number
  public_id: number
  username: string
  email: string
  nickname: string
  phone: string
  room_code: string
  room_name: string
  room_logo: string
	workspace_id: number
  balance: number
  status: number
  member_count: number
  rebate_rate: number
  profit_share_rate: number
  remark: string
  created_at: string
  last_login_at: string
  login_count: number
  tenant_id?: number | null
  tenant_name?: string
}

export type TenantItem = {
  id: number
  public_id: number
  username: string
  email: string
  nickname: string
  phone: string
	room_code: string
	room_name: string
	room_logo: string
	workspace_id: number
  balance: number
  status: number
  agent_count: number
  member_count: number
  remark: string
  created_at: string
  last_login_at: string
  login_count: number
}

export type TenantListResponse = {
  items: TenantItem[]
  total: number
  page: number
  page_size: number
  active: number
  agents: number
  members: number
}

export type TenantDashboard = {
  tenant_id: number
  tenant_name: string
  agent_count: number
  active_agent_count: number
  member_count: number
}

export type AgentListResponse = {
  items: AgentItem[]
  total: number
  page: number
  page_size: number
  summary: { total: number; active: number; disabled: number; members: number }
}

export type RoomResolve = {
  room_code: string
  room_name: string
  agent_id: number
  agent_username: string
  agent_nickname: string
}

export type UserTradingConfig = {
  user_id: number
  username: string
	odds_multiplier?: number
  fly: { mode: 'inherit' | 'custom' | 'off' | string; rate: number }
  rebate: { mode: 'inherit' | 'custom' | 'off' | string; rate: number; effective: number; source: string }
  game_id: string
  game_name: string
  room_fly_rate: number
  room_rebate_rate: number
  odds: Array<{
    play_code: string
    play_name: string
    base_odds: number
    room_odds: number
    override: number | null
    effective: number
    has_override: boolean
  }>
}

export type RoomTradingConfig = {
  workspace_id: number
  agent_id: number
  room_code: string
  rebate_rate: number
  game_id: string
  game_name: string
  odds: Array<{ play_code: string; play_name: string; base_odds: number; override: number | null; effective: number; has_override: boolean }>
}

export type UserListResponse = {
  items: AdminUser[]
  total: number
  page: number
  page_size: number
}

export type UserStats = {
  total: number
  active: number
  disabled: number
  new_today: number
  administrators: number
  total_balance: number
}

export type UserPayload = {
  username?: string
  password?: string
  email: string
  nickname: string
  phone: string
  role: AdminUser['role']
  remark: string
  risk_level: AdminUser['risk_level']
  status: 0 | 1
  parent_agent_id?: number
}

export type BalanceRecord = {
  id: number
  user_id: number
  amount: number
  before: number
  after: number
  type: string
  remark: string
  operator: string
  created_at: string
}

export type AdminChatConversation = {
  scope: string
  room_scope: string
  game_id: string
  room_type: 'group' | 'service'
  title: string
  subtitle: string
  user_id?: number
  username?: string
  nickname?: string
  latest_text: string
  latest_is_staff: boolean
  latest_message_type?: 'text' | 'redpacket' | string
  latest_at?: string
  latest_message_id?: number
  message_count: number
  unread_count?: number
  pinned?: boolean
  muted_until?: string | null
  group_chat_enabled: boolean
  lobby_category?: string
  enabled: boolean
}

export type LotteryRoomStatus = { agent_id: number; game_id: string; enabled: boolean }

export type AdminChatConversationList = {
  items: AdminChatConversation[]
  total: number
  page: number
  page_size: number
}

export type AdminChatMessage = {
  id: number
  user_id: number
  username: string
  nickname: string
  avatar?: string
  title?: string
  badge?: string
  room_type: 'group' | 'service'
  scope: string
  room_scope: string
  game_id: string
  content: string
  message_type: 'text' | 'redpacket' | string
  reference_id?: number
  red_packet_count?: number
  red_packet_total?: number
  red_packet_min_turnover?: number
  red_packet_cover?: 'classic' | 'celebration' | 'lucky' | string
  red_packet_status?: 'active' | 'empty' | 'expired' | 'closed' | string
  red_packet_funding_status?: 'reserved' | 'partially_released' | 'released' | 'refunded' | 'legacy_unfunded' | string
  red_packet_claimed_count?: number
  red_packet_remaining?: number
  red_packet_refunded?: number
  red_packet_expires_at?: string
  red_packet_closed_at?: string
  red_packet_close_reason?: string
  is_staff: boolean
  created_at: string
}

export type AdminChatMessageList = {
  items: AdminChatMessage[]
  has_more: boolean
  next_before_id?: number
}

export type AdminChatUnreadSummary = {
  items: AdminChatConversation[]
  total_unread: number
}

export type AdminChatReadPayload = {
  scope: string
  room_scope: string
  game_id: string
  through_message_id?: number
}

export type AdminChatReadResult = {
  scope: string
  room_scope: string
  game_id: string
  room_type: 'service'
  last_read_message_id: number
}

export type AdminApplication = {
  id: number
	request_id?: string
	workspace_id: number
  user_id: number
  username: string
	user_balance: number
	balance_before?: number
	balance_after?: number
  account_type: AdminUser['role'] | string
  request_type: 'credit' | 'debit' | 'agent' | 'join'
  target_room_code?: string
	room_code?: string
	room_name?: string
  game_id?: string
	chat_message_id?: number
  payment_type: string
	payment_account_id?: number
	payment_account_label?: string
  requested_amount: number
  received_amount: number
  remark: string
  status: 'pending' | 'approved' | 'rejected'
  operator: string
  review_remark: string
	odds_multiplier?: number
  reviewed_at: string | null
  created_at: string
  updated_at: string
}

export type ApplicationListResponse = {
  items: AdminApplication[]
  total: number
  page: number
  page_size: number
}

export type ApplicationStats = {
  pending: number
	pending_by_category: Record<'wallet' | 'join' | 'entertainment', number>
  approved_today: number
  rejected_today: number
  today_amount: number
}

export type ApplicationPayload = {
  user_id: number
	workspace_id?: number
	request_id?: string
  request_type: AdminApplication['request_type']
  payment_type: string
  game_id?: string
  amount: number
  remark: string
}

export type ReportDefinition = {
  key: string
  title: string
  group: '经营分析' | '财务结算' | '风控会员' | '系统审计' | string
}

export type ReportMetric = { key: string; label: string; value: number }
export type ReportColumn = { key: string; label: string }
export type ReportCenterResult = {
  key: string
  title: string
  period_start: string
  period_end: string
  metrics: ReportMetric[]
  columns: ReportColumn[]
  items: Array<Record<string, unknown>>
  total: number
  page: number
  page_size: number
}

export type ReportCenterParams = {
  query?: string
  start?: string
  end?: string
  workspaceId?: number
  gameId?: string
  category?: string
  issue?: string
  status?: string
  page?: number
  pageSize?: number
}

export type PlanRecommendation = {
  id: number
  workspace_id: number
  game_id: string
  issue: string
  master_name: string
  master_title: string
  master_color: string
  numbers: number[]
  size: '' | '大' | '小'
  parity: '' | '单' | '双'
  result: 'pending' | 'hit' | 'miss'
  note: string
  enabled: boolean
  sort_order: number
  master_hit_rate: number | null
  created_at: string
  updated_at: string
}

export type PlanRecommendationPayload = Omit<PlanRecommendation, 'id' | 'created_at' | 'updated_at' | 'master_hit_rate'>

function reportQuery(params: ReportCenterParams) {
  return new URLSearchParams({
    query: params.query ?? '', start: params.start ?? '', end: params.end ?? '',
    workspace_id: params.workspaceId ? String(params.workspaceId) : '',
    game_id: params.gameId ?? '', category: params.category ?? '', issue: params.issue ?? '',
    status: params.status ?? 'all', page: String(params.page ?? 1), page_size: String(params.pageSize ?? 20),
  })
}

export type FinancialReportSummary = {
  period_start: string
  period_end: string
  total_balance: number
  credit_amount: number
  debit_amount: number
  net_change: number
  finance_credit: number
  finance_debit: number
  betting_credit: number
  betting_debit: number
  welfare_credit: number
  agent_share_credit: number
  record_count: number
  active_users: number
  pending_applications: number
}

export type FinancialReportPoint = {
  date: string
  credit: number
  debit: number
  net: number
  record_count: number
}

export type FinancialRecord = {
  id: number
  user_id: number
  username: string
  nickname: string
  amount: number
  before: number
  after: number
  type: 'manual' | 'application_credit' | 'application_debit' | 'bet' | 'bet_cancel' | 'settlement' | 'rebate' | 'checkin' | 'redpacket' | 'invite' | 'agent_share' | string
  category?: 'finance' | 'betting' | 'welfare' | 'share' | 'other' | string
  remark: string
  operator: string
  created_at: string
}

export type FinancialReport = {
  summary: FinancialReportSummary
  trend: FinancialReportPoint[]
  items: FinancialRecord[]
  total: number
  page: number
  page_size: number
}

export type OperatingReportSummary = {
  period_start: string
  period_end: string
  settled_turnover: number
  payout: number
  member_net: number
  gross_profit: number
  gross_margin: number
  rebate_accrued: number
  welfare_cost: number
  agent_share: number
  platform_net_profit: number
  net_margin: number
  pending_turnover: number
  fly_amount: number
  settled_tickets: number
  pending_tickets: number
  bettors: number
}

export type OperatingReportPoint = {
  date: string
  turnover: number
  payout: number
  gross_profit: number
  rebate: number
  welfare: number
  agent_share: number
  platform_profit: number
}

export type OperatingBreakdown = {
  key: string
  label: string
  turnover: number
  payout: number
  gross_profit: number
  rebate: number
  welfare: number
  agent_share: number
  platform_profit: number
  tickets: number
}

export type OperatingBetRecord = {
  id: number
  room_scope: string
  game_id: string
  game_name: string
  issue: string
  user_id: number
  username: string
  play_name: string
  selection: string
  status: string
  stake: number
  payout: number
  member_net: number
  gross_profit: number
  rebate_rate: number
  rebate: number
  agent_share_rate: number
  agent_share: number
  platform_profit: number
  fly_amount: number
  settled_at?: string
}

export type OperatingReport = {
  summary: OperatingReportSummary
  trend: OperatingReportPoint[]
  breakdown: OperatingBreakdown[]
  items: OperatingBetRecord[]
  total: number
  page: number
  page_size: number
}

export type OperatingReportParams = {
  query?: string
  start?: string
  end?: string
  roomScope?: string
  gameId?: string
  dimension?: 'room' | 'game' | 'user'
  userId?: number
  page?: number
  pageSize?: number
}

export type ProfitShareItem = {
  record_id?: number
  biz_date: string
  agent_id: number
  agent_username: string
  room_code: string
  room_scope: string
  bet_count: number
  turnover: number
  payout: number
  gross_profit: number
  rebate: number
  accrued_share: number
  paid_share: number
  pending_share: number
  status: 'pending' | 'partial' | 'credited' | 'no_share' | string
  last_transaction_id?: number
  last_paid_at?: string
}

export type ProfitShareStatement = {
  biz_date: string
  items: ProfitShareItem[]
  agent_count: number
  total_turnover: number
  total_gross_profit: number
  total_accrued_share: number
  total_paid_share: number
  total_pending_share: number
}

export type ProfitShareRunResult = {
  biz_date: string
  credited_rooms: number
  skipped_rooms: number
  credited: number
  pending: number
}

function operatingQuery(params: OperatingReportParams) {
  const query = new URLSearchParams({
    query: params.query ?? '',
    start: params.start ?? '',
    end: params.end ?? '',
    room_scope: params.roomScope ?? '',
    game_id: params.gameId ?? '',
    dimension: params.dimension ?? 'room',
    page: String(params.page ?? 1),
    page_size: String(params.pageSize ?? 20),
  })
  if (params.userId) query.set('user_id', String(params.userId))
  return query
}

export type SystemSettings = {
  room_name: string
  room_logo: string
  chat_nickname: string
  nickname_display_length: number
  min_chat_score: number
  min_credit_amount: number
  min_debit_amount: number
  room_enabled: boolean
  require_join_review: boolean
  sound_enabled: boolean
  show_odds: boolean
  prediction_enabled: boolean
  abnormal_login_alert: boolean
  security_password_check: boolean
  room_notice: string
  announcements: Array<{
    id: string
    title: string
    content: string
    enabled: boolean
    popup_on_login: boolean
    sort_order: number
  }>
  game: {
    seal_seconds?: number
    allow_cancel?: boolean
    default_fly_rate?: number
    max_open_games?: number
    room_activity_enabled?: boolean
    room_activity_interval_secs?: number
    room_activity_bots_per_room?: number
    room_activity_bets_per_cycle?: number
    room_activity_chat_chance_percent?: number
    show_member_turnover?: boolean
    show_member_profit?: boolean
    show_member_rebate?: boolean
    web_keyboard_enabled?: boolean
    show_mipai_tool?: boolean
    show_orders_tool?: boolean
    show_streak_tool?: boolean
    show_prediction_tool?: boolean
    [key: string]: unknown
  }
  quick_replies: Array<{ title?: string; content?: string; [key: string]: unknown }>
  rebate: {
    enabled?: boolean
    rate_percent?: number
    min_turnover?: number
    settle_mode?: string
    auto_credit?: boolean
    [key: string]: unknown
  }
}

export type RoomActivityStatus = {
  running: boolean
  enabled: boolean
  interval_secs: number
  bots_per_room: number
  bets_per_cycle: number
  chat_chance_percent: number
  target_rooms: number
  enabled_games: number
  bot_accounts: number
  cycles: number
  bets_placed: number
  chats_posted: number
  paused_reason?: string
  last_run_at?: string
  last_error?: string
}

export type OpsActivity = {
  id: number
  type: 'checkin' | 'banner' | 'invite' | 'redpacket' | string
  title: string
  subtitle: string
  status: 'draft' | 'active' | 'ended' | string
  cover: string
  reward: number
  pool_total?: number
  pool_remaining?: number
  config: Record<string, unknown>
  participants: number
  sort_order: number
  starts_at: string | null
  ends_at: string | null
  created_at: string
}

export type SpecialOverview = {
  available: number
  reserved: number
  granted: number
  campaigns: Array<{ id: number; title: string; status: string; rule_text: string; granted_count: number; starts_at: string | null; ends_at: string | null; created_at: string }>
  resources: Array<{ id: number; number: string; level: string; status: string; owner_user_id?: number | null; owner_username?: string; price?: number; remark: string; created_at: string }>
}

export type EntertainmentPlatform = {
  id: number
  code: string
  name: string
  category: string
  merchant_no: string
  api_base: string
  launch_path?: string
  has_secret: boolean
  status: 'enabled' | 'maintenance' | 'disabled' | string
  remark: string
  sort_order: number
}

export type SyncTargetGamesResult = {
  created: string[]
  total: number
  missing: string[]
}

export type AdminNotification = {
  id: number
  title: string
  content: string
  level: 'info' | 'success' | 'warning' | 'error' | string
  link: string
  read: boolean
  created_at: string
}

export type AdminBet = {
  id: number
  game_id: string
  issue: string
  user_id: number
  username: string
  play_code: string
  play_name: string
  position: number
  selection: string
  amount: number
  odds: number
  status: string
  payout: number
  fly_amount: number
  remark: string
  operator: string
  created_at: string
}

export type BetListResponse = {
  items: AdminBet[]
  total: number
  page: number
  page_size: number
}

export type RebatePreview = {
  biz_date: string
  enabled: boolean
  rate_percent: number
  estimated: number
  credited: number
  pending_credit: number
}

export type PlayLimitItem = {
  play_code: string
  play_name: string
  odds: number
  min_bet: number
  max_bet: number
  max_user_period: number
  max_period_total: number
  sort_order: number
}

export type PlayCatalogItem = {
  play_code: string
  play_name: string
  category: string
  description: string
  example: string
  default_odds: number
  sort_order: number
}

export type SyncOddsLimitsResult = {
  game_count: number
  seeded_games: string[]
}

export type GameOddsLimits = {
  game_id: string
  game_name: string
  items: PlayLimitItem[]
}

export type PaymentChannel = {
  id: number
  provider: string
  name: string
  merchant_no: string
  credit_type: string
  fee_rate: number
  min_amount: number
  max_amount: number
  status: 'enabled' | 'disabled'
  remark: string
  sort_order: number
  mode: 'manual' | 'gateway'
  api_base: string
  create_order_path: string
  query_order_path: string
  callback_path: string
  has_secret: boolean
  timeout_seconds: number
}

export type PaymentChannelPayload = {
  provider: string
  name: string
  merchant_no: string
  credit_type: string
  fee_rate: number
  min_amount: number
  max_amount: number
  status: PaymentChannel['status']
  remark: string
  sort_order: number
  mode: PaymentChannel['mode']
  api_base: string
  create_order_path: string
  query_order_path: string
  callback_path: string
  secret_key: string
  timeout_seconds: number
}

export type MonitorSnapshot = {
  game_id: string
  game_name: string
  issue: string
  total_amount: number
  bettor_count: number
  bet_count: number
  next_draw_at: string
  draw_at_label: string
  matrix: number[][]
  updated_at: string
  settlement?: SettlementStatus
}

export type SettlementStatus = {
  game_id: string
  issue: string
  has_draw: boolean
  numbers: number[]
  draw_at: string | null
  pending: number
  won: number
  lost: number
  stake_amount: number
  payout_amount: number
  settled: boolean
}

export type SettlementResult = {
  game_id: string
  game_name: string
  issue: string
  numbers: number[]
  pending_before: number
  won: number
  lost: number
  skipped: number
  stake_amount: number
  payout_amount: number
  settled_at: string
}

export type BoardReportRow = {
  game_id: string
  game_name: string
  issue: string
  bet_count: number
  total_amount: number
  fly_amount: number
  status: string
  draw_at: string | null
  draw_result: string
}

export type BoardReport = {
  items: BoardReportRow[]
  total: number
  page: number
  page_size: number
}

type ApiResponse<T> = { code: number; message: string; data: T }
const apiBase = (() => {
  const configured = String(import.meta.env.VITE_API_BASE_URL ?? '').trim()
  if (configured) return configured.replace(/\/$/, '')
  if (import.meta.env.DEV) return `${window.location.protocol}//${window.location.hostname}:8080/api`
  return `${window.location.origin}/api`
})()
const healthBase = apiBase.replace(/\/api\/?$/, '')
const memberAssetBase = (() => {
  const configured = String(import.meta.env.VITE_MEMBER_ASSET_BASE_URL ?? '').trim()
  if (configured) return configured.replace(/\/$/, '')
  if (window.location.port === '5174') return `${window.location.protocol}//${window.location.hostname}:5173`
  return window.location.origin
})()

export function resolveApiAsset(value: string) {
  const path = String(value || '').trim()
  if (!path) return ''
  if (/^(https?:|data:|blob:)/i.test(path)) return path
  if (path.startsWith('/api/')) return `${healthBase}${path}`
  if (path.startsWith('/images/')) return `${memberAssetBase}${path}`
  return path
}

export class AuthError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'AuthError'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const isFormData = init?.body instanceof FormData
  const controller = init?.signal ? undefined : new AbortController()
  const timeout = controller ? window.setTimeout(() => controller.abort(), 15_000) : undefined
  let response: Response
  try {
    response = await fetch(`${apiBase}${path}`, {
      ...init,
	  credentials: 'include',
      signal: init?.signal ?? controller?.signal,
      headers: { ...(isFormData ? {} : { 'Content-Type': 'application/json' }), ...init?.headers },
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw new Error('请求超时，请稍后重试')
    throw new Error('无法连接服务器，请检查后端服务和网络')
  } finally {
    if (timeout !== undefined) window.clearTimeout(timeout)
  }
  const raw = await response.text()
  let body: ApiResponse<T>
  try {
    body = JSON.parse(raw) as ApiResponse<T>
  } catch {
    if (response.status === 401) {
	  broadcastAdminLogout()
      window.dispatchEvent(new CustomEvent('yaotu-auth-expired'))
      throw new AuthError('登录状态已失效，请重新登录')
    }
    throw new Error('服务返回了无效响应')
  }
  if (response.status === 401) {
	broadcastAdminLogout()
    window.dispatchEvent(new CustomEvent('yaotu-auth-expired'))
    throw new AuthError(body.message || '请先登录')
  }
  if (!response.ok) throw new Error(body.message || '服务暂时不可用')
  return body.data
}

async function downloadAuthenticated(path: string, filename: string) {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), 30_000)
  try {
	const response = await fetch(`${apiBase}${path}`, { credentials: 'include', signal: controller.signal })
    if (!response.ok) throw new Error('导出报表失败')
    const link = document.createElement('a')
    link.href = URL.createObjectURL(await response.blob())
    link.download = filename
    link.click()
    URL.revokeObjectURL(link.href)
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw new Error('导出超时，请缩小查询范围后重试')
    throw error
  } finally {
    window.clearTimeout(timeout)
  }
}

export type ManagementWsEvent = {
  event_id?: string
  type: string
  workspace_id?: number
  room_scope?: string
  game_id?: string
  issue?: string
  server_at?: string
  data: Record<string, unknown>
}

export const realtimeApi = {
  ticket: (role: string) => request<{ ticket: string; expires_at: string }>(role === 'agent' ? '/agent/ws-ticket' : role === 'tenant' ? '/tenant/ws-ticket' : '/admin/ws-ticket', { method: 'POST' }),
}

export function managementWebSocketURL(ticket: string) {
  const base = apiBase.startsWith('https') ? apiBase.replace(/^https/i, 'wss') : apiBase.replace(/^http/i, 'ws')
  return `${base}/ws?ticket=${encodeURIComponent(ticket)}`
}

export type LoginResult = {
	token?: string
  user: {
    id: number
    username: string
    email: string
    nickname: string
    role: string
    status: number
  }
}

export type AgentDashboard = {
  agent_id: number
  room_code: string
  room_name: string
  room_logo: string
  member_count: number
  active_member_count: number
  member_balance: number
  today_stake: number
  today_payout: number
  today_net: number
  pending_bets: number
  pending_applications: number
}

export const adminApi = {
  health: async () => {
    const response = await fetch(`${healthBase}/health`)
    if (!response.ok) throw new Error('后端离线')
    return true
  },
  login: (username: string, password: string, workspace = '') => request<LoginResult>('/login', { method: 'POST', body: JSON.stringify({ username, password, workspace }) }),
	me: () => request<LoginResult['user']>('/session'),
	refreshSession: () => request<{ expires_in: number }>('/session/refresh', { method: 'POST' }),
	logout: async () => {
	  try { await request<null>('/logout', { method: 'POST' }) } finally { clearLegacyAdminSession() }
	},
  dashboard: () => request<DashboardData>('/admin/dashboard'),
  auditLogs: (beforeId?: number, limit = 50) => request<AuditLogPage>(`/admin/audit-logs?limit=${limit}${beforeId ? `&before_id=${beforeId}` : ''}`),
  reconciliation: () => request<ReconciliationSummary>('/admin/reconciliation'),
  refundAbnormalBet: (betId: number) => request<ReconciliationRefundResult>(`/admin/reconciliation/bets/${encodeURIComponent(String(betId))}/refund`, { method: 'POST' }),
  retentionPolicies: (workspaceId = 0) => request<RetentionPolicyView[] | null>(`/admin/data-lifecycle/policies?workspace_id=${encodeURIComponent(String(workspaceId))}`),
  updateRetentionPolicy: (dataClass: LifecycleDataClass, payload: UpdateRetentionPolicyInput) =>
    request<RetentionPolicyView>(`/admin/data-lifecycle/policies/${encodeURIComponent(dataClass)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  dataMaintenanceSummary: () => request<DataMaintenanceSummary>('/admin/data-lifecycle/summary'),
  previewDataCleanup: (payload: CleanupPreviewInput) =>
    request<CleanupPreview>('/admin/data-lifecycle/preview', { method: 'POST', body: JSON.stringify(payload) }),
  executeDataCleanup: (requestId: string) =>
    request<CleanupExecution>('/admin/data-lifecycle/execute', { method: 'POST', body: JSON.stringify({ request_id: requestId }) }),
  dataCleanupRuns: (params?: { beforeId?: number; limit?: number; workspaceId?: number }) => {
    const query = new URLSearchParams({ limit: String(params?.limit ?? 30) })
    if (params?.beforeId) query.set('before_id', String(params.beforeId))
    if (params?.workspaceId) query.set('workspace_id', String(params.workspaceId))
    return request<CleanupRunPage>(`/admin/data-lifecycle/runs?${query}`)
  },
  dataCleanupRun: (requestId: string) =>
    request<CleanupRunView>(`/admin/data-lifecycle/runs/${encodeURIComponent(requestId)}`),
  dataCleanupArchives: (requestId: string, kind: LifecycleArchiveKind, beforeId?: number, limit = 50) => {
    const query = new URLSearchParams({ kind, limit: String(limit) })
    if (beforeId) query.set('before_id', String(beforeId))
    return request<LifecycleArchivePage>(`/admin/data-lifecycle/runs/${encodeURIComponent(requestId)}/archives?${query}`)
  },
  restoreSoftDeleted: (requestId: string) =>
    request<LifecycleRestoreResult>(`/admin/data-lifecycle/runs/${encodeURIComponent(requestId)}/restore-soft-deleted`, { method: 'POST' }),
  restoreRobotArchive: (requestId: string) =>
    request<LifecycleRestoreResult>(`/admin/data-lifecycle/runs/${encodeURIComponent(requestId)}/restore-robot-archive`, { method: 'POST' }),
  games: () => request<AdminGame[]>('/admin/games'),
  gameCategories: () => request<GameCategory[]>('/admin/game-categories'),
  createGameCategory: (payload: { name: string; sort_order: number }) => request<GameCategory>('/admin/game-categories', { method: 'POST', body: JSON.stringify(payload) }),
  updateGameCategory: (id: number, payload: { name: string; sort_order: number }) => request<GameCategory>(`/admin/game-categories/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGameCategory: (id: number) => request<{ id: number }>(`/admin/game-categories/${id}`, { method: 'DELETE' }),
  assignGameCategory: (id: string, payload: { category: string; sort_order: number }) => request<AdminGame>(`/admin/games/${id}/category`, { method: 'PATCH', body: JSON.stringify(payload) }),
  syncTargetGames: () => request<SyncTargetGamesResult>('/admin/games/sync-target', { method: 'POST' }),
  draws: (id: string) => request<DrawResult[]>(`/admin/games/${id}/draws?limit=30`),
  clock: () => request<ServerClock>('/public/clock'),
  feedStatus: () => request<FeedStatus>('/public/lottery/status'),
  users: (params: { query?: string; status?: string; role?: string; kind?: 'member' | 'account' | 'robot'; workspaceId?: number; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({
      query: params.query ?? '',
      status: params.status ?? 'all',
      role: params.role ?? 'all',
		kind: params.kind ?? '',
		workspace_id: params.workspaceId ? String(params.workspaceId) : '',
      page: String(params.page ?? 1),
      page_size: String(params.pageSize ?? 20),
    })
    return request<UserListResponse>(`/admin/users?${query}`)
  },
  userStats: (kind?: 'member' | 'account' | 'robot') => request<UserStats>(`/admin/users/stats${kind ? `?kind=${kind}` : ''}`),
  user: (id: number) => request<AdminUser>(`/admin/users/${id}`),
  createUser: (payload: UserPayload) => request<AdminUser>('/admin/users', { method: 'POST', body: JSON.stringify(payload) }),
  updateUser: (id: number, payload: UserPayload) => request<AdminUser>(`/admin/users/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
	updateRobot: (id: number, payload: { nickname: string; avatar?: string; status: 0 | 1; game_ids: string[]; active_start: string; active_end: string; min_bet: number; max_bet: number }) => request<AdminUser>(`/admin/robots/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
	resetRobots: (payload: RobotResetInput) => request<RobotResetResult>('/admin/robots/reset', { method: 'POST', headers: { 'Idempotency-Key': payload.request_id }, body: JSON.stringify(payload) }),
	robotWorkspaces: () => request<RobotWorkspaceOption[]>('/admin/robot-workspaces'),
	robotWorkspaceGames: (workspaceId: number) => request<WorkspaceGame[]>(`/admin/robot-workspaces/${encodeURIComponent(String(workspaceId))}/games`),
	robotSetting: (workspaceId: number) => request<RobotSetting>(`/admin/robot-settings?workspace_id=${encodeURIComponent(String(workspaceId))}`),
	updateRobotSetting: (workspaceId: number, payload: Partial<Pick<RobotSetting, 'enabled' | 'interval_secs' | 'bets_per_cycle' | 'daily_bet_limit' | 'max_pending_bets'>>) => request<RobotSetting>(`/admin/robot-settings?workspace_id=${encodeURIComponent(String(workspaceId))}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  setUserStatus: (id: number, status: 0 | 1) => request<AdminUser>(`/admin/users/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  resetUserPassword: (id: number, password: string) => request<{ id: number }>(`/admin/users/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ password }) }),
  adjustUserBalance: (id: number, amount: number, remark: string) => request<AdminUser>(`/admin/users/${id}/balance`, { method: 'POST', body: JSON.stringify({ amount, remark }) }),
  userBalanceHistory: (id: number) => request<BalanceRecord[]>(`/admin/users/${id}/balance-history?limit=20`),
  userTrading: (id: number, gameId?: string) => {
    const query = gameId ? `?game_id=${encodeURIComponent(gameId)}` : ''
    return request<UserTradingConfig>(`/admin/users/${id}/trading${query}`)
  },
  updateUserTrading: (id: number, payload: {
		odds_multiplier?: number
    fly_mode: string
    fly_rate: number
    rebate_mode: string
    rebate_rate: number
    game_id: string
    odds: Array<{ play_code: string; override: number | null }>
  }) => request<UserTradingConfig>(`/admin/users/${id}/trading`, { method: 'PUT', body: JSON.stringify(payload) }),
  applications: (params: { query?: string; status?: string; type?: string; date?: string; start?: string; end?: string; workspaceId?: number; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({
      query: params.query ?? '',
      status: params.status ?? 'all',
      type: params.type ?? 'all',
      date: params.date ?? '',
	  start: params.start ?? '',
	  end: params.end ?? '',
	  workspace_id: params.workspaceId ? String(params.workspaceId) : '',
      page: String(params.page ?? 1),
      page_size: String(params.pageSize ?? 20),
    })
    return request<ApplicationListResponse>(`/admin/applications?${query}`)
  },
  applicationStats: (workspaceId?: number) => request<ApplicationStats>(`/admin/applications/stats${workspaceId ? `?workspace_id=${workspaceId}` : ''}`),
  application: (id: number) => request<AdminApplication>(`/admin/applications/${id}`),
  createApplication: (payload: ApplicationPayload) => request<AdminApplication>('/admin/applications', { method: 'POST', body: JSON.stringify({ ...payload, request_id: payload.request_id ?? createRequestId() }) }),
  reviewApplication: (id: number, payload: { decision: 'approved' | 'rejected'; received_amount: number; odds_multiplier?: number; remark: string }) => request<AdminApplication>(`/admin/applications/${id}/review`, { method: 'POST', body: JSON.stringify(payload) }),
  financialReport: (params: { query?: string; type?: string; start?: string; end?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({
      query: params.query ?? '',
      type: params.type ?? 'all',
      start: params.start ?? '',
      end: params.end ?? '',
      page: String(params.page ?? 1),
      page_size: String(params.pageSize ?? 20),
    })
    return request<FinancialReport>(`/admin/reports/financial?${query}`)
  },
	reportCatalog: () => request<ReportDefinition[]>('/admin/reports/catalog'),
	reportCenter: (key: string, params: ReportCenterParams) => request<ReportCenterResult>(`/admin/reports/${encodeURIComponent(key)}?${reportQuery(params)}`),
	exportReport: (key: string, params: ReportCenterParams) => downloadAuthenticated(`/admin/reports/${encodeURIComponent(key)}?${reportQuery(params)}&format=csv`, `${key}-${params.start || 'report'}-${params.end || 'report'}.csv`),
  operatingReport: (params: OperatingReportParams) => request<OperatingReport>(`/admin/reports/operating?${operatingQuery(params)}`),
  profitShares: (date = '') => request<ProfitShareStatement>(`/admin/reports/profit-shares${date ? `?date=${encodeURIComponent(date)}` : ''}`),
  runProfitShares: (date: string) => request<ProfitShareRunResult>('/admin/reports/profit-shares/run', { method: 'POST', body: JSON.stringify({ date }) }),
  syncOfficialSources: () => request<OfficialSyncResponse>('/admin/sources/sync', { method: 'POST' }),
  testOfficialSource: (group: string) => request<OfficialSourceTestResponse>(`/admin/sources/${encodeURIComponent(group)}/test`, { method: 'POST' }),
  updateGameStatus: (id: string, enabled: boolean) => request<AdminGame>(`/admin/games/${id}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
  settings: () => request<SystemSettings>('/admin/settings'),
  updateSettings: (payload: SystemSettings) => request<SystemSettings>('/admin/settings', { method: 'PUT', body: JSON.stringify(payload) }),
  roomActivityStatus: () => request<RoomActivityStatus>('/admin/room-activity/status'),
  runRoomActivityOnce: () => request<RoomActivityStatus>('/admin/room-activity/run-once', { method: 'POST' }),
  oddsLimits: (gameId: string) => request<GameOddsLimits>(`/admin/games/${gameId}/odds-limits`),
  updateOddsLimits: (gameId: string, items: PlayLimitItem[]) => request<GameOddsLimits>(`/admin/games/${gameId}/odds-limits`, { method: 'PUT', body: JSON.stringify({ items }) }),
  playCatalog: () => request<PlayCatalogItem[]>('/admin/plays/catalog'),
  resetOddsLimits: (gameId: string) => request<GameOddsLimits>(`/admin/games/${gameId}/odds-limits/reset`, { method: 'POST' }),
  syncOddsLimits: () => request<SyncOddsLimitsResult>('/admin/games/sync-odds-limits', { method: 'POST' }),
  walletChannels: (params?: { query?: string; status?: string }) => {
    const query = new URLSearchParams({
      query: params?.query ?? '',
      status: params?.status ?? 'all',
    })
    return request<PaymentChannel[]>(`/admin/wallet/channels?${query}`)
  },
  createWalletChannel: (payload: PaymentChannelPayload) => request<PaymentChannel>('/admin/wallet/channels', { method: 'POST', body: JSON.stringify(payload) }),
  updateWalletChannel: (id: number, payload: PaymentChannelPayload) => request<PaymentChannel>(`/admin/wallet/channels/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  setWalletChannelStatus: (id: number, status: PaymentChannel['status']) => request<PaymentChannel>(`/admin/wallet/channels/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  deleteWalletChannel: (id: number) => request<{ id: number }>(`/admin/wallet/channels/${id}`, { method: 'DELETE' }),
  monitor: (gameId?: string, issue?: string) => {
    const query = new URLSearchParams()
    if (gameId) query.set('game_id', gameId)
    if (issue) query.set('issue', issue)
    const suffix = query.toString() ? `?${query}` : ''
    return request<MonitorSnapshot>(`/admin/monitor${suffix}`)
  },
  placeBet: (payload: { game_id: string; issue?: string; user_id: number; play_code?: string; play_name?: string; position: number; selection: string; amount: number; odds?: number; fly_amount?: number; remark?: string }) =>
    request('/admin/bets', { method: 'POST', body: JSON.stringify(payload) }),
  publishDraw: (gameId: string, payload?: { issue?: string; numbers?: number[] }) =>
    request<SettlementResult>(`/admin/games/${gameId}/publish-draw`, { method: 'POST', body: JSON.stringify(payload ?? {}) }),
  settleIssue: (gameId: string, issue: string) =>
    request<SettlementResult>(`/admin/games/${gameId}/issues/${encodeURIComponent(issue)}/settle`, { method: 'POST' }),
  settlementStatus: (gameId: string, issue: string) =>
    request<SettlementStatus>(`/admin/games/${gameId}/issues/${encodeURIComponent(issue)}/settlement`),
  boardReport: (params?: { query?: string; gameId?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({
      query: params?.query ?? '',
      game_id: params?.gameId ?? 'all',
      page: String(params?.page ?? 1),
      page_size: String(params?.pageSize ?? 20),
    })
    return request<BoardReport>(`/admin/reports/board?${query}`)
  },
  bets: (params?: { query?: string; gameId?: string; issue?: string; userId?: number; status?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({
      query: params?.query ?? '',
      game_id: params?.gameId ?? 'all',
      issue: params?.issue ?? '',
      status: params?.status ?? 'all',
      page: String(params?.page ?? 1),
      page_size: String(params?.pageSize ?? 20),
    })
    if (params?.userId) query.set('user_id', String(params.userId))
    return request<BetListResponse>(`/admin/bets?${query}`)
  },
  cancelBet: (id: number) => request<AdminBet>(`/admin/bets/${id}/cancel`, { method: 'POST' }),
  activities: (status = 'all') => request<OpsActivity[]>(`/admin/activities?status=${encodeURIComponent(status)}`),
  createActivity: (payload: Partial<OpsActivity> & { type: string; title: string }) => request<OpsActivity>('/admin/activities', { method: 'POST', body: JSON.stringify(payload) }),
  updateActivity: (id: number, payload: Partial<OpsActivity> & { type: string; title: string }) => request<OpsActivity>(`/admin/activities/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  setActivityStatus: (id: number, status: string) => request<OpsActivity>(`/admin/activities/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  deleteActivity: (id: number) => request<{ id: number }>(`/admin/activities/${id}`, { method: 'DELETE' }),
  plans: (workspaceId: number) => request<PlanRecommendation[]>(`/admin/plans?workspace_id=${workspaceId}`),
  createPlan: (payload: PlanRecommendationPayload) => request<PlanRecommendation>('/admin/plans', { method: 'POST', body: JSON.stringify(payload) }),
  updatePlan: (id: number, payload: PlanRecommendationPayload) => request<PlanRecommendation>(`/admin/plans/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deletePlan: (id: number, workspaceId: number) => request<{ id: number }>(`/admin/plans/${id}?workspace_id=${workspaceId}`, { method: 'DELETE' }),
  uploadActivityImage: (file: File) => {
    const body = new FormData()
    body.append('file', file)
    return request<{ url: string }>('/admin/activities/upload', { method: 'POST', body })
  },
  specialOverview: () => request<SpecialOverview>('/admin/special-numbers'),
  addSpecialNumbers: (payload: { numbers: string; level?: string; remark?: string }) => request<{ created: number }>('/admin/special-numbers/resources', { method: 'POST', body: JSON.stringify(payload) }),
  createSpecialCampaign: (payload: { title: string; rule_text?: string; status?: string }) => request('/admin/special-numbers/campaigns', { method: 'POST', body: JSON.stringify(payload) }),
  grantSpecialNumber: (payload: { campaign_id: number; resource_id: number; user_id: number }) => request('/admin/special-numbers/grant', { method: 'POST', body: JSON.stringify(payload) }),
  assignAgentRoom: (payload: { resource_id: number; user_id: number }) => request('/admin/special-numbers/assign', { method: 'POST', body: JSON.stringify(payload) }),
  agents: (params?: { query?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({
      query: params?.query ?? '',
      page: String(params?.page ?? 1),
      page_size: String(params?.pageSize ?? 20),
    })
    return request<AgentListResponse>(`/admin/agents?${query}`)
  },
  tenants: (params?: { query?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    return request<TenantListResponse>(`/admin/tenants?${query}`)
  },
  createTenant: (payload: { username: string; password: string; email?: string; nickname?: string; phone?: string; room_code?: string; room_name?: string; room_logo?: string; remark?: string; status: number }) => request<TenantItem>('/admin/tenants', { method: 'POST', body: JSON.stringify(payload) }),
  updateTenant: (id: number, payload: { email?: string; nickname?: string; phone?: string; room_code?: string; room_name?: string; room_logo?: string; remark?: string; status: number }) => request<TenantItem>(`/admin/tenants/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  resetTenantPassword: (id: number, password: string) => request<{ id: number }>(`/admin/tenants/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ password }) }),
  tenantRoomTrading: (id: number, gameId?: string) => request<RoomTradingConfig>(`/admin/tenants/${id}/trading${gameId ? `?game_id=${encodeURIComponent(gameId)}` : ''}`),
  updateTenantRoomTrading: (id: number, payload: { rebate_rate: number; game_id: string; odds: Array<{ play_code: string; override: number | null }> }) => request<RoomTradingConfig>(`/admin/tenants/${id}/trading`, { method: 'PUT', body: JSON.stringify(payload) }),
  tenantRoomSettings: (id: number) => request<SystemSettings>(`/admin/tenants/${id}/settings`),
  updateTenantRoomSettings: (id: number, payload: SystemSettings) => request<SystemSettings>(`/admin/tenants/${id}/settings`, { method: 'PUT', body: JSON.stringify(payload) }),
  tenantRoomGames: (id: number) => request<WorkspaceGame[]>(`/admin/tenants/${id}/games`),
  setTenantRoomGameStatus: (id: number, gameId: string, enabled: boolean) => request<LotteryRoomStatus>(`/admin/tenants/${id}/games/${encodeURIComponent(gameId)}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
  createAgent: (payload: { username: string; password: string; email?: string; nickname?: string; phone?: string; room_code: string; room_name?: string; room_logo?: string; rebate_rate?: number; profit_share_rate?: number; remark?: string; status: number; tenant_id?: number }) => request<AgentItem>('/admin/agents', { method: 'POST', body: JSON.stringify(payload) }),
  updateAgent: (id: number, payload: { email?: string; nickname?: string; phone?: string; room_code: string; room_name?: string; room_logo?: string; rebate_rate?: number; profit_share_rate?: number; remark?: string; status: number; tenant_id?: number }) => request<AgentItem>(`/admin/agents/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  roomTrading: (id: number, gameId?: string) => request<RoomTradingConfig>(`/admin/agents/${id}/trading${gameId ? `?game_id=${encodeURIComponent(gameId)}` : ''}`),
  updateRoomTrading: (id: number, payload: { rebate_rate: number; game_id: string; odds: Array<{ play_code: string; override: number | null }> }) => request<RoomTradingConfig>(`/admin/agents/${id}/trading`, { method: 'PUT', body: JSON.stringify(payload) }),
  agentRoomSettings: (id: number) => request<SystemSettings>(`/admin/agents/${id}/settings`),
  updateAgentRoomSettings: (id: number, payload: SystemSettings) => request<SystemSettings>(`/admin/agents/${id}/settings`, { method: 'PUT', body: JSON.stringify(payload) }),
  agentRoomGames: (id: number) => request<WorkspaceGame[]>(`/admin/agents/${id}/games`),
  setAgentRoomGameStatus: (id: number, gameId: string, enabled: boolean) => request<LotteryRoomStatus>(`/admin/agents/${id}/games/${encodeURIComponent(gameId)}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
  resetAgentPassword: (id: number, password: string) => request<{ id: number }>(`/admin/agents/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ password }) }),
  promoteAgent: (id: number, roomCode?: string) => request<AgentItem>(`/admin/agents/${id}/promote`, { method: 'POST', body: JSON.stringify({ room_code: roomCode ?? '' }) }),
  entertainment: () => request<EntertainmentPlatform[]>('/admin/entertainment'),
  upsertEntertainment: (payload: Partial<EntertainmentPlatform> & { code: string; name: string }) => request<EntertainmentPlatform>('/admin/entertainment', { method: 'POST', body: JSON.stringify(payload) }),
  setEntertainmentStatus: (id: number, status: string) => request<EntertainmentPlatform>(`/admin/entertainment/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  notifications: (limit = 20) => request<AdminNotification[]>(`/admin/notifications?limit=${limit}`),
  markNotificationRead: (id: number) => request(`/admin/notifications/${id}/read`, { method: 'POST' }),
  markAllNotificationsRead: () => request('/admin/notifications/read-all', { method: 'POST' }),
  chatConversations: (params: { roomType?: string; roomScope?: string; channel?: 'service' | 'room' | 'lottery'; query?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ room_type: params.roomType ?? '', room_scope: params.roomScope ?? '', channel: params.channel ?? '', query: params.query ?? '', page: String(params.page ?? 1), page_size: String(params.pageSize ?? 30) })
    return request<AdminChatConversationList>(`/admin/chat/conversations?${query}`)
  },
  chatMessages: (params: { scope: string; roomScope: string; gameId: string; roomType: string; beforeId?: number; limit?: number }) => {
    const query = new URLSearchParams({ scope: params.scope, room_scope: params.roomScope, game_id: params.gameId, room_type: params.roomType, limit: String(params.limit ?? 50) })
    if (params.beforeId) query.set('before_id', String(params.beforeId))
    return request<AdminChatMessageList>(`/admin/chat/messages?${query}`)
  },
  chatUnread: (limit = 30) => request<AdminChatUnreadSummary>(`/admin/chat/unread?limit=${limit}`),
  markChatRead: (payload: AdminChatReadPayload) => request<AdminChatReadResult>('/admin/chat/read', { method: 'POST', body: JSON.stringify(payload) }),
  replyChat: (payload: { scope: string; room_scope: string; game_id: string; room_type: string; content: string }) => request<AdminChatMessage>('/admin/chat/messages', { method: 'POST', body: JSON.stringify(payload) }),
  sendChatRedPacket: (payload: { request_id: string; scope: string; room_scope: string; game_id: string; count: number; total_amount: number; min_daily_turnover?: number; greeting?: string; cover?: string }) => request<AdminChatMessage>('/admin/chat/redpackets', { method: 'POST', body: JSON.stringify(payload) }),
  deleteChatMessage: (id: number) => request<{ id: number }>(`/admin/chat/messages/${id}`, { method: 'DELETE' }),
  setChatMute: (userId: number, payload: { minutes: number; reason?: string }) => request<{ user_id: number; muted_until?: string | null; mute_reason?: string }>(`/admin/chat/users/${userId}/mute`, { method: 'PATCH', body: JSON.stringify(payload) }),
  setRoomGroupChat: (agentId: number, enabled: boolean) => request<{ agent_id: number; group_chat_enabled: boolean }>(`/admin/chat/rooms/${agentId}/group-chat`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
  setChatAnnouncement: (content: string) => request<{ content: string }>('/admin/chat/announcement', { method: 'PUT', body: JSON.stringify({ content }) }),
  setLotteryRoomStatus: (roomScope: string, gameId: string, enabled: boolean) => request<LotteryRoomStatus>('/admin/chat/lottery-rooms/status', { method: 'PATCH', body: JSON.stringify({ room_scope: roomScope, game_id: gameId, enabled }) }),
  rebatePreview: () => request<RebatePreview>('/admin/rebates/preview'),
  runRebate: () => request('/admin/rebates/run', { method: 'POST' }),
}

export const tenantApi = {
  dashboard: () => request<TenantDashboard>('/tenant/dashboard'),
	menuTemplate: () => request<unknown>('/tenant/menu-template'),
	roomDashboard: () => request<AgentDashboard>('/tenant/room/dashboard'),
	games: () => request<WorkspaceGame[]>('/tenant/games'),
	setGameStatus: (gameId: string, enabled: boolean) => request<LotteryRoomStatus>(`/tenant/games/${encodeURIComponent(gameId)}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
	trading: (gameId?: string) => request<RoomTradingConfig>(`/tenant/trading${gameId ? `?game_id=${encodeURIComponent(gameId)}` : ''}`),
	updateTrading: (payload: { rebate_rate: number; game_id: string; odds: Array<{ play_code: string; override: number | null }> }) => request<RoomTradingConfig>('/tenant/trading', { method: 'PUT', body: JSON.stringify(payload) }),
	updateDirectRoomSettings: (roomName: string, roomLogo: string) => request<AgentDashboard>('/tenant/room/settings', { method: 'PATCH', body: JSON.stringify({ room_name: roomName, room_logo: roomLogo }) }),
  agents: (params?: { query?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    return request<AgentListResponse>(`/tenant/agents?${query}`)
  },
  createAgent: (payload: { username: string; password: string; email?: string; nickname?: string; phone?: string; room_code: string; room_name?: string; room_logo?: string; rebate_rate?: number; profit_share_rate?: number; remark?: string; status: number }) => request<AgentItem>('/tenant/agents', { method: 'POST', body: JSON.stringify(payload) }),
  updateAgent: (id: number, payload: { email?: string; nickname?: string; phone?: string; room_code: string; room_name?: string; room_logo?: string; rebate_rate?: number; profit_share_rate?: number; remark?: string; status: number }) => request<AgentItem>(`/tenant/agents/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  resetAgentPassword: (id: number, password: string) => request<{ id: number }>(`/tenant/agents/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ password }) }),
	users: (params?: { query?: string; status?: string; page?: number; pageSize?: number }) => {
		const query = new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
		return request<UserListResponse>(`/tenant/users?${query}`)
	},
	setUserStatus: (id: number, status: 0 | 1) => request<AdminUser>(`/tenant/users/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
	adjustUserBalance: (id: number, amount: number, remark: string) => request<AdminUser>(`/tenant/users/${id}/balance`, { method: 'POST', body: JSON.stringify({ amount, remark }) }),
	userTrading: (id: number, gameId?: string) => request<UserTradingConfig>(`/tenant/users/${id}/trading${gameId ? `?game_id=${encodeURIComponent(gameId)}` : ''}`),
	updateUserTrading: (id: number, payload: { odds_multiplier?: number; fly_mode: string; fly_rate: number; rebate_mode: string; rebate_rate: number; game_id: string; odds: Array<{ play_code: string; override: number | null }> }) => request<UserTradingConfig>(`/tenant/users/${id}/trading`, { method: 'PUT', body: JSON.stringify(payload) }),
	bets: (params?: { query?: string; gameId?: string; issue?: string; userId?: number; status?: string; page?: number; pageSize?: number }) => {
		const query = new URLSearchParams({ query: params?.query ?? '', game_id: params?.gameId ?? 'all', issue: params?.issue ?? '', status: params?.status ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
		if (params?.userId) query.set('user_id', String(params.userId))
		return request<BetListResponse>(`/tenant/bets?${query}`)
	},
	applications: (params?: { query?: string; status?: string; type?: string; start?: string; end?: string; page?: number; pageSize?: number }) => {
		const query = new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', type: params?.type ?? 'all', start: params?.start ?? '', end: params?.end ?? '', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
		return request<ApplicationListResponse>(`/tenant/applications?${query}`)
	},
	applicationStats: () => request<ApplicationStats>('/tenant/applications/stats'),
	reviewApplication: (id: number, payload: { decision: 'approved' | 'rejected'; received_amount: number; odds_multiplier?: number; remark: string }) => request<AdminApplication>(`/tenant/applications/${id}/review`, { method: 'POST', body: JSON.stringify(payload) }),
	chatConversations: (params: { roomType?: string; channel?: 'service' | 'room' | 'lottery'; query?: string; page?: number; pageSize?: number }) => {
		const query = new URLSearchParams({ room_type: params.roomType ?? '', channel: params.channel ?? '', query: params.query ?? '', page: String(params.page ?? 1), page_size: String(params.pageSize ?? 30) })
		return request<AdminChatConversationList>(`/tenant/chat/conversations?${query}`)
	},
	chatMessages: (params: { scope: string; roomScope: string; gameId: string; roomType: string; beforeId?: number; limit?: number }) => {
		const query = new URLSearchParams({ scope: params.scope, room_scope: params.roomScope, game_id: params.gameId, room_type: params.roomType, limit: String(params.limit ?? 50) })
		if (params.beforeId) query.set('before_id', String(params.beforeId))
		return request<AdminChatMessageList>(`/tenant/chat/messages?${query}`)
	},
	chatUnread: (limit = 30) => request<AdminChatUnreadSummary>(`/tenant/chat/unread?limit=${limit}`),
	markChatRead: (payload: AdminChatReadPayload) => request<AdminChatReadResult>('/tenant/chat/read', { method: 'POST', body: JSON.stringify(payload) }),
	replyChat: (payload: { scope: string; room_scope: string; game_id: string; room_type: string; content: string }) => request<AdminChatMessage>('/tenant/chat/messages', { method: 'POST', body: JSON.stringify(payload) }),
	sendChatRedPacket: (payload: { request_id: string; game_id: string; count: number; total_amount: number; min_daily_turnover?: number; greeting?: string; cover?: string }) => request<AdminChatMessage>('/tenant/chat/redpackets', { method: 'POST', body: JSON.stringify(payload) }),
	setLotteryRoomStatus: (gameId: string, enabled: boolean) => request<LotteryRoomStatus>(`/tenant/chat/lottery-rooms/${encodeURIComponent(gameId)}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
	settings: () => request<SystemSettings>('/tenant/settings'),
	updateSettings: (payload: SystemSettings) => request<SystemSettings>('/tenant/settings', { method: 'PUT', body: JSON.stringify(payload) }),
	robotSetting: () => request<RobotSetting>('/tenant/robots/settings'),
	robots: (params?: { query?: string; status?: string }) => request<UserListResponse>(`/tenant/robots?${new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', page: '1', page_size: '100' })}`),
	updateRobot: (id: number, payload: { nickname: string; avatar?: string; status: 0 | 1; game_ids: string[]; active_start: string; active_end: string; min_bet: number; max_bet: number }) => request<AdminUser>(`/tenant/robots/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
	resetRobots: (payload: RobotResetInput) => request<RobotResetResult>('/tenant/robots/reset', { method: 'POST', headers: { 'Idempotency-Key': payload.request_id }, body: JSON.stringify(payload) }),
	updateRobotSetting: (payload: Partial<Pick<RobotSetting, 'enabled' | 'interval_secs' | 'bets_per_cycle' | 'daily_bet_limit' | 'max_pending_bets'>>) => request<RobotSetting>('/tenant/robots/settings', { method: 'PATCH', body: JSON.stringify(payload) }),
		runRobotOnce: () => request<RoomActivityStatus>('/tenant/robots/run-once', { method: 'POST' }),
		activities: (status = 'all') => request<OpsActivity[]>(`/tenant/activities?status=${encodeURIComponent(status)}`),
		createActivity: (payload: Partial<OpsActivity> & { type: string; title: string }) => request<OpsActivity>('/tenant/activities', { method: 'POST', body: JSON.stringify(payload) }),
		updateActivity: (id: number, payload: Partial<OpsActivity> & { type: string; title: string }) => request<OpsActivity>(`/tenant/activities/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
		setActivityStatus: (id: number, status: string) => request<OpsActivity>(`/tenant/activities/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
		deleteActivity: (id: number) => request<{ id: number }>(`/tenant/activities/${id}`, { method: 'DELETE' }),
		plans: () => request<PlanRecommendation[]>('/tenant/plans'),
		createPlan: (payload: PlanRecommendationPayload) => request<PlanRecommendation>('/tenant/plans', { method: 'POST', body: JSON.stringify(payload) }),
		updatePlan: (id: number, payload: PlanRecommendationPayload) => request<PlanRecommendation>(`/tenant/plans/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
		deletePlan: (id: number) => request<{ id: number }>(`/tenant/plans/${id}`, { method: 'DELETE' }),
		walletChannels: (params?: { query?: string; status?: string }) => request<PaymentChannel[]>(`/tenant/wallet/channels?${new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all' })}`),
		createWalletChannel: (payload: PaymentChannelPayload) => request<PaymentChannel>('/tenant/wallet/channels', { method: 'POST', body: JSON.stringify(payload) }),
		updateWalletChannel: (id: number, payload: PaymentChannelPayload) => request<PaymentChannel>(`/tenant/wallet/channels/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
		setWalletChannelStatus: (id: number, status: PaymentChannel['status']) => request<PaymentChannel>(`/tenant/wallet/channels/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
		deleteWalletChannel: (id: number) => request<{ id: number }>(`/tenant/wallet/channels/${id}`, { method: 'DELETE' }),
	reportCatalog: () => request<ReportDefinition[]>('/tenant/reports/catalog'),
	reportCenter: (key: string, params: ReportCenterParams) => request<ReportCenterResult>(`/tenant/reports/${encodeURIComponent(key)}?${reportQuery(params)}`),
	exportReport: (key: string, params: ReportCenterParams) => downloadAuthenticated(`/tenant/reports/${encodeURIComponent(key)}?${reportQuery(params)}&format=csv`, `${key}-${params.start || 'report'}-${params.end || 'report'}.csv`),
}

export const agentApi = {
  dashboard: () => request<AgentDashboard>('/agent/dashboard'),
	games: () => request<WorkspaceGame[]>('/agent/games'),
	setGameStatus: (gameId: string, enabled: boolean) => request<LotteryRoomStatus>(`/agent/games/${encodeURIComponent(gameId)}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
	menuTemplate: () => request<unknown>('/agent/menu-template'),
  updateRoomSettings: (roomName: string, roomLogo: string) => request<AgentDashboard>('/agent/room/settings', { method: 'PATCH', body: JSON.stringify({ room_name: roomName, room_logo: roomLogo }) }),
  settings: () => request<SystemSettings>('/agent/settings'),
  updateSettings: (payload: SystemSettings) => request<SystemSettings>('/agent/settings', { method: 'PUT', body: JSON.stringify(payload) }),
  users: (params?: { query?: string; status?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    return request<UserListResponse>(`/agent/users?${query}`)
  },
  setUserStatus: (id: number, status: 0 | 1) => request<AdminUser>(`/agent/users/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  adjustUserBalance: (id: number, amount: number, remark: string) => request<AdminUser>(`/agent/users/${id}/balance`, { method: 'POST', body: JSON.stringify({ amount, remark }) }),
  userTrading: (id: number, gameId?: string) => request<UserTradingConfig>(`/agent/users/${id}/trading${gameId ? `?game_id=${encodeURIComponent(gameId)}` : ''}`),
  updateUserTrading: (id: number, payload: { odds_multiplier?: number; fly_mode: string; fly_rate: number; rebate_mode: string; rebate_rate: number; game_id: string; odds: Array<{ play_code: string; override: number | null }> }) => request<UserTradingConfig>(`/agent/users/${id}/trading`, { method: 'PUT', body: JSON.stringify(payload) }),
  bets: (params?: { query?: string; gameId?: string; issue?: string; userId?: number; status?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', game_id: params?.gameId ?? 'all', issue: params?.issue ?? '', status: params?.status ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    if (params?.userId) query.set('user_id', String(params.userId))
    return request<BetListResponse>(`/agent/bets?${query}`)
  },
	applications: (params?: { query?: string; status?: string; type?: string; date?: string; start?: string; end?: string; page?: number; pageSize?: number }) => {
		const query = new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', type: params?.type ?? 'all', date: params?.date ?? '', start: params?.start ?? '', end: params?.end ?? '', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    return request<ApplicationListResponse>(`/agent/applications?${query}`)
  },
	applicationStats: () => request<ApplicationStats>('/agent/applications/stats'),
  reviewApplication: (id: number, payload: { decision: 'approved' | 'rejected'; received_amount: number; odds_multiplier?: number; remark: string }) => request<AdminApplication>(`/agent/applications/${id}/review`, { method: 'POST', body: JSON.stringify(payload) }),
  trading: (gameId?: string) => request<RoomTradingConfig>(`/agent/trading${gameId ? `?game_id=${encodeURIComponent(gameId)}` : ''}`),
  updateTrading: (payload: { rebate_rate: number; game_id: string; odds: Array<{ play_code: string; override: number | null }> }) => request<RoomTradingConfig>('/agent/trading', { method: 'PUT', body: JSON.stringify(payload) }),
  chatConversations: (params: { roomType?: string; channel?: 'service' | 'room' | 'lottery'; query?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ room_type: params.roomType ?? '', channel: params.channel ?? '', query: params.query ?? '', page: String(params.page ?? 1), page_size: String(params.pageSize ?? 30) })
    return request<AdminChatConversationList>(`/agent/chat/conversations?${query}`)
  },
  chatMessages: (params: { scope: string; roomScope: string; gameId: string; roomType: string; beforeId?: number; limit?: number }) => {
    const query = new URLSearchParams({ scope: params.scope, room_scope: params.roomScope, game_id: params.gameId, room_type: params.roomType, limit: String(params.limit ?? 50) })
    if (params.beforeId) query.set('before_id', String(params.beforeId))
    return request<AdminChatMessageList>(`/agent/chat/messages?${query}`)
  },
	chatUnread: (limit = 30) => request<AdminChatUnreadSummary>(`/agent/chat/unread?limit=${limit}`),
	markChatRead: (payload: AdminChatReadPayload) => request<AdminChatReadResult>('/agent/chat/read', { method: 'POST', body: JSON.stringify(payload) }),
	replyChat: (payload: { scope: string; room_scope: string; game_id: string; room_type: string; content: string }) => request<AdminChatMessage>('/agent/chat/messages', { method: 'POST', body: JSON.stringify(payload) }),
	sendChatRedPacket: (payload: { request_id: string; game_id: string; count: number; total_amount: number; min_daily_turnover?: number; greeting?: string; cover?: string }) => request<AdminChatMessage>('/agent/chat/redpackets', { method: 'POST', body: JSON.stringify(payload) }),
	setLotteryRoomStatus: (gameId: string, enabled: boolean) => request<LotteryRoomStatus>(`/agent/chat/lottery-rooms/${encodeURIComponent(gameId)}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
	robotStatus: () => request<RobotSetting>('/agent/robots/status'),
	robotSetting: () => request<RobotSetting>('/agent/robots/settings'),
	robots: (params?: { query?: string; status?: string }) => request<UserListResponse>(`/agent/robots?${new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', page: '1', page_size: '100' })}`),
	updateRobot: (id: number, payload: { nickname: string; avatar?: string; status: 0 | 1; game_ids: string[]; active_start: string; active_end: string; min_bet: number; max_bet: number }) => request<AdminUser>(`/agent/robots/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
	resetRobots: (payload: RobotResetInput) => request<RobotResetResult>('/agent/robots/reset', { method: 'POST', headers: { 'Idempotency-Key': payload.request_id }, body: JSON.stringify(payload) }),
	updateRobotSetting: (payload: Partial<Pick<RobotSetting, 'enabled' | 'interval_secs' | 'bets_per_cycle' | 'daily_bet_limit' | 'max_pending_bets'>>) => request<RobotSetting>('/agent/robots/settings', { method: 'PATCH', body: JSON.stringify(payload) }),
	  runRobotOnce: () => request<RoomActivityStatus>('/agent/robots/run-once', { method: 'POST' }),
	activities: (status = 'all') => request<OpsActivity[]>(`/agent/activities?status=${encodeURIComponent(status)}`),
	createActivity: (payload: Partial<OpsActivity> & { type: string; title: string }) => request<OpsActivity>('/agent/activities', { method: 'POST', body: JSON.stringify(payload) }),
	updateActivity: (id: number, payload: Partial<OpsActivity> & { type: string; title: string }) => request<OpsActivity>(`/agent/activities/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
	setActivityStatus: (id: number, status: string) => request<OpsActivity>(`/agent/activities/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
	deleteActivity: (id: number) => request<{ id: number }>(`/agent/activities/${id}`, { method: 'DELETE' }),
	plans: () => request<PlanRecommendation[]>('/agent/plans'),
	createPlan: (payload: PlanRecommendationPayload) => request<PlanRecommendation>('/agent/plans', { method: 'POST', body: JSON.stringify(payload) }),
	updatePlan: (id: number, payload: PlanRecommendationPayload) => request<PlanRecommendation>(`/agent/plans/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
	deletePlan: (id: number) => request<{ id: number }>(`/agent/plans/${id}`, { method: 'DELETE' }),
	walletChannels: (params?: { query?: string; status?: string }) => request<PaymentChannel[]>(`/agent/wallet/channels?${new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all' })}`),
	createWalletChannel: (payload: PaymentChannelPayload) => request<PaymentChannel>('/agent/wallet/channels', { method: 'POST', body: JSON.stringify(payload) }),
	updateWalletChannel: (id: number, payload: PaymentChannelPayload) => request<PaymentChannel>(`/agent/wallet/channels/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
	setWalletChannelStatus: (id: number, status: PaymentChannel['status']) => request<PaymentChannel>(`/agent/wallet/channels/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
	deleteWalletChannel: (id: number) => request<{ id: number }>(`/agent/wallet/channels/${id}`, { method: 'DELETE' }),
	reportCatalog: () => request<ReportDefinition[]>('/agent/reports/catalog'),
	reportCenter: (key: string, params: ReportCenterParams) => request<ReportCenterResult>(`/agent/reports/${encodeURIComponent(key)}?${reportQuery(params)}`),
	exportReport: (key: string, params: ReportCenterParams) => downloadAuthenticated(`/agent/reports/${encodeURIComponent(key)}?${reportQuery(params)}&format=csv`, `${key}-${params.start || 'report'}-${params.end || 'report'}.csv`),
  operatingReport: (params: OperatingReportParams) => request<OperatingReport>(`/agent/reports/operating?${operatingQuery({ ...params, dimension: params.dimension ?? 'game' })}`),
  profitShares: (date = '') => request<ProfitShareStatement>(`/agent/reports/profit-shares${date ? `?date=${encodeURIComponent(date)}` : ''}`),
}
