import { expect, test } from '@playwright/test'

const memberBaseURL = process.env.E2E_MEMBER_BASE_URL || 'https://127.0.0.1:4173'
const adminBaseURL = process.env.E2E_ADMIN_BASE_URL || 'https://127.0.0.1:4174'
const adminUsername = process.env.E2E_ADMIN_USERNAME || 'e2e_platform'
const adminPassword = process.env.E2E_ADMIN_PASSWORD || 'ProdBootstrap#2026_x9Q'
const memberUsername = process.env.E2E_MEMBER_USERNAME || 'e2e_member'
const memberPassword = process.env.E2E_MEMBER_PASSWORD || 'MemberPass#2026_x9Q'
const boundaryUsername = process.env.E2E_BOUNDARY_USERNAME || `${'u'.repeat(46)}2026`
const boundaryPassword = process.env.E2E_BOUNDARY_PASSWORD || `Aa1#${'z'.repeat(68)}`
const roomCode = process.env.E2E_ROOM_CODE || '88001'

test('release mode rejects legacy public management registration', async ({ request }) => {
  const response = await request.post(`${adminBaseURL}/api/register`, {
    headers: { 'x-forwarded-for': '198.51.100.40' },
    data: {
      username: `forbidden_${Date.now()}`,
      password: 'ShouldNeverCreate#2026',
      email: `forbidden_${Date.now()}@example.invalid`,
    },
  })
  expect(response.status()).toBe(404)
})

test('member registration UI uses the release endpoint and receives a protected session', async ({ page, context }) => {
  const username = `ui_${Date.now().toString(36)}`
  const password = 'BrowserMember#2026_x9Q'
  expect(username.length).toBeLessThanOrEqual(20)

  await page.setExtraHTTPHeaders({ 'x-forwarded-for': '198.51.100.41' })
  await page.goto(`${memberBaseURL}/register`)
  const astralRegistrationAccount = '🚀'.repeat(20)
  await page.getByPlaceholder('3–20 位').fill(astralRegistrationAccount)
  await expect(page.getByPlaceholder('3–20 位')).toHaveValue(astralRegistrationAccount)
  await page.getByPlaceholder('3–20 位').fill(username)
  await page.getByPlaceholder('8–72 字节').fill(password)
  const registrationPromise = page.waitForResponse(response =>
    response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/member/register')
  await page.getByRole('button', { name: /注册并登录/ }).click()
  const registration = await registrationPromise
  expect(registration.status()).toBe(201)
  const registrationBody = await registration.json()
  expect(registrationBody.data?.user?.username).toBe(username)
  expect(registrationBody.data).not.toHaveProperty('token')
  await expect(page.getByRole('heading', { name: '输入房间号' })).toBeVisible()

  // The session is intentionally scoped to /api and therefore is not
  // applicable to the site's root URL.
  const cookies = await context.cookies(`${memberBaseURL}/api/member/me`)
  const session = cookies.find(cookie => cookie.name === 'wangzhe_member_session')
  expect(session).toMatchObject({ httpOnly: true, secure: true, sameSite: 'Lax' })

  await context.clearCookies()
  await page.goto(`${memberBaseURL}/register`)
  await page.getByPlaceholder('3–20 位').fill(username)
  await page.getByPlaceholder('8–72 字节').fill(password)
  const duplicatePromise = page.waitForResponse(response =>
    response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/member/register')
  await page.getByRole('button', { name: /注册并登录/ }).click()
  expect((await duplicatePromise).status()).toBe(409)
  await expect(page.getByRole('alert')).toBeVisible()

  const tooLongStatus = await page.evaluate(async ({ overlongUsername, validPassword }) => {
    const response = await fetch('/api/member/register', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ username: overlongUsername, password: validPassword }),
    })
    return response.status
  }, { overlongUsername: 'u'.repeat(21), validPassword: password })
  expect(tooLongStatus).toBe(400)
})

