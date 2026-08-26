export type AdminGame = {
  id: string
  code: string
  name: string
  category: string
  badge: string
  badge_color: string
  enabled: boolean
  issue: string
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
  abnormal_bets: Array<{ id: number; game_id: string; issue: string; user_id: number; username: string; status: string; reconciliation_note?: string; created_at: string }>
  issue_error_count: number
  abnormal_bet_count: number
  pending_on_closed_count: number
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

export type AdminUser = {
  id: number
  public_id: number
  username: string
  email: string
  nickname: string
  phone: string
  role: 'member' | 'agent' | 'tenant' | 'admin'
  remark: string
  risk_level: 'normal' | 'watch' | 'restricted'
  balance: number
  fly_mode?: 'inherit' | 'custom' | 'off' | string
  fly_rate?: number
  agent_room_code?: string
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
  message_count: number
  pinned?: boolean
  muted_until?: string | null
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
  is_staff: boolean
  created_at: string
}

export type AdminChatMessageList = {
  items: AdminChatMessage[]
  has_more: boolean
  next_before_id?: number
}

export type AdminApplication = {
  id: number
  user_id: number
  username: string
  account_type: AdminUser['role'] | string
  request_type: 'credit' | 'debit' | 'agent' | 'join'
  target_room_code?: string
  game_id?: string
  payment_type: string
  requested_amount: number
  received_amount: number
  remark: string
  status: 'pending' | 'approved' | 'rejected'
  operator: string
  review_remark: string
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
  approved_today: number
  rejected_today: number
  today_amount: number
}

export type ApplicationPayload = {
  user_id: number
  request_type: AdminApplication['request_type']
  payment_type: string
  game_id?: string
  amount: number
  remark: string
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
  return `${window.location.protocol}//${window.location.hostname}:8080/api`
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

function authHeaders(): HeadersInit {
  const token = window.localStorage.getItem('yaotu-admin-token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const isFormData = init?.body instanceof FormData
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers: { ...(isFormData ? {} : { 'Content-Type': 'application/json' }), ...authHeaders(), ...init?.headers },
  })
  const body = (await response.json()) as ApiResponse<T>
  if (response.status === 401) {
    window.localStorage.removeItem('yaotu-admin-token')
    window.localStorage.removeItem('yaotu-admin-user')
    window.dispatchEvent(new CustomEvent('yaotu-auth-expired'))
    throw new AuthError(body.message || '请先登录')
  }
  if (!response.ok) throw new Error(body.message || '服务暂时不可用')
  return body.data
}

export type ManagementWsEvent = {
  event_id?: string
  type: string
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
  token: string
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

function sessionRole() {
  try { return JSON.parse(window.localStorage.getItem('yaotu-admin-user') ?? '{}')?.role ?? '' } catch { return '' }
}

export const adminApi = {
  health: async () => {
    const response = await fetch(`${healthBase}/health`)
    if (!response.ok) throw new Error('后端离线')
    return true
  },
  login: (username: string, password: string, workspace = '') => request<LoginResult>('/login', { method: 'POST', body: JSON.stringify({ username, password, workspace }) }),
  me: () => request<LoginResult['user']>(sessionRole() === 'agent' ? '/agent/me' : sessionRole() === 'tenant' ? '/tenant/me' : '/admin/me'),
  dashboard: () => request<DashboardData>('/admin/dashboard'),
  auditLogs: (beforeId?: number, limit = 50) => request<AuditLogPage>(`/admin/audit-logs?limit=${limit}${beforeId ? `&before_id=${beforeId}` : ''}`),
  reconciliation: () => request<ReconciliationSummary>('/admin/reconciliation'),
  games: () => request<AdminGame[]>('/admin/games'),
  syncTargetGames: () => request<SyncTargetGamesResult>('/admin/games/sync-target', { method: 'POST' }),
  draws: (id: string) => request<DrawResult[]>(`/admin/games/${id}/draws?limit=30`),
  clock: () => request<ServerClock>('/public/clock'),
  feedStatus: () => request<FeedStatus>('/public/lottery/status'),
  users: (params: { query?: string; status?: string; role?: string; kind?: 'member' | 'account'; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({
      query: params.query ?? '',
      status: params.status ?? 'all',
      role: params.role ?? 'all',
      kind: params.kind ?? '',
      page: String(params.page ?? 1),
      page_size: String(params.pageSize ?? 20),
    })
    return request<UserListResponse>(`/admin/users?${query}`)
  },
  userStats: (kind?: 'member' | 'account') => request<UserStats>(`/admin/users/stats${kind ? `?kind=${kind}` : ''}`),
  user: (id: number) => request<AdminUser>(`/admin/users/${id}`),
  createUser: (payload: UserPayload) => request<AdminUser>('/admin/users', { method: 'POST', body: JSON.stringify(payload) }),
  updateUser: (id: number, payload: UserPayload) => request<AdminUser>(`/admin/users/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  setUserStatus: (id: number, status: 0 | 1) => request<AdminUser>(`/admin/users/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  resetUserPassword: (id: number, password: string) => request<{ id: number }>(`/admin/users/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ password }) }),
  adjustUserBalance: (id: number, amount: number, remark: string) => request<AdminUser>(`/admin/users/${id}/balance`, { method: 'POST', body: JSON.stringify({ amount, remark }) }),
  userBalanceHistory: (id: number) => request<BalanceRecord[]>(`/admin/users/${id}/balance-history?limit=20`),
  userTrading: (id: number, gameId?: string) => {
    const query = gameId ? `?game_id=${encodeURIComponent(gameId)}` : ''
    return request<UserTradingConfig>(`/admin/users/${id}/trading${query}`)
  },
  updateUserTrading: (id: number, payload: {
    fly_mode: string
    fly_rate: number
    rebate_mode: string
    rebate_rate: number
    game_id: string
    odds: Array<{ play_code: string; override: number | null }>
  }) => request<UserTradingConfig>(`/admin/users/${id}/trading`, { method: 'PUT', body: JSON.stringify(payload) }),
  applications: (params: { query?: string; status?: string; type?: string; date?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({
      query: params.query ?? '',
      status: params.status ?? 'all',
      type: params.type ?? 'all',
      date: params.date ?? '',
      page: String(params.page ?? 1),
      page_size: String(params.pageSize ?? 20),
    })
    return request<ApplicationListResponse>(`/admin/applications?${query}`)
  },
  applicationStats: () => request<ApplicationStats>('/admin/applications/stats'),
  application: (id: number) => request<AdminApplication>(`/admin/applications/${id}`),
  createApplication: (payload: ApplicationPayload) => request<AdminApplication>('/admin/applications', { method: 'POST', body: JSON.stringify(payload) }),
  reviewApplication: (id: number, payload: { decision: 'approved' | 'rejected'; received_amount: number; remark: string }) => request<AdminApplication>(`/admin/applications/${id}/review`, { method: 'POST', body: JSON.stringify(payload) }),
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
  operatingReport: (params: OperatingReportParams) => request<OperatingReport>(`/admin/reports/operating?${operatingQuery(params)}`),
  profitShares: (date = '') => request<ProfitShareStatement>(`/admin/reports/profit-shares${date ? `?date=${encodeURIComponent(date)}` : ''}`),
  runProfitShares: (date: string) => request<ProfitShareRunResult>('/admin/reports/profit-shares/run', { method: 'POST', body: JSON.stringify({ date }) }),
  syncOfficialSources: () => request<OfficialSyncResponse>('/admin/sources/sync', { method: 'POST' }),
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
  createTenant: (payload: { username: string; password: string; email?: string; nickname?: string; phone?: string; remark?: string; status: number }) => request<TenantItem>('/admin/tenants', { method: 'POST', body: JSON.stringify(payload) }),
  updateTenant: (id: number, payload: { email?: string; nickname?: string; phone?: string; remark?: string; status: number }) => request<TenantItem>(`/admin/tenants/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  resetTenantPassword: (id: number, password: string) => request<{ id: number }>(`/admin/tenants/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ password }) }),
  createAgent: (payload: { username: string; password: string; email?: string; nickname?: string; phone?: string; room_code: string; room_name?: string; room_logo?: string; rebate_rate?: number; profit_share_rate?: number; remark?: string; status: number; tenant_id?: number }) => request<AgentItem>('/admin/agents', { method: 'POST', body: JSON.stringify(payload) }),
  updateAgent: (id: number, payload: { email?: string; nickname?: string; phone?: string; room_code: string; room_name?: string; room_logo?: string; rebate_rate?: number; profit_share_rate?: number; remark?: string; status: number; tenant_id?: number }) => request<AgentItem>(`/admin/agents/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  roomTrading: (id: number, gameId?: string) => request<RoomTradingConfig>(`/admin/agents/${id}/trading${gameId ? `?game_id=${encodeURIComponent(gameId)}` : ''}`),
  updateRoomTrading: (id: number, payload: { rebate_rate: number; game_id: string; odds: Array<{ play_code: string; override: number | null }> }) => request<RoomTradingConfig>(`/admin/agents/${id}/trading`, { method: 'PUT', body: JSON.stringify(payload) }),
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
  replyChat: (payload: { scope: string; room_scope: string; game_id: string; room_type: string; content: string }) => request<AdminChatMessage>('/admin/chat/messages', { method: 'POST', body: JSON.stringify(payload) }),
  sendChatRedPacket: (payload: { scope: string; room_scope: string; game_id: string; count: number; total_amount: number; min_daily_turnover?: number; greeting?: string; cover?: string }) => request<AdminChatMessage>('/admin/chat/redpackets', { method: 'POST', body: JSON.stringify(payload) }),
  deleteChatMessage: (id: number) => request<{ id: number }>(`/admin/chat/messages/${id}`, { method: 'DELETE' }),
  setChatMute: (userId: number, payload: { minutes: number; reason?: string }) => request<{ user_id: number; muted_until?: string | null; mute_reason?: string }>(`/admin/chat/users/${userId}/mute`, { method: 'PATCH', body: JSON.stringify(payload) }),
  setChatAnnouncement: (content: string) => request<{ content: string }>('/admin/chat/announcement', { method: 'PUT', body: JSON.stringify({ content }) }),
  setLotteryRoomStatus: (roomScope: string, gameId: string, enabled: boolean) => request<LotteryRoomStatus>('/admin/chat/lottery-rooms/status', { method: 'PATCH', body: JSON.stringify({ room_scope: roomScope, game_id: gameId, enabled }) }),
  rebatePreview: () => request<RebatePreview>('/admin/rebates/preview'),
  runRebate: () => request('/admin/rebates/run', { method: 'POST' }),
}

export const tenantApi = {
  dashboard: () => request<TenantDashboard>('/tenant/dashboard'),
  agents: (params?: { query?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    return request<AgentListResponse>(`/tenant/agents?${query}`)
  },
  createAgent: (payload: { username: string; password: string; email?: string; nickname?: string; phone?: string; room_code: string; room_name?: string; room_logo?: string; rebate_rate?: number; profit_share_rate?: number; remark?: string; status: number }) => request<AgentItem>('/tenant/agents', { method: 'POST', body: JSON.stringify(payload) }),
  updateAgent: (id: number, payload: { email?: string; nickname?: string; phone?: string; room_code: string; room_name?: string; room_logo?: string; rebate_rate?: number; profit_share_rate?: number; remark?: string; status: number }) => request<AgentItem>(`/tenant/agents/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  resetAgentPassword: (id: number, password: string) => request<{ id: number }>(`/tenant/agents/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ password }) }),
  roomDashboard: (agentId: number) => request<AgentDashboard>(`/tenant/rooms/${agentId}/dashboard`),
  updateRoomSettings: (agentId: number, roomName: string, roomLogo: string) => request<AgentDashboard>(`/tenant/rooms/${agentId}/settings`, { method: 'PATCH', body: JSON.stringify({ room_name: roomName, room_logo: roomLogo }) }),
  roomUsers: (agentId: number, params?: { query?: string; status?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    return request<UserListResponse>(`/tenant/rooms/${agentId}/users?${query}`)
  },
  setRoomUserStatus: (agentId: number, userId: number, status: 0 | 1) => request<AdminUser>(`/tenant/rooms/${agentId}/users/${userId}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  adjustRoomUserBalance: (agentId: number, userId: number, amount: number, remark: string) => request<AdminUser>(`/tenant/rooms/${agentId}/users/${userId}/balance`, { method: 'POST', body: JSON.stringify({ amount, remark }) }),
  roomBets: (agentId: number, params?: { query?: string; gameId?: string; issue?: string; userId?: number; status?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', game_id: params?.gameId ?? 'all', issue: params?.issue ?? '', status: params?.status ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    if (params?.userId) query.set('user_id', String(params.userId))
    return request<BetListResponse>(`/tenant/rooms/${agentId}/bets?${query}`)
  },
  roomApplications: (agentId: number, params?: { query?: string; status?: string; type?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', type: params?.type ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    return request<ApplicationListResponse>(`/tenant/rooms/${agentId}/applications?${query}`)
  },
  reviewRoomApplication: (agentId: number, id: number, payload: { decision: 'approved' | 'rejected'; received_amount: number; remark: string }) => request<AdminApplication>(`/tenant/rooms/${agentId}/applications/${id}/review`, { method: 'POST', body: JSON.stringify(payload) }),
  roomOperatingReport: (agentId: number, params: OperatingReportParams) => request<OperatingReport>(`/tenant/rooms/${agentId}/reports/operating?${operatingQuery({ ...params, dimension: params.dimension ?? 'game' })}`),
  roomChatConversations: (agentId: number, params: { roomType?: string; channel?: 'service' | 'room' | 'lottery'; query?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ room_type: params.roomType ?? '', channel: params.channel ?? '', query: params.query ?? '', page: String(params.page ?? 1), page_size: String(params.pageSize ?? 30) })
    return request<AdminChatConversationList>(`/tenant/rooms/${agentId}/chat/conversations?${query}`)
  },
  roomChatMessages: (agentId: number, params: { scope: string; roomScope: string; gameId: string; roomType: string; beforeId?: number; limit?: number }) => {
    const query = new URLSearchParams({ scope: params.scope, room_scope: params.roomScope, game_id: params.gameId, room_type: params.roomType, limit: String(params.limit ?? 50) })
    if (params.beforeId) query.set('before_id', String(params.beforeId))
    return request<AdminChatMessageList>(`/tenant/rooms/${agentId}/chat/messages?${query}`)
  },
  replyRoomChat: (agentId: number, payload: { scope: string; room_scope: string; game_id: string; room_type: string; content: string }) => request<AdminChatMessage>(`/tenant/rooms/${agentId}/chat/messages`, { method: 'POST', body: JSON.stringify(payload) }),
  sendRoomRedPacket: (agentId: number, payload: { game_id: string; count: number; total_amount: number; min_daily_turnover?: number; greeting?: string; cover?: string }) => request<AdminChatMessage>(`/tenant/rooms/${agentId}/chat/redpackets`, { method: 'POST', body: JSON.stringify(payload) }),
  setRoomLotteryStatus: (agentId: number, gameId: string, enabled: boolean) => request<LotteryRoomStatus>(`/tenant/rooms/${agentId}/chat/lottery-rooms/${encodeURIComponent(gameId)}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
  roomRobotStatus: (agentId: number) => request<RoomActivityStatus>(`/tenant/rooms/${agentId}/robots/status`),
  runRoomRobotOnce: (agentId: number) => request<RoomActivityStatus>(`/tenant/rooms/${agentId}/robots/run-once`, { method: 'POST' }),
}

export const agentApi = {
  dashboard: () => request<AgentDashboard>('/agent/dashboard'),
  updateRoomSettings: (roomName: string, roomLogo: string) => request<AgentDashboard>('/agent/room/settings', { method: 'PATCH', body: JSON.stringify({ room_name: roomName, room_logo: roomLogo }) }),
  users: (params?: { query?: string; status?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    return request<UserListResponse>(`/agent/users?${query}`)
  },
  setUserStatus: (id: number, status: 0 | 1) => request<AdminUser>(`/agent/users/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  adjustUserBalance: (id: number, amount: number, remark: string) => request<AdminUser>(`/agent/users/${id}/balance`, { method: 'POST', body: JSON.stringify({ amount, remark }) }),
  bets: (params?: { query?: string; gameId?: string; issue?: string; userId?: number; status?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', game_id: params?.gameId ?? 'all', issue: params?.issue ?? '', status: params?.status ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    if (params?.userId) query.set('user_id', String(params.userId))
    return request<BetListResponse>(`/agent/bets?${query}`)
  },
  applications: (params?: { query?: string; status?: string; type?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ query: params?.query ?? '', status: params?.status ?? 'all', type: params?.type ?? 'all', page: String(params?.page ?? 1), page_size: String(params?.pageSize ?? 20) })
    return request<ApplicationListResponse>(`/agent/applications?${query}`)
  },
  reviewApplication: (id: number, payload: { decision: 'approved' | 'rejected'; received_amount: number; remark: string }) => request<AdminApplication>(`/agent/applications/${id}/review`, { method: 'POST', body: JSON.stringify(payload) }),
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
	replyChat: (payload: { scope: string; room_scope: string; game_id: string; room_type: string; content: string }) => request<AdminChatMessage>('/agent/chat/messages', { method: 'POST', body: JSON.stringify(payload) }),
	sendChatRedPacket: (payload: { game_id: string; count: number; total_amount: number; min_daily_turnover?: number; greeting?: string; cover?: string }) => request<AdminChatMessage>('/agent/chat/redpackets', { method: 'POST', body: JSON.stringify(payload) }),
	setLotteryRoomStatus: (gameId: string, enabled: boolean) => request<LotteryRoomStatus>(`/agent/chat/lottery-rooms/${encodeURIComponent(gameId)}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
	robotStatus: () => request<RoomActivityStatus>('/agent/robots/status'),
  runRobotOnce: () => request<RoomActivityStatus>('/agent/robots/run-once', { method: 'POST' }),
  operatingReport: (params: OperatingReportParams) => request<OperatingReport>(`/agent/reports/operating?${operatingQuery({ ...params, dimension: params.dimension ?? 'game' })}`),
  profitShares: (date = '') => request<ProfitShareStatement>(`/agent/reports/profit-shares${date ? `?date=${encodeURIComponent(date)}` : ''}`),
}
