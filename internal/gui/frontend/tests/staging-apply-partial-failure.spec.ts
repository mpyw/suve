import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  createStagedValue,
  navigateTo,
} from './fixtures/wails-mock';

// "Apply All" is one all-service backend call (#982), and handleApply reloads
// the staging view in a finally block (#477), so the view always matches the
// backend after a rejection: a hard failure applies nothing, and every service
// stays staged with the badge unchanged.

const applyAllButton = (page: Page) => page.getByRole('button', { name: 'Apply All' });
const stagingBadge = (page: Page) => page.locator('.staging-count');
const paramEntry = (page: Page) => page.locator('.entry-item').filter({ hasText: '/app/param-a' });
const secretEntry = (page: Page) => page.locator('.entry-item').filter({ hasText: 'secret-a' });

test.describe('Staging "Apply All" hard failure (#477, #982)', () => {
  test('a failing service applies nothing and the view stays reconciled', async ({ page }) => {
    await setupWailsMocks(page, {
      stagedParam: [createStagedValue('/app/param-a', 'create', 'pv')],
      stagedSecret: [createStagedValue('secret-a', 'create', 'sv')],
      stagingApplyFailService: 'secret',
    });
    await page.goto('/');
    await navigateTo(page, 'Staging');

    await expect(paramEntry(page)).toBeVisible();
    await expect(secretEntry(page)).toBeVisible();
    await expect(stagingBadge(page)).toHaveText('2');

    await applyAllButton(page).click();
    await page.locator('.form-actions').getByRole('button', { name: 'Apply', exact: true }).click();

    // The failure surfaces as a modal error.
    await expect(page.locator('.modal-error')).toContainText('staging apply failed for secret');

    // Nothing was applied: both services are still staged after the reload.
    await expect(paramEntry(page)).toHaveCount(1);
    await expect(secretEntry(page)).toHaveCount(1);
    await expect(stagingBadge(page)).toHaveText('2');
  });
});
