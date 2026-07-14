import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  createGoogleCloudState,
  createAzureState,
  createStagedForExportState,
  waitForItemList,
  navigateTo,
  clickItemByName,
} from './fixtures/wails-mock';

const nav = (page: Page) => page.locator('.nav');

async function switchToGoogleCloud(page: Page, project = 'proj') {
  await page.locator('#provider-select').selectOption('googlecloud');
  await page.locator('#gcloud-project').fill(project);
  await page.getByRole('button', { name: 'Connect' }).click();
  await waitForItemList(page);
}

test.describe('Multi-provider staging (#270)', () => {
  test('staging is scope-isolated across providers (sidebar badge)', async ({ page }) => {
    await setupWailsMocks(page, createStagedForExportState()); // AWS: 1 param + 1 secret staged
    await page.goto('/');
    await waitForItemList(page);

    // AWS shows its staged count.
    await expect(page.locator('.staging-count')).toHaveText('2');

    // Switch to Google Cloud → its own (empty) staging set, no badge.
    await switchToGoogleCloud(page);
    await expect(page.locator('.staging-count')).toHaveCount(0);

    // Back to AWS → the AWS staged entries reappear.
    await page.locator('#provider-select').selectOption('aws');
    await waitForItemList(page);
    await expect(page.locator('.staging-count')).toHaveText('2');
  });

  test('Google Cloud exposes a Staging tab with only the Secret section', async ({ page }) => {
    await setupWailsMocks(page, createGoogleCloudState());
    await page.goto('/');
    await waitForItemList(page);

    await expect(nav(page).getByRole('button', { name: /Staging/i })).toBeVisible();
    await navigateTo(page, 'Staging');

    await expect(page.locator('.section')).toHaveCount(1);
    await expect(page.getByRole('heading', { name: /Secret/i })).toBeVisible();
  });

  test('Azure exposes both App Configuration and Key Vault staging sections', async ({ page }) => {
    await setupWailsMocks(page, createAzureState());
    await page.goto('/');
    await waitForItemList(page);
    await navigateTo(page, 'Staging');

    await expect(page.locator('.section')).toHaveCount(2);
    await expect(page.getByRole('heading', { name: 'App Configuration' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Key Vault' })).toBeVisible();
  });

  test('staging a Google Cloud secret lands in the Google Cloud staging set', async ({ page }) => {
    await setupWailsMocks(page, createGoogleCloudState());
    await page.goto('/');
    await waitForItemList(page);

    // Stage an edit (staging toggle defaults to on).
    await clickItemByName(page, 'gcloud-secret-1');
    await page.getByRole('button', { name: 'Edit' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    await page.locator('#secret-value, textarea').first().fill('new-gcloud-value');
    await page.getByRole('button', { name: 'Stage' }).click();

    await navigateTo(page, 'Staging');
    await expect(page.locator('.entry-name').filter({ hasText: 'gcloud-secret-1' })).toBeVisible();
    await expect(page.locator('.staging-count')).toHaveText('1');
  });

  test('Azure App Configuration staged section shows tag controls', async ({ page }) => {
    await setupWailsMocks(page, createAzureState());
    await page.goto('/');
    await waitForItemList(page);

    // Stage an App Configuration edit (immediate is forced only when staging is
    // unavailable; Azure App Config now stages).
    await clickItemByName(page, 'app/config/key');
    await page.getByRole('button', { name: 'Edit' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    await page.locator('textarea').first().fill('new-appconfig-value');
    await page.getByRole('button', { name: 'Stage' }).click();

    await navigateTo(page, 'Staging');
    const appConfigSection = page.locator('.section').filter({ hasText: 'App Configuration' });
    await expect(appConfigSection).toBeVisible();
    // App Configuration tags are writable (azappconfig/v2), so its staged section
    // exposes the "+ Add Tag" control just like the tag-capable providers.
    await expect(appConfigSection.getByRole('button', { name: /Add Tag/i })).toHaveCount(1);
  });
});
