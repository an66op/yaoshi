import { performance } from 'node:perf_hooks'
import process from 'node:process'
import WebSocket from 'ws'
import { readMemberSessionFile } from './member-session-file.mjs'

const baseURL = process.env.LOAD_TEST_BASE_URL || process.env.SYSTEM_TEST_BACKEND_ORIGIN || 'http://127.0.0.1:18080'
const parsedBaseURL = new URL(baseURL)
const loopback = ['127.0.0.1', 'localhost', '::1'].includes(parsedBaseURL.hostname)
const remoteAuthorized = process.env.LOAD_TEST_AUTHORIZED_ORIGIN === parsedBaseURL.origin &&
  process.env.LOAD_TEST_CONFIRM === 'I_UNDERSTAND_THIS_GENERATES_TRAFFIC'
if (loopback) {
  if (process.env.SYSTEM_TEST_ALLOW_LOCAL !== '1') throw new Error('SYSTEM_TEST_ALLOW_LOCAL=1 is required for loopback load smoke')
} else if (!remoteAuthorized) {
  throw new Error('Refusing non-loopback traffic. Set LOAD_TEST_AUTHORIZED_ORIGIN to the exact origin and provide LOAD_TEST_CONFIRM.')
}

const reads = positiveInteger('LOAD_READ_REQUESTS', 500, 1, 20_000)
const writes = positiveInteger('LOAD_WRITE_REQUESTS', 20, 0, 500)
const websocketConnections = positiveInteger('LOAD_WEBSOCKET_CONNECTIONS', 10, 1, 100)
const concurrency = positiveInteger('LOAD_CONCURRENCY', 20, 1, 100)
const maxErrorRate = boundedNumber('LOAD_MAX_ERROR_RATE', 0.01, 0, 1)
const maxP95Ms = boundedNumber('LOAD_MAX_P95_MS', 1500, 1, 120_000)
const samples = []
let errors = 0

function fetchWithTimeout(url, options = {}) {
  // A pre-authenticated session is bound to this exact origin; never follow
  // a redirect that could forward its explicit Cookie header elsewhere.
  return fetch(url, { signal: AbortSignal.timeout(10_000), ...options, redirect: 'error' })
}

function positiveInteger(name, fallback, minimum, maximum) {
  const value = Number(process.env[name] || fallback)
  if (!Number.isInteger(value) || value < minimum || value > maximum) throw new Error(`${name} must be ${minimum}-${maximum}`)
  return value
}

function boundedNumber(name, fallback, minimum, maximum) {
  const value = Number(process.env[name] || fallback)
  if (!Number.isFinite(value) || value < minimum || value > maximum) throw new Error(`${name} must be ${minimum}-${maximum}`)
  return value
}

