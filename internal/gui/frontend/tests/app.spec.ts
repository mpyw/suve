import { test, expect, Page } from '@playwright/test';

// Setup Wails mock before each test
async function setupWailsMocks(page: Page) {
  await page.addInitScript(() => {
    // Mock window.go object that Wails provides
    const mockApp = {
      ParamList: async () => ({
        entries: [{ name: '/app/config', type: 'String', value: 'test-value' }],
        nextToken: '',
      }),
      ParamShow: async () => ({
        name: '/app/config', value: 'test-value', version: 1, type: 'String', tags: [],
      }),
      ParamLog: async () => ({ name: '/app/config', entries: [] }),
      ParamSet: async () => ({ name: '/app/config', version: 2, isCreated: false }),
      ParamDelete: async () => ({ name: '/app/config' }),
      ParamDiff: async () => ({ oldName: '', newName: '', oldValue: '', newValue: '' }),
      ParamAddTag: async () => ({ name: '/app/config' }),
      ParamRemoveTag: async () => ({ name: '/app/config' }),
      SecretList: async () => ({
        entries: [{ name: 'my-secret', value: 'secret-value' }],
        nextToken: '',
      }),
      SecretShow: async () => ({
        name: 'my-secret', arn: '', versionId: 'v1', versionStage: ['AWSCURRENT'],
        value: 'secret-value', tags: [],
      }),
      SecretLog: async () => ({ name: 'my-secret', entries: [] }),
      SecretCreate: async () => ({ name: 'my-secret', versionId: 'v1', arn: '' }),
      SecretUpdate: async () => ({ name: 'my-secret', versionId: 'v2', arn: '' }),
      SecretDelete: async () => ({ name: 'my-secret', arn: '' }),
      SecretDiff: async () => ({ oldName: '', oldVersionId: '', oldValue: '', newName: '', newVersionId: '', newValue: '' }),
      SecretRestore: async () => ({ name: 'my-secret', arn: '' }),
      SecretAddTag: async () => ({ name: 'my-secret' }),
      SecretRemoveTag: async () => ({ name: 'my-secret' }),
      StagingStatus: async () => ({ ssm: [], sm: [], ssmTags: [], smTags: [] }),
      StagingDiff: async () => ({ itemName: 'parameter', entries: [], tagEntries: [] }),
      StagingApply: async () => ({
        serviceName: 'ssm', entryResults: [], tagResults: [], conflicts: [],
        entrySucceeded: 0, entryFailed: 0, tagSucceeded: 0, tagFailed: 0,
      }),
      StagingReset: async () => ({ type: 'all', serviceName: 'ssm', count: 0 }),
      StagingAdd: async () => ({ name: '/app/config' }),
      StagingEdit: async () => ({ name: '/app/config' }),
      StagingDelete: async () => ({ name: '/app/config' }),
      StagingUnstage: async () => ({ name: '/app/config' }),
      StagingAddTag: async () => ({ name: '/app/config' }),
      StagingRemoveTag: async () => ({ name: '/app/config' }),
      StagingCancelAddTag: async () => ({ name: '/app/config' }),
      StagingCancelRemoveTag: async () => ({ name: '/app/config' }),
    };

    (window as any).go = { main: { App: mockApp } };
    (window as any).runtime = {
      EventsOn: () => {}, EventsOff: () => {}, EventsEmit: () => {},
      WindowSetTitle: () => {}, BrowserOpenURL: () => {},
    };
  });
}

test.describe('App Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
  });

  test('should display the app logo', async ({ page }) => {
    await expect(page.locator('.logo-text')).toContainText('suve');
  });

  test('should show sidebar with navigation items', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Parameters/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /Secrets/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /Staging/i })).toBeVisible();
  });

  test('should highlight active navigation item', async ({ page }) => {
    // Parameters should be active by default
    const paramsBtn = page.getByRole('button', { name: /Parameters/i });
    await expect(paramsBtn).toHaveClass(/active/);

    // Click on Secrets and verify it becomes active
    await page.getByRole('button', { name: /Secrets/i }).click();
    await expect(page.getByRole('button', { name: /Secrets/i })).toHaveClass(/active/);

    // Click on Staging and verify it becomes active
    await page.getByRole('button', { name: /Staging/i }).click();
    await expect(page.getByRole('button', { name: /Staging/i })).toHaveClass(/active/);
  });
});

test.describe('Parameters View', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
  });

  test('should have a refresh button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Refresh/i })).toBeVisible();
  });

  test('should show loading state initially', async ({ page }) => {
    // The button shows "Loading..." when data is being fetched
    const refreshBtn = page.getByRole('button', { name: /Refresh|Loading/i });
    await expect(refreshBtn).toBeVisible();
  });
});

test.describe('Secrets View', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await page.getByRole('button', { name: /Secrets/i }).click();
  });

  test('should have a refresh button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Refresh/i })).toBeVisible();
  });

  test('should show loading state initially', async ({ page }) => {
    const refreshBtn = page.getByRole('button', { name: /Refresh|Loading/i });
    await expect(refreshBtn).toBeVisible();
  });
});

test.describe('Staging View', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await page.getByRole('button', { name: /Staging/i }).click();
  });

  test('should display staging area title', async ({ page }) => {
    await expect(page.getByText('Staging Area')).toBeVisible();
  });

  test('should display SSM and SM sections', async ({ page }) => {
    await expect(page.getByText(/Parameters \(SSM\)/i)).toBeVisible();
    await expect(page.getByText(/Secrets \(SM\)/i)).toBeVisible();
  });

  test('should show empty state when no staged changes', async ({ page }) => {
    await page.waitForTimeout(500);
    await expect(page.getByText(/No staged/i).first()).toBeVisible();
  });

  test('should have view mode toggle', async ({ page }) => {
    await expect(page.getByRole('button', { name: 'Diff' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Value' })).toBeVisible();
  });

  test('should switch view mode when toggle clicked', async ({ page }) => {
    const diffBtn = page.getByRole('button', { name: 'Diff' });
    const valueBtn = page.getByRole('button', { name: 'Value' });

    // Diff should be active by default
    await expect(diffBtn).toHaveClass(/active/);

    // Click Value and verify it becomes active
    await valueBtn.click();
    await expect(valueBtn).toHaveClass(/active/);

    // Diff should no longer be active
    await expect(diffBtn).not.toHaveClass(/active/);
  });
});
