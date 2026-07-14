import { test, expect } from './fixtures/coverage';
import {
  setupWailsMocks,
  createAzureNamespaceState,
  waitForItemList,
  openCreateModal,
  navigateTo,
} from './fixtures/wails-mock';

// #431 — the staging view shows each staged App Configuration entry's namespace
// (the label axis) as a badge, so entries staged under different namespaces are
// distinguishable. Staging is per-store (one bucket across namespaces); each
// entry carries its own namespace.

const nsSelect = (page: import('@playwright/test').Page) => page.locator('.sidebar .namespace-select');

async function stageCreate(page: import('@playwright/test').Page, name: string, value: string) {
  await openCreateModal(page);
  await page.locator('#param-name').fill(name);
  await page.locator('#param-value').fill(value);
  // Default (immediate unchecked) => staged.
  await page.getByRole('button', { name: 'Stage' }).click();
}

test.describe('App Configuration staging view namespace (#431)', () => {
  test('a staged create shows its namespace badge in the staging view', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    // Stage a create under the concrete "dev" namespace.
    await nsSelect(page).selectOption('dev');
    await stageCreate(page, 'app/staged-dev', 'dev-val');

    await navigateTo(page, 'Staging');

    const entry = page.locator('.entry-item').filter({ hasText: 'app/staged-dev' });
    await expect(entry).toBeVisible();
    await expect(entry.locator('.namespace-badge')).toHaveText('dev');
  });

  test('a staged create under the null namespace shows (NULL)', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    // Default filter is (NULL) -> the create targets the null namespace.
    await stageCreate(page, 'app/staged-null', 'null-val');

    await navigateTo(page, 'Staging');

    const entry = page.locator('.entry-item').filter({ hasText: 'app/staged-null' });
    await expect(entry).toBeVisible();
    await expect(entry.locator('.namespace-badge')).toHaveText('(NULL)');
  });

  test('entries staged under different namespaces are both shown, each badged', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    await nsSelect(page).selectOption('dev');
    await stageCreate(page, 'app/multi', 'dev-val');

    // Switch the filter to prd and stage the SAME key under prd.
    await nsSelect(page).selectOption('prd');
    await stageCreate(page, 'app/multi', 'prd-val');

    await navigateTo(page, 'Staging');

    const rows = page.locator('.entry-item').filter({ hasText: 'app/multi' });
    await expect(rows).toHaveCount(2);
    await expect(rows.locator('.namespace-badge')).toHaveText(['dev', 'prd']);
  });
});
