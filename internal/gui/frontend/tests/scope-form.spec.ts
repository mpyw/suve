import { test, expect, type Page } from '@playwright/test';
import {
  setupWailsMocks,
  createAzureState,
  getSelectScopeCalls,
  waitForItemList,
  waitForViewLoaded,
  navigateTo,
  type MockState,
} from './fixtures/wails-mock';

// Select a provider from the sidebar dropdown.
async function pickProvider(page: Page, value: string) {
  await page.locator('#provider-select').selectOption(value);
}

// An Azure environment where subscription/RG are env-derived but no vault/store
// yet → the app parks in the Azure scope form (prefilled, not auto-applied).
const azurePartialScope: Partial<MockState> = {
  initialProvider: 'azure',
  currentScope: {
    provider: 'azure',
    projectId: '',
    subscriptionId: 'env-sub',
    resourceGroup: 'env-rg',
    vaultName: '',
    storeName: '',
  },
  detectResult: { param: 'azure', secret: 'azure', stage: '', paramActive: ['azure'], secretActive: ['azure'], stageActive: [] },
};

test.describe('Azure scope form', () => {
  test('renders subscription/RG + vault/store fields for Azure only', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await pickProvider(page, 'azure');
    for (const id of ['#azure-subscription', '#azure-resource-group', '#azure-vault', '#azure-store']) {
      await expect(page.locator(id)).toBeVisible();
    }
    await expect(page.locator('#gcloud-project')).toHaveCount(0);
  });

  test('Google Cloud shows only a project field; AWS shows no scope form', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await pickProvider(page, 'googlecloud');
    await expect(page.locator('#gcloud-project')).toBeVisible();
    await expect(page.locator('#azure-vault')).toHaveCount(0);

    await pickProvider(page, 'aws');
    // AWS needs no scope → auto-applied, no form.
    await expect(page.locator('#gcloud-project')).toHaveCount(0);
    await expect(page.locator('#azure-subscription')).toHaveCount(0);
  });

  test('submit sends the exact ScopeSelection (vault vs store not swapped, trimmed)', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await pickProvider(page, 'azure');
    await page.locator('#azure-subscription').fill('sub-1');
    await page.locator('#azure-resource-group').fill('rg-1');
    await page.locator('#azure-vault').fill('  the-vault  '); // whitespace trimmed
    await page.locator('#azure-store').fill('the-store');
    await page.getByRole('button', { name: 'Connect' }).click();
    await waitForItemList(page);

    const calls = await getSelectScopeCalls(page);
    const last = calls[calls.length - 1];
    expect(last).toMatchObject({
      provider: 'azure',
      subscriptionId: 'sub-1',
      resourceGroup: 'rg-1',
      vaultName: 'the-vault', // NOT the store
      storeName: 'the-store', // NOT the vault
    });
  });

  test('Connect is disabled until a vault or store name is given', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await pickProvider(page, 'azure');
    const connect = page.getByRole('button', { name: 'Connect' });
    await page.locator('#azure-subscription').fill('sub-only');
    await expect(connect).toBeDisabled(); // sub/rg alone is not enough
    await page.locator('#azure-vault').fill('v');
    await expect(connect).toBeEnabled();
    // whitespace-only does not count
    await page.locator('#azure-vault').fill('   ');
    await expect(connect).toBeDisabled();
  });

  test('a rejected SelectScope shows the error and keeps the previous scope browsable', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
    // AWS is active and browsable.
    await expect(page.locator('.item-name.param').first()).toBeVisible();

    // Make the next SelectScope fail, then try to switch to Azure.
    await page.evaluate(() => {
      (window as any).__forceSelectScopeError = true;
    });
    await pickProvider(page, 'azure');
    await page.locator('#azure-vault').fill('v');
    await page.getByRole('button', { name: 'Connect' }).click();

    // Error surfaced in the form; AWS list still works (previous scope kept).
    await expect(page.locator('.scope-error')).toBeVisible();
    await expect(page.locator('.item-name.param').first()).toBeVisible();
  });

  test('prefills subscription/RG from env-derived GetCurrentScope', async ({ page }) => {
    await setupWailsMocks(page, azurePartialScope);
    await page.goto('/');
    await expect(page.locator('#azure-subscription')).toHaveValue('env-sub');
    await expect(page.locator('#azure-resource-group')).toHaveValue('env-rg');
    await expect(page.locator('#azure-vault')).toHaveValue('');
  });

  test('retains scope when switching provider away and back (localStorage)', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    // Establish an Azure scope.
    await pickProvider(page, 'azure');
    await page.locator('#azure-vault').fill('kept-vault');
    await page.getByRole('button', { name: 'Connect' }).click();
    await waitForItemList(page);
    await expect(page.locator('.nav').getByRole('button', { name: /Key Vault/i })).toBeVisible();

    // Away to AWS, then back to Azure → retained (no form, auto-applied).
    await pickProvider(page, 'aws');
    await waitForItemList(page);
    await pickProvider(page, 'azure');
    await waitForItemList(page);
    await expect(page.locator('#azure-vault')).toHaveCount(0); // no form: applied from cache
    await expect(page.locator('.nav').getByRole('button', { name: /Key Vault/i })).toBeVisible();
  });

  test.describe('a11y', () => {
    test('fields are labeled and the first field is focused on open', async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await waitForItemList(page);
      await pickProvider(page, 'azure');

      await expect(page.getByLabel('Subscription ID')).toBeVisible();
      await expect(page.getByLabel('Key Vault name')).toBeVisible();
      await expect(page.locator('#azure-subscription')).toBeFocused();
    });

    test('Enter submits and Escape cancels', async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await waitForItemList(page);

      // Escape cancels → back to the active AWS provider, form gone.
      await pickProvider(page, 'azure');
      await expect(page.locator('#azure-subscription')).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator('#azure-subscription')).toHaveCount(0);

      // Enter in a field submits.
      await pickProvider(page, 'azure');
      await page.locator('#azure-vault').fill('via-enter');
      await page.locator('#azure-vault').press('Enter');
      await waitForItemList(page);
      const calls = await getSelectScopeCalls(page);
      expect(calls[calls.length - 1]).toMatchObject({ provider: 'azure', vaultName: 'via-enter' });
    });
  });

  test('form is usable at 375px width', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForViewLoaded(page);

    await pickProvider(page, 'azure');
    await expect(page.locator('#azure-subscription')).toBeVisible();
    const noOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    );
    expect(noOverflow).toBe(true);
  });
});
