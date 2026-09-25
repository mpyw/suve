import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  createStagedValue,
  navigateTo,
  getRecordedCalls,
} from './fixtures/wails-mock';

// #982 — "Apply All" goes through the all-service apply (StagingApplyAll), the
// same GlobalApplyUseCase as the CLI's `stage apply`: every service is
// conflict-checked before any is applied. A conflict in one service must leave
// the other service unapplied and staged, in either service order.

const applyAllButton = (page: Page) => page.getByRole('button', { name: 'Apply All' });
const confirmApply = (page: Page) =>
  page.locator('.form-actions').getByRole('button', { name: 'Apply', exact: true }).click();
const stagingBadge = (page: Page) => page.locator('.staging-count');
const paramEntry = (page: Page) => page.locator('.entry-item').filter({ hasText: '/app/param-a' });
const secretEntry = (page: Page) => page.locator('.entry-item').filter({ hasText: 'secret-a' });

function bothStaged(conflicts: Partial<Record<'param' | 'secret', string[]>>) {
  return {
    stagedParam: [createStagedValue('/app/param-a', 'update', 'pv')],
    stagedSecret: [createStagedValue('secret-a', 'update', 'sv')],
    stagingApplyConflicts: conflicts,
  };
}

test.describe('Staging "Apply All" is atomic across services (#982)', () => {
  for (const { label, conflicts, shown } of [
    { label: 'secret conflicts', conflicts: { secret: ['secret-a'] }, shown: 'Secrets Manager: secret-a' },
    { label: 'param conflicts', conflicts: { param: ['/app/param-a'] }, shown: 'Parameter Store: /app/param-a' },
  ]) {
    test(`${label}: nothing is applied`, async ({ page }) => {
      await setupWailsMocks(page, bothStaged(conflicts));
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await expect(stagingBadge(page)).toHaveText('2');

      await applyAllButton(page).click();
      await confirmApply(page);

      await expect(page.locator('.conflicts li')).toHaveText([shown]);
      await expect(page.locator('.result-success').first()).toContainText('Entries: 0 succeeded');

      // Both services stay staged; the conflict-free one was not applied.
      await page.getByRole('button', { name: 'Close' }).click();
      await expect(paramEntry(page)).toHaveCount(1);
      await expect(secretEntry(page)).toHaveCount(1);
      await expect(stagingBadge(page)).toHaveText('2');

      // One all-service call, never the per-service loop.
      const calls = await getRecordedCalls(page);
      expect(calls.filter((c) => c === 'StagingApplyAll')).toHaveLength(1);
      expect(calls).not.toContain('StagingApply');
    });
  }

  test('Ignore conflicts applies every service', async ({ page }) => {
    await setupWailsMocks(page, bothStaged({ secret: ['secret-a'] }));
    await page.goto('/');
    await navigateTo(page, 'Staging');

    await applyAllButton(page).click();
    await page.locator('.modal-apply').getByText('Ignore conflicts').click();
    await confirmApply(page);

    const resultNames = page.locator('.result-name');
    await expect(resultNames.filter({ hasText: '/app/param-a' })).toHaveCount(1);
    await expect(resultNames.filter({ hasText: 'secret-a' })).toHaveCount(1);
    await expect(page.locator('.conflicts')).toHaveCount(0);

    await page.getByRole('button', { name: 'Close' }).click();
    await expect(paramEntry(page)).toHaveCount(0);
    await expect(secretEntry(page)).toHaveCount(0);
  });
});
