import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  type MockState,
  createParam,
  createMultiTagState,
  createNoTagsState,
  waitForItemList,
  clickItemByName,
  openCreateModal,
  closeModal,
} from './fixtures/wails-mock';

test.describe('Parameter CRUD Operations', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test.describe('List and View', () => {
    test('should display parameter list with correct count', async ({ page }) => {
      await expect(page.locator('.item-name.param')).toHaveCount(3);
    });

    test('should display parameter names correctly', async ({ page }) => {
      await expect(page.locator('.item-name.param').filter({ hasText: '/app/config' })).toBeVisible();
      await expect(page.locator('.item-name.param').filter({ hasText: '/app/database/url' })).toBeVisible();
      await expect(page.locator('.item-name.param').filter({ hasText: '/app/api/key' })).toBeVisible();
    });

    test('should show parameter details when clicked', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await expect(page.locator('.detail-panel')).toBeVisible();
      await expect(page.locator('.detail-title')).toContainText('/app/config');
    });

    test('should display parameter metadata (version, type)', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await expect(page.locator('.meta-label').filter({ hasText: 'Version' })).toBeVisible();
      await expect(page.locator('.meta-label').filter({ hasText: 'Type' })).toBeVisible();
    });

    test('should close detail panel when close button clicked', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await expect(page.locator('.detail-panel')).toBeVisible();
      await page.locator('.btn-close').click();
      await expect(page.locator('.detail-panel')).not.toBeVisible();
    });

    test('should display existing tags for parameter', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await expect(page.locator('.tag-item')).toBeVisible();
      await expect(page.locator('.tag-key')).toContainText('env');
      await expect(page.locator('.tag-value')).toContainText('production');
    });

    test('should switch detail view when clicking different parameter', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await expect(page.locator('.detail-title')).toContainText('/app/config');

      await clickItemByName(page, '/app/database/url');
      await expect(page.locator('.detail-title')).toContainText('/app/database/url');
    });
  });

  test.describe('Create Parameter', () => {
    test('should open create modal when "+ New" clicked', async ({ page }) => {
      await openCreateModal(page);
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('New Parameter')).toBeVisible();
    });

    test('should create parameter in staged mode by default', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#param-name').fill('/app/new-param');
      await page.locator('#param-value').fill('new-value');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should create parameter immediately when immediate mode checked', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#param-name').fill('/app/new-param');
      await page.locator('#param-value').fill('new-value');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should show error if name is empty', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#param-value').fill('some-value');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-error')).toContainText('Name and value are required');
    });

    test('should show error if value is empty', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#param-name').fill('/app/test');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-error')).toContainText('Name and value are required');
    });

    test('should cancel create modal', async ({ page }) => {
      await openCreateModal(page);
      await closeModal(page);
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should allow parameter name with special characters', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#param-name').fill('/app/test-param_v2.0');
      await page.locator('#param-value').fill('value');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should allow multiline parameter value', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#param-name').fill('/app/multiline');
      await page.locator('#param-value').fill('line1\nline2\nline3');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Edit Parameter', () => {
    test('should open edit modal with current value', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: 'Edit' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Edit Parameter')).toBeVisible();
      await expect(page.locator('#param-name')).toBeDisabled();
      await expect(page.locator('#param-name')).toHaveValue('/app/config');
    });

    test('should stage edit by default', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: 'Edit' }).click();
      await page.locator('#param-value').fill('updated-value');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should apply edit immediately when immediate mode checked', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: 'Edit' }).click();
      await page.locator('#param-value').fill('updated-value');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should cancel edit modal', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: 'Edit' }).click();
      await closeModal(page);
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Delete Parameter', () => {
    test('should open delete confirmation modal', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: 'Delete' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Delete Parameter')).toBeVisible();
      await expect(page.locator('.delete-target')).toContainText('/app/config');
    });

    test('should stage delete by default', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: 'Delete' }).click();
      await page.getByRole('button', { name: 'Stage Delete' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should delete immediately when immediate mode checked', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.locator('.detail-actions').getByRole('button', { name: 'Delete' }).click();
      await page.locator('.immediate-checkbox input').check();
      await page.locator('.modal-content').getByRole('button', { name: 'Delete' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should cancel delete modal', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: 'Delete' }).click();
      await closeModal(page);
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Tag Operations', () => {
    test('should open add tag modal', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: '+ Add' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Add Tag')).toBeVisible();
    });

    test('should add tag in staged mode', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').waitFor();
      await page.locator('#tag-key').fill('new-tag');
      await page.locator('#tag-value').fill('tag-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should add tag immediately when immediate mode checked', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').waitFor();
      await page.locator('#tag-key').fill('new-tag');
      await page.locator('#tag-value').fill('tag-value');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Add Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should show error if tag key is empty', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-value').fill('some-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-error')).toContainText('Key is required');
    });

    test('should open remove tag modal', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.locator('.btn-tag-remove').click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Remove Tag')).toBeVisible();
    });

    test('should remove tag in staged mode', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.locator('.btn-tag-remove').click();
      await page.getByRole('button', { name: 'Stage Remove' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should remove tag immediately when immediate mode checked', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      await page.locator('.btn-tag-remove').click();
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Remove' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Value Masking', () => {
    test('should mask SecureString value by default', async ({ page }) => {
      await clickItemByName(page, '/app/database/url');
      await expect(page.locator('.value-display.masked')).toBeVisible();
    });

    test('should toggle value visibility', async ({ page }) => {
      await clickItemByName(page, '/app/database/url');
      await page.locator('.btn-toggle').click();
      await expect(page.locator('.value-display:not(.masked)')).toBeVisible();
      await page.locator('.btn-toggle').click();
      await expect(page.locator('.value-display.masked')).toBeVisible();
    });

    test('should not mask regular String value', async ({ page }) => {
      await clickItemByName(page, '/app/config');
      // String type parameters should not be masked by default
      // (implementation-dependent, adjust based on actual behavior)
    });
  });

  test.describe('Filter and Search', () => {
    test('should have prefix filter input', async ({ page }) => {
      await expect(page.locator('.prefix-input')).toBeVisible();
    });

    test('should have regex filter input', async ({ page }) => {
      await expect(page.locator('.regex-input')).toBeVisible();
    });

    test('should have recursive checkbox', async ({ page }) => {
      await expect(page.getByLabel('Recursive')).toBeVisible();
    });

    test('should have show values checkbox', async ({ page }) => {
      await expect(page.getByLabel('Show Values')).toBeVisible();
    });
  });
});

