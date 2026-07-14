import { test, expect, type Page } from './fixtures/coverage';
import { setupWailsMocks, createAzureState, navigateTo } from './fixtures/wails-mock';

// Azure Key Vault scopes tags PER VERSION (each version has its own tags). The
// detail view must therefore show tags inside each Version History entry — not
// one resource-level list — and offer add/remove only on the latest (current)
// version, since writes target the current version.

function kvWithVersionTags() {
  return createAzureState({
    secretVersions: {
      'kv-secret': [
        {
          versionId: 'vLatest',
          state: 'enabled',
          value: 'v2',
          isCurrent: true,
          created: new Date().toISOString(),
          tags: [{ key: 'env', value: 'prod' }],
        },
        {
          versionId: 'vOld',
          state: 'enabled',
          value: 'v1',
          isCurrent: false,
          created: new Date().toISOString(),
          tags: [],
        },
      ],
    },
  });
}

async function openKvSecret(page: Page) {
  await navigateTo(page, 'Key Vault');
  await page.locator('.item-button').filter({ hasText: 'kv-secret' }).click();
  await expect(page.locator('.detail-panel, .secret-detail, .detail-content').first()).toBeVisible();
}

test.describe('Key Vault per-version tags', () => {
  test('tags render inside each Version History entry, +Add only on latest', async ({ page }) => {
    await setupWailsMocks(page, kvWithVersionTags());
    await page.goto('/');
    await openKvSecret(page);

    // One per-version tags block per version.
    await expect(page.locator('.history-tags')).toHaveCount(2);

    const current = page.locator('.history-item.current-secret');
    await expect(current.locator('.history-tags')).toContainText('env');
    await expect(current.locator('.history-tags')).toContainText('prod');
    // Latest version can add + remove.
    await expect(current.locator('.btn-tag-add-sm')).toHaveCount(1);
    await expect(current.locator('.btn-tag-remove')).toHaveCount(1);

    // A non-current version shows its (empty) tags read-only: no add/remove.
    const older = page.locator('.history-item:not(.current-secret)');
    await expect(older.locator('.no-tags-inline')).toBeVisible();
    await expect(older.locator('.btn-tag-add-sm')).toHaveCount(0);
    await expect(older.locator('.btn-tag-remove')).toHaveCount(0);
  });

  test('the resource-level top-level TAGS list is folded for Key Vault', async ({ page }) => {
    await setupWailsMocks(page, kvWithVersionTags());
    await page.goto('/');
    await openKvSecret(page);

    // The standalone TagList renders an <h4>Tags</h4> section heading; for a
    // per-version provider it must be gone (tags live in the history instead,
    // labelled by a <span>, not a heading).
    await expect(page.getByRole('heading', { name: 'Tags', exact: true })).toHaveCount(0);
    // Version History heading is still there, and the per-version add control is
    // the only tag-add button.
    await expect(page.getByRole('heading', { name: 'Version History' })).toBeVisible();
    await expect(page.locator('.btn-tag-add-sm')).toHaveCount(1);
  });
});
