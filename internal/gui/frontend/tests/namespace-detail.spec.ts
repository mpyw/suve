import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  createAzureState,
  waitForItemList,
} from './fixtures/wails-mock';

// Regression for the reported bug: selecting a namespaced Azure App Configuration
// setting must READ it under its OWN namespace (the label axis), not the shared
// read scope's. Before the fix, ParamShow/Delete/Tag resolved the store under the
// scope namespace (the footer filter is client-side only), so a `dev` entry was
// looked up under the default namespace → "provider: entry not found".
//
// The wails mock now rejects a read issued under the wrong namespace (like the
// real provider) and records the (name, namespace) each ParamShow asked for.

const NULL = '(NULL)';

const nsSelect = (page: Page) => page.locator('.sidebar .namespace-select');

async function paramShowCalls(page: Page): Promise<Array<{ name: string; namespace: string }>> {
  return page.evaluate(() => ((window as any).__paramShowCalls as Array<{ name: string; namespace: string }>) ?? []);
}

// Two settings that SHARE a key across namespaces, with distinct values — so a
// read under the wrong namespace resolves the wrong value (or errors).
function sharedKeyState() {
  return createAzureState({
    params: [
      { name: 'app/db-url', type: 'String', value: 'dev-conn', namespace: 'dev' },
      { name: 'app/db-url', type: 'String', value: 'prd-conn', namespace: 'prd' },
      { name: 'app/only-null', type: 'String', value: 'null-val', namespace: '' },
    ],
  });
}

test.describe('App Configuration namespaced detail read (#431 regression)', () => {
  test('selecting a namespaced entry reads it under its own namespace', async ({ page }) => {
    await setupWailsMocks(page, sharedKeyState());
    await page.goto('/');
    await waitForItemList(page);

    // Show every namespace so both app/db-url rows are visible.
    await nsSelect(page).selectOption('*');

    // Click the `dev` app/db-url row (the dev badge disambiguates the shared key).
    await page.locator('.item-entry').filter({ hasText: 'app/db-url' }).filter({ hasText: 'dev' }).locator('.item-button').click();

    await expect(page.locator('.detail-panel')).toBeVisible();
    // No "entry not found" — the read hit the dev namespace, not the default.
    await expect(page.locator('.error-banner')).toHaveCount(0);
    await expect(page.locator('.value-display')).toHaveText('dev-conn');
    await expect(page.locator('.meta-item').filter({ hasText: 'Namespace' })).toContainText('dev');

    // The frontend must have asked ParamShow for the dev namespace specifically.
    const calls = await paramShowCalls(page);
    expect(calls.at(-1)).toEqual({ name: 'app/db-url', namespace: 'dev' });
  });

  test('the other namespace of the same key reads independently', async ({ page }) => {
    await setupWailsMocks(page, sharedKeyState());
    await page.goto('/');
    await waitForItemList(page);

    await nsSelect(page).selectOption('*');

    await page.locator('.item-entry').filter({ hasText: 'app/db-url' }).filter({ hasText: 'prd' }).locator('.item-button').click();

    await expect(page.locator('.error-banner')).toHaveCount(0);
    await expect(page.locator('.value-display')).toHaveText('prd-conn');

    const calls = await paramShowCalls(page);
    expect(calls.at(-1)).toEqual({ name: 'app/db-url', namespace: 'prd' });
  });

  test('a null/default-namespace entry reads under the empty namespace', async ({ page }) => {
    await setupWailsMocks(page, sharedKeyState());
    await page.goto('/');
    await waitForItemList(page);

    await nsSelect(page).selectOption(NULL);

    await page.locator('.item-button').filter({ hasText: 'app/only-null' }).click();

    await expect(page.locator('.error-banner')).toHaveCount(0);
    await expect(page.locator('.value-display')).toHaveText('null-val');

    const calls = await paramShowCalls(page);
    expect(calls.at(-1)).toEqual({ name: 'app/only-null', namespace: '' });
  });
});
