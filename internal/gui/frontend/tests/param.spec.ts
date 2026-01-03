import { test, expect } from '@playwright/test';
import { setupWailsMocks, defaultMockState, type MockState } from './fixtures/wails-mock';

test.describe('Parameter CRUD Operations', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    // Wait for initial load
    await page.waitForSelector('.item-list');
  });

  test.describe('List and View', () => {
    test('should display parameter list', async ({ page }) => {
      await expect(page.locator('.item-name.param')).toHaveCount(3);
      await expect(page.locator('.item-name.param').first()).toContainText('/app/config');
    });

    test('should show parameter details when clicked', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await expect(page.locator('.detail-panel')).toBeVisible();
      await expect(page.locator('.detail-title')).toContainText('/app/config');
    });

    test('should display parameter metadata', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await expect(page.locator('.meta-label').filter({ hasText: 'Version' })).toBeVisible();
      await expect(page.locator('.meta-label').filter({ hasText: 'Type' })).toBeVisible();
    });

    test('should close detail panel when close button clicked', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await expect(page.locator('.detail-panel')).toBeVisible();
      await page.locator('.btn-close').click();
      await expect(page.locator('.detail-panel')).not.toBeVisible();
    });

    test('should display existing tags for parameter', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await expect(page.locator('.tag-item')).toBeVisible();
      await expect(page.locator('.tag-key')).toContainText('env');
      await expect(page.locator('.tag-value')).toContainText('production');
    });
  });

  test.describe('Create Parameter', () => {
    test('should open create modal when "+ New" clicked', async ({ page }) => {
      await page.getByRole('button', { name: '+ New' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('New Parameter')).toBeVisible();
    });

    test('should create parameter in staged mode by default', async ({ page }) => {
      await page.getByRole('button', { name: '+ New' }).click();
      await page.locator('#param-name').fill('/app/new-param');
      await page.locator('#param-value').fill('new-value');
      await page.getByRole('button', { name: 'Stage' }).click();
      // Modal should close on success
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should create parameter immediately when immediate mode checked', async ({ page }) => {
      await page.getByRole('button', { name: '+ New' }).click();
      await page.locator('#param-name').fill('/app/new-param');
      await page.locator('#param-value').fill('new-value');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should show error if name is empty', async ({ page }) => {
      await page.getByRole('button', { name: '+ New' }).click();
      await page.locator('#param-value').fill('some-value');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-error')).toContainText('Name and value are required');
    });

    test('should cancel create modal', async ({ page }) => {
      await page.getByRole('button', { name: '+ New' }).click();
      await page.getByRole('button', { name: 'Cancel' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Edit Parameter', () => {
    test('should open edit modal with current value', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Edit' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Edit Parameter')).toBeVisible();
      // Name field should be disabled and have current value
      await expect(page.locator('#param-name')).toBeDisabled();
      await expect(page.locator('#param-name')).toHaveValue('/app/config');
    });

    test('should stage edit by default', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Edit' }).click();
      await page.locator('#param-value').fill('updated-value');
      await page.getByRole('button', { name: 'Stage' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should apply edit immediately when immediate mode checked', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Edit' }).click();
      await page.locator('#param-value').fill('updated-value');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Delete Parameter', () => {
    test('should open delete confirmation modal', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Delete' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Delete Parameter')).toBeVisible();
      await expect(page.locator('.delete-target')).toContainText('/app/config');
    });

    test('should stage delete by default', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Delete' }).click();
      await page.getByRole('button', { name: 'Stage Delete' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should delete immediately when immediate mode checked', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.locator('.detail-actions').getByRole('button', { name: 'Delete' }).click();
      await page.locator('.immediate-checkbox input').check();
      await page.locator('.modal-content').getByRole('button', { name: 'Delete' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should cancel delete modal', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: 'Delete' }).click();
      await page.getByRole('button', { name: 'Cancel' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Tag Operations', () => {
    test('should open add tag modal', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: '+ Add' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Add Tag')).toBeVisible();
    });

    test('should add tag in staged mode', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').fill('new-tag');
      await page.locator('#tag-value').fill('tag-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should add tag immediately when immediate mode checked', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-key').fill('new-tag');
      await page.locator('#tag-value').fill('tag-value');
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Add Tag' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should show error if tag key is empty', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.getByRole('button', { name: '+ Add' }).click();
      await page.locator('#tag-value').fill('some-value');
      await page.getByRole('button', { name: 'Stage Tag' }).click();
      await expect(page.locator('.modal-error')).toContainText('Key is required');
    });

    test('should open remove tag modal', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.locator('.btn-tag-remove').click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await expect(page.getByText('Remove Tag')).toBeVisible();
    });

    test('should remove tag in staged mode', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.locator('.btn-tag-remove').click();
      await page.getByRole('button', { name: 'Stage Remove' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should remove tag immediately when immediate mode checked', async ({ page }) => {
      await page.locator('.item-button').first().click();
      await page.locator('.btn-tag-remove').click();
      await page.locator('.immediate-checkbox input').check();
      await page.getByRole('button', { name: 'Remove' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Value Masking', () => {
    test('should mask SecureString value by default', async ({ page }) => {
      // Click on a SecureString parameter
      await page.locator('.item-button').nth(1).click(); // /app/database/url is SecureString
      await expect(page.locator('.value-display.masked')).toBeVisible();
    });

    test('should toggle value visibility', async ({ page }) => {
      await page.locator('.item-button').nth(1).click();
      // Click show button
      await page.locator('.btn-toggle').click();
      await expect(page.locator('.value-display:not(.masked)')).toBeVisible();
      // Click hide button
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

    test('should have recursive checkbox', async ({ page }) => {
      await expect(page.getByLabel('Recursive')).toBeVisible();
    });

    test('should have show values checkbox', async ({ page }) => {
      await expect(page.getByLabel('Show Values')).toBeVisible();
    });
  });
});
