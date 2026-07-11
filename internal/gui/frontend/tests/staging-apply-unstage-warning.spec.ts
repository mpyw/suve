import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createStagedValue,
  navigateTo,
} from './fixtures/wails-mock';

// Regression (#447): when the cloud write succeeds but clearing the staged
// entry afterwards fails, the apply result carries an unstageError. The GUI used
// to drop it (the DTO had no field and the mapping ignored it), so the entry
// silently survived while the UI reported success. The apply-result view must
// now surface it as a warning row.

test.describe('Staging apply surfaces a post-apply unstage failure (#447)', () => {
  test('an applied-but-still-staged entry shows a warning', async ({ page }) => {
    await setupWailsMocks(page, {
      stagedParam: [createStagedValue('/app/param-a', 'create', 'pv')],
      stagingApplyUnstageErrorService: 'param',
    });
    await page.goto('/');
    await navigateTo(page, 'Staging');

    await expect(page.locator('.entry-item').filter({ hasText: '/app/param-a' })).toBeVisible();

    // Open the param apply flow and confirm.
    await page.getByRole('button', { name: /Apply/i }).first().click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    await page.locator('.form-actions').getByRole('button', { name: /Apply|Confirm/i }).first().click();

    // The result view renders and the entry reports its applied status plus the
    // unstage warning (before the fix, no warning appeared at all).
    await expect(page.getByRole('button', { name: 'Close' })).toBeVisible({ timeout: 10000 });

    const warning = page.getByTestId('unstage-warning');
    await expect(warning).toBeVisible();
    await expect(warning).toContainText('keychain locked');
  });
});
