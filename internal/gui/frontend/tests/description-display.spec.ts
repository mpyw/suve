import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  waitForItemList,
  clickItemByName,
  navigateTo,
} from './fixtures/wails-mock';

// #753: AWS param/secret descriptions are written but were never shown back.
// Once the adapter Get populates Entry.Description and the CLI show renders it,
// the GUI needs no code change — the detail pane already gates on a non-empty
// description. These specs prove the description flows through to the detail
// pane for both AWS services (and stays hidden when empty).
test.describe('AWS description display (#753)', () => {
  test.describe('Parameter', () => {
    test.beforeEach(async ({ page }) => {
      await setupWailsMocks(page, {
        params: [
          { name: '/app/described', type: 'String', value: 'v', description: 'parameter description text' },
          { name: '/app/plain', type: 'String', value: 'v' },
        ],
      });
      await page.goto('/');
      await waitForItemList(page);
    });

    test('renders the description in the detail pane', async ({ page }) => {
      await clickItemByName(page, '/app/described');
      const section = page.locator('.detail-section').filter({ hasText: 'Description' });
      await expect(section).toBeVisible();
      await expect(section.locator('.description-text')).toContainText('parameter description text');
    });

    test('omits the description section when empty', async ({ page }) => {
      await clickItemByName(page, '/app/plain');
      await expect(page.locator('.detail-panel')).toBeVisible();
      await expect(page.locator('.description-text')).toHaveCount(0);
    });
  });

  test.describe('Secret', () => {
    test.beforeEach(async ({ page }) => {
      await setupWailsMocks(page, {
        secrets: [
          { name: 'described-secret', value: 'v', description: 'secret description text' },
          { name: 'plain-secret', value: 'v' },
        ],
      });
      await page.goto('/');
      await waitForItemList(page);
      await navigateTo(page, 'Secret');
    });

    test('renders the description in the detail pane', async ({ page }) => {
      await clickItemByName(page, 'described-secret');
      const section = page.locator('.detail-section').filter({ hasText: 'Description' });
      await expect(section).toBeVisible();
      await expect(section.locator('.description-text')).toContainText('secret description text');
    });

    test('omits the description section when empty', async ({ page }) => {
      await clickItemByName(page, 'plain-secret');
      await expect(page.locator('.detail-panel')).toBeVisible();
      await expect(page.locator('.description-text')).toHaveCount(0);
    });
  });
});
