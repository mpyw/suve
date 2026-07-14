import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  navigateTo,
  createStagedForExportState,
  createImportFileState,
  createStagedValue,
  createAzureState,
  awsScopeKey,
  type MockState,
} from './fixtures/wails-mock';

// ============================================================================
// Export / Import (replaces the removed stash flow, #451 / #455)
//
// Files are per-service (one service per envelope). Every flow carries a
// concrete service, so Azure App Configuration (param) and Key Vault (secret)
// stay in their own on-disk buckets — the #445 regression this suite guards.
// ============================================================================

async function openTransferMenu(page: Page) {
  await page.getByRole('button', { name: /Export \/ Import/ }).click();
}

// Drive the encrypt PassphraseModal with an empty passphrase (plaintext path).
async function submitPlaintextExport(page: Page) {
  await page.locator('.modal button[type="submit"]').click();
  await page.getByRole('button', { name: /Continue without encryption/ }).click();
}

// Drive the encrypt PassphraseModal with a real passphrase.
async function submitEncryptedExport(page: Page, passphrase: string) {
  await page.locator('#passphrase').fill(passphrase);
  await page.locator('#confirm-passphrase').fill(passphrase);
  await page.locator('.modal button[type="submit"]').click();
}

async function readExportFiles(page: Page): Promise<Record<string, any>> {
  return page.evaluate(() => (window as any).__exportFiles());
}

// ----------------------------------------------------------------------------
// Export
// ----------------------------------------------------------------------------

test.describe('Export', () => {
  test('shows the Export / Import dropdown with per-service export items', async ({ page }) => {
    await setupWailsMocks(page, createStagedForExportState());
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    await openTransferMenu(page);
    await expect(page.getByTestId('export-param')).toBeEnabled();
    await expect(page.getByTestId('export-secret')).toBeEnabled();
    await expect(page.getByTestId('import-param')).toBeEnabled();
    await expect(page.getByTestId('import-secret')).toBeEnabled();
  });

  test('disables Export for a service with no staged changes', async ({ page }) => {
    // Only param is staged.
    await setupWailsMocks(page, { stagedParam: [createStagedValue('/p', 'create', 'v')], stagedSecret: [] });
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    await openTransferMenu(page);
    await expect(page.getByTestId('export-param')).toBeEnabled();
    await expect(page.getByTestId('export-secret')).toBeDisabled();
  });

  test('exports a service to the chosen file (plaintext) and clears the working area', async ({ page }) => {
    await setupWailsMocks(page, createStagedForExportState());
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    await openTransferMenu(page);
    await page.getByTestId('export-param').click();

    // Encrypt passphrase modal → empty passphrase → plaintext confirmation.
    await expect(page.locator('.modal-title')).toHaveText(/Export Param/);
    await submitPlaintextExport(page);

    // Result modal reports the exported counts.
    await expect(page.locator('.modal-title')).toHaveText(/Export Complete/);
    await expect(page.locator('.result-stats')).toContainText('1 entries');

    // The file was written to the dialog-chosen path with the AWS scope header.
    const files = await readExportFiles(page);
    const file = files['/mock/exports/export.json'];
    expect(file.service).toBe('param');
    expect(file.scope).toBe(awsScopeKey);
    expect(file.encrypted).toBe(false);
    expect(file.entries.map((e: any) => e.name)).toEqual(['/test/param']);

    // Working param area was cleared; secret remains staged.
    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('.section').nth(0).locator('.entry-item')).toHaveCount(0);
    await expect(page.locator('.section').nth(1).locator('.entry-item')).toHaveCount(1);
  });

  test('keeps the working area when the Keep option is checked', async ({ page }) => {
    await setupWailsMocks(page, createStagedForExportState());
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    await openTransferMenu(page);
    await page.getByTestId('export-param').click();

    await expect(page.locator('.modal-title')).toHaveText(/Export Param/);
    // Opt in to retaining the working area, then export as plaintext.
    await page.getByTestId('export-keep').check();
    await submitPlaintextExport(page);

    await expect(page.locator('.modal-title')).toHaveText(/Export Complete/);

    // The file was still written.
    const files = await readExportFiles(page);
    expect(files['/mock/exports/export.json'].service).toBe('param');

    // Working param area was NOT cleared (Keep was checked); secret remains too.
    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('.section').nth(0).locator('.entry-item')).toHaveCount(1);
    await expect(page.locator('.section').nth(1).locator('.entry-item')).toHaveCount(1);
  });

  test('encrypts the export when a passphrase is supplied', async ({ page }) => {
    await setupWailsMocks(page, createStagedForExportState());
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    await openTransferMenu(page);
    await page.getByTestId('export-secret').click();
    await submitEncryptedExport(page, 'hunter2');

    await expect(page.locator('.modal-title')).toHaveText(/Export Complete/);

    const files = await readExportFiles(page);
    expect(files['/mock/exports/export.json'].service).toBe('secret');
    expect(files['/mock/exports/export.json'].encrypted).toBe(true);
  });

  test('aborts silently when the save dialog is cancelled', async ({ page }) => {
    await setupWailsMocks(page, { ...createStagedForExportState(), savePath: '' });
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    await openTransferMenu(page);
    await page.getByTestId('export-param').click();

    // No passphrase modal, no result modal — the flow aborted.
    await expect(page.locator('.modal-title')).toHaveCount(0);
    const files = await readExportFiles(page);
    expect(Object.keys(files)).toHaveLength(0);
  });
});