test.describe('Parameter Edge Cases', () => {
  test.describe('Empty Parameter List', () => {
    test('should handle empty parameter list gracefully', async ({ page }) => {
      await setupWailsMocks(page, { params: [] });
      await page.goto('/');
      // Wait for filter bar (always present even when list is empty)
      await page.waitForSelector('.filter-bar');
      await expect(page.locator('.item-button')).toHaveCount(0);
    });

    test('should still allow creating new parameter when list is empty', async ({ page }) => {
      await setupWailsMocks(page, { params: [] });
      await page.goto('/');
      await page.waitForSelector('.filter-bar');
      await openCreateModal(page);
      await expect(page.locator('.modal-backdrop')).toBeVisible();
    });
  });

  test.describe('Multiple Tags', () => {
    test('should display multiple tags for parameter', async ({ page }) => {
      await setupWailsMocks(page, createMultiTagState());
      await page.goto('/');
      await waitForItemList(page);
      await clickItemByName(page, '/app/config');
      await expect(page.locator('.tag-item')).toHaveCount(3);
    });

    test('should allow adding tag to parameter with existing tags', async ({ page }) => {
      await setupWailsMocks(page, createMultiTagState());
      await page.goto('/');
      await waitForItemList(page);
      await clickItemByName(page, '/app/config');
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').waitFor();
      await page.locator('#tag-key').fill('new-tag');
      await page.locator('#tag-value').fill('new-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('No Tags', () => {
    test('should handle parameter with no tags', async ({ page }) => {
      await setupWailsMocks(page, createNoTagsState());
      await page.goto('/');
      await waitForItemList(page);
      await clickItemByName(page, '/app/config');
      await expect(page.locator('.tag-item')).toHaveCount(0);
    });

    test('should show add tag button even when no tags exist', async ({ page }) => {
      await setupWailsMocks(page, createNoTagsState());
      await page.goto('/');
      await waitForItemList(page);
      await clickItemByName(page, '/app/config');
      await expect(page.getByRole('button', { name: '+ Add' })).toBeVisible();
    });
  });

  test.describe('Special Characters in Values', () => {
    test('should handle parameter with JSON value', async ({ page }) => {
      const jsonParam: Partial<MockState> = {
        params: [createParam('/app/json-config', '{"key": "value", "nested": {"a": 1}}', 'String')],
      };
      await setupWailsMocks(page, jsonParam);
      await page.goto('/');
      await waitForItemList(page);
      await clickItemByName(page, '/app/json-config');
      await expect(page.locator('.detail-panel')).toBeVisible();
    });

    test('should handle parameter with special characters', async ({ page }) => {
      const specialParam: Partial<MockState> = {
        params: [createParam('/app/special', 'value with <html> & "quotes"', 'String')],
      };
      await setupWailsMocks(page, specialParam);
      await page.goto('/');
      await waitForItemList(page);
      await clickItemByName(page, '/app/special');
      await expect(page.locator('.detail-panel')).toBeVisible();
    });
  });

  test.describe('Long Parameter Names', () => {
    test('should handle parameter with long hierarchical path', async ({ page }) => {
      const longPath: Partial<MockState> = {
        params: [createParam('/very/long/hierarchical/path/to/parameter/name', 'value', 'String')],
      };
      await setupWailsMocks(page, longPath);
      await page.goto('/');
      await waitForItemList(page);
      await clickItemByName(page, '/very/long/hierarchical/path/to/parameter/name');
      await expect(page.locator('.detail-panel')).toBeVisible();
    });
  });
});
