import { test, expect, type Page } from '@playwright/test';
import { setupWailsMocks, createAzureState, navigateTo } from './fixtures/wails-mock';

// Azure Key Vault soft-deletes secrets like AWS Secrets Manager, so the GUI must
// offer Restore and a force-delete (purge) option — capability-gated on
// hasRestore / hasForceDelete, which are true only for soft-delete providers.

async function openKeyVault(page: Page) {
  await navigateTo(page, 'Key Vault');
  await expect(page.locator('.item-button').filter({ hasText: 'kv-secret' })).toBeVisible();
}

test.describe('Key Vault soft-delete UI', () => {
  test('offers Restore and a force-delete (purge) option', async ({ page }) => {
    await setupWailsMocks(page, createAzureState());
    await page.goto('/');
    await openKeyVault(page);

    // Restore affordance is present for Key Vault.
    await expect(page.locator('.btn-restore')).toBeVisible();

    // The delete modal offers force-delete (purge, skip recovery window).
    await page.locator('.item-button').filter({ hasText: 'kv-secret' }).click();
    await page.locator('.btn-action-sm.btn-danger').filter({ hasText: 'Delete' }).click();
    await expect(page.locator('.force-delete')).toContainText('Force delete');
  });

  test('force delete purges the secret from the list', async ({ page }) => {
    await setupWailsMocks(page, createAzureState());
    await page.goto('/');
    await openKeyVault(page);

    await page.locator('.item-button').filter({ hasText: 'kv-secret' }).click();
    await page.locator('.btn-action-sm.btn-danger').filter({ hasText: 'Delete' }).click();

    // Apply immediately (skip staging) so the purge hits the provider now, and
    // force-delete to purge.
    await page.locator('.immediate-checkbox input[type="checkbox"]').check();
    await page.locator('.force-delete input[type="checkbox"]').check();
    await page.locator('.form-actions .btn-danger').click();

    await expect(page.locator('.item-button').filter({ hasText: 'kv-secret' })).toHaveCount(0);
  });
});