// ----------------------------------------------------------------------------
// Import
// ----------------------------------------------------------------------------

test.describe('Import', () => {
  test('imports a plaintext file into the working area', async ({ page }) => {
    await setupWailsMocks(page, createImportFileState('/mock/in.json', {
      service: 'param',
      entries: [createStagedValue('/imported/param', 'create', 'v')],
    }));
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.getByTestId('transfer-menu').waitFor();

    await openTransferMenu(page);
    await page.getByTestId('import-param').click();

    await expect(page.locator('.modal-title')).toHaveText(/Import Complete/);
    await expect(page.locator('.result-stats')).toContainText('1 entries');

    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('.entry-name').filter({ hasText: '/imported/param' })).toBeVisible();
  });

  test('prompts for a passphrase when the file is encrypted', async ({ page }) => {
    await setupWailsMocks(page, createImportFileState('/mock/enc.json', {
      service: 'secret',
      entries: [createStagedValue('imported-secret', 'create', 'v')],
      encrypted: true,
    }));
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.getByTestId('transfer-menu').waitFor();

    await openTransferMenu(page);
    await page.getByTestId('import-secret').click();

    // Decrypt passphrase modal appears (only because the payload is encrypted).
    await expect(page.locator('.modal-title')).toHaveText(/Import Secret/);
    await page.locator('#passphrase').fill('hunter2');
    await page.locator('.modal button[type="submit"]').click();

    await expect(page.locator('.modal-title')).toHaveText(/Import Complete/);
  });

  test('warns on a scope mismatch and imports after confirmation', async ({ page }) => {
    await setupWailsMocks(page, createImportFileState('/mock/other.json', {
      service: 'param',
      entries: [createStagedValue('/foreign/param', 'create', 'v')],
      scope: 'aws/999999999999/us-east-1',
    }));
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.getByTestId('transfer-menu').waitFor();

    await openTransferMenu(page);
    await page.getByTestId('import-param').click();

    await expect(page.locator('.modal-title')).toHaveText(/Scope Mismatch/);
    await page.getByTestId('import-warn-continue').click();

    await expect(page.locator('.modal-title')).toHaveText(/Import Complete/);
    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('.entry-name').filter({ hasText: '/foreign/param' })).toBeVisible();
  });

  test('refuses a provider mismatch even after confirming the warning (#486)', async ({ page }) => {
    // An Azure App Config param file imported into the default AWS scope. The
    // frontend surfaces the scope warning; confirming it passes force=true, but
    // the backend still refuses a provider change (defense-in-depth parity).
    await setupWailsMocks(page, createImportFileState('/mock/azure.json', {
      service: 'param',
      entries: [createStagedValue('/foreign/param', 'create', 'v')],
      provider: 'azure',
      scope: 'azure/appconfig/mystore',
    }));
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.getByTestId('transfer-menu').waitFor();

    await openTransferMenu(page);
    await page.getByTestId('import-param').click();

    await expect(page.locator('.modal-title')).toHaveText(/Scope Mismatch/);
    await page.getByTestId('import-warn-continue').click();

    // The backend refuses the provider change; an error modal appears.
    await expect(page.locator('.modal-title')).toHaveText(/Import Failed/);
    await expect(page.getByTestId('import-error')).toContainText('provider');

    // Nothing was imported.
    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('.entry-item')).toHaveCount(0);
  });

  test('refuses a service mismatch (file holds another service)', async ({ page }) => {
    // A secret file, but the user picked "Import Param".
    await setupWailsMocks(page, createImportFileState('/mock/secret.json', {
      service: 'secret',
      entries: [createStagedValue('a-secret', 'create', 'v')],
    }));
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.getByTestId('transfer-menu').waitFor();

    await openTransferMenu(page);
    await page.getByTestId('import-param').click();

    await expect(page.locator('.modal-title')).toHaveText(/Import Failed/);
    await expect(page.getByTestId('import-error')).toContainText('secret');

    // Nothing was imported.
    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('.entry-item')).toHaveCount(0);
  });

  test('aborts silently when the open dialog is cancelled', async ({ page }) => {
    await setupWailsMocks(page, { openPath: '' });
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.getByTestId('transfer-menu').waitFor();

    await openTransferMenu(page);
    await page.getByTestId('import-param').click();

    await expect(page.locator('.modal-title')).toHaveCount(0);
  });

  test('offers merge / overwrite when the working area already has changes', async ({ page }) => {
    const seeded = createImportFileState('/mock/in.json', {
      service: 'param',
      entries: [createStagedValue('/imported/param', 'create', 'v')],
    }) as Partial<MockState>;
    await setupWailsMocks(page, {
      ...seeded,
      stagedParam: [createStagedValue('/existing/param', 'create', 'v')],
    });
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    await openTransferMenu(page);
    await page.getByTestId('import-param').click();

    // Mode modal appears because the param service already has a staged change.
    await expect(page.locator('.modal-title')).toHaveText(/Import Param/);
    await page.getByRole('radio', { name: 'Overwrite' }).check();
    await page.getByTestId('import-mode-continue').click();

    await expect(page.locator('.modal-title')).toHaveText(/Import Complete/);
    await page.getByRole('button', { name: 'Close' }).click();

    // Overwrite replaced the existing change with the imported one.
    await expect(page.locator('.entry-name').filter({ hasText: '/imported/param' })).toBeVisible();
    await expect(page.locator('.entry-name').filter({ hasText: '/existing/param' })).toHaveCount(0);
  });
});

