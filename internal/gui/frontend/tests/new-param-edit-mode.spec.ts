import { test, expect } from './fixtures/coverage';
import {
  setupWailsMocks,
  createAzureNamespaceState,
  waitForItemList,
  clickItemByName,
  openCreateModal,
  recordBindingArgs,
  getBindingArgs,
} from './fixtures/wails-mock';

// #980 — "+ New" is a create even when a row is selected and the typed name
// equals that row's name. Edit vs create comes from how the modal was opened,
// so the create targets the namespace typed in the form, never the selected
// row's namespace, and the Name input stays editable.

const nsSelect = (page: import('@playwright/test').Page) => page.locator('.sidebar .namespace-select');

test.describe('New while a row is selected (#980)', () => {
  test('immediate create of the same key in another namespace writes the typed namespace', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await recordBindingArgs(page, ['ParamSet']);
    await page.goto('/');
    await waitForItemList(page);

    await nsSelect(page).selectOption('dev');
    await clickItemByName(page, 'app/db');

    await openCreateModal(page);
    await page.locator('#param-name').fill('app/db');
    await expect(page.locator('#param-name')).toBeEnabled();
    await page.locator('#param-namespace').fill('prd');
    await page.locator('#param-value').fill('new-prd-value');
    await page.locator('.immediate-checkbox input').check();
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.locator('#param-name')).toHaveCount(0);

    expect(await getBindingArgs(page, 'ParamSet')).toEqual([['app/db', 'new-prd-value', '', 'prd', '']]);
  });

  test('staged create of the same key in another namespace calls StagingAdd with the typed namespace', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await recordBindingArgs(page, ['StagingAdd', 'StagingEdit']);
    await page.goto('/');
    await waitForItemList(page);

    await nsSelect(page).selectOption('prd');
    await clickItemByName(page, 'app/queue');

    await openCreateModal(page);
    await page.locator('#param-name').fill('app/queue');
    await page.locator('#param-namespace').fill('dev');
    await page.locator('#param-value').fill('dev-q');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('#param-name')).toHaveCount(0);

    expect(await getBindingArgs(page, 'StagingAdd')).toEqual([['param', 'app/queue', 'dev-q', 'dev']]);
    expect(await getBindingArgs(page, 'StagingEdit')).toEqual([]);
  });

  test('Edit still targets the selected row and locks the Name input', async ({ page }) => {
    await setupWailsMocks(page, createAzureNamespaceState());
    await recordBindingArgs(page, ['StagingAdd', 'StagingEdit']);
    await page.goto('/');
    await waitForItemList(page);

    await nsSelect(page).selectOption('dev');
    await clickItemByName(page, 'app/db');
    await page.getByRole('button', { name: 'Edit' }).click();
    await expect(page.locator('#param-name')).toBeDisabled();
    await page.locator('#param-value').fill('edited');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('#param-name')).toHaveCount(0);

    expect(await getBindingArgs(page, 'StagingEdit')).toEqual([['param', 'app/db', 'edited', 'dev']]);
    expect(await getBindingArgs(page, 'StagingAdd')).toEqual([]);
  });
});
