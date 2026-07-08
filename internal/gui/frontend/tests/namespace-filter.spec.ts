import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createAzureNamespaceState,
  waitForItemList,
} from './fixtures/wails-mock';

// #425 / #427 follow-up — Azure App Configuration namespace filter. The dropdown
// now lives in the SIDEBAR FOOTER's connected-scope block (the "Namespace" row is
// itself the <select>), not ParamView's top-right filter bar. It still filters
// the loaded rows client-side by namespace, with per-entry / detail namespace
// display. The list is loaded across ALL namespaces (LabelFilter "*") so each row
// carries its namespace; the dropdown narrows the displayed rows without a
// backend round-trip and without re-scoping. Azure App Configuration only.

const NULL = '(NULL)';

// The namespace dropdown is the footer scope-info "Namespace" row's <select>.
const nsSelect = (page: import('@playwright/test').Page) => page.locator('.sidebar .namespace-select');

/** Names shown in the (visible) list, in DOM order. */
async function visibleNames(page: import('@playwright/test').Page): Promise<string[]> {
  return page.locator('.item-list .item-name').allInnerTexts();
}

test.describe('App Configuration namespace filter (#425)', () => {
  test('footer dropdown lists [(NULL), dev, prd, *] and defaults to the scope namespace', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    const select = nsSelect(page);
    await expect(select).toBeVisible();

    // Options: (NULL) first, discovered namespaces sorted, * last.
    await expect(select.locator('option')).toHaveText([NULL, 'dev', 'prd', '*']);

    // Scope namespace is empty → default selection is (NULL), so only the
    // null/default-namespace row is shown initially.
    await expect(select).toHaveValue(NULL);
    expect(await visibleNames(page)).toEqual(['app/config']);
  });

  test('selecting a namespace in the footer filters the displayed rows client-side', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    const select = nsSelect(page);

    // dev → only the two dev rows.
    await select.selectOption('dev');
    expect((await visibleNames(page)).sort()).toEqual(['app/cache', 'app/db']);

    // (NULL) → only the null-namespace row.
    await select.selectOption(NULL);
    expect(await visibleNames(page)).toEqual(['app/config']);

    // * → every row across all namespaces.
    await select.selectOption('*');
    expect((await visibleNames(page)).sort()).toEqual(['app/cache', 'app/config', 'app/db', 'app/queue']);
  });

  test('each row shows its namespace as a badge', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    // Show everything so all namespaces are visible at once.
    await nsSelect(page).selectOption('*');

    const badgeFor = (name: string) =>
      page.locator('.item-entry').filter({ hasText: name }).locator('.namespace-badge');

    await expect(badgeFor('app/config')).toHaveText(NULL); // null/default
    await expect(badgeFor('app/db')).toHaveText('dev');
    await expect(badgeFor('app/queue')).toHaveText('prd');
  });

  test('detail panel shows the selected entry\'s namespace', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    await nsSelect(page).selectOption('*');

    const namespaceMeta = page.locator('.meta-item').filter({ hasText: 'Namespace' });

    // A namespaced row.
    await page.locator('.item-button').filter({ hasText: 'app/db' }).click();
    await expect(page.locator('.detail-panel')).toBeVisible();
    await expect(namespaceMeta).toContainText('dev');

    // A null/default-namespace row shows (NULL).
    await page.locator('.item-button').filter({ hasText: 'app/config' }).click();
    await expect(namespaceMeta).toContainText(NULL);
  });

  test('non-Azure-App-Config providers do not show the dropdown', async ({ page }) => {
    // Default state is AWS Parameter Store — no namespace axis.
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await expect(page.locator('.namespace-select')).toHaveCount(0);
    await expect(page.locator('.namespace-badge')).toHaveCount(0);
  });
});
