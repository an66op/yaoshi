import process from 'node:process'

const baseURL = process.env.SYSTEM_TEST_BACKEND_ORIGIN || 'http://127.0.0.1:18080'
const adminUsername = process.env.E2E_ADMIN_USERNAME || 'e2e_platform'
const adminPassword = process.env.E2E_ADMIN_PASSWORD || 'ProdBootstrap#2026_x9Q'
const agentUsername = process.env.E2E_AGENT_USERNAME || 'e2e_agent'
const agentPassword = process.env.E2E_AGENT_PASSWORD || 'AgentPass#2026_x9Q'
const secondAgentUsername = process.env.E2E_SECOND_AGENT_USERNAME || 'e2e_agent_cap'
const secondAgentPassword = process.env.E2E_SECOND_AGENT_PASSWORD || 'AgentCap#2026_v8K4'
const memberUsername = process.env.E2E_MEMBER_USERNAME || 'e2e_member'
const memberPassword = process.env.E2E_MEMBER_PASSWORD || 'MemberPass#2026_x9Q'
const boundaryUsername = process.env.E2E_BOUNDARY_USERNAME || `${'u'.repeat(46)}2026`
const boundaryPassword = process.env.E2E_BOUNDARY_PASSWORD || `Aa1#${'z'.repeat(68)}`
const roomCode = process.env.E2E_ROOM_CODE || '88001'