async function timed(label, operation) {
  const start = performance.now()
  try {
    const response = await operation()
    if (!response.ok) throw new Error(`${label}: HTTP ${response.status}`)
  } catch (error) {
    errors++
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`)
  } finally {
    samples.push(performance.now() - start)
  }
}

async function inPool(tasks) {
  let cursor = 0
  await Promise.all(Array.from({ length: Math.min(concurrency, tasks.length) }, async () => {
    while (cursor < tasks.length) {
      const current = cursor++
      await tasks[current]()
    }
  }))
}

// Load smoke measures business traffic, not CAPTCHA solving. Fresh local runs
// receive the final session from the authenticated API fixture; authorized
// remote runs must be given an operator-authenticated, origin-bound 0600 file.
const memberCookie = await readMemberSessionFile(
  process.env.LOAD_TEST_MEMBER_COOKIE_FILE || process.env.SYSTEM_TEST_MEMBER_COOKIE_FILE,
  parsedBaseURL.origin,
)
const memberHeaders = { cookie: memberCookie }
const sessionCheck = await fetchWithTimeout(`${baseURL}/api/member/me`, { headers: memberHeaders })
if (!sessionCheck.ok) throw new Error(`pre-authenticated load session is not usable: HTTP ${sessionCheck.status}`)

const readPaths = ['/health', '/ready', '/api/member/me', '/api/member/chat/messages?room_type=group&game_id=lobby&limit=5']
const tasks = []
for (let index = 0; index < reads; index++) {
  const path = readPaths[index % readPaths.length]
  tasks.push(() => timed(`GET ${path}`, () => fetchWithTimeout(`${baseURL}${path}`, { headers: path.startsWith('/api/member/') ? memberHeaders : undefined })))
}
const marker = `load-smoke-${Date.now()}`
const writeContents = Array.from({ length: writes }, (_, index) => `${marker}-${index}`)
if (new Set(writeContents).size !== writes) throw new Error('load write fixture did not generate unique content')
const acknowledgedWrites = new Map()
for (let index = 0; index < writes; index++) {
  const expectedContent = writeContents[index]
  tasks.push(() => timed(`POST chat message ${index + 1}/${writes}`, async () => {
    const response = await fetchWithTimeout(`${baseURL}/api/member/chat/messages`, {
      method: 'POST', headers: { ...memberHeaders, 'content-type': 'application/json' },
      body: JSON.stringify({ room_type: 'group', game_id: 'lobby', content: expectedContent }),
    })
    if (response.ok) {
      const body = await response.clone().json()
      const messageID = Number(body.data?.id)
      if (!Number.isSafeInteger(messageID) || messageID <= 0) throw new Error(`write ${expectedContent} returned an invalid message id`)
      if (body.data?.content !== expectedContent) throw new Error(`write ${expectedContent} was acknowledged with different content`)
      if (acknowledgedWrites.has(messageID)) throw new Error(`write ${expectedContent} reused message id ${messageID}`)
      acknowledgedWrites.set(messageID, expectedContent)
    }
    return response
  }))
}

await inPool(tasks)

if (writes > 0) {
  if (acknowledgedWrites.size !== writes) {
    throw new Error(`only ${acknowledgedWrites.size}/${writes} unique writes returned verifiable acknowledgements`)
  }
  const unverifiedWrites = new Map(acknowledgedWrites)
  let beforeID = 0
  const maxPages = Math.ceil(writes / 100) + 2
  for (let page = 0; page < maxPages && unverifiedWrites.size > 0; page++) {
    const query = new URLSearchParams({ room_type: 'group', game_id: 'lobby', limit: '100' })
    if (beforeID > 0) query.set('before_id', String(beforeID))
    const persistedResponse = await fetchWithTimeout(`${baseURL}/api/member/chat/messages?${query}`, { headers: memberHeaders })
    if (!persistedResponse.ok) throw new Error(`chat persistence verification page ${page + 1} failed: HTTP ${persistedResponse.status}`)
    const persistedBody = await persistedResponse.json()
    const items = persistedBody.data?.items
    if (!Array.isArray(items)) throw new Error(`chat persistence verification page ${page + 1} did not return items`)
    for (const item of items) {
      const messageID = Number(item?.id)
      if (!unverifiedWrites.has(messageID)) continue
      const expectedContent = unverifiedWrites.get(messageID)
      if (item?.content !== expectedContent) throw new Error(`persisted write ${messageID} content does not match its acknowledgement`)
      unverifiedWrites.delete(messageID)
    }
    if (unverifiedWrites.size === 0 || persistedBody.data?.has_more !== true) break
    const nextBeforeID = Number(persistedBody.data?.next_before_id)
    if (!Number.isSafeInteger(nextBeforeID) || nextBeforeID <= 0 || nextBeforeID === beforeID) {
      throw new Error('chat persistence pagination returned an invalid next_before_id')
    }
    beforeID = nextBeforeID
  }
  if (unverifiedWrites.size > 0) {
    throw new Error(`chat persistence verification missed ${unverifiedWrites.size}/${writes} acknowledged unique writes`)
  }
}

const wsProtocol = parsedBaseURL.protocol === 'https:' ? 'wss:' : 'ws:'
async function openWebSocket(index) {
  const started = performance.now()
  try {
    const ticketResponse = await fetchWithTimeout(`${baseURL}/api/member/ws-ticket`, { method: 'POST', headers: memberHeaders })
    if (!ticketResponse.ok) throw new Error(`WebSocket ticket ${index} failed: HTTP ${ticketResponse.status}`)
    const ticketBody = await ticketResponse.json()
    const ticket = ticketBody.data?.ticket
    if (!ticket) throw new Error(`WebSocket ticket ${index} was empty`)
    const socketURL = new URL('/api/ws', baseURL)
    socketURL.protocol = wsProtocol
    socketURL.searchParams.set('ticket', ticket)
    await new Promise((resolve, reject) => {
      const socket = new WebSocket(socketURL, {
        origin: process.env.LOAD_TEST_ORIGIN || 'https://127.0.0.1:4173',
        rejectUnauthorized: !loopback,
      })
      const timeout = setTimeout(() => { socket.terminate(); reject(new Error(`WebSocket ${index} open timed out`)) }, 5000)
      socket.once('open', () => { clearTimeout(timeout); socket.close(); resolve() })
      socket.once('error', (error) => { clearTimeout(timeout); reject(error) })
    })
  } catch (error) {
    errors++
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`)
  } finally {
    samples.push(performance.now() - started)
  }
}
await inPool(Array.from({ length: websocketConnections }, (_, index) => () => openWebSocket(index)))

samples.sort((left, right) => left - right)
const total = samples.length
const p95 = total ? samples[Math.min(total - 1, Math.ceil(total * 0.95) - 1)] : 0
const errorRate = total ? errors / total : 1
const summary = {
  target: parsedBaseURL.origin,
  http_requests: reads + writes,
  websocket_connections: websocketConnections,
  samples: total,
  errors,
  error_rate: errorRate,
  p95_ms: Math.round(p95 * 10) / 10,
}
process.stdout.write(`${JSON.stringify(summary)}\n`)
if (errorRate > maxErrorRate) throw new Error(`error rate ${errorRate} exceeds ${maxErrorRate}`)
if (p95 > maxP95Ms) throw new Error(`p95 ${p95.toFixed(1)}ms exceeds ${maxP95Ms}ms`)
