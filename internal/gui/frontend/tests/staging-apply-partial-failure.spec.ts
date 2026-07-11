import { test, expect, type Page } from '@playwright/test';
import {
  setupWailsMocks,
  createStagedValue,
  navigateTo,
} from './fixtures/wails-mock';

// Regression (#477): "Apply All" applies each service in a separate backend
// call, and every success unstages that service's entries on disk. When a later
// service rejects (conflict / per-entry failure) the GUI used to jump to catch
// without reconciling, so the already-applied service kept rendering as pending
// with a stale badge until a manual Refresh. handleApply/handleReset now run
// loadStatus() in a finally block so the view always matches the backend.

const applyAllButton = (page: Page) => page.getByRole('button', { name: 'Apply All' });
const stagingBadge = (page: Page) => page.locator('.staging-count');
const paramEntry = (page: Page) => page.locator('.entry-item').filter({ hasText: '/app/param-a' });
const secretEntry = (page: Page) => page.locator('.entry-item').filter({ hasText: 'secret-a' });

test.describe('Staging "Apply All" partial failure (#477)', () => {
  test('applied service is reconciled after a later service rejects', async ({ page }) => {
    // param applies first and unstages; secret rejects and stays staged.
    await setupWailsMocks(page, {
      stagedParam: [createStagedValue('/app/param-a', 'create', 'pv')],
      stagedSecret: [createStagedValue('secret-a', 'create', 'sv')],
      stagingApplyFailService: 'secret',
    });
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // Both services staged before applying; badge shows the combined count of 2.
    await expect(paramEntry(page)).toBeVisible();
    await expect(secretEntry(page)).toBeVisible();
    await expect(stagingBadge(page)).toHaveText('2');

    await applyAllButton(page).click();
    await page.locator('.form-actions').getByRole('button', { name: 'Apply', exact: true }).click();

    // The secret failure surfaces as a modal error (the apply rejected).
    await expect(page.locator('.modal-error')).toContainText('staging apply failed for secret');

    // Despite the rejection, the finally-block loadStatus reconciled the view:
    // the applied param entry is gone (before the fix it stayed pending), while
    // the failed secret entry remains staged. The badge dropped 2 -> 1.
    await expect(paramEntry(page)).toHaveCount(0);
    await expect(secretEntry(page)).toHaveCount(1);
    await expect(stagingBadge(page)).toHaveText('1');
  });
});
