import { defineConfig, devices } from '@playwright/test'
import path from 'node:path'

const memberBaseURL = process.env.E2E_MEMBER_BASE_URL || 'https://127.0.0.1:4173'
const adminBaseURL = process.env.E2E_ADMIN_BASE_URL || 'https://127.0.0.1:4174'
const browserExecutable = process.env.E2E_BROWSER_EXECUTABLE_PATH || ''
if (browserExecutable && !path.isAbsolute(browserExecutable)) {
  throw new Error('E2E_BROWSER_EXECUTABLE_PATH must be an absolute path')
}

export default defineConfig({
  testDir: '.',
  testMatch: '**/*.spec.mjs',
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [['line'], ['html', { open: 'never' }]] : 'line',
  outputDir: 'test-results',
  use: {
    ...devices['Desktop Chrome'],
    ignoreHTTPSErrors: true,
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    video: 'retain-on-failure',
    launchOptions: browserExecutable ? { executablePath: browserExecutable } : undefined,
  },
  metadata: { memberBaseURL, adminBaseURL },
})
