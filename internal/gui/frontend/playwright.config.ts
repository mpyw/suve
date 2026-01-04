import { defineConfig, devices } from '@playwright/test';

const PORT = process.env.VITE_PORT || '5173';
const BASE_URL = `http://localhost:${PORT}`;

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  // Use more workers locally, and configurable in CI via env
  workers: process.env.CI ? (process.env.PW_WORKERS ? parseInt(process.env.PW_WORKERS) : 4) : undefined,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'html',
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    // Enable cross-browser testing locally (CI only installs chromium for speed)
    ...(!process.env.CI ? [
      {
        name: 'firefox',
        use: { ...devices['Desktop Firefox'] },
      },
      {
        name: 'webkit',
        use: { ...devices['Desktop Safari'] },
      },
    ] : []),
  ],
  webServer: {
    command: `npx vite --port ${PORT}`,
    url: BASE_URL,
    reuseExistingServer: !process.env.CI,
    timeout: 120000,
  },
});
