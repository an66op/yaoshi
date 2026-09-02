import childProcess from 'node:child_process'
import { createHash, randomBytes, randomInt } from 'node:crypto'
import { isIP } from 'node:net'

const loopbackHosts = new Set(['127.0.0.1', 'localhost', '[::1]'])

/** Test-process-only access to this run's Redis namespace, never an API bypass. */
export function validateLoginCaptchaFixtureEnvironment(environment = process.env) {
  if (environment.SYSTEM_TEST_ALLOW_LOCAL !== '1') throw new Error('SYSTEM_TEST_ALLOW_LOCAL=1 is required for captcha fixtures')
  let backend
  try { backend = new URL(environment.SYSTEM_TEST_BACKEND_ORIGIN) } catch { throw new Error('Captcha fixtures require an explicit loopback backend origin') }
  if (!['http:', 'https:'].includes(backend.protocol) || !loopbackHosts.has(backend.hostname) ||
    backend.username || backend.password || backend.pathname !== '/' || backend.search || backend.hash) {
    throw new Error('Captcha fixtures require a loopback HTTP(S) backend origin without credentials or a path')
  }
  const redis = /^(127\.0\.0\.1|localhost|\[::1\]):([0-9]{1,5})$/.exec(environment.SYSTEM_TEST_REDIS_ADDR || '')
  if (!redis || Number(redis[2]) < 1 || Number(redis[2]) > 65535) throw new Error('Captcha fixtures require an explicit loopback Redis host:port')
  if (!/^wangzhe-system-test:[a-z0-9_]{1,52}$/.test(environment.SYSTEM_TEST_REDIS_PREFIX || '')) {
    throw new Error('Captcha fixtures require an isolated wangzhe-system-test:<runid> Redis namespace')
  }
  if (!/^[A-Za-z0-9_.-]{1,64}$/.test(environment.SYSTEM_TEST_REDIS_USERNAME || '') ||
    typeof environment.SYSTEM_TEST_REDIS_PASSWORD !== 'string' || environment.SYSTEM_TEST_REDIS_PASSWORD.length < 24) {
    throw new Error('Captcha fixtures require explicit Redis ACL credentials with a password of at least 24 characters')
  }
  return {
    host: redis[1].replace(/^\[|\]$/g, ''), port: redis[2],
    username: environment.SYSTEM_TEST_REDIS_USERNAME,
    password: environment.SYSTEM_TEST_REDIS_PASSWORD,
    prefix: environment.SYSTEM_TEST_REDIS_PREFIX,
  }
}

export async function createLoginCaptchaFixture(purpose, clientIP) {
  const config = validateLoginCaptchaFixtureEnvironment()
  if (purpose !== 'management' && purpose !== 'member') throw new Error('Captcha fixture purpose must be management or member')
  if (typeof clientIP !== 'string' || isIP(clientIP) === 0) throw new Error('Captcha fixtures require an explicit valid client IP')
  const id = randomBytes(16).toString('hex')
  const code = String(randomInt(1_000_000)).padStart(6, '0')
  const digest = createHash('sha256').update(`${id}\0${purpose}\0${clientIP}\0${code}`).digest('hex')
  const args = [
    '--no-auth-warning', '--raw', '--user', config.username,
    '-h', config.host, '-p', config.port, '-n', '0',
    'SET', `${config.prefix}:captcha:${id}`, digest, 'EX', '120', 'NX',
  ]
  try {
    const stdout = await new Promise((resolve, reject) => {
      childProcess.execFile('redis-cli', args, {
        env: { ...process.env, REDISCLI_AUTH: config.password },
        timeout: 5000, maxBuffer: 4096, encoding: 'utf8',
      }, (error, output) => error ? reject(error) : resolve(output))
    })
    if (stdout.trim() !== 'OK') throw new Error('Fixture was not inserted')
  } catch {
    // Child-process diagnostics can contain credentials or fixture material.
    // Keep all such details out of test logs and assertion error messages.
    throw new Error('Could not prepare the isolated login captcha fixture')
  }
  return { captcha_id: id, captcha_code: code }
}
