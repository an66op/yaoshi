import assert from 'node:assert/strict'
import { chmod, mkdtemp, readFile, rm, stat, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { readMemberSessionFile, writeMemberSessionFile } from './member-session-file.mjs'

const origin = 'http://127.0.0.1:18080'
const cookie = 'wangzhe_member_session=fixture.header.signature'

async function temporaryFile(t) {
  const directory = await mkdtemp(join(tmpdir(), 'yaoshi-session-file-test-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  return join(directory, 'member-session.json')
}

test('stores a 0600 origin-bound session and reads it only for that origin', async t => {
  const path = await temporaryFile(t)
  await writeMemberSessionFile(path, origin, cookie)
  assert.equal((await stat(path)).mode & 0o777, 0o600)
  assert.equal(await readMemberSessionFile(path, origin), cookie)
  await assert.rejects(readMemberSessionFile(path, 'https://example.invalid'), /different target origin/)
})

test('refuses to overwrite an existing session file', async t => {
  const path = await temporaryFile(t)
  await writeMemberSessionFile(path, origin, cookie)
  await assert.rejects(writeMemberSessionFile(path, origin, 'wangzhe_member_session=other'), { code: 'EEXIST' })
  assert.equal(JSON.parse(await readFile(path, 'utf8')).cookie, cookie)
})

test('rejects unsafe permissions and symlink inputs', async t => {
  const path = await temporaryFile(t)
  await writeMemberSessionFile(path, origin, cookie)
  await chmod(path, 0o644)
  await assert.rejects(readMemberSessionFile(path, origin), /0600/)
  await chmod(path, 0o600)
  await symlink(path, `${path}.link`)
  await assert.rejects(readMemberSessionFile(`${path}.link`, origin))
})

test('rejects relative paths and header-injection cookie content', async t => {
  const path = await temporaryFile(t)
  await assert.rejects(writeMemberSessionFile('relative.json', origin, cookie), /absolute/)
  await assert.rejects(readMemberSessionFile('relative.json', origin), /absolute/)
  await assert.rejects(writeMemberSessionFile(path, origin, `${cookie}\r\nInjected: value`), /invalid/)
  await writeFile(path, JSON.stringify({ origin, cookie: `${cookie}; other=secret` }), { mode: 0o600 })
  await assert.rejects(readMemberSessionFile(path, origin), /invalid/)
})
