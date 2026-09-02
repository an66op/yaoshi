import { constants } from 'node:fs'
import { open } from 'node:fs/promises'
import { isAbsolute } from 'node:path'

const memberCookiePattern = /^wangzhe_member_session=[A-Za-z0-9._~-]+$/

function validateSession(session, expectedOrigin) {
  if (!session || session.origin !== new URL(expectedOrigin).origin || typeof session.cookie !== 'string' || !memberCookiePattern.test(session.cookie)) {
    throw new Error('Member session file is invalid or belongs to a different target origin')
  }
  return session.cookie
}

export async function writeMemberSessionFile(path, origin, cookie) {
  if (!path || !isAbsolute(path)) throw new Error('An absolute SYSTEM_TEST_MEMBER_COOKIE_FILE path is required')
  const session = { origin: new URL(origin).origin, cookie }
  validateSession(session, origin)
  // Exclusive creation in the private harness directory rejects symlinks and
  // avoids overwriting an operator-provided session or another test run.
  const file = await open(path, 'wx', 0o600)
  try { await file.writeFile(`${JSON.stringify(session)}\n`, 'utf8') } finally { await file.close() }
}

export async function readMemberSessionFile(path, expectedOrigin) {
  if (!path || !isAbsolute(path)) throw new Error('An absolute pre-authenticated member cookie file path is required')
  const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW)
  try {
    const stat = await file.stat()
    if (!stat.isFile() || (stat.mode & 0o777) !== 0o600 || stat.size > 8192 ||
      (typeof process.getuid === 'function' && stat.uid !== process.getuid())) {
      throw new Error('Member session file must be an owned regular 0600 file of at most 8192 bytes')
    }
    let session
    try { session = JSON.parse(await file.readFile('utf8')) } catch { throw new Error('Member session file does not contain valid JSON') }
    return validateSession(session, expectedOrigin)
  } finally {
    await file.close()
  }
}
