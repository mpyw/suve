import { test, expect, type Page } from '@playwright/test';
import {
  setupWailsMocks,
  createStagedValue,
  navigateTo,
} from './fixtures/wails-mock';

// Regression: "Reset All" must reset EVERY service with staged changes in a
// single confirm. It used to fire openResetModal('param') then
// openResetModal('secret') back-to-back, so resetService ended up 'secret' and
// only the secret service was reset — param changes silently remained staged.

function bothStagedState() {
  return {
    stagedParam: [createStagedValue('/app/param-a', 'create', 'pv')],
    stagedSecret: [createStagedValue('secret-a', 'create', 'sv')],
  };
}

const resetAllButton = (page: Page) => page.getByRole('button', { name: 'Reset All' });

test.describe('Staging "Reset All"', () => {
  test('resets both Param and Secret in a single confirm', async ({ page }) => {
    await setupWailsMocks(page, bothStagedState());
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // Both services are staged before resetting.
    await expect(page.locator('.entry-item').filter({ hasText: '/app/param-a' })).toBeVisible();
    await expect(page.locator('.entry-item').filter({ hasText: 'secret-a' })).toBeVisible();

    await resetAllButton(page).click();

    // The modal addresses ALL services, not just one.
    await expect(page.locator('.modal')).toContainText('all services');

    // Confirm (the modal's own danger button in .form-actions).
    await page.locator('.form-actions').locator('.btn-danger').click();

    // BOTH services are now empty — before the fix the param entry remained.
    await expect(page.locator('.entry-item').filter({ hasText: '/app/param-a' })).toHaveCount(0);
    await expect(page.locator('.entry-item').filter({ hasText: 'secret-a' })).toHaveCount(0);
  });
});
