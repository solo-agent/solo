import { defineConfig } from '@playwright/test';

const frontendPort = process.env.FRONTEND_PORT ?? '3000';
const serverPort = process.env.SERVER_PORT ?? '8080';

process.env.SOLO_E2E_API_URL ??= `http://127.0.0.1:${serverPort}`;

export default defineConfig({
  testDir: './e2e',
  timeout: 600000,
  expect: { timeout: 180000 },
  use: {
    baseURL: `http://localhost:${frontendPort}`,
    headless: Boolean(process.env.CI),
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  retries: 0,
  workers: 1,
});
