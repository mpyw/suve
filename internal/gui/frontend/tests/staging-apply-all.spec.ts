import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  createStagedValue,
  navigateTo,
} from './fixtures/wails-mock';

// Regression: "Apply All" must apply EVERY service with staged changes in a
// single click. It used to open the apply modal for just the first non-empty
// service (param if present, else secret), so applying both Param and Secret
// took two clicks.

function bothStagedState() {
  return {
    stagedParam: [createStagedValue('/app/param-a', 'create', 'pv')],
    stagedSecret: [createStagedValue('secret-a', 'create', 'sv')],
  };
}

const applyAllButton = (page: Page) => page.getByRole('button', { name: 'Apply All' });

test.describe('Staging "Apply All"', () => {
  test('applies both Param and Secret in a single click', async ({ page }) => {
    await setupWailsMocks(page, bothStagedState());
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // Both services are staged before applying.
    await expect(page.locator('.entry-item').filter({ hasText: '/app/param-a' })).toBeVisible();
    await expect(page.locator('.entry-item').filter({ hasText: 'secret-a' })).toBeVisible();

    await applyAllButton(page).click();

    // Confirm in the modal (its own "Apply" button in .form-actions, not the
    // per-section "Apply" buttons nor "Apply All").
    await page.locator('.form-actions').getByRole('button', { name: 'Apply', exact: true }).click();

    // The result lists BOTH services' entries — proof one click applied both.
    const resultNames = page.locator('.result-name');
    await expect(resultNames.filter({ hasText: '/app/param-a' })).toHaveCount(1);
    await expect(resultNames.filter({ hasText: 'secret-a' })).toHaveCount(1);

    // Close: the staging area is now empty for BOTH services (before the fix the
    // secret would remain, needing a second click).
    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('.entry-item').filter({ hasText: '/app/param-a' })).toHaveCount(0);
    await expect(page.locator('.entry-item').filter({ hasText: 'secret-a' })).toHaveCount(0);
  });
});
