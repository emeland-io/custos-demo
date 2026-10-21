import { defineConfig } from '@playwright/test'

// Dex's issuer hostname (dex.notebook-e2e.svc.cluster.local) only
// resolves inside the cluster's own DNS, but the OIDC authorization
// redirect the app sends is followed by this suite's real browser,
// running outside the cluster. E2E_DEX_ADDR is a local port-forward
// to Dex's Service (see test/e2e/internal/chart.go's
// StartDexPortForward and the "dexaddr" devcluster subcommand); the
// Chromium host-resolver-rules flag below routes that one hostname
// (including its literal :5556 port) to it.
const dexAddr = process.env.E2E_DEX_ADDR
const launchArgs = dexAddr
  ? [`--host-resolver-rules=MAP dex.notebook-e2e.svc.cluster.local:5556 ${dexAddr}`]
  : []

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: process.env.E2E_BASE_URL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    launchOptions: {
      args: launchArgs,
    },
  },
})
