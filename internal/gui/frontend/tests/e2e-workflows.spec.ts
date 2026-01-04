import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  waitForItemList,
  clickItemByName,
  navigateTo,
} from './fixtures/wails-mock';

// ============================================================================
// E2E - Parameter CRUD Workflow Tests
// ============================================================================

test.describe('E2E - Parameter CRUD Workflow', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should complete staging workflow for parameter creation', async ({ page }) => {
    // 1. Open create modal
    await page.getByRole('button', { name: '+ New' }).click();

    // 2. Fill form
    await page.locator('#param-name').fill('/new/workflow/param');
    await page.locator('#param-type').selectOption('SecureString');
    await page.locator('#param-value').fill('workflow-secret-value');

    // 3. Stage the parameter
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // 4. Navigate to staging
    await navigateTo(page, 'Staging');
    await expect(page.locator('.staging-content')).toBeVisible();

    // 5. Verify staged change appears
    await expect(page.getByText('/new/workflow/param')).toBeVisible();
  });

  test('should complete full parameter edit workflow', async ({ page }) => {
    // 1. Select parameter
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.detail-panel')).toBeVisible();

    // 2. Open edit modal
    await page.getByRole('button', { name: 'Edit' }).click();

    // 3. Modify value
    await page.locator('#param-value').fill('updated-workflow-value');

    // 4. Stage edit
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // 5. Navigate to staging
    await navigateTo(page, 'Staging');

    // 6. Verify staged change
    await expect(page.getByText('/app/config')).toBeVisible();
  });

  test('should complete full parameter delete workflow', async ({ page }) => {
    // 1. Select parameter
    await clickItemByName(page, '/app/config');

    // 2. Open delete modal
    await page.getByRole('button', { name: 'Delete' }).click();

    // 3. Stage delete
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // 4. Navigate to staging
    await navigateTo(page, 'Staging');

    // 5. Verify staged delete
    await expect(page.getByText('/app/config')).toBeVisible();
  });
});

// ============================================================================
// E2E - Secret CRUD Workflow Tests
// ============================================================================

test.describe('E2E - Secret CRUD Workflow', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
  });

  test('should complete full secret creation workflow', async ({ page }) => {
    // 1. Open create modal
    await page.getByRole('button', { name: '+ New' }).click();

    // 2. Fill form
    await page.locator('#secret-name').fill('new-workflow-secret');
    await page.locator('#secret-value').fill('{"api_key": "secret123"}');

    // 3. Stage
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // 4. Navigate to staging
    await navigateTo(page, 'Staging');

    // 5. Verify
    await expect(page.getByText('new-workflow-secret')).toBeVisible();
  });

  test('should complete full secret edit workflow', async ({ page }) => {
    // 1. Select secret
    await clickItemByName(page, 'my-secret');

    // 2. Edit - use the edit modal's selector
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.locator('#edit-secret-value').fill('updated-secret-value');

    // 3. Stage
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // 4. Navigate to staging
    await navigateTo(page, 'Staging');

    // 5. Verify
    await expect(page.getByText('my-secret')).toBeVisible();
  });
});

// ============================================================================
// E2E - Tag Management Workflow Tests
// ============================================================================

test.describe('E2E - Tag Management Workflow', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should open add tag modal from detail panel', async ({ page }) => {
    // 1. Select parameter
    await clickItemByName(page, '/app/config');

    // 2. Open add tag modal - button text is "+ Add"
    await page.locator('.btn-action-sm').filter({ hasText: '+ Add' }).click();

    // 3. Verify tag modal opened
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    await expect(page.locator('#tag-key')).toBeVisible();
    await expect(page.locator('#tag-value')).toBeVisible();
  });

  test('should open remove tag confirmation modal', async ({ page }) => {
    // 1. Select parameter with tags
    await setupWailsMocks(page, {
      params: [{ name: '/app/config', type: 'String', value: 'test' }],
      paramTags: { '/app/config': [{ key: 'env', value: 'prod' }] },
    });
    await page.goto('/');
    await waitForItemList(page);

    // 2. Select the parameter
    await clickItemByName(page, '/app/config');

    // 3. Click remove on existing tag
    await page.locator('.btn-tag-remove').first().click();

    // 4. Verify remove modal opened
    await expect(page.locator('.modal-backdrop')).toBeVisible();
  });
});

// ============================================================================
// Staging Integration Tests
// ============================================================================

test.describe('Staging Integration', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should reflect staged changes in staging view after creating', async ({ page }) => {
    // Create a new parameter (staged)
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-name').fill('/new/staged/param');
    await page.locator('#param-value').fill('staged-value');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // Navigate to staging
    await navigateTo(page, 'Staging');
    await expect(page.locator('.staging-content')).toBeVisible();

    // Should see the staged change
    await expect(page.getByText('/new/staged/param')).toBeVisible();
  });

  test('should reflect staged changes in staging view after editing', async ({ page }) => {
    // Select and edit a parameter (staged)
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.locator('#param-value').fill('updated-value');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // Navigate to staging
    await navigateTo(page, 'Staging');

    // Should see the staged change
    await expect(page.getByText('/app/config')).toBeVisible();
  });

  test('should reflect staged changes in staging view after deleting', async ({ page }) => {
    // Select and delete a parameter (staged)
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // Navigate to staging
    await navigateTo(page, 'Staging');

    // Should see the staged delete
    await expect(page.getByText('/app/config')).toBeVisible();
  });
});

// ============================================================================
// Cross-Service Tests (SSM + SM)
// ============================================================================

test.describe('Cross-Service Operations', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
  });

  test('should navigate between Parameters and Secrets views', async ({ page }) => {
    // Start on Parameters
    await waitForItemList(page);

    // Navigate to Secrets
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);

    // Navigate back to Parameters
    await navigateTo(page, 'Parameters');
    await waitForItemList(page);

    // Should still show item list
    await expect(page.locator('.item-list')).toBeVisible();
  });

  test('should show staging view sections', async ({ page }) => {
    await navigateTo(page, 'Staging');

    // Should show staging content
    await expect(page.locator('.staging-content')).toBeVisible();
  });
});
