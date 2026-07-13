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
// GUI secret-service diff reveal vs passive masking (#714, #715, #735)
//
// Per the confirmed policy, an EXPLICIT diff surface — the "Version Comparison"
// modal and the staging DIFF view's remote-vs-staged ± comparison — is REVEALED
// by default (the user opened it to inspect the change), with a Hide/Show toggle
// (#735). PASSIVE surfaces stay masked by default:
//
//   - the staging VALUE view (plain staged-value listing),
//   - a staged CREATE (a lone new value, not a remote-vs-staged comparison).
//
// These tests assert the diff comparison reveals and the toggle hides, while the
// passive value view and create values remain masked with bullets/asterisks.
// ============================================================================

test.describe('Secret-service version diff reveal + hide toggle (#714/#735)', () => {
  test.beforeEach(async ({ page }) => {
    // Default mock state ships my-secret with three versions carrying
    // distinctive cleartext values.
    await setupWailsMocks(page);
    await page.goto('/');
    await navigateTo(page, 'Secret');
    await waitForItemList(page);
  });

  test('reveals both sides of a secret version comparison by default, hides on toggle', async ({ page }) => {
    await clickItemByName(page, 'my-secret');

    await page.getByRole('button', { name: /Compare/i }).click();
    await page.locator('.diff-checkbox input').first().check();
    await page.locator('.diff-checkbox input').nth(1).check();
    await page.getByRole('button', { name: 'Show Diff' }).click();

    await expect(page.getByText('Version Comparison')).toBeVisible();

    // Revealed by default: the explicit comparison shows real values so the diff
    // is meaningful (#735).
    let oldSide = (await page.locator('.diff-value.diff-old').textContent()) ?? '';
    let newSide = (await page.locator('.diff-value.diff-new').textContent()) ?? '';
    expect(oldSide + newSide).toContain('secret-value');

    // The Hide toggle masks both sides — no cleartext version value remains.
    await page.locator('.btn-mask-toggle').click();
    oldSide = (await page.locator('.diff-value.diff-old').textContent()) ?? '';
    newSide = (await page.locator('.diff-value.diff-new').textContent()) ?? '';
    for (const side of [oldSide, newSide]) {
      expect(side).not.toContain('secret-value-1');
      expect(side).not.toContain('secret-value-old');
      expect(side).not.toContain('secret-value-initial');
      expect(side).not.toContain('secret-value');
    }
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

  test('reveals staged/remote comparison values in diff view (create stays masked), hides on toggle', async ({ page }) => {
    await gotoSecretStaging(page);
    // Default view mode is Diff.
    await expect(page.getByRole('button', { name: 'Diff' })).toHaveClass(/active/);

    let body = (await page.locator('.staging-content').textContent()) ?? '';
    // An update/delete is a real remote-vs-staged comparison → revealed by
    // default so the change is meaningful (#735).
    expect(body).toContain(UPDATE_VALUE);
    expect(body).toContain(REMOTE_VALUE);
    // A create is a lone new value (no comparison) → stays masked (#719).
    expect(body).not.toContain(CREATE_VALUE);
    expect(body).toMatch(/\*/);
    // The delete sentinel stays readable (it is not secret material).
    expect(body).toContain('(deleted)');

    // Hiding every comparison (each toggle clicked once) masks the revealed values.
    const toggles = page.locator('.btn-mask-toggle');
    const count = await toggles.count();
    for (let i = 0; i < count; i++) {
      await toggles.nth(i).click();
    }
    body = (await page.locator('.staging-content').textContent()) ?? '';
    expect(body).not.toContain(UPDATE_VALUE);
    expect(body).not.toContain(REMOTE_VALUE);
    expect(body).toMatch(/\*/);
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
