import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createGoogleCloudState,
  waitForItemList,
  clickItemByName,
} from './fixtures/wails-mock';

// The Tags field keeps one vocabulary across every provider. Google Cloud
// natively calls the same key=value metadata "labels", so TagList surfaces a
// secondary hint ("(= Google Cloud: labels)") for Google Cloud only — the field
// is still titled "Tags" everywhere.
test.describe('Tags terminology hint', () => {
  test('Google Cloud: a tagged secret shows the "(= Google Cloud: labels)" hint', async ({ page }) => {
    await setupWailsMocks(
      page,
      createGoogleCloudState({
        secretTags: { 'gcloud-secret-1': [{ key: 'team', value: 'backend' }] },
      }),
    );
    await page.goto('/');
    await waitForItemList(page);

    // Google Cloud launches into the secret view.
    await clickItemByName(page, 'gcloud-secret-1');
    await expect(page.locator('.detail-panel')).toBeVisible();

    // The field still reads "Tags", with the native-vocabulary hint beside it.
    await expect(page.getByRole('heading', { name: 'Tags' })).toBeVisible();
    const hint = page.locator('.tag-native-hint');
    await expect(hint).toBeVisible();
    await expect(hint).toHaveText('(= Google Cloud: labels)');
    await expect(page.locator('.tag-item')).toBeVisible();
  });

  test('AWS: a tagged parameter shows no native hint (field still titled "Tags")', async ({ page }) => {
    // Default state is AWS; /app/config carries an existing tag.
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/config');
    await expect(page.locator('.detail-panel')).toBeVisible();

    await expect(page.getByRole('heading', { name: 'Tags' })).toBeVisible();
    await expect(page.locator('.tag-item')).toBeVisible();
    // No native-vocabulary hint outside Google Cloud.
    await expect(page.locator('.tag-native-hint')).toHaveCount(0);
  });
});