test('platform administrator can sign in and open critical management pages', async ({ page }) => {
  await page.setExtraHTTPHeaders({ 'x-forwarded-for': '198.51.100.42' })
  await page.goto(adminBaseURL)
  const astralAdminAccount = '🚀'.repeat(50)
  await page.getByLabel('登录帐号').fill(astralAdminAccount)
  await expect(page.getByLabel('登录帐号')).toHaveValue(astralAdminAccount)
  await page.getByLabel('登录帐号').fill(adminUsername)
  await page.getByLabel('密码').fill(adminPassword)
  await page.getByRole('button', { name: '登录平台管理员' }).click()

  await expect(page.getByText('运营首页', { exact: true }).first()).toBeVisible()
  await page.getByText('会员管理', { exact: true }).first().click()
  await expect(page).toHaveURL(/\/members$/)
  await expect(page.getByText('会员管理', { exact: true }).first()).toBeVisible()

  await page.getByText('飞单管理', { exact: true }).first().click()
  await expect(page).toHaveURL(/\/fly-orders$/)
  await expect(page.getByText('飞单管理', { exact: true }).first()).toBeVisible()

  await page.goto(`${adminBaseURL}/results`)
  await expect(page.getByText('开奖管理', { exact: true }).first()).toBeVisible()
  await page.goto(`${adminBaseURL}/limits`)
  await expect(page.getByText('赔率与回水', { exact: true }).first()).toBeVisible()
  await page.goto(`${adminBaseURL}/monitor`)
  await expect(page.getByLabel('登录帐号')).toHaveCount(0)
  await expect(page.locator('main, [role="main"]').first()).toBeVisible()
})

test('member can sign in, enter the assigned room and use primary navigation', async ({ page }) => {
  await page.setExtraHTTPHeaders({ 'x-forwarded-for': '198.51.100.43' })
  await page.goto(`${memberBaseURL}/login`)
  await page.getByPlaceholder('输入帐号').fill(memberUsername)
  await page.getByPlaceholder('输入登录密码').fill(memberPassword)
  await page.getByRole('button', { name: /验证并继续/ }).click()

  await expect(page.getByRole('heading', { name: '输入房间号' })).toBeVisible()
  await page.locator('#room-entry-code').fill(roomCode)
  await page.getByRole('button', { name: '进入房间' }).click()

  await expect(page.getByRole('button', { name: /游戏大厅/ })).toBeVisible()
  await page.getByRole('button', { name: /聊天/ }).click()
  await expect(page).toHaveURL(/\/messages$/)
  await page.getByRole('button', { name: /我的/ }).click()
  await expect(page).toHaveURL(/\/profile$/)
  await page.getByRole('button', { name: /游戏大厅/ }).click()
  await expect(page).toHaveURL(/\/lobby$/)
})

test('member login accepts the backend 50-character username and 72-byte password limits', async ({ page }) => {
  expect([...boundaryUsername]).toHaveLength(50)
  expect(Buffer.byteLength(boundaryPassword)).toBe(72)
  await page.setExtraHTTPHeaders({ 'x-forwarded-for': '198.51.100.44' })
  await page.goto(`${memberBaseURL}/login`)
  const astralMemberAccount = '🚀'.repeat(50)
  await page.getByPlaceholder('输入帐号').fill(astralMemberAccount)
  await expect(page.getByPlaceholder('输入帐号')).toHaveValue(astralMemberAccount)
  await page.getByPlaceholder('输入帐号').fill(boundaryUsername)
  await page.getByPlaceholder('输入登录密码').fill(boundaryPassword)
  await expect(page.getByPlaceholder('输入帐号')).toHaveValue(boundaryUsername)
  await expect(page.getByPlaceholder('输入登录密码')).toHaveValue(boundaryPassword)
  await page.getByRole('button', { name: /验证并继续/ }).click()
  await expect(page.getByRole('heading', { name: '输入房间号' })).toBeVisible()
})
