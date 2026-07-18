import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  timeout: 600000,
  expect: { timeout: 180000 },
  use: {
    baseURL: 'http://localhost:3000',
    headless: Boolean(process.env.CI),
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  retries: 0,
  workers: 1,
});
