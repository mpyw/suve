import { test, expect } from './fixtures/coverage';
import {
  setupWailsMocks,
  createErrorState,
  navigateTo,
  getRecordedCalls,
} from './fixtures/wails-mock';

// #1009 — StagingDiff makes network calls, so the Staging view calls it once per
// service without the startup retry helper: a real failure (expired
// credentials) shows at once instead of after ~50 retries over 5 s.

test('a failing StagingDiff is shown at once, without retries', async ({ page }) => {
  await setupWailsMocks(page, createErrorState('StagingDiff', 'ExpiredToken: credentials expired'));
  await page.goto('/');
  await navigateTo(page, 'Staging');

  await expect(page.getByText('ExpiredToken: credentials expired')).toBeVisible({ timeout: 2000 });

  // One call per service (AWS: param + secret), no retry loop.
  const diffCalls = (await getRecordedCalls(page)).filter((c) => c === 'StagingDiff');
  expect(diffCalls).toHaveLength(2);
});
