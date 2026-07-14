import { test, expect, type Page } from './fixtures/coverage';
import { setupWailsMocks, createAzureState, navigateTo } from './fixtures/wails-mock';

// Azure Key Vault soft-deletes secrets like AWS Secrets Manager, so the GUI must
// offer Restore (capability-gated on hasRestore). Force-delete/purge is NOT
// supported for Key Vault (hasForceDelete=false): retention is a vault property
// and staged deletes can't carry a purge flag, so the delete modal must not show
// a force-delete option there — deletes are always soft and recoverable.

async function openKeyVault(page: Page) {
  await navigateTo(page, 'Key Vault');
  await expect(page.locator('.item-button').filter({ hasText: 'kv-secret' })).toBeVisible();
}

test.describe('Key Vault soft-delete UI', () => {
  test('offers Restore but no force-delete (purge) option', async ({ page }) => {
    await setupWailsMocks(page, createAzureState());
    await page.goto('/');
    await openKeyVault(page);

    // Restore affordance is present for Key Vault.
    await expect(page.locator('.btn-restore')).toBeVisible();

    // The delete modal has NO force-delete checkbox (purge unsupported).
    await page.locator('.item-button').filter({ hasText: 'kv-secret' }).click();
    await page.locator('.btn-action-sm.btn-danger').filter({ hasText: 'Delete' }).click();
    await expect(page.locator('.force-delete')).toHaveCount(0);
  });

  test('soft-delete then restore round-trips the secret', async ({ page }) => {
    await setupWailsMocks(page, createAzureState());
    await page.goto('/');
    await openKeyVault(page);

    // Delete immediately (soft-delete — no force option exists for Key Vault).
    await page.locator('.item-button').filter({ hasText: 'kv-secret' }).click();
    await page.locator('.btn-action-sm.btn-danger').filter({ hasText: 'Delete' }).click();
    await page.locator('.immediate-checkbox input[type="checkbox"]').check();
    await page.locator('.form-actions .btn-danger').click();

    // Soft-deleted: gone from the listing.
    await expect(page.locator('.item-button').filter({ hasText: 'kv-secret' })).toHaveCount(0);

    // Restore it by name — it is still within the recovery window.
    await page.locator('.btn-restore').click();
    await page.locator('#restore-name').fill('kv-secret');
    await page.locator('.btn-restore-confirm').click();

    // Recovered: back in the listing.
    await expect(page.locator('.item-button').filter({ hasText: 'kv-secret' })).toBeVisible();
  });
});
