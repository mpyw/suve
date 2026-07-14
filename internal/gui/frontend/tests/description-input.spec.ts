import { test, expect } from './fixtures/coverage';
import {
  setupWailsMocks,
  waitForItemList,
  waitForViewLoaded,
  clickItemByName,
  navigateTo,
  openCreateModal,
  createGoogleCloudState,
  createAzureState,
} from './fixtures/wails-mock';

// #767: the GUI can DISPLAY a description but had no INPUT to set one on
// create/edit. These specs prove the Description input appears for the services
// that persist a description (AWS param/secret, Google Cloud secret), is HIDDEN
// for Azure (App Configuration / Key Vault, which ignore it), and that its value
// flows through create (round-trips to the detail pane) and edit (pre-filled
// from the current description).
test.describe('Description input (#767)', () => {
  test.describe('AWS Parameter', () => {
    test.beforeEach(async ({ page }) => {
      await setupWailsMocks(page, {
        params: [
          { name: '/app/described', type: 'String', value: 'v', description: 'existing description' },
        ],
      });
      await page.goto('/');
      await waitForItemList(page);
    });

    test('shows the Description input on create', async ({ page }) => {
      await openCreateModal(page);
      await expect(page.getByTestId('param-description-input')).toBeVisible();
    });

    test('flows the description through create and round-trips to the detail pane', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#param-name').fill('/app/new');
      await page.locator('#param-value').fill('new-value');
      await page.getByTestId('param-description-input').fill('brand new description');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Save' }).click();

      await clickItemByName(page, '/app/new');
      const section = page.locator('.detail-section').filter({ hasText: 'Description' });
      await expect(section.locator('.description-text')).toContainText('brand new description');
    });

    test('pre-fills the current description on edit', async ({ page }) => {
      await clickItemByName(page, '/app/described');
      await page.getByRole('button', { name: 'Edit' }).click();
      await expect(page.getByTestId('param-description-input')).toHaveValue('existing description');
    });
  });

  test.describe('AWS Secret', () => {
    test.beforeEach(async ({ page }) => {
      await setupWailsMocks(page, {
        secrets: [
          { name: 'described-secret', value: 'v', description: 'existing secret description' },
        ],
      });
      await page.goto('/');
      await waitForItemList(page);
      await navigateTo(page, 'Secret');
    });

    test('shows the Description input on create', async ({ page }) => {
      await openCreateModal(page);
      await expect(page.getByTestId('secret-description-input')).toBeVisible();
    });

    test('flows the description through create and round-trips to the detail pane', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#secret-name').fill('new-secret');
      await page.locator('#secret-value').fill('new-value');
      await page.getByTestId('secret-description-input').fill('brand new secret description');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Create' }).click();

      await clickItemByName(page, 'new-secret');
      const section = page.locator('.detail-section').filter({ hasText: 'Description' });
      await expect(section.locator('.description-text')).toContainText('brand new secret description');
    });

    test('pre-fills the current description on edit', async ({ page }) => {
      await clickItemByName(page, 'described-secret');
      await page.getByRole('button', { name: 'Edit' }).click();
      await expect(page.getByTestId('edit-secret-description-input')).toHaveValue('existing secret description');
    });
  });

  test.describe('Google Cloud Secret', () => {
    test.beforeEach(async ({ page }) => {
      await setupWailsMocks(page, createGoogleCloudState());
      await page.goto('/');
      await waitForItemList(page);
      await navigateTo(page, 'Secret');
    });

    test('shows the Description input on create', async ({ page }) => {
      await openCreateModal(page);
      await expect(page.getByTestId('secret-description-input')).toBeVisible();
    });
  });

  test.describe('Azure (hidden)', () => {
    test('App Configuration hides the Description input on create', async ({ page }) => {
      await setupWailsMocks(page, createAzureState());
      await page.goto('/');
      await waitForViewLoaded(page);
      await navigateTo(page, 'App Configuration');
      await openCreateModal(page);
      await expect(page.getByTestId('param-description-input')).toHaveCount(0);
    });

    test('Key Vault hides the Description input on create and edit', async ({ page }) => {
      await setupWailsMocks(page, createAzureState());
      await page.goto('/');
      await waitForViewLoaded(page);
      await navigateTo(page, 'Key Vault');
      await openCreateModal(page);
      await expect(page.getByTestId('secret-description-input')).toHaveCount(0);
      await page.getByRole('button', { name: 'Cancel' }).click();

      await clickItemByName(page, 'kv-secret');
      await page.getByRole('button', { name: 'Edit' }).click();
      await expect(page.getByTestId('edit-secret-description-input')).toHaveCount(0);
    });
  });
});
