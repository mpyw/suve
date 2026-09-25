import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  createStagedValue,
  navigateTo,
} from './fixtures/wails-mock';

// #993 — a staged Update/Delete whose remote fetch failed (not a not-found)
// arrives from StagingDiff as a warning entry with only the reason. The staging
// view shows that reason, keeps Unstage, and offers no Edit or tag controls.

const REASON = 'failed to get parameter: ExpiredToken: the security token has expired';

function warnState() {
  return {
    stagedParam: [{ ...createStagedValue('/app/expired', 'delete'), diffWarning: REASON }],
    stagedParamTags: [{ name: '/app/expired', addTags: { team: 'ops' }, removeTags: {} }],
  };
}

const warnRow = (page: Page) =>
  page.locator('.entry-item').filter({ has: page.locator('[data-testid="entry-warning"]') });

test.describe('Staging warning entries (#993)', () => {
  test('a warning entry shows its reason and only Unstage', async ({ page }) => {
    await setupWailsMocks(page, warnState());
    await page.goto('/');
    await navigateTo(page, 'Staging');

    await expect(warnRow(page)).toHaveCount(1);
    await expect(warnRow(page).locator('.operation-badge')).toHaveText('warning');
    await expect(warnRow(page).locator('[data-testid="entry-warning"]')).toHaveText(REASON);
    await expect(warnRow(page).getByRole('button', { name: 'Unstage' })).toBeVisible();
    await expect(warnRow(page).getByRole('button', { name: 'Edit' })).toHaveCount(0);
    await expect(warnRow(page).locator('.btn-add-tag')).toHaveCount(0);

    // The key's staged tag change is still listed, on its own row.
    const tagRow = page.locator('.entry-item').filter({ hasText: 'team=ops' });
    await expect(tagRow).toHaveCount(1);
    await expect(tagRow.locator('[data-testid="entry-warning"]')).toHaveCount(0);
  });

  test('Unstage removes the warning entry', async ({ page }) => {
    await setupWailsMocks(page, { stagedParam: [{ ...createStagedValue('/app/expired', 'update', 'v'), diffWarning: REASON }] });
    await page.goto('/');
    await navigateTo(page, 'Staging');

    await warnRow(page).getByRole('button', { name: 'Unstage' }).click();
    await expect(warnRow(page)).toHaveCount(0);
  });
});
