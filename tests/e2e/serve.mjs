import fs from 'node:fs'
import https from 'node:https'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'
import httpProxy from 'http-proxy'

const here = path.dirname(fileURLToPath(import.meta.url))
const repositoryRoot = path.resolve(here, '..', '..')
const backendOrigin = process.env.SYSTEM_TEST_BACKEND_ORIGIN || 'http://127.0.0.1:18080'
const certFile = process.env.SYSTEM_TEST_TLS_CERT
const keyFile = process.env.SYSTEM_TEST_TLS_KEY
const memberRoot = path.resolve(process.env.SYSTEM_TEST_MEMBER_ROOT || path.join(repositoryRoot, 'release', 'member'))
const adminRoot = path.resolve(process.env.SYSTEM_TEST_ADMIN_ROOT || path.join(repositoryRoot, 'release', 'admin'))

if (!certFile || !keyFile) throw new Error('SYSTEM_TEST_TLS_CERT and SYSTEM_TEST_TLS_KEY are required')
const backendURL = new URL(backendOrigin)
if (!['127.0.0.1', 'localhost', '::1'].includes(backendURL.hostname)) {
  throw new Error(`Refusing to proxy a non-loopback backend: ${backendURL.hostname}`)
}

const tls = { cert: fs.readFileSync(certFile), key: fs.readFileSync(keyFile) }
const proxy = httpProxy.createProxyServer({ target: backendOrigin, changeOrigin: true, xfwd: true, ws: true })
proxy.on('error', (error, _request, response) => {
  if (response && typeof response.writeHead === 'function' && !response.headersSent) {
    response.writeHead(502, { 'content-type': 'text/plain; charset=utf-8' })
    response.end('test proxy unavailable')
  }
  process.stderr.write(`test proxy error: ${error.message}\n`)
})

const contentTypes = new Map([
  ['.css', 'text/css; charset=utf-8'], ['.gif', 'image/gif'], ['.html', 'text/html; charset=utf-8'],
  ['.ico', 'image/x-icon'], ['.jpeg', 'image/jpeg'], ['.jpg', 'image/jpeg'], ['.js', 'text/javascript; charset=utf-8'],
  ['.json', 'application/json; charset=utf-8'], ['.png', 'image/png'], ['.svg', 'image/svg+xml'], ['.webp', 'image/webp'],
])

function isBackendPath(url) {
  const pathname = new URL(url || '/', 'https://localhost').pathname
  return pathname === '/health' || pathname === '/ready' || pathname === '/api' || pathname.startsWith('/api/')
}

function staticHandler(root) {
  const normalizedRoot = path.resolve(root)
  return (request, response) => {
    if (isBackendPath(request.url)) {
      proxy.web(request, response)
      return
    }
    let pathname
    try {
      pathname = decodeURIComponent(new URL(request.url || '/', 'https://localhost').pathname)
    } catch {
      response.writeHead(400).end()
      return
    }
    const relative = pathname.replace(/^\/+/, '')
    let target = path.resolve(normalizedRoot, relative || 'index.html')
    if (target !== normalizedRoot && !target.startsWith(`${normalizedRoot}${path.sep}`)) {
      response.writeHead(403).end()
      return
    }
    if (!fs.existsSync(target) || !fs.statSync(target).isFile()) target = path.join(normalizedRoot, 'index.html')
    const extension = path.extname(target).toLowerCase()
    response.writeHead(200, {
      'cache-control': extension === '.html' ? 'no-store' : 'public, max-age=60',
      'content-type': contentTypes.get(extension) || 'application/octet-stream',
      'x-content-type-options': 'nosniff',
    })
    fs.createReadStream(target).pipe(response)
  }
}

function listen(name, root, port) {
  if (!fs.existsSync(path.join(root, 'index.html'))) throw new Error(`${name} build is missing: ${root}`)
  const server = https.createServer(tls, staticHandler(root))
  server.on('upgrade', (request, socket, head) => {
    if (!isBackendPath(request.url)) {
      socket.destroy()
      return
    }
    proxy.ws(request, socket, head)
  })
  server.listen(port, '127.0.0.1', () => process.stdout.write(`${name} test server: https://127.0.0.1:${port}\n`))
  return server
}

const servers = [
  listen('member release', memberRoot, Number(process.env.E2E_MEMBER_PORT || 4173)),
  listen('admin release', adminRoot, Number(process.env.E2E_ADMIN_PORT || 4174)),
]

const shutdown = () => {
  for (const server of servers) server.close()
  proxy.close()
}
process.once('SIGINT', shutdown)
process.once('SIGTERM', shutdown)
