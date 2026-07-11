import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createGoogleCloudState,
  createAzureState,
  createAmbiguousProviderState,
  createNoActiveProviderState,
  createStagedForExportState,
  getRecordedCalls,
  waitForItemList,
  waitForViewLoaded,
  navigateTo,
  clickItemByName,
} from './fixtures/wails-mock';

const nav = (page: import('@playwright/test').Page) => page.locator('.nav');

test.describe('Provider selection', () => {
  test.describe('Initial pick', () => {
    test('AWS preselected, no prompt', async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await waitForViewLoaded(page);

      await expect(page.locator('#provider-select')).toHaveValue('aws');
      await expect(nav(page).getByRole('button', { name: /Param/i })).toBeVisible();
      await expect(page.getByText('Select a provider')).toHaveCount(0);
    });

    test('ambiguous detection → selector prompt, nothing preselected', async ({ page }) => {
      await setupWailsMocks(page, createAmbiguousProviderState());
      await page.goto('/');

      await expect(page.getByText('Select a provider to begin.')).toBeVisible();
      await expect(page.locator('#provider-select')).toHaveValue('');
      // No tabs until a scope is active.
      await expect(nav(page)).toHaveCount(0);
    });

    test('zero active providers → prompt, no crash', async ({ page }) => {
      await setupWailsMocks(page, createNoActiveProviderState());
      await page.goto('/');

      await expect(page.getByText('Select a provider to begin.')).toBeVisible();
      await expect(page.locator('.logo-text')).toContainText('suve');
    });
  });

  test.describe('Google Cloud', () => {
    test('no Param tab; Secret + Staging tabs; secret view active', async ({ page }) => {
      await setupWailsMocks(page, createGoogleCloudState());
      await page.goto('/');
      await waitForItemList(page);

      await expect(nav(page).getByRole('button', { name: /Param/i })).toHaveCount(0);
      // Google Cloud secret staging is available (#270).
      await expect(nav(page).getByRole('button', { name: /Staging/i })).toBeVisible();
      const secretTab = nav(page).getByRole('button', { name: /Secret/i });
      await expect(secretTab).toBeVisible();
      await expect(secretTab).toHaveClass(/active/);
    });

    test('sidebar shows project, not AWS account/region', async ({ page }) => {
      await setupWailsMocks(page, createGoogleCloudState());
      await page.goto('/');
      await waitForItemList(page);

      await expect(page.getByText('my-project')).toBeVisible();
      await expect(page.locator('.aws-info-profile')).toHaveCount(0);
    });

    test('no GetAWSIdentity (STS) call under a Google Cloud scope', async ({ page }) => {
      await setupWailsMocks(page, createGoogleCloudState());
      await page.goto('/');
      await waitForItemList(page);

      // Staging is now multi-provider, so StagingStatus may run under Google
      // Cloud — but it must resolve the scope without any AWS STS round-trip.
      const calls = await getRecordedCalls(page);
      expect(calls).not.toContain('GetAWSIdentity');
    });

    test('secret detail: no Restore, no ARN (capability/presence-gated)', async ({ page }) => {
      await setupWailsMocks(page, createGoogleCloudState());
      await page.goto('/');
      await waitForItemList(page);

      // Restore lives in the toolbar; hidden because gcloud secret hasRestore=false.
      await expect(page.locator('.btn-restore')).toHaveCount(0);

      await clickItemByName(page, 'gcloud-secret-1');
      await expect(page.locator('.detail-panel')).toBeVisible();
      // No ARN section (empty arn) — presence-gated (#268).
      await expect(page.locator('.arn-display')).toHaveCount(0);
    });
  });

  test.describe('Azure', () => {
    test('is selectable in the provider selector (un-greyed by #267)', async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await waitForViewLoaded(page);

      await expect(page.locator('#provider-select option[value="azure"]')).toBeEnabled();
    });

    test('App Configuration (param): no Type dropdown, no version history, but tags supported', async ({ page }) => {
      await setupWailsMocks(page, createAzureState());
      await page.goto('/');
      await waitForItemList(page);

      // Default view clamps to the first service (param = App Configuration).
      await clickItemByName(page, 'app/config/key');
      await expect(page.locator('.detail-panel')).toBeVisible();
      // Unversioned, but tags ARE writable (azappconfig/v2), so the tags list
      // renders and shows the item's tag.
      await expect(page.getByRole('heading', { name: 'Version History' })).toHaveCount(0);
      await expect(page.locator('.tags-list')).toBeVisible();
      await expect(page.locator('.tags-list')).toContainText('env');
      await expect(page.locator('.tags-list')).toContainText('prod');

      // New-item modal has no Type dropdown (ParamTypeOptions empty).
      await page.getByRole('button', { name: '+ New' }).click();
      await expect(page.locator('#param-type')).toHaveCount(0);
    });

    test('App Configuration detail shows the value even though history is unsupported', async ({ page }) => {
      // Regression: ParamLog fails on App Config (no history). The detail (value)
      // must still render and no history-error banner must appear — the value
      // fetch must not be coupled to the failing history fetch.
      await setupWailsMocks(page, createAzureState());
      await page.goto('/');
      await waitForItemList(page);

      await clickItemByName(page, 'app/config/key');
      await expect(page.locator('.detail-panel')).toBeVisible();
      await expect(page.locator('.value-display')).toBeVisible();
      await expect(page.locator('.error-banner')).toHaveCount(0);
    });

    test('Key Vault (secret): version history yes, Restore yes (soft-delete)', async ({ page }) => {
      await setupWailsMocks(page, createAzureState());
      await page.goto('/');
      await waitForItemList(page);
      await navigateTo(page, 'Key Vault');
      await waitForItemList(page);

      // Key Vault soft-deletes secrets, so Restore is offered (hasRestore=true).
      await expect(page.locator('.btn-restore')).toBeVisible();
      await clickItemByName(page, 'kv-secret');
      await expect(page.locator('.detail-panel')).toBeVisible();
    });
  });

  test.describe('Provider switch', () => {
    test('switching AWS → Google Cloud remounts (detail closed, Param tab gone)', async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await waitForItemList(page);

      // Open an AWS param detail.
      await clickItemByName(page, '/app/config');
      await expect(page.locator('.detail-panel')).toBeVisible();

      // Switch provider → Google Cloud needs a project first.
      await page.locator('#provider-select').selectOption('googlecloud');
      await page.locator('#gcloud-project').fill('switch-project');
      await page.getByRole('button', { name: 'Connect' }).click();
      await waitForItemList(page);

      // Remounted into Google Cloud: the AWS detail is gone and there is no Param tab.
      await expect(page.locator('.detail-panel')).toHaveCount(0);
      await expect(nav(page).getByRole('button', { name: /Param/i })).toHaveCount(0);
    });

    test('staging badge is scope-keyed (AWS count does not leak to Google Cloud)', async ({ page }) => {
      await setupWailsMocks(page, createStagedForExportState());
      await page.goto('/');
      await waitForItemList(page);

      // AWS has staged changes → badge present.
      await expect(page.locator('.staging-count')).toBeVisible();

      await page.locator('#provider-select').selectOption('googlecloud');
      await page.locator('#gcloud-project').fill('switch-project');
      await page.getByRole('button', { name: 'Connect' }).click();
      await waitForItemList(page);

      // Google Cloud has its own (empty) staging set → no badge (AWS count gone).
      await expect(page.locator('.staging-count')).toHaveCount(0);
    });
  });

  test.describe('Per-provider env scope (#518)', () => {
    // Mixed env: AWS + Google Cloud both active → detection is ambiguous, so the
    // app preselects nothing. Switching to Google Cloud must still resolve its
    // project from GOOGLE_CLOUD_PROJECT (via EnvScope) and auto-apply, without
    // any manual entry — mirroring the CLI's per-provider env resolution.
    // Before the fix, buildSelection only consulted the launch provider's env,
    // so a switched-to provider fell back to the (empty) cache and forced the
    // user to hand-type the project.
    test('switching to Google Cloud in a mixed env prefills project from env', async ({ page }) => {
      await setupWailsMocks(page, {
        ...createAmbiguousProviderState(),
        envScopes: { googlecloud: { projectId: 'env-project' } },
      });
      await page.goto('/');

      // Ambiguous detection → nothing preselected.
      await expect(page.getByText('Select a provider to begin.')).toBeVisible();
      await expect(page.locator('#provider-select')).toHaveValue('');

      // Switch to Google Cloud WITHOUT touching the project field.
      await page.locator('#provider-select').selectOption('googlecloud');

      // Env-derived project auto-applies: the secret view mounts and the sidebar
      // shows the project — no scope-entry prompt.
      await waitForItemList(page);
      await expect(page.getByText('env-project')).toBeVisible();
      await expect(page.getByText('Enter the required scope in the sidebar to continue.')).toHaveCount(0);
    });
  });
});
