import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright config for GUI demo recording.
 * Enables video recording and uses larger viewport for better visibility.
 *
 * Usage: npx playwright test gui-demo.spec.ts -c playwright.recording.config.ts --headed
 */

const PORT = process.env.VITE_PORT || '5173';
const BASE_URL = `http://localhost:${PORT}`;

export default defineConfig({
  testDir: '../../../demo',
  fullyParallel: false,
  workers: 1, // Single worker for recording
  reporter: 'list',
  timeout: 300000, // 5 minutes for demo with slow typing
  use: {
    baseURL: BASE_URL,
    // Enable video recording
    video: {
      mode: 'on',
      size: { width: 1280, height: 720 },
    },
    // Larger viewport for demo
    viewport: { width: 1280, height: 720 },
    // No trace needed for recording
    trace: 'off',
    screenshot: 'off',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // Note: No webServer config - wails dev starts the Vite server
  // The server should be running before running this test
  webServer: {
    command: 'echo "Waiting for wails dev..."',
    url: BASE_URL,
    reuseExistingServer: true,
    timeout: 60000,
  },
  // Output directory for video files
  outputDir: './test-results-recording',
});
