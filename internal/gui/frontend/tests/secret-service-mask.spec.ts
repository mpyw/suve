import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createSecretStagedState,
  createParamStagedState,
  createStagedValue,
  navigateTo,
  waitForItemList,
  clickItemByName,
} from './fixtures/wails-mock';

// ============================================================================
// GUI secret-service masking leaks (#714, #715)
//
// Pre-existing GUI leaks surfaced during the #704 audit while fixing #677/#702
// (PR #711 fixed the value-type SecureString-param diff + the TUI staging
// review, but left these two GUI secret-SERVICE surfaces unmasked):
//
//   #714 — SecretView "Version Comparison" rendered both secret values in
//          cleartext (the detail pane already masks by default).
//   #715 — the GUI staging review rendered secret-service staged/remote values
//          in cleartext, in both diff and value view modes.
//
// The reference behavior is the GUI's own masked-by-default secret handling
// (detail pane / Version History). These tests assert no plaintext secret value
// reaches either surface and that masking bullets/asterisks are present.
// ============================================================================

test.describe('Secret-service version diff masking (#714)', () => {
  test.beforeEach(async ({ page }) => {
    // Default mock state ships my-secret with three versions carrying
    // distinctive cleartext values.
    await setupWailsMocks(page);
    await page.goto('/');
    await navigateTo(page, 'Secret');
    await waitForItemList(page);
  });

  test('masks both sides of a secret version comparison', async ({ page }) => {
    await clickItemByName(page, 'my-secret');

    await page.getByRole('button', { name: /Compare/i }).click();
    await page.locator('.diff-checkbox input').first().check();
    await page.locator('.diff-checkbox input').nth(1).check();
    await page.getByRole('button', { name: 'Show Diff' }).click();

    await expect(page.getByText('Version Comparison')).toBeVisible();

    const oldSide = (await page.locator('.diff-value.diff-old').textContent()) ?? '';
    const newSide = (await page.locator('.diff-value.diff-new').textContent()) ?? '';

    // No cleartext version value may appear on either side.
    for (const side of [oldSide, newSide]) {
      expect(side).not.toContain('secret-value-1');
      expect(side).not.toContain('secret-value-old');
      expect(side).not.toContain('secret-value-initial');
      expect(side).not.toContain('secret-value');
    }

    // Both sides are masked (a change still shows as differing asterisk runs).
    expect(oldSide).toMatch(/\*/);
    expect(newSide).toMatch(/\*/);
  });
});

test.describe('Secret-service staging review masking (#715)', () => {
  // Distinctive cleartext that must never reach the staging review.
  const UPDATE_VALUE = 'super-secret-staged-update';
  const CREATE_VALUE = 'super-secret-staged-create';
  const REMOTE_VALUE = 'remote-value'; // the mock's remote value for non-create entries

  async function gotoSecretStaging(page: import('@playwright/test').Page) {
    await setupWailsMocks(
      page,
      createSecretStagedState([
        createStagedValue('my-secret', 'update', UPDATE_VALUE),
        createStagedValue('new-secret', 'create', CREATE_VALUE),
        createStagedValue('old-secret', 'delete'),
      ]),
    );
    await page.goto('/');
    await navigateTo(page, 'Staging');
    // Wait until the secret section's staged rows are rendered.
    await expect(page.locator('.entry-item').first()).toBeVisible();
  }

  test('masks staged/remote values in diff view', async ({ page }) => {
    await gotoSecretStaging(page);
    // Default view mode is Diff.
    await expect(page.getByRole('button', { name: 'Diff' })).toHaveClass(/active/);

    const body = (await page.locator('.staging-content').textContent()) ?? '';
    expect(body).not.toContain(UPDATE_VALUE);
    expect(body).not.toContain(CREATE_VALUE);
    expect(body).not.toContain(REMOTE_VALUE);
    // Something is still rendered as masked bullets/asterisks.
    expect(body).toMatch(/\*/);
    // The delete sentinel stays readable (it is not secret material).
    expect(body).toContain('(deleted)');
  });

  test('masks staged values in value view', async ({ page }) => {
    await gotoSecretStaging(page);
    await page.getByRole('button', { name: 'Value' }).click();
    await expect(page.getByRole('button', { name: 'Value' })).toHaveClass(/active/);

    const body = (await page.locator('.staging-content').textContent()) ?? '';
    expect(body).not.toContain(UPDATE_VALUE);
    expect(body).not.toContain(CREATE_VALUE);
    expect(body).toMatch(/\*/);
  });
});

test.describe('SecureString-param staging review masking (per-entry flag, #715)', () => {
  // A SecureString param lives in the non-secret Param section, so only the
  // per-entry DiffEntry.Secret flag can mask it — this isolates that path from
  // the secret section's service-axis flag.
  const SECURE_VALUE = 'securestring-staged-plaintext';

  test('masks a SecureString param staged value in the Param section', async ({ page }) => {
    await setupWailsMocks(page, {
      ...createParamStagedState([
        { name: '/app/secure/token', operation: 'update', value: SECURE_VALUE, secret: true },
      ]),
    });
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await expect(page.locator('.entry-item').first()).toBeVisible();

    // Value view renders the staged value directly.
    await page.getByRole('button', { name: 'Value' }).click();
    const body = (await page.locator('.staging-content').textContent()) ?? '';
    expect(body).not.toContain(SECURE_VALUE);
    expect(body).toMatch(/\*/);
  });
});

test.describe('SecureString-param staging review CREATE masking (#719)', () => {
  // A create has no remote to fetch, so the diff usecase used to leave its
  // Secret flag false — the create-staged value rendered in cleartext. The flag
  // now derives from the staged value type (SecureString ⇒ secret), so a
  // create-staged SecureString param is masked like every other secret value.
  const SECURE_CREATE_VALUE = 'securestring-created-plaintext';

  async function gotoParamCreateStaging(page: import('@playwright/test').Page) {
    await setupWailsMocks(page, {
      ...createParamStagedState([
        { name: '/app/secure/new-token', operation: 'create', value: SECURE_CREATE_VALUE, secret: true },
      ]),
    });
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await expect(page.locator('.entry-item').first()).toBeVisible();
  }

  test('masks a create-staged SecureString param value in diff view', async ({ page }) => {
    await gotoParamCreateStaging(page);
    // Default view mode is Diff; a create renders its staged value directly.
    await expect(page.getByRole('button', { name: 'Diff' })).toHaveClass(/active/);

    const body = (await page.locator('.staging-content').textContent()) ?? '';
    expect(body).not.toContain(SECURE_CREATE_VALUE);
    expect(body).toMatch(/\*/);
  });

  test('masks a create-staged SecureString param value in value view', async ({ page }) => {
    await gotoParamCreateStaging(page);
    await page.getByRole('button', { name: 'Value' }).click();
    await expect(page.getByRole('button', { name: 'Value' })).toHaveClass(/active/);

    const body = (await page.locator('.staging-content').textContent()) ?? '';
    expect(body).not.toContain(SECURE_CREATE_VALUE);
    expect(body).toMatch(/\*/);
  });
});
