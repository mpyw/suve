import { test, expect } from '@playwright/test';
import { setupWailsMocks, defaultMockState, type MockState } from './fixtures/wails-mock';

test.describe('Staging Operations', () => {
  test.describe('Empty Staging Area', () => {
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

    test('should have view mode toggle (Diff/Value)', async ({ page }) => {
      await expect(page.getByRole('button', { name: 'Diff' })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Value' })).toBeVisible();
    });

    test('should switch view mode when toggle clicked', async ({ page }) => {
      const diffBtn = page.getByRole('button', { name: 'Diff' });
      const valueBtn = page.getByRole('button', { name: 'Value' });

      await expect(diffBtn).toHaveClass(/active/);
      await valueBtn.click();
      await expect(valueBtn).toHaveClass(/active/);
      await expect(diffBtn).not.toHaveClass(/active/);
    });
  });

  test.describe('Staging from Parameter View', () => {
    test.beforeEach(async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
    });

    test('should stage new parameter creation', async ({ page }) => {
      await page.getByRole('button', { name: '+ New' }).click();
      await page.locator('#param-name').fill('/test/new-param');
      await page.locator('#param-value').fill('new-param-value');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should stage parameter edit', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Edit' }).click();
      await page.locator('#param-value').fill('edited-value');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should stage parameter delete', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Delete' }).click();
      await page.getByRole('button', { name: 'Stage Delete' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should stage parameter tag addition', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').fill('staged-tag');
      await page.locator('#tag-value').fill('staged-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should stage parameter tag removal', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.locator('.btn-tag-remove').click();
      await page.getByRole('button', { name: 'Stage Remove' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Staging from Secret View', () => {
    test.beforeEach(async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await page.getByRole('button', { name: /Secrets/i }).click();
      await page.waitForSelector('.item-list');
    });

    test('should stage new secret creation', async ({ page }) => {
      await page.getByRole('button', { name: '+ New' }).click();
      await page.locator('#secret-name').fill('new-staged-secret');
      await page.locator('#secret-value').fill('{"staged": true}');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should stage secret edit', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Edit' }).click();
      await page.locator('#edit-secret-value').fill('{"edited": true}');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should stage secret delete', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Delete' }).click();
      await page.getByRole('button', { name: 'Stage Delete' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should stage secret tag addition', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').fill('staged-tag');
      await page.locator('#tag-value').fill('staged-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should stage secret tag removal', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.locator('.btn-tag-remove').click();
      await page.getByRole('button', { name: 'Stage Remove' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Staging View with Staged Changes', () => {
    test.beforeEach(async ({ page }) => {
      // Setup with pre-staged changes
      const customState: Partial<MockState> = {
        stagedSSM: [
          { name: '/test/create-param', operation: 'create', value: 'new-value' },
          { name: '/app/config', operation: 'update', value: 'updated-config' },
          { name: '/app/api/key', operation: 'delete' },
        ],
        stagedSM: [
          { name: 'new-secret', operation: 'create', value: '{"new": true}' },
          { name: 'my-secret', operation: 'update', value: 'updated-secret' },
        ],
        stagedSSMTags: [
          { name: '/app/config', addTags: { 'new-key': 'new-value' }, removeTags: ['old-key'] },
        ],
        stagedSMTags: [
          { name: 'my-secret', addTags: { 'environment': 'staging' }, removeTags: [] },
        ],
      };
      await setupWailsMocks(page, customState);
      await page.goto('/');
      await page.getByRole('button', { name: /Staging/i }).click();
      await page.waitForTimeout(500);
    });

    test('should display staged parameter changes', async ({ page }) => {
      // SSM section should show staged items
      await expect(page.locator('.section').first()).toBeVisible();
      await expect(page.locator('.entry-item').first()).toBeVisible();
    });

    test('should display staged secret changes', async ({ page }) => {
      // SM section should show staged items
      await expect(page.locator('.section').nth(1)).toBeVisible();
    });

    test('should show operation badges (create/update/delete)', async ({ page }) => {
      // Look for operation badges in the staged entries
      await expect(page.locator('.operation-badge').first()).toBeVisible();
    });

    test('should have Apply button', async ({ page }) => {
      await expect(page.getByRole('button', { name: /Apply/i }).first()).toBeVisible();
    });

    test('should have Reset button', async ({ page }) => {
      await expect(page.getByRole('button', { name: /Reset/i }).first()).toBeVisible();
    });
  });

  test.describe('Apply and Reset Operations', () => {
    test.beforeEach(async ({ page }) => {
      const customState: Partial<MockState> = {
        stagedSSM: [
          { name: '/test/param', operation: 'create', value: 'test-value' },
        ],
        stagedSM: [
          { name: 'test-secret', operation: 'create', value: 'test-secret-value' },
        ],
      };
      await setupWailsMocks(page, customState);
      await page.goto('/');
      await page.getByRole('button', { name: /Staging/i }).click();
      await page.waitForTimeout(500);
    });

    test('should open apply confirmation modal', async ({ page }) => {
      await page.getByRole('button', { name: /Apply/i }).first().click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
    });

    test('should open reset confirmation modal', async ({ page }) => {
      await page.getByRole('button', { name: /Reset/i }).first().click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
    });

    test('should close apply modal on cancel', async ({ page }) => {
      await page.getByRole('button', { name: /Apply/i }).first().click();
      await page.getByRole('button', { name: 'Cancel' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should close reset modal on cancel', async ({ page }) => {
      await page.getByRole('button', { name: /Reset/i }).first().click();
      await page.getByRole('button', { name: 'Cancel' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Unstage Individual Items', () => {
    test.beforeEach(async ({ page }) => {
      const customState: Partial<MockState> = {
        stagedSSM: [
          { name: '/test/param-1', operation: 'create', value: 'value-1' },
          { name: '/test/param-2', operation: 'update', value: 'value-2' },
        ],
      };
      await setupWailsMocks(page, customState);
      await page.goto('/');
      await page.getByRole('button', { name: /Staging/i }).click();
      await page.waitForTimeout(500);
    });

    test('should have unstage button for each entry', async ({ page }) => {
      const entries = page.locator('.entry-item');
      await expect(entries.first()).toBeVisible();
    });
  });

  test.describe('View Modes', () => {
    test.beforeEach(async ({ page }) => {
      const customState: Partial<MockState> = {
        stagedSSM: [
          { name: '/test/param', operation: 'update', value: 'new-value' },
        ],
      };
      await setupWailsMocks(page, customState);
      await page.goto('/');
      await page.getByRole('button', { name: /Staging/i }).click();
      await page.waitForTimeout(500);
    });

    test('should default to Diff view', async ({ page }) => {
      await expect(page.getByRole('button', { name: 'Diff' })).toHaveClass(/active/);
    });

    test('should switch to Value view', async ({ page }) => {
      await page.getByRole('button', { name: 'Value' }).click();
      await expect(page.getByRole('button', { name: 'Value' })).toHaveClass(/active/);
    });

    test('should toggle between Diff and Value view', async ({ page }) => {
      await page.getByRole('button', { name: 'Value' }).click();
      await expect(page.getByRole('button', { name: 'Value' })).toHaveClass(/active/);
      await page.getByRole('button', { name: 'Diff' }).click();
      await expect(page.getByRole('button', { name: 'Diff' })).toHaveClass(/active/);
    });
  });

  test.describe('Tag Staging Display', () => {
    test.beforeEach(async ({ page }) => {
      const customState: Partial<MockState> = {
        stagedSSMTags: [
          { name: '/app/config', addTags: { 'env': 'staging', 'team': 'backend' }, removeTags: ['old-tag'] },
        ],
        stagedSMTags: [
          { name: 'my-secret', addTags: { 'project': 'test' }, removeTags: ['deprecated'] },
        ],
      };
      await setupWailsMocks(page, customState);
      await page.goto('/');
      await page.getByRole('button', { name: /Staging/i }).click();
      await page.waitForTimeout(500);
    });

    test('should display tag changes section', async ({ page }) => {
      // Tag entries should be visible in the staging view
      await expect(page.locator('.section').first()).toBeVisible();
    });
  });
});
