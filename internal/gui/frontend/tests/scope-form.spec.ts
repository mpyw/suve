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

// An Azure environment with no vault/store env-derived yet → the app parks in
// the Azure scope form (empty, not auto-applied).
const azurePartialScope: Partial<MockState> = {
  initialProvider: 'azure',
  currentScope: {
    provider: 'azure',
    projectId: '',
    vaultName: '',
    storeName: '',
  },
  detectResult: { param: 'azure', secret: 'azure', stage: '', paramActive: ['azure'], secretActive: ['azure'], stageActive: [] },
};

test.describe('Azure scope form', () => {
  test('renders vault/store fields for Azure only (no subscription/RG)', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await pickProvider(page, 'azure');
    for (const id of ['#azure-vault', '#azure-store']) {
      await expect(page.locator(id)).toBeVisible();
    }
    // Subscription / resource group are no longer collected (unused).
    await expect(page.locator('#azure-subscription')).toHaveCount(0);
    await expect(page.locator('#azure-resource-group')).toHaveCount(0);
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
    await expect(page.locator('#azure-vault')).toHaveCount(0);
  });

  test('submit sends the exact ScopeSelection (vault vs store not swapped, trimmed)', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await pickProvider(page, 'azure');
    await page.locator('#azure-vault').fill('  the-vault  '); // whitespace trimmed
    await page.locator('#azure-store').fill('the-store');
    await page.getByRole('button', { name: 'Connect' }).click();
    await waitForItemList(page);

    const calls = await getSelectScopeCalls(page);
    const last = calls[calls.length - 1];
    expect(last).toMatchObject({
      provider: 'azure',
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
    await expect(connect).toBeDisabled(); // nothing entered yet
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

  test('parks in an empty Azure form when env supplies no vault/store', async ({ page }) => {
    await setupWailsMocks(page, azurePartialScope);
    await page.goto('/');
    await expect(page.locator('#azure-vault')).toHaveValue('');
    await expect(page.locator('#azure-store')).toHaveValue('');
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

  test('Change scope re-opens the prefilled form and re-points to a different resource', async ({ page }) => {
    // Azure launched with vault+store already applied (no form on start).
    await setupWailsMocks(page, createAzureState());
    await page.goto('/');
    await waitForItemList(page);
    await expect(page.locator('#azure-vault')).toHaveCount(0); // connected: no form

    // "Change scope" re-opens the form, prefilled with the current values —
    // without auto-reconnecting the cached scope.
    await page.getByRole('button', { name: 'Change scope' }).click();
    await expect(page.locator('#azure-vault')).toHaveValue('my-vault');
    await expect(page.locator('#azure-store')).toHaveValue('my-store');

    // Re-point to a different vault and reconnect.
    await page.locator('#azure-vault').fill('other-vault');
    await page.getByRole('button', { name: 'Connect' }).click();
    await waitForItemList(page);

    const calls = await getSelectScopeCalls(page);
    expect(calls[calls.length - 1]).toMatchObject({
      provider: 'azure',
      vaultName: 'other-vault', // replaced
      storeName: 'my-store', // preserved from the prefill
    });
  });

  test('scope info always renders every row ("?" when unset); Change scope stays present but disables while editing', async ({ page }) => {
    // Only a Key Vault is configured — no App Configuration store.
    await setupWailsMocks(
      page,
      createAzureState({ currentScope: { provider: 'azure', projectId: '', vaultName: 'only-vault', storeName: '' } }),
    );
    await page.goto('/');
    const info = page.locator('.scope-info');
    await expect(info).toBeVisible();

    // Every row renders; the unset App Configuration store shows "?" (not hidden).
    await expect(info).toContainText('only-vault');
    await expect(info).toContainText('?');

    // Change scope is present and enabled while connected...
    const change = page.getByRole('button', { name: 'Change scope' });
    await expect(change).toBeEnabled();

    // ...and stays present but disabled once a form is pending.
    await change.click();
    await expect(page.locator('#azure-vault')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Change scope' })).toBeDisabled();
  });

  test.describe('a11y', () => {
    test('fields are labeled and the first field is focused on open', async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await waitForItemList(page);
      await pickProvider(page, 'azure');

      await expect(page.getByLabel('Key Vault name')).toBeVisible();
      await expect(page.getByLabel('App Configuration store')).toBeVisible();
      // The Key Vault field leads the form and takes initial focus.
      await expect(page.locator('#azure-vault')).toBeFocused();
    });

    test('Enter submits and Escape cancels', async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await waitForItemList(page);

      // Escape cancels → back to the active AWS provider, form gone.
      await pickProvider(page, 'azure');
      await expect(page.locator('#azure-vault')).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator('#azure-vault')).toHaveCount(0);

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
    await expect(page.locator('#azure-vault')).toBeVisible();
    const noOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    );
    expect(noOverflow).toBe(true);
  });
});
