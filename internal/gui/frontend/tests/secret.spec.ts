import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  type MockState,
  createSecret,
  createMultiTagState,
  createNoTagsState,
  waitForItemList,
  clickItemByName,
  openCreateModal,
  closeModal,
  navigateTo,
} from './fixtures/wails-mock';

test.describe('Secret CRUD Operations', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
  });

  test.describe('List and View', () => {
    test('should display secret list with correct count', async ({ page }) => {
      await expect(page.locator('.item-name.secret')).toHaveCount(3);
    });

    test('should display secret names correctly', async ({ page }) => {
      await expect(page.locator('.item-name.secret').filter({ hasText: 'my-secret' })).toBeVisible();
      await expect(page.locator('.item-name.secret').filter({ hasText: 'api-credentials' })).toBeVisible();
      await expect(page.locator('.item-name.secret').filter({ hasText: 'database-password' })).toBeVisible();
    });

    test('should show secret details when clicked', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await expect(page.locator('.detail-panel')).toBeVisible();
      await expect(page.locator('.detail-title')).toContainText('my-secret');
    });

    test('should display secret metadata (version ID, labels)', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await expect(page.locator('.meta-label').filter({ hasText: 'Version ID' })).toBeVisible();
      await expect(page.locator('.meta-label').filter({ hasText: 'Labels' })).toBeVisible();
    });

    test('should display ARN', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await expect(page.locator('.arn-display')).toContainText('arn:aws:secretsmanager');
    });

    test('should close detail panel when close button clicked', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await expect(page.locator('.detail-panel')).toBeVisible();
      await page.locator('.btn-close').click();
      await expect(page.locator('.detail-panel')).not.toBeVisible();
    });

    test('should display existing tags for secret', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await expect(page.locator('.tag-item')).toBeVisible();
      await expect(page.locator('.tag-key')).toContainText('team');
      await expect(page.locator('.tag-value')).toContainText('backend');
    });

    test('should switch detail view when clicking different secret', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await expect(page.locator('.detail-title')).toContainText('my-secret');

      await clickItemByName(page, 'api-credentials');
      await expect(page.locator('.detail-title')).toContainText('api-credentials');
    });
  });

  test.describe('Create Secret', () => {
    test('should open create modal when "+ New" clicked', async ({ page }) => {
      await openCreateModal(page);
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('New Secret')).toBeVisible();
    });

    test('should create secret in staged mode by default', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#secret-name').fill('new-secret');
      await page.locator('#secret-value').fill('{"key": "value"}');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should create secret immediately when immediate mode checked', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#secret-name').fill('new-secret');
      await page.locator('#secret-value').fill('{"key": "value"}');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Create' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should show error if name is empty', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#secret-value').fill('some-value');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-error')).toContainText('Name and value are required');
    });

    test('should show error if value is empty', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#secret-name').fill('test-secret');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-error')).toContainText('Name and value are required');
    });

    test('should cancel create modal', async ({ page }) => {
      await openCreateModal(page);
      await closeModal(page);
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should allow plaintext secret value', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#secret-name').fill('plaintext-secret');
      await page.locator('#secret-value').fill('just a plain string');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should allow multiline secret value', async ({ page }) => {
      await openCreateModal(page);
      await page.locator('#secret-name').fill('multiline-secret');
      await page.locator('#secret-value').fill('line1\nline2\nline3');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Edit Secret', () => {
    test('should open edit modal with current value', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: 'Edit' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Edit Secret')).toBeVisible();
      await expect(page.locator('#edit-secret-name')).toBeDisabled();
      await expect(page.locator('#edit-secret-name')).toHaveValue('my-secret');
    });

    test('should stage edit by default', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: 'Edit' }).click();
      await page.locator('#edit-secret-value').fill('{"updated": "value"}');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should apply edit immediately when immediate mode checked', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: 'Edit' }).click();
      await page.locator('#edit-secret-value').fill('{"updated": "value"}');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should cancel edit modal', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: 'Edit' }).click();
      await closeModal(page);
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Delete Secret', () => {
    test('should open delete confirmation modal', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: 'Delete' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Delete Secret')).toBeVisible();
      await expect(page.locator('.delete-target')).toContainText('my-secret');
    });

    test('should have force delete option', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: 'Delete' }).click();
      await expect(page.getByLabel(/Force delete/i)).toBeVisible();
    });

    test('should stage delete by default', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: 'Delete' }).click();
      await page.getByRole('button', { name: 'Stage Delete' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should delete immediately when immediate mode checked', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.locator('.detail-actions').getByRole('button', { name: 'Delete' }).click();
      await page.locator('.immediate-checkbox input').check();
      await page.locator('.modal-content').getByRole('button', { name: 'Delete' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should cancel delete modal', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: 'Delete' }).click();
      await closeModal(page);
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Restore Secret', () => {
    test('should open restore modal', async ({ page }) => {
      await page.getByRole('button', { name: 'Restore' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Restore Secret')).toBeVisible();
    });

    test('should restore secret by name', async ({ page }) => {
      await page.locator('.filter-bar').getByRole('button', { name: 'Restore' }).click();
      await page.locator('#restore-name').fill('deleted-secret');
      await page.locator('.modal-content').getByRole('button', { name: 'Restore' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should cancel restore modal', async ({ page }) => {
      await page.getByRole('button', { name: 'Restore' }).click();
      await closeModal(page);
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Tag Operations', () => {
    test('should open add tag modal', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: '+ Add' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Add Tag')).toBeVisible();
    });

    test('should add tag in staged mode', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').waitFor();
      await page.locator('#tag-key').fill('new-tag');
      await page.locator('#tag-value').fill('tag-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should add tag immediately when immediate mode checked', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').waitFor();
      await page.locator('#tag-key').fill('new-tag');
      await page.locator('#tag-value').fill('tag-value');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Add Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should show error if tag key is empty', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-value').fill('some-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-error')).toContainText('Key is required');
    });

    test('should open remove tag modal', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.locator('.btn-tag-remove').click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Remove Tag')).toBeVisible();
    });

    test('should remove tag in staged mode', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.locator('.btn-tag-remove').click();
      await page.getByRole('button', { name: 'Stage Remove' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should remove tag immediately when immediate mode checked', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.locator('.btn-tag-remove').click();
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Remove' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Value Masking', () => {
    test('should mask secret value by default', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await expect(page.locator('.value-display.masked')).toBeVisible();
    });

    test('should toggle value visibility', async ({ page }) => {
      await clickItemByName(page, 'my-secret');
      await page.locator('.btn-toggle').click();
      await expect(page.locator('.value-display:not(.masked)')).toBeVisible();
      await page.locator('.btn-toggle').click();
      await expect(page.locator('.value-display.masked')).toBeVisible();
    });
  });

  test.describe('Filter and Search', () => {
    test('should have prefix filter input', async ({ page }) => {
      await expect(page.locator('.prefix-input')).toBeVisible();
    });

    test('should have regex filter input', async ({ page }) => {
      await expect(page.locator('.regex-input')).toBeVisible();
    });

    test('should have show values checkbox', async ({ page }) => {
      await expect(page.getByLabel('Show Values')).toBeVisible();
    });
  });
});

test.describe('Secret Edge Cases', () => {
  test.describe('Empty Secret List', () => {
    test('should handle empty secret list gracefully', async ({ page }) => {
      await setupWailsMocks(page, { secrets: [] });
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      // Wait for filter bar (always present even when list is empty)
      await page.waitForSelector('.filter-bar');
      await expect(page.locator('.item-button')).toHaveCount(0);
    });

    test('should still allow creating new secret when list is empty', async ({ page }) => {
      await setupWailsMocks(page, { secrets: [] });
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await page.waitForSelector('.filter-bar');
      await openCreateModal(page);
      await expect(page.locator('.modal-backdrop')).toBeVisible();
    });

    test('should still show restore button when list is empty', async ({ page }) => {
      await setupWailsMocks(page, { secrets: [] });
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await page.waitForSelector('.filter-bar');
      await expect(page.getByRole('button', { name: 'Restore' })).toBeVisible();
    });
  });

  test.describe('Multiple Tags', () => {
    test('should display multiple tags for secret', async ({ page }) => {
      await setupWailsMocks(page, createMultiTagState());
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await waitForItemList(page);
      await clickItemByName(page, 'my-secret');
      await expect(page.locator('.tag-item')).toHaveCount(2);
    });

    test('should allow adding tag to secret with existing tags', async ({ page }) => {
      await setupWailsMocks(page, createMultiTagState());
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await waitForItemList(page);
      await clickItemByName(page, 'my-secret');
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').waitFor();
      await page.locator('#tag-key').fill('new-tag');
      await page.locator('#tag-value').fill('new-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('No Tags', () => {
    test('should handle secret with no tags', async ({ page }) => {
      await setupWailsMocks(page, createNoTagsState());
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await waitForItemList(page);
      await clickItemByName(page, 'my-secret');
      await expect(page.locator('.tag-item')).toHaveCount(0);
    });

    test('should show add tag button even when no tags exist', async ({ page }) => {
      await setupWailsMocks(page, createNoTagsState());
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await waitForItemList(page);
      await clickItemByName(page, 'my-secret');
      await expect(page.getByRole('button', { name: '+ Add' })).toBeVisible();
    });
  });

  test.describe('JSON vs Plaintext Values', () => {
    test('should handle secret with JSON value', async ({ page }) => {
      const jsonSecret: Partial<MockState> = {
        secrets: [createSecret('json-secret', '{"database": {"host": "localhost", "port": 5432}}')],
      };
      await setupWailsMocks(page, jsonSecret);
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await waitForItemList(page);
      await clickItemByName(page, 'json-secret');
      await expect(page.locator('.detail-panel')).toBeVisible();
    });

    test('should handle secret with complex nested JSON', async ({ page }) => {
      const complexJson: Partial<MockState> = {
        secrets: [createSecret('complex-secret', '{"a": [1, 2, {"b": "c"}], "d": null}')],
      };
      await setupWailsMocks(page, complexJson);
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await waitForItemList(page);
      await clickItemByName(page, 'complex-secret');
      await expect(page.locator('.detail-panel')).toBeVisible();
    });

    test('should handle secret with plaintext value', async ({ page }) => {
      const plaintext: Partial<MockState> = {
        secrets: [createSecret('plaintext-secret', 'just-a-password-123!')],
      };
      await setupWailsMocks(page, plaintext);
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await waitForItemList(page);
      await clickItemByName(page, 'plaintext-secret');
      await expect(page.locator('.detail-panel')).toBeVisible();
    });
  });

  test.describe('Special Characters', () => {
    test('should handle secret name with special characters', async ({ page }) => {
      const specialName: Partial<MockState> = {
        secrets: [createSecret('my-secret_v2.0-test', 'value')],
      };
      await setupWailsMocks(page, specialName);
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await waitForItemList(page);
      await clickItemByName(page, 'my-secret_v2.0-test');
      await expect(page.locator('.detail-panel')).toBeVisible();
    });

    test('should handle secret value with special characters', async ({ page }) => {
      const specialValue: Partial<MockState> = {
        secrets: [createSecret('special-secret', 'value with <html> & "quotes" and \'apostrophes\'')],
      };
      await setupWailsMocks(page, specialValue);
      await page.goto('/');
      await navigateTo(page, 'Secrets');
      await waitForItemList(page);
      await clickItemByName(page, 'special-secret');
      await expect(page.locator('.detail-panel')).toBeVisible();
    });
  });
});
