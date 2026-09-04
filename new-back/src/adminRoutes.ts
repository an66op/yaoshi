const routes = new Set(['/', '/members', '/robots', '/fly-orders', '/tenants', '/agents', '/applications', '/room-reviews', '/chat', '/lottery-chat', '/announcements', '/plans', '/reports', '/wallet', '/activities', '/monitor', '/bets', '/results', '/limits', '/board-report', '/lottery-network', '/interface-test', '/entertainment', '/special-numbers', '/menu-management', '/logs', '/data-maintenance', '/game-guide', '/system'])

export const isRetiredAccountPath = (pathname: string) => pathname === '/users' || pathname === '/users/'

// The retired mixed-account page must not be restored by bookmarks or saved menus.
// Each role already has its own correctly scoped member page at /members.
export function resolveAdminPath(pathname: string): string {
  if (isRetiredAccountPath(pathname)) return '/members'
  if (pathname === '/audit' || pathname === '/audit/') return '/logs'
  return routes.has(pathname) ? pathname : '/'
}
