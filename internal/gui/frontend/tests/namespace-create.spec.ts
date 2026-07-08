import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createAzureNamespaceState,
  waitForItemList,
  openCreateModal,
} from './fixtures/wails-mock';

// #431 — creating an App Configuration setting under a chosen namespace from the
// GUI, and blocking the create when the namespace filter is not a single concrete
// namespace (`*` = all, or a `,`-list = multiple). A create must target exactly
// one (key, namespace), so the create form carries a namespace field prefilled
// from the current filter, and submission is disabled while viewing all.

const NULL = '(NULL)';

const nsSelect = (page: import('@playwright/test').Page) => page.locator('.sidebar .namespace-select');

test.describe('App Configuration namespaced create (#431)', () => {
  test('create form shows a namespace field prefilled from the current filter', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    // Narrow to a concrete namespace, then open the create form.
    await nsSelect(page).selectOption('dev');
    await openCreateModal(page);

    const nsField = page.locator('#param-namespace');
    await expect(nsField).toBeVisible();
    await expect(nsField).toHaveValue('dev');
  });

  test('null-namespace filter prefills an empty (null) namespace', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    // Default filter is (NULL).
    await expect(nsSelect(page)).toHaveValue(NULL);
    await openCreateModal(page);

    await expect(page.locator('#param-namespace')).toHaveValue('');
  });

  test('create under a concrete namespace round-trips (immediate)', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    await nsSelect(page).selectOption('dev');
    await openCreateModal(page);

    await page.locator('#param-name').fill('app/new-dev-key');
    await page.locator('#param-value').fill('dev-value');
    // Namespace prefilled to dev; create immediately so it lands in the list.
    await page.locator('.immediate-checkbox input').check();
    await page.getByRole('button', { name: 'Save' }).click();

    // The new key appears in the dev-filtered list, badged with its namespace.
    const entry = page.locator('.item-entry').filter({ hasText: 'app/new-dev-key' });
    await expect(entry).toBeVisible();
    await expect(entry.locator('.namespace-badge')).toHaveText('dev');
  });

  test('create is blocked while the namespace filter is * (all)', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    await nsSelect(page).selectOption('*');
    await openCreateModal(page);

    // The namespace field is replaced by a block message and submit is disabled.
    await expect(page.locator('[data-testid="ns-blocked"]')).toBeVisible();
    await expect(page.locator('#param-namespace')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Stage' })).toBeDisabled();
  });

  test('typing a brand-new namespace creates under it (free-form)', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await page.goto('/');
    await waitForItemList(page);

    // A concrete filter unblocks the form; the user can still type a new namespace.
    await nsSelect(page).selectOption('dev');
    await openCreateModal(page);

    await page.locator('#param-name').fill('app/staging-key');
    await page.locator('#param-value').fill('v');
    await page.locator('#param-namespace').fill('qa');
    await page.locator('.immediate-checkbox input').check();
    await page.getByRole('button', { name: 'Save' }).click();

    // Reveal all namespaces; the new key shows under the freshly-typed "qa".
    await nsSelect(page).selectOption('*');
    const entry = page.locator('.item-entry').filter({ hasText: 'app/staging-key' });
    await expect(entry).toBeVisible();
    await expect(entry.locator('.namespace-badge')).toHaveText('qa');
  });
});
