import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './',
  testMatch: 'all-14-scenarios.spec.ts',
  timeout: 180000,
  expect: { timeout: 10000 },
  use: {
    baseURL: 'http://localhost:3000',
    headless: true,
    viewport: { width: 1440, height: 900 },
    actionTimeout: 15000,
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
});
