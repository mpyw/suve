import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  createAzureNamespaceState,
  waitForItemList,
  navigateTo,
} from './fixtures/wails-mock';

// #988 — the staging view pairs tag changes with value changes on (name,
// namespace). A value edit on app/db@dev and a tag-only change on app/db@prd are
// two rows, and the prd chip's actions target prd, never dev.

async function stageDevEditAndPrdTag(page: Page) {
  await setupWailsMocks(page, createAzureNamespaceState());
  await page.goto('/');
  await waitForItemList(page);
  await page.evaluate(async () => {
    const app = (window as any).go.gui.App;
    await app.StagingEdit('param', 'app/db', 'dev-db-2', 'dev');
    await app.StagingAddTag('param', 'app/db', 'owner', 'prd-team', 'prd');
  });
  await navigateTo(page, 'Staging');
}

const rows = (page: Page) => page.locator('.entry-item').filter({ hasText: 'app/db' });
const row = (page: Page, ns: string) => rows(page).filter({ has: page.locator('.namespace-badge', { hasText: ns }) });

async function stagedTags(page: Page) {
  return page.evaluate(async () => (await (window as any).go.gui.App.StagingDiff('param', '')).tagEntries);
}

test.describe('Staging tag changes are matched per namespace (#988)', () => {
  test('a tag-only change in another namespace gets its own row', async ({ page }) => {
    await stageDevEditAndPrdTag(page);

    await expect(rows(page)).toHaveCount(2);
    await expect(row(page, 'dev').locator('.tag-changes')).toHaveCount(0);
    await expect(row(page, 'prd').locator('.tag-add')).toContainText('owner=prd-team');
    await expect(page.locator('.section .count-badge').first()).toHaveText('2');
  });

  test('removing the prd chip cancels the prd tag, not a dev one', async ({ page }) => {
    await stageDevEditAndPrdTag(page);

    await row(page, 'prd').locator('.tag-delete-btn').click();

    await expect(row(page, 'prd')).toHaveCount(0);
    expect(await stagedTags(page)).toEqual([]);
  });

  test('editing the prd chip stages the tag on prd', async ({ page }) => {
    await stageDevEditAndPrdTag(page);

    await row(page, 'prd').locator('.tag-item-editable').click();
    await page.locator('.modal input').last().fill('ops');
    await page.locator('.modal').getByRole('button', { name: /Save|Stage/ }).click();

    await expect(row(page, 'prd').locator('.tag-add')).toContainText('owner=ops');
    const tags = await stagedTags(page);
    expect(tags.map((t: any) => t.namespace)).toEqual(['prd']);
  });
});