// ----------------------------------------------------------------------------
// #445 regression: Azure App Configuration param round-trips under the App
// Configuration bucket, never the Key Vault bucket.
// ----------------------------------------------------------------------------

test.describe('#445 Azure App Configuration param round-trip', () => {
  test('exports/imports an App Config param under the App Configuration scope and stays visible', async ({ page }) => {
    await setupWailsMocks(page, createAzureState({
      savePath: '/mock/appconfig.json',
      openPath: '/mock/appconfig.json',
    }));
    await page.goto('/');

    // Stage an App Configuration param under the Azure scope.
    await page.evaluate(async () => {
      await (window as any).go.gui.App.StagingAdd('param', 'app/feature/flag', 'on', '');
    });

    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');
    await expect(page.locator('.entry-name').filter({ hasText: 'app/feature/flag' })).toBeVisible();

    // Export the param — the header scope MUST be the App Configuration bucket,
    // NOT the Key Vault bucket (the #445 bug resolved the combined scope, which
    // keys to Key Vault).
    await openTransferMenu(page);
    await page.getByTestId('export-param').click();
    await submitPlaintextExport(page);
    await expect(page.locator('.modal-title')).toHaveText(/Export Complete/);

    const files = await readExportFiles(page);
    const file = files['/mock/appconfig.json'];
    expect(file.service).toBe('param');
    expect(file.scope).toBe('azure/appconfig/my-store');
    expect(file.scope).not.toContain('keyvault');

    await page.getByRole('button', { name: 'Close' }).click();
    // Working param area cleared after export.
    await expect(page.locator('.entry-name').filter({ hasText: 'app/feature/flag' })).toHaveCount(0);

    // Import it back — it resolves under the App Configuration scope
    // (scopeMatches), stays a param, and is visible to status again.
    await openTransferMenu(page);
    await page.getByTestId('import-param').click();
    await expect(page.locator('.modal-title')).toHaveText(/Import Complete/);
    await page.getByRole('button', { name: 'Close' }).click();

    await expect(page.locator('.entry-name').filter({ hasText: 'app/feature/flag' })).toBeVisible();
  });
});