const parsedBaseURL = new URL(baseURL)
if (!['127.0.0.1', 'localhost', '::1'].includes(parsedBaseURL.hostname)) {
  throw new Error(`Refusing release fixture setup against non-loopback host ${parsedBaseURL.hostname}`)
}
if (process.env.SYSTEM_TEST_ALLOW_LOCAL !== '1') throw new Error('SYSTEM_TEST_ALLOW_LOCAL=1 is required')
if ([...boundaryUsername].length !== 50 || Buffer.byteLength(boundaryPassword) !== 72) {
  throw new Error('Boundary credentials must remain exactly 50 characters / 72 bytes')
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function raw(path, options = {}) {
  return fetch(`${baseURL}${path}`, { redirect: 'manual', signal: AbortSignal.timeout(10_000), ...options })
}

async function json(path, options = {}, expected = [200]) {
  const response = await raw(path, options)
  const text = await response.text()
  let body
  try { body = text ? JSON.parse(text) : {} } catch { throw new Error(`${path} returned non-JSON (${response.status})`) }
  if (!expected.includes(response.status)) {
    throw new Error(`${path} returned ${response.status}, expected ${expected.join('/')}: ${body.message || text.slice(0, 160)}`)
  }
  return { response, body }
}

const jsonHeaders = { 'content-type': 'application/json' }

const health = await json('/health')
assert(health.body.status === 'ok', '/health did not report ok')
const ready = await json('/ready')
assert(ready.body.status === 'ready', '/ready did not report ready')

await json('/api/session', {}, [401])
const forbiddenRegistration = await raw('/api/register', {
  method: 'POST', headers: jsonHeaders,
  body: JSON.stringify({ username: 'must_not_exist', password: 'NeverCreate#2026', email: 'no@example.invalid' }),
})
assert(forbiddenRegistration.status === 404, `/api/register returned ${forbiddenRegistration.status}, expected 404`)

const publicRegistration = {
  username: 'fresh_public_member',
  password: 'PublicMember#2026_x9Q',
}
const publicRegisterHeaders = { ...jsonHeaders, 'x-forwarded-for': '198.51.100.10' }
const registeredMember = await json('/api/member/register', {
  method: 'POST', headers: publicRegisterHeaders, body: JSON.stringify(publicRegistration),
}, [201])
assert(registeredMember.body.data?.user?.username === publicRegistration.username, 'public member registration did not return the created member')
assert(registeredMember.body.data?.user?.role === 'member', 'public member registration created the wrong role')
assert(!('token' in (registeredMember.body.data || {})), 'public member registration exposed a bearer token')
const registrationCookie = registeredMember.response.headers.get('set-cookie') || ''
assert(/wangzhe_member_session=/.test(registrationCookie), 'public member registration did not set the member session cookie')
assert(/HttpOnly/i.test(registrationCookie) && /Secure/i.test(registrationCookie) && /SameSite=Lax/i.test(registrationCookie), 'public member registration cookie flags are incomplete')
assert(registeredMember.response.headers.get('cache-control') === 'no-store', 'public member registration response may be cached')

const duplicateRegistration = await json('/api/member/register', {
  method: 'POST', headers: publicRegisterHeaders, body: JSON.stringify(publicRegistration),
}, [409])
assert(!duplicateRegistration.response.headers.get('set-cookie'), 'duplicate registration unexpectedly set a session cookie')

await json('/api/member/register', {
  method: 'POST', headers: { ...jsonHeaders, 'x-forwarded-for': '198.51.100.11' },
  body: JSON.stringify({ username: 'u'.repeat(21), password: 'TooLongUsername#2026' }),
}, [400])
await json('/api/member/register', {
  method: 'POST', headers: { ...jsonHeaders, 'x-forwarded-for': '198.51.100.12' },
  body: JSON.stringify({ username: 'long_password_member', password: `Aa1#${'z'.repeat(69)}` }),
}, [400])

const limitedIP = '198.51.100.20'
for (let attempt = 1; attempt <= 10; attempt++) {
  const response = await raw('/api/member/register', {
    method: 'POST', headers: { ...jsonHeaders, 'x-forwarded-for': limitedIP },
    body: JSON.stringify({ username: 'u'.repeat(21), password: 'RateLimit#2026_x9Q' }),
  })
  assert(response.status === 400, `rate-limit warm-up ${attempt} returned ${response.status}, expected validation 400`)
}
const rateLimited = await raw('/api/member/register', {
  method: 'POST', headers: { ...jsonHeaders, 'x-forwarded-for': limitedIP },
  body: JSON.stringify({ username: 'u'.repeat(21), password: 'RateLimit#2026_x9Q' }),
})
assert(rateLimited.status === 429, `11th registration from one X-Forwarded-For returned ${rateLimited.status}, expected 429`)
assert(/^\d+$/.test(rateLimited.headers.get('retry-after') || ''), '429 registration response omitted numeric Retry-After')
const independentIP = await raw('/api/member/register', {
  method: 'POST', headers: { ...jsonHeaders, 'x-forwarded-for': '198.51.100.21' },
  body: JSON.stringify({ username: 'u'.repeat(21), password: 'Independent#2026_x9Q' }),
})
assert(independentIP.status === 400, `independent X-Forwarded-For inherited another client's limit (${independentIP.status})`)

const login = await json('/api/login', {
  method: 'POST', headers: { ...jsonHeaders, 'x-forwarded-for': '198.51.100.30' },
  body: JSON.stringify({ username: adminUsername, password: adminPassword, workspace: '平台' }),
})
assert(login.body.data?.user?.role === 'admin', 'bootstrap administrator could not log in')
assert(!('token' in (login.body.data || {})), 'login response exposed a bearer token')
const setCookie = login.response.headers.get('set-cookie') || ''
assert(/wangzhe_management_session=/.test(setCookie), 'management session cookie was not set')
assert(/HttpOnly/i.test(setCookie) && /Secure/i.test(setCookie) && /SameSite=Lax/i.test(setCookie), 'release cookie flags are incomplete')
const managementCookie = setCookie.split(';', 1)[0]
const managementHeaders = { ...jsonHeaders, cookie: managementCookie }

await json('/api/session', { headers: { cookie: managementCookie } })
await json('/api/admin/dashboard', { headers: { cookie: managementCookie } })

const agent = await json('/api/admin/agents', {
  method: 'POST', headers: managementHeaders,
  body: JSON.stringify({
    username: agentUsername, password: agentPassword, nickname: 'E2E 房主', room_code: roomCode,
    room_name: '生产验收房', room_logo: '', rebate_rate: 0, profit_share_rate: 0,
    remark: 'fresh DB system test fixture', status: 1,
  }),
}, [201])
const agentID = Number(agent.body.data?.id)
const agentWorkspaceID = Number(agent.body.data?.workspace_id)
assert(Number.isInteger(agentID) && agentID > 0, 'agent fixture did not return an id')
assert(Number.isInteger(agentWorkspaceID) && agentWorkspaceID > 0, 'agent fixture did not return a workspace id')

const secondAgent = await json('/api/admin/agents', {
  method: 'POST', headers: managementHeaders,
  body: JSON.stringify({
    username: secondAgentUsername, password: secondAgentPassword, nickname: 'E2E 门禁房主', room_code: '88002',
    room_name: '机器人门禁验收房', room_logo: '', rebate_rate: 0, profit_share_rate: 0,
    remark: 'fresh DB robot cap fixture', status: 1,
  }),
}, [201])
const secondWorkspaceID = Number(secondAgent.body.data?.workspace_id)
assert(Number.isInteger(secondWorkspaceID) && secondWorkspaceID > 0, 'second agent fixture did not return a workspace id')

const enableRobotSetting = workspaceID => raw(`/api/admin/robot-settings?workspace_id=${workspaceID}`, {
  method: 'PATCH', headers: managementHeaders,
  body: JSON.stringify({ enabled: true, bets_per_cycle: 1, daily_bet_limit: 1, max_pending_bets: 1 }),
})
const enableResponses = await Promise.all([enableRobotSetting(agentWorkspaceID), enableRobotSetting(secondWorkspaceID)])
const enableStatuses = enableResponses.map(response => response.status).sort((left, right) => left - right)
assert(enableStatuses[0] === 200 && enableStatuses[1] === 409, `robot cap race returned ${enableStatuses.join('/')}, expected 200/409`)
const firstRobotSetting = await json(`/api/admin/robot-settings?workspace_id=${agentWorkspaceID}`, { headers: { cookie: managementCookie } })
const secondRobotSetting = await json(`/api/admin/robot-settings?workspace_id=${secondWorkspaceID}`, { headers: { cookie: managementCookie } })
const enabledSettings = [
  { workspaceID: agentWorkspaceID, enabled: firstRobotSetting.body.data?.enabled === true },
  { workspaceID: secondWorkspaceID, enabled: secondRobotSetting.body.data?.enabled === true },
]
assert(enabledSettings.filter(item => item.enabled).length === 1, 'robot cap transaction did not leave exactly one enabled workspace')
const enabledWorkspaceID = enabledSettings.find(item => item.enabled)?.workspaceID
await json(`/api/admin/robot-settings?workspace_id=${enabledWorkspaceID}`, {
  method: 'PATCH', headers: managementHeaders, body: JSON.stringify({ enabled: false }),
})

// Exercise the production robot gate against the fresh PostgreSQL database:
// provisioned profile -> persisted bet -> immutable draw -> settlement.  This
// catches integration regressions that dry-run SQL/unit tests cannot see.
await json(`/api/admin/robot-settings?workspace_id=${agentWorkspaceID}`, {
  method: 'PATCH', headers: managementHeaders,
  body: JSON.stringify({ enabled: true, interval_secs: 3600, bets_per_cycle: 1, daily_bet_limit: 1, max_pending_bets: 1 }),
})
await json('/api/admin/room-activity/run-once', {
  method: 'POST', headers: { ...managementHeaders, 'x-forwarded-for': '198.51.100.33' },
  body: JSON.stringify({}),
})

let robotBet
for (let attempt = 0; attempt < 20 && !robotBet; attempt++) {
  const bets = await json('/api/admin/bets?query=room_robot_&status=pending&page_size=100', {
    headers: { cookie: managementCookie },
  })
  robotBet = bets.body.data?.items?.find(item => item.username?.startsWith('room_robot_'))
  if (!robotBet) await new Promise(resolve => setTimeout(resolve, 100))
}
assert(robotBet, 'controlled production robot run did not persist a bet')
assert(robotBet.fly_amount === 0, 'robot bet unexpectedly carried a fly amount')
assert(robotBet.status === 'pending', 'robot bet was not pending before the test draw')

await json(`/api/admin/games/${encodeURIComponent(robotBet.game_id)}/publish-draw`, {
  method: 'POST', headers: managementHeaders,
  body: JSON.stringify({ issue: robotBet.issue, numbers: [] }),
})
const settledRobotBets = await json(
  `/api/admin/bets?query=${encodeURIComponent(robotBet.username)}&game_id=${encodeURIComponent(robotBet.game_id)}&issue=${encodeURIComponent(robotBet.issue)}&page_size=100`,
  { headers: { cookie: managementCookie } },
)
const settledRobotBet = settledRobotBets.body.data?.items?.find(item => item.id === robotBet.id)
assert(['won', 'lost'].includes(settledRobotBet?.status), 'robot bet did not complete settlement')
assert(settledRobotBet?.fly_amount === 0, 'settled robot bet unexpectedly carried a fly amount')

const dashboardAfterRobotSettlement = await json('/api/admin/dashboard', { headers: { cookie: managementCookie } })
for (const field of [
  'today_turnover', 'today_settled_turnover', 'today_gross_profit', 'today_net_profit',
  'today_rebate', 'today_agent_share', 'total_gross_profit', 'total_net_profit',
  'total_rebate', 'total_agent_share', 'pending_settlement',
]) {
  assert(Number(dashboardAfterRobotSettlement.body.data?.stats?.[field]) === 0, `robot activity leaked into dashboard stat ${field}`)
}
await json(`/api/admin/robot-settings?workspace_id=${agentWorkspaceID}`, {
  method: 'PATCH', headers: managementHeaders, body: JSON.stringify({ enabled: false }),
})

const settings = await json(`/api/admin/agents/${agentID}/settings`, { headers: { cookie: managementCookie } })
const settingsPayload = { ...settings.body.data, require_join_review: false, room_enabled: true }
await json(`/api/admin/agents/${agentID}/settings`, {
  method: 'PUT', headers: managementHeaders, body: JSON.stringify(settingsPayload),
})
const groupChat = await json(`/api/admin/chat/rooms/${agentID}/group-chat`, {
  method: 'PATCH', headers: managementHeaders, body: JSON.stringify({ enabled: true }),
})
assert(groupChat.body.data?.group_chat_enabled === true, 'fresh room group chat was not enabled for the load fixture')

async function createMember(username, password, nickname) {
  const created = await json('/api/admin/users', {
    method: 'POST', headers: managementHeaders,
    body: JSON.stringify({
      username, password, email: '', nickname, phone: '', role: 'member', remark: 'fresh DB system test fixture',
      risk_level: 'normal', status: 1, parent_agent_id: agentID,
    }),
  }, [201])
  assert(Number(created.body.data?.id) > 0, `member fixture ${username} was not created`)
}

await createMember(memberUsername, memberPassword, 'E2E 会员')
await createMember(boundaryUsername, boundaryPassword, '边界会员')

async function memberLogin(username, password, clientIP) {
  const result = await json('/api/member/login', {
    method: 'POST', headers: { ...jsonHeaders, 'x-forwarded-for': clientIP }, body: JSON.stringify({ username, password, workspace: '' }),
  })
  const cookieHeader = result.response.headers.get('set-cookie') || ''
  assert(/wangzhe_member_session=/.test(cookieHeader), `member session cookie missing for ${username}`)
  assert(/HttpOnly/i.test(cookieHeader) && /Secure/i.test(cookieHeader), 'member release cookie flags are incomplete')
  return cookieHeader.split(';', 1)[0]
}

let memberCookie = await memberLogin(memberUsername, memberPassword, '198.51.100.31')
await memberLogin(boundaryUsername, boundaryPassword, '198.51.100.32')
const joined = await json('/api/member/room/join', {
  method: 'POST', headers: { ...jsonHeaders, cookie: memberCookie, 'idempotency-key': 'fresh-db-room-join-0001' },
  body: JSON.stringify({ room_code: roomCode, request_id: 'fresh-db-room-join-0001' }),
})
assert(joined.body.data?.status === 'joined', 'member did not enter the fresh room without manual review')
const joinedCookie = joined.response.headers.get('set-cookie') || ''
assert(/wangzhe_member_session=/.test(joinedCookie), 'room activation did not rotate the invalidated member session')
assert(/HttpOnly/i.test(joinedCookie) && /Secure/i.test(joinedCookie) && /SameSite=Lax/i.test(joinedCookie), 'rotated member cookie flags are incomplete')
assert(joined.response.headers.get('cache-control') === 'no-store', 'room activation response with a rotated cookie may be cached')
memberCookie = joinedCookie.split(';', 1)[0]
await json('/api/member/me', { headers: { cookie: memberCookie } })

process.stdout.write('Fresh release API checks and disposable E2E fixtures are ready.\n')
