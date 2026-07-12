import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createParam,
  waitForItemList,
  clickItemByName,
  type MockState,
} from './fixtures/wails-mock';

// ============================================================================
// SecureString param diff masking (GUI counterpart of #677 — see #702)
//
// A SecureString param is masked in the detail pane, but its version diff used
// to render both plaintext values. The diff DTO now carries a value-type secret
// flag (SecureString ⇒ true) that DiffDisplay masks on, so neither cleartext
// value reaches the diff modal.
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

  test('masks both sides of a SecureString version diff', async ({ page }) => {
    await clickItemByName(page, '/app/secure/token');

    await page.getByRole('button', { name: /Compare/i }).click();
    await page.locator('.diff-checkbox input').first().check();
    await page.locator('.diff-checkbox input').nth(1).check();
    await page.getByRole('button', { name: 'Show Diff' }).click();

    await expect(page.getByText('Version Comparison')).toBeVisible();

    // Neither cleartext value may appear anywhere in the diff modal.
    const oldSide = (await page.locator('.diff-value.diff-old').textContent()) ?? '';
    const newSide = (await page.locator('.diff-value.diff-new').textContent()) ?? '';

    expect(oldSide).not.toContain(OLD_SECRET);
    expect(oldSide).not.toContain('plaintext-old');
    expect(newSide).not.toContain(NEW_SECRET);
    expect(newSide).not.toContain('plaintext-new');

    // Both sides are masked with bullets/asterisks, so a change still shows.
    expect(oldSide).toMatch(/\*/);
    expect(newSide).toMatch(/\*/);
  });
});
