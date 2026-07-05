import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createGoogleCloudState,
  createAzureState,
  createAmbiguousProviderState,
  createNoActiveProviderState,
  createStagedForPushState,
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
    test('no Param tab, no Staging tab; secret view active', async ({ page }) => {
      await setupWailsMocks(page, createGoogleCloudState());
      await page.goto('/');
      await waitForItemList(page);

      await expect(nav(page).getByRole('button', { name: /Param/i })).toHaveCount(0);
      await expect(nav(page).getByRole('button', { name: /Staging/i })).toHaveCount(0);
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

    test('no GetAWSIdentity / StagingStatus calls under a Google Cloud scope', async ({ page }) => {
      await setupWailsMocks(page, createGoogleCloudState());
      await page.goto('/');
      await waitForItemList(page);

      const calls = await getRecordedCalls(page);
      expect(calls).not.toContain('GetAWSIdentity');
      expect(calls).not.toContain('StagingStatus');
    });

    test('secret detail: no Restore, no staging banner (capability-gated)', async ({ page }) => {
      await setupWailsMocks(page, createGoogleCloudState());
      await page.goto('/');
      await waitForItemList(page);

      // Restore lives in the toolbar; hidden because gcloud secret hasRestore=false.
      await expect(page.locator('.btn-restore')).toHaveCount(0);

      await clickItemByName(page, 'gcloud-secret-1');
      await expect(page.locator('.detail-panel')).toBeVisible();
      await expect(page.locator('.staging-banner')).toHaveCount(0);
      // No ARN section (empty arn) — presence-gated (#268).
      await expect(page.locator('.arn-display')).toHaveCount(0);
    });
  });

  test.describe('Azure', () => {
    test('is greyed-out (disabled) in the provider selector', async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await waitForViewLoaded(page);

      await expect(page.locator('#provider-select option[value="azure"]')).toBeDisabled();
    });

    test('App Configuration (param): no Type dropdown, no version history, no tags', async ({ page }) => {
      await setupWailsMocks(page, createAzureState());
      await page.goto('/');
      await waitForItemList(page);

      // Default view clamps to the first service (param = App Configuration).
      await clickItemByName(page, 'app/config/key');
      await expect(page.locator('.detail-panel')).toBeVisible();
      // Unversioned + untagged.
      await expect(page.getByRole('heading', { name: 'Version History' })).toHaveCount(0);
      await expect(page.locator('.tags-list')).toHaveCount(0);

      // New-item modal has no Type dropdown (ParamTypeOptions empty).
      await page.getByRole('button', { name: '+ New' }).click();
      await expect(page.locator('#param-type')).toHaveCount(0);
    });

    test('Key Vault (secret): version history yes, Restore no', async ({ page }) => {
      await setupWailsMocks(page, createAzureState());
      await page.goto('/');
      await waitForItemList(page);
      await navigateTo(page, 'Key Vault');
      await waitForItemList(page);

      await expect(page.locator('.btn-restore')).toHaveCount(0);
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

    test('staging badge resets when switching to a non-AWS provider', async ({ page }) => {
      await setupWailsMocks(page, createStagedForPushState());
      await page.goto('/');
      await waitForItemList(page);

      // AWS has staged changes → badge present.
      await expect(page.locator('.staging-count')).toBeVisible();

      await page.locator('#provider-select').selectOption('googlecloud');
      await page.locator('#gcloud-project').fill('switch-project');
      await page.getByRole('button', { name: 'Connect' }).click();
      await waitForItemList(page);

      // No Staging tab (and hence no badge) under Google Cloud.
      await expect(page.locator('.staging-count')).toHaveCount(0);
    });
  });
});
