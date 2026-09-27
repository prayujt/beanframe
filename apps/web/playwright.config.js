import { defineConfig } from '@playwright/test';
import { fileURLToPath } from 'node:url';

export default defineConfig({
  testDir: './tests',
  workers: 1,
  timeout: 30_000,
  use: { baseURL: 'http://127.0.0.1:18762', headless: true },
  webServer: {
    command: 'node apps/web/tests/start-server.mjs',
    cwd: fileURLToPath(new URL('../../', import.meta.url)),
    env: {
      AUTH_DISABLED: 'true',
      LOG_LEVEL: 'warn',
      PYTHON: fileURLToPath(new URL('../../.venv/bin/python', import.meta.url)),
      LISTEN_ADDR: '127.0.0.1:18762',
      WEB_DIR: 'apps/web/build'
    },
    url: 'http://127.0.0.1:18762/healthz',
    gracefulShutdown: { signal: 'SIGTERM', timeout: 10_000 },
    reuseExistingServer: false
  }
});
