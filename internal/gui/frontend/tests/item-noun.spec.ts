import { test, expect } from './fixtures/coverage';
import {
  setupWailsMocks,
  createAzureNamespaceState,
  waitForItemList,
  waitForViewLoaded,
  clickItemByName,
  openCreateModal,
  closeModal,
} from './fixtures/wails-mock';

// #1010 — dialog titles and prompts take the item word from the capability's
// itemNoun, so App Configuration says "setting" (as the TUI does), not
// "parameter".

const modalTitle = (page: import('@playwright/test').Page) => page.locator('.modal-title');

test.describe('GUI item noun from the capability (#1010)', () => {
  test('App Configuration dialogs say "Setting"', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    await openCreateModal(page);
    await expect(modalTitle(page)).toHaveText('New Setting');
    await expect(page.locator('#param-value')).toHaveAttribute('placeholder', 'Setting value');
    await closeModal(page);

    await clickItemByName(page, 'app/config');
    await page.locator('.detail-actions').getByRole('button', { name: 'Edit' }).click();
    await expect(modalTitle(page)).toHaveText('Edit Setting');
    await closeModal(page);

    await page.locator('.detail-actions').getByRole('button', { name: 'Delete' }).click();
    await expect(modalTitle(page)).toHaveText('Delete Setting');
    await expect(page.getByText('Are you sure you want to delete this setting?')).toBeVisible();
  });

  test('App Configuration empty list says "No settings found"', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState({ params: [] }));
    await page.goto('/');
    await waitForViewLoaded(page);

    await expect(page.getByText('No settings found.')).toBeVisible();
  });

  test('Parameter Store keeps "Parameter"', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await openCreateModal(page);
    await expect(modalTitle(page)).toHaveText('New Parameter');
  });
});
