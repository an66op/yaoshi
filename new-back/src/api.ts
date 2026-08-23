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

export type AdminUser = {
  id: number
  public_id: number
  username: string
  email: string
  nickname: string
  phone: string
  role: 'member' | 'agent' | 'admin'
  remark: string
  risk_level: 'normal' | 'watch' | 'restricted'
  balance: number
  fly_mode?: 'inherit' | 'custom' | 'off' | string
  fly_rate?: number
  agent_room_code?: string
  parent_agent_id?: number | null
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
  balance: number
  status: number
  member_count: number
  rebate_rate: number
  remark: string
  created_at: string
  last_login_at: string
  login_count: number
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
  room_type: 'group' | 'service'
  title: string
  subtitle: string
  user_id?: number
  username?: string
  nickname?: string
  latest_text: string
  latest_at: string
  message_count: number
  muted_until?: string | null
}

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
  content: string
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
  type: 'manual' | 'application_credit' | 'application_debit' | 'bet' | 'bet_cancel' | 'settlement' | 'rebate' | 'checkin' | 'redpacket' | 'invite' | string
  category?: 'finance' | 'betting' | 'welfare' | 'other' | string
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

export type SystemSettings = {
  room_name: string
  room_code: string
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
  secret_key?: string
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
const apiBase = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api'
const healthBase = apiBase.replace(/\/api\/?$/, '')

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
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...authHeaders(), ...init?.headers },
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

export const adminApi = {
  health: async () => {
    const response = await fetch(`${healthBase}/health`)
    if (!response.ok) throw new Error('后端离线')
    return true
  },
  login: (username: string, password: string) => request<LoginResult>('/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  me: () => request<LoginResult['user']>('/admin/me'),
  dashboard: () => request<DashboardData>('/admin/dashboard'),
  games: () => request<AdminGame[]>('/admin/games'),
  syncTargetGames: () => request<SyncTargetGamesResult>('/admin/games/sync-target', { method: 'POST' }),
  draws: (id: string) => request<DrawResult[]>(`/admin/games/${id}/draws?limit=30`),
  clock: () => request<ServerClock>('/public/clock'),
  feedStatus: () => request<FeedStatus>('/public/lottery/status'),
  users: (params: { query?: string; status?: string; role?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({
      query: params.query ?? '',
      status: params.status ?? 'all',
      role: params.role ?? 'all',
      page: String(params.page ?? 1),
      page_size: String(params.pageSize ?? 20),
    })
    return request<UserListResponse>(`/admin/users?${query}`)
  },
  userStats: () => request<UserStats>('/admin/users/stats'),
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
  syncOfficialSources: () => request<OfficialSyncResponse>('/admin/sources/sync', { method: 'POST' }),
  updateGameStatus: (id: string, enabled: boolean) => request<AdminGame>(`/admin/games/${id}/status`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
  settings: () => request<SystemSettings>('/admin/settings'),
  updateSettings: (payload: SystemSettings) => request<SystemSettings>('/admin/settings', { method: 'PUT', body: JSON.stringify(payload) }),
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
  seedMonitor: (gameId: string) => request<MonitorSnapshot>('/admin/monitor/seed', { method: 'POST', body: JSON.stringify({ game_id: gameId }) }),
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
  createAgent: (payload: { username: string; password: string; email?: string; nickname?: string; phone?: string; room_code: string; rebate_rate?: number; remark?: string; status: number }) => request<AgentItem>('/admin/agents', { method: 'POST', body: JSON.stringify(payload) }),
  updateAgent: (id: number, payload: { email?: string; nickname?: string; phone?: string; room_code: string; rebate_rate?: number; remark?: string; status: number }) => request<AgentItem>(`/admin/agents/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
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
  chatConversations: (params: { roomType?: string; query?: string; page?: number; pageSize?: number }) => {
    const query = new URLSearchParams({ room_type: params.roomType ?? '', query: params.query ?? '', page: String(params.page ?? 1), page_size: String(params.pageSize ?? 30) })
    return request<AdminChatConversationList>(`/admin/chat/conversations?${query}`)
  },
  chatMessages: (params: { scope: string; roomType: string; beforeId?: number; limit?: number }) => {
    const query = new URLSearchParams({ scope: params.scope, room_type: params.roomType, limit: String(params.limit ?? 50) })
    if (params.beforeId) query.set('before_id', String(params.beforeId))
    return request<AdminChatMessageList>(`/admin/chat/messages?${query}`)
  },
  replyChat: (payload: { scope: string; room_type: string; content: string }) => request<AdminChatMessage>('/admin/chat/messages', { method: 'POST', body: JSON.stringify(payload) }),
  deleteChatMessage: (id: number) => request<{ id: number }>(`/admin/chat/messages/${id}`, { method: 'DELETE' }),
  setChatMute: (userId: number, payload: { minutes: number; reason?: string }) => request<{ user_id: number; muted_until?: string | null; mute_reason?: string }>(`/admin/chat/users/${userId}/mute`, { method: 'PATCH', body: JSON.stringify(payload) }),
  setChatAnnouncement: (content: string) => request<{ content: string }>('/admin/chat/announcement', { method: 'PUT', body: JSON.stringify({ content }) }),
  rebatePreview: () => request<RebatePreview>('/admin/rebates/preview'),
  runRebate: () => request('/admin/rebates/run', { method: 'POST' }),
}
