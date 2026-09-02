import assert from 'node:assert/strict'
import childProcess from 'node:child_process'
import { createHash } from 'node:crypto'
import { test } from 'node:test'
import { createLoginCaptchaFixture, validateLoginCaptchaFixtureEnvironment } from './login-captcha-fixture.mjs'

const environment = {
  SYSTEM_TEST_ALLOW_LOCAL: '1',
  SYSTEM_TEST_BACKEND_ORIGIN: 'http://127.0.0.1:18080',
  SYSTEM_TEST_REDIS_ADDR: '127.0.0.1:16379',
  SYSTEM_TEST_REDIS_USERNAME: 'wangzhe-app',
  SYSTEM_TEST_REDIS_PASSWORD: 'FixturePassword#2026_NotReal',
  SYSTEM_TEST_REDIS_PREFIX: 'wangzhe-system-test:unit_123',
}

function installEnvironment(t) {
  const previous = Object.fromEntries(Object.keys(environment).map(key => [key, process.env[key]]))
  Object.assign(process.env, environment)
  t.after(() => {
    for (const [key, value] of Object.entries(previous)) {
      if (value === undefined) delete process.env[key]
      else process.env[key] = value
    }
  })
}

test('allows only explicit local isolated fixture environments', () => {
  const config = validateLoginCaptchaFixtureEnvironment(environment)
  assert.equal(config.host, '127.0.0.1')
  assert.equal(config.prefix, 'wangzhe-system-test:unit_123')
  assert.equal(validateLoginCaptchaFixtureEnvironment({ ...environment, SYSTEM_TEST_REDIS_ADDR: '[::1]:6379', SYSTEM_TEST_BACKEND_ORIGIN: 'https://[::1]:18080' }).host, '::1')
})

for (const [key, value] of [
  ['SYSTEM_TEST_ALLOW_LOCAL', undefined],
  ['SYSTEM_TEST_ALLOW_LOCAL', 'true'],
  ['SYSTEM_TEST_BACKEND_ORIGIN', undefined],
  ['SYSTEM_TEST_BACKEND_ORIGIN', 'https://example.invalid'],
  ['SYSTEM_TEST_BACKEND_ORIGIN', 'http://127.0.0.1.example.invalid'],
  ['SYSTEM_TEST_BACKEND_ORIGIN', 'http://user:password@127.0.0.1:18080'],
  ['SYSTEM_TEST_BACKEND_ORIGIN', 'http://127.0.0.1:18080/api'],
  ['SYSTEM_TEST_BACKEND_ORIGIN', 'http://127.0.0.1:18080/?remote=1'],
  ['SYSTEM_TEST_BACKEND_ORIGIN', 'ftp://127.0.0.1:18080'],
  ['SYSTEM_TEST_REDIS_ADDR', undefined],
  ['SYSTEM_TEST_REDIS_ADDR', 'redis.example.invalid:6379'],
  ['SYSTEM_TEST_REDIS_ADDR', '127.0.0.1:0'],
  ['SYSTEM_TEST_REDIS_ADDR', '127.0.0.1:65536'],
  ['SYSTEM_TEST_REDIS_ADDR', '127.0.0.1:6379;other-command'],
  ['SYSTEM_TEST_REDIS_PREFIX', undefined],
  ['SYSTEM_TEST_REDIS_PREFIX', 'wangzhe'],
  ['SYSTEM_TEST_REDIS_PREFIX', 'wangzhe-system-test:'],
  ['SYSTEM_TEST_REDIS_PREFIX', 'wangzhe-system-test:run:other'],
  ['SYSTEM_TEST_REDIS_PREFIX', 'wangzhe-system-test:run*'],
  ['SYSTEM_TEST_REDIS_PREFIX', `wangzhe-system-test:${'a'.repeat(53)}`],
  ['SYSTEM_TEST_REDIS_USERNAME', ''],
  ['SYSTEM_TEST_REDIS_USERNAME', 'name --raw'],
  ['SYSTEM_TEST_REDIS_PASSWORD', ''],
  ['SYSTEM_TEST_REDIS_PASSWORD', 'short'],
]) {
  test(`rejects unsafe fixture ${key}: ${key.includes('PASSWORD') ? '<redacted>' : String(value)}`, () => {
    assert.throws(() => validateLoginCaptchaFixtureEnvironment({ ...environment, [key]: value }))
  })
}

test('creates a random one-use purpose/IP-bound digest through argv, never a shell or a plaintext answer', async t => {
  installEnvironment(t)
  const calls = []
  t.mock.method(childProcess, 'execFile', (command, args, options, callback) => {
    calls.push({ command, args, options })
    callback(null, 'OK\n')
  })
  const first = await createLoginCaptchaFixture('management', '198.51.100.30')
  const second = await createLoginCaptchaFixture('member', '198.51.100.31')
  assert.match(first.captcha_id, /^[a-f0-9]{32}$/)
  assert.match(first.captcha_code, /^\d{6}$/)
  assert.notEqual(first.captcha_id, second.captcha_id)
  assert.equal(calls.length, 2)
  const { command, args, options } = calls[0]
  assert.equal(command, 'redis-cli')
  assert.equal(options.shell, undefined)
  assert.equal(options.timeout, 5000)
  assert.equal(options.env.REDISCLI_AUTH, environment.SYSTEM_TEST_REDIS_PASSWORD)
  assert.equal(args.includes(environment.SYSTEM_TEST_REDIS_PASSWORD), false)
  assert.equal(args.includes(first.captcha_code), false)
  const digest = createHash('sha256').update([first.captcha_id, 'management', '198.51.100.30', first.captcha_code].join('\0')).digest('hex')
  assert.deepEqual(args.slice(-6), ['SET', `${environment.SYSTEM_TEST_REDIS_PREFIX}:captcha:${first.captcha_id}`, digest, 'EX', '120', 'NX'])
})

test('rejects invalid purpose/IP and unguarded creation before executing redis-cli', async t => {
  installEnvironment(t)
  const execute = t.mock.method(childProcess, 'execFile', () => { throw new Error('Must not execute') })
  await assert.rejects(createLoginCaptchaFixture('admin', '127.0.0.1'), /purpose/)
  await assert.rejects(createLoginCaptchaFixture('member', '127.0.0.1,198.51.100.1'), /client IP/)
  delete process.env.SYSTEM_TEST_ALLOW_LOCAL
  await assert.rejects(createLoginCaptchaFixture('member', '127.0.0.1'), /SYSTEM_TEST_ALLOW_LOCAL/)
  assert.equal(execute.mock.callCount(), 0)
})

test('does not disclose Redis diagnostics or fixture credentials on failure', async t => {
  installEnvironment(t)
  t.mock.method(childProcess, 'execFile', (_command, _args, _options, callback) => {
    callback(new Error(`AUTH failed ${environment.SYSTEM_TEST_REDIS_PASSWORD}`))
  })
  await assert.rejects(createLoginCaptchaFixture('member', '127.0.0.1'), { message: 'Could not prepare the isolated login captcha fixture' })
})

test('rejects a failed NX insertion instead of using an existing challenge', async t => {
  installEnvironment(t)
  t.mock.method(childProcess, 'execFile', (_command, _args, _options, callback) => callback(null, '\n'))
  await assert.rejects(createLoginCaptchaFixture('member', '127.0.0.1'), /Could not prepare/)
})
