import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  waitForItemList,
  clickItemByName,
  navigateTo,
  recordBindingArgs,
  getBindingArgs,
} from './fixtures/wails-mock';

// #981 — the detail pane shows only the latest selection. A Show that resolves
// after a newer selection is dropped, and a failed Show clears the previous
// item's detail, so Edit never targets or prefills the previously selected item.

// Wrap a mock binding after the mock is installed (init scripts run in order).
async function wrapBinding(page: Page, name: string, mode: 'delay' | 'fail', arg: string) {
  await page.addInitScript(({ name, mode, arg }) => {
    const app = (window as any).go.gui.App;
    const original = app[name];
    app[name] = async (...args: unknown[]) => {
      if (args[0] === arg) {
        if (mode === 'fail') throw new Error('AccessDenied');
        await new Promise((resolve) => setTimeout(resolve, 800));
      }
      return original(...args);
    };
  }, { name, mode, arg });
}

test.describe('Detail pane stale-response guard (#981)', () => {
  test('a late Show for an earlier secret does not replace the newer selection', async ({ page }) => {
    await setupWailsMocks(page);
    await wrapBinding(page, 'SecretShow', 'delay', 'my-secret');
    await recordBindingArgs(page, ['StagingEdit']);
    await page.goto('/');
    await navigateTo(page, 'Secrets Manager');
    await waitForItemList(page);

    await clickItemByName(page, 'my-secret');
    await clickItemByName(page, 'api-credentials');
    await expect(page.locator('.detail-title')).toHaveText('api-credentials');
    await expect(page.locator('.value-display')).toBeVisible();

    // Let the delayed my-secret Show resolve; the pane must stay on api-credentials.
    await page.waitForTimeout(1000);
    await expect(page.locator('.detail-title')).toHaveText('api-credentials');
    await expect(page.getByText('team')).toHaveCount(0); // my-secret's tag

    await page.locator('.detail-actions').getByRole('button', { name: 'Edit' }).click();
    await expect(page.locator('#edit-secret-name')).toHaveValue('api-credentials');
    await page.locator('#edit-secret-value').fill('NEW');
    await page.getByRole('button', { name: 'Stage' }).click();

    await expect.poll(() => getBindingArgs(page, 'StagingEdit')).toEqual([['secret', 'api-credentials', 'NEW', '']]);
  });

  test('a failed Show clears the previous param detail and disables Edit', async ({ page }) => {
    await setupWailsMocks(page);
    await wrapBinding(page, 'ParamShow', 'fail', '/app/api/key');
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/config');
    await expect(page.locator('.value-display')).toHaveText('config-value');

    await clickItemByName(page, '/app/api/key');
    await expect(page.locator('.detail-title')).toHaveText('/app/api/key');
    await expect(page.getByText('AccessDenied')).toBeVisible();
    // The previous item's value is gone, and Edit cannot prefill it.
    await expect(page.getByText('config-value')).toHaveCount(0);
    await expect(page.locator('.detail-actions').getByRole('button', { name: 'Edit' })).toBeDisabled();
  });
});
