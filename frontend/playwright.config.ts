import { defineConfig } from '@playwright/test';

const frontendPort = process.env.FRONTEND_PORT ?? '3000';

export default defineConfig({
  testDir: './e2e',
  timeout: 600000,
  expect: { timeout: 180000 },
  use: {
    baseURL:
      process.env.SOLO_E2E_BASE_URL ?? `http://localhost:${frontendPort}`,
    headless: Boolean(process.env.CI),
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  retries: 0,
  workers: 1,
});
