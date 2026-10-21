import { defineConfig } from 'vite'
import { configDefaults } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'jsdom',
    globals: true,
    // e2e/ holds Playwright specs (run via `npm run test:e2e`, not
    // vitest) — without this, vitest's default *.spec.js glob also
    // picks them up and fails, since Playwright's `test` API isn't
    // valid inside vitest's runner.
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
})
