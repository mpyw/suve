import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createParam,
  waitForItemList,
  clickItemByName,
  type MockState,
} from './fixtures/wails-mock';

// ============================================================================
// SecureString param diff reveal + hide toggle (GUI counterpart of #677 — #702)
//
// A SecureString param's Version Comparison is a surface the user explicitly
// opened to inspect the change, so its values are REVEALED by default (#702/
// #735). The DiffDisplay Hide/Show toggle can still mask both sides, matching
// the detail pane's masked-by-default behavior on demand.
// ============================================================================

// Distinctive cleartext values that must NEVER appear in the diff modal.
const OLD_SECRET = 'securestring-plaintext-old';
const NEW_SECRET = 'securestring-plaintext-new-longer';

function secureStringDiffState(): Partial<MockState> {
  return {
    params: [createParam('/app/secure/token', NEW_SECRET, 'SecureString')],
    paramVersions: {
      '/app/secure/token': [
        { version: 2, value: NEW_SECRET, type: 'SecureString', isCurrent: true, lastModified: new Date().toISOString() },
        { version: 1, value: OLD_SECRET, type: 'SecureString', isCurrent: false, lastModified: new Date(Date.now() - 86400000).toISOString() },
      ],
    },
  };
}

test.describe('SecureString param diff masking', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, secureStringDiffState());
    await page.goto('/');
    await waitForItemList(page);
  });

  test('reveals both sides of a SecureString version diff by default, and hides on toggle', async ({ page }) => {
    await clickItemByName(page, '/app/secure/token');

    await page.getByRole('button', { name: /Compare/i }).click();
    await page.locator('.diff-checkbox input').first().check();
    await page.locator('.diff-checkbox input').nth(1).check();
    await page.getByRole('button', { name: 'Show Diff' }).click();

    await expect(page.getByText('Version Comparison')).toBeVisible();

    // Revealed by default: both cleartext values are shown so the diff is useful.
    let oldSide = (await page.locator('.diff-value.diff-old').textContent()) ?? '';
    let newSide = (await page.locator('.diff-value.diff-new').textContent()) ?? '';
    expect(oldSide).toContain(OLD_SECRET);
    expect(newSide).toContain(NEW_SECRET);

    // The Hide toggle masks both sides (bullets/asterisks), so a change still
    // shows without disclosing content.
    await page.locator('.btn-mask-toggle').click();
    oldSide = (await page.locator('.diff-value.diff-old').textContent()) ?? '';
    newSide = (await page.locator('.diff-value.diff-new').textContent()) ?? '';
    expect(oldSide).not.toContain(OLD_SECRET);
    expect(oldSide).not.toContain('plaintext-old');
    expect(newSide).not.toContain(NEW_SECRET);
    expect(newSide).not.toContain('plaintext-new');
    expect(oldSide).toMatch(/\*/);
    expect(newSide).toMatch(/\*/);

    // Show again brings the values back.
    await page.locator('.btn-mask-toggle').click();
    expect((await page.locator('.diff-value.diff-new').textContent()) ?? '').toContain(NEW_SECRET);
  });
});
