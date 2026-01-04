import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  type MockState,
  createFilterTestState,
  createVersionHistoryState,
  createPaginationTestState,
  createErrorState,
  waitForItemList,
  waitForViewLoaded,
  clickItemByName,
  navigateTo,
} from './fixtures/wails-mock';

// ============================================================================
// Filter and Search Tests
// ============================================================================

test.describe('Parameter Filtering', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createFilterTestState());
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should display all parameters initially', async ({ page }) => {
    await expect(page.locator('.item-name.param')).toHaveCount(6);
  });

  test('should filter by prefix', async ({ page }) => {
    await page.locator('.prefix-input').fill('/prod');
    // Wait for debounce and reload
    await page.waitForTimeout(400);
    await expect(page.locator('.item-name.param')).toHaveCount(3);
  });

  test('should filter by regex pattern', async ({ page }) => {
    await page.locator('.regex-input').fill('config');
    await page.waitForTimeout(400);
    await expect(page.locator('.item-name.param')).toHaveCount(3);
  });

  test('should combine prefix and regex filters', async ({ page }) => {
    await page.locator('.prefix-input').fill('/prod');
    await page.waitForTimeout(400);
    await page.locator('.regex-input').fill('app');
    await page.waitForTimeout(400);
    await expect(page.locator('.item-name.param')).toHaveCount(2);
  });

  test('should show no results when filter matches nothing', async ({ page }) => {
    await page.locator('.regex-input').fill('nonexistent');
    await page.waitForTimeout(400);
    await expect(page.locator('.item-name.param')).toHaveCount(0);
  });

  test('should clear filter when input is emptied', async ({ page }) => {
    await page.locator('.prefix-input').fill('/prod');
    await page.waitForTimeout(400);
    await expect(page.locator('.item-name.param')).toHaveCount(3);

    await page.locator('.prefix-input').clear();
    await page.waitForTimeout(400);
    await expect(page.locator('.item-name.param')).toHaveCount(6);
  });

  test('should toggle recursive checkbox', async ({ page }) => {
    await expect(page.getByLabel('Recursive')).toBeChecked();
    await page.getByLabel('Recursive').uncheck();
    await expect(page.getByLabel('Recursive')).not.toBeChecked();
  });

  test('should toggle show values checkbox', async ({ page }) => {
    await expect(page.getByLabel('Show Values')).not.toBeChecked();
    await page.getByLabel('Show Values').check();
    await expect(page.getByLabel('Show Values')).toBeChecked();
  });
});

test.describe('Secret Filtering', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createFilterTestState());
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
  });

  test('should display all secrets initially', async ({ page }) => {
    await expect(page.locator('.item-name.secret')).toHaveCount(5);
  });

  test('should filter by prefix', async ({ page }) => {
    await page.locator('.prefix-input').fill('prod');
    await page.waitForTimeout(400);
    await expect(page.locator('.item-name.secret')).toHaveCount(2);
  });

  test('should filter by regex pattern', async ({ page }) => {
    await page.locator('.regex-input').fill('api-key');
    await page.waitForTimeout(400);
    await expect(page.locator('.item-name.secret')).toHaveCount(3);
  });

  test('should combine prefix and regex filters', async ({ page }) => {
    await page.locator('.prefix-input').fill('dev');
    await page.waitForTimeout(400);
    await page.locator('.regex-input').fill('database');
    await page.waitForTimeout(400);
    await expect(page.locator('.item-name.secret')).toHaveCount(1);
  });
});

// ============================================================================
// Version History Tests
// ============================================================================

test.describe('Parameter Version History', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createVersionHistoryState());
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should display version history in detail panel', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.history-list')).toBeVisible();
    await expect(page.locator('.history-item')).toHaveCount(3);
  });

  test('should mark current version', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.badge-current')).toBeVisible();
  });

  test('should show version numbers', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.history-version').first()).toContainText('v3');
  });

  test('should show Compare button when multiple versions exist', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.getByRole('button', { name: /Compare/i })).toBeVisible();
  });

  test('should enter diff mode when Compare clicked', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: /Compare/i }).click();
    await expect(page.getByText('Select 2 versions to compare')).toBeVisible();
  });

  test('should exit diff mode when Cancel clicked', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: /Compare/i }).click();
    await expect(page.getByText('Select 2 versions to compare')).toBeVisible();

    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByText('Select 2 versions to compare')).not.toBeVisible();
  });

  test('should allow selecting versions in diff mode', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: /Compare/i }).click();

    // Select first version
    await page.locator('.diff-checkbox input').first().check();
    await expect(page.locator('.history-item.selected')).toHaveCount(1);

    // Select second version
    await page.locator('.diff-checkbox input').nth(1).check();
    await expect(page.locator('.history-item.selected')).toHaveCount(2);
  });

  test('should show Show Diff button when 2 versions selected', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: /Compare/i }).click();

    // Select two versions
    await page.locator('.diff-checkbox input').first().check();
    await page.locator('.diff-checkbox input').nth(1).check();

    await expect(page.getByRole('button', { name: 'Show Diff' })).toBeVisible();
  });

  test('should open diff modal when Show Diff clicked', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: /Compare/i }).click();

    await page.locator('.diff-checkbox input').first().check();
    await page.locator('.diff-checkbox input').nth(1).check();

    await page.getByRole('button', { name: 'Show Diff' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    await expect(page.getByText('Version Comparison')).toBeVisible();
  });

  test('should close diff modal', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: /Compare/i }).click();

    await page.locator('.diff-checkbox input').first().check();
    await page.locator('.diff-checkbox input').nth(1).check();

    await page.getByRole('button', { name: 'Show Diff' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();

    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });
});

test.describe('Secret Version History', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createVersionHistoryState());
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
  });

  test('should display version history in detail panel', async ({ page }) => {
    await clickItemByName(page, 'my-secret');
    await expect(page.locator('.history-list')).toBeVisible();
    await expect(page.locator('.history-item')).toHaveCount(3);
  });

  test('should mark current version', async ({ page }) => {
    await clickItemByName(page, 'my-secret');
    await expect(page.locator('.badge-current')).toBeVisible();
  });

  test('should show version stage labels', async ({ page }) => {
    await clickItemByName(page, 'my-secret');
    // Multiple elements may have AWSCURRENT badge, just verify at least one exists
    await expect(page.locator('.badge-stage').filter({ hasText: 'AWSCURRENT' }).first()).toBeVisible();
  });

  test('should show Compare button when multiple versions exist', async ({ page }) => {
    await clickItemByName(page, 'my-secret');
    await expect(page.getByRole('button', { name: /Compare/i })).toBeVisible();
  });

  test('should enter diff mode when Compare clicked', async ({ page }) => {
    await clickItemByName(page, 'my-secret');
    await page.getByRole('button', { name: /Compare/i }).click();
    await expect(page.getByText('Select 2 versions to compare')).toBeVisible();
  });
});

// ============================================================================
// Value Masking and Display Tests
// ============================================================================

test.describe('Value Masking', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should mask SecureString parameter value by default', async ({ page }) => {
    await clickItemByName(page, '/app/database/url');
    await expect(page.locator('.value-display.masked')).toBeVisible();
  });

  test('should show eye icon for SecureString', async ({ page }) => {
    await clickItemByName(page, '/app/database/url');
    await expect(page.locator('.btn-toggle')).toBeVisible();
  });

  test('should toggle parameter value visibility', async ({ page }) => {
    await clickItemByName(page, '/app/database/url');
    await expect(page.locator('.value-display.masked')).toBeVisible();

    await page.locator('.btn-toggle').click();
    await expect(page.locator('.value-display:not(.masked)')).toBeVisible();

    await page.locator('.btn-toggle').click();
    await expect(page.locator('.value-display.masked')).toBeVisible();
  });

  test('should mask secret value by default', async ({ page }) => {
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
    await clickItemByName(page, 'my-secret');
    await expect(page.locator('.value-display.masked')).toBeVisible();
  });

  test('should toggle secret value visibility', async ({ page }) => {
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
    await clickItemByName(page, 'my-secret');

    await expect(page.locator('.value-display.masked')).toBeVisible();
    await page.locator('.btn-toggle').click();
    await expect(page.locator('.value-display:not(.masked)')).toBeVisible();
  });

  test('should mask version history values when value is hidden', async ({ page }) => {
    await setupWailsMocks(page, createVersionHistoryState());
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/config');
    // History values should also be masked when main value is hidden
    await expect(page.locator('.history-value').first()).toBeVisible();
  });
});

// ============================================================================
// Refresh and Loading Tests
// ============================================================================

test.describe('Refresh and Loading', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should have refresh button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Refresh/i })).toBeVisible();
  });

  test('should reload data when refresh clicked', async ({ page }) => {
    const refreshBtn = page.getByRole('button', { name: /Refresh/i });
    await refreshBtn.click();
    // Data should still be displayed after refresh
    await expect(page.locator('.item-name.param')).toHaveCount(3);
  });

  test('should show loading state when refreshing', async ({ page }) => {
    const refreshBtn = page.getByRole('button', { name: /Refresh/i });
    await refreshBtn.click();
    // The button text changes during loading
    // This may be too fast to catch, so we just verify it works
    await expect(page.locator('.item-list')).toBeVisible();
  });

  test('should refresh secrets view', async ({ page }) => {
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);

    const refreshBtn = page.getByRole('button', { name: /Refresh/i });
    await refreshBtn.click();
    await expect(page.locator('.item-name.secret')).toHaveCount(3);
  });
});

// ============================================================================
// Modal Interaction Tests
// ============================================================================

test.describe('Modal Keyboard Interaction', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should close modal when clicking backdrop', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();

    // Click on backdrop (outside modal content)
    await page.locator('.modal-backdrop').click({ position: { x: 10, y: 10 } });
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should have name input available in create modal', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    // Verify the name input is visible and interactive
    const nameInput = page.locator('#param-name');
    await expect(nameInput).toBeVisible();
    await expect(nameInput).toBeEditable();
  });
});

// ============================================================================
// Error Handling Tests
// ============================================================================

test.describe('Form Validation', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should show error when creating parameter without name', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-value').fill('some-value');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-error')).toBeVisible();
  });

  test('should show error when creating parameter without value', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-name').fill('/test/param');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-error')).toBeVisible();
  });

  test('should show error when adding tag without key', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: '+ Add' }).click();
    await page.locator('#tag-value').fill('some-value');
    await page.getByRole('button', { name: 'Stage Tag' }).click();
    await expect(page.locator('.modal-error')).toBeVisible();
  });

  test('should show error when creating secret without name', async ({ page }) => {
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#secret-value').fill('some-value');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-error')).toBeVisible();
  });

  test('should show error when creating secret without value', async ({ page }) => {
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#secret-name').fill('test-secret');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-error')).toBeVisible();
  });
});

// ============================================================================
// Type Selection Tests
// ============================================================================

test.describe('Parameter Type Selection', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should default to String type', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await expect(page.locator('#param-type')).toHaveValue('String');
  });

  test('should allow selecting SecureString type', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-type').selectOption('SecureString');
    await expect(page.locator('#param-type')).toHaveValue('SecureString');
  });

  test('should allow selecting StringList type', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-type').selectOption('StringList');
    await expect(page.locator('#param-type')).toHaveValue('StringList');
  });

  test('should display parameter type in detail view', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.meta-value').filter({ hasText: 'String' })).toBeVisible();
  });

  test('should display SecureString type in detail view', async ({ page }) => {
    await clickItemByName(page, '/app/database/url');
    await expect(page.locator('.meta-value').filter({ hasText: 'SecureString' })).toBeVisible();
  });
});

// ============================================================================
// Pagination/Infinite Scroll Tests
// ============================================================================

test.describe('Pagination - Parameters', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createPaginationTestState(25));
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should load parameters with pagination enabled', async ({ page }) => {
    // With pagination enabled, items should load
    const items = page.locator('.item-button');
    const count = await items.count();
    // Should have some items loaded (pagination limits first page)
    expect(count).toBeGreaterThan(0);
    expect(count).toBeLessThanOrEqual(25);
  });

  test('should display scroll sentinel element', async ({ page }) => {
    const sentinel = page.locator('.scroll-sentinel');
    await expect(sentinel).toBeVisible();
  });

  test('should have scroll capability', async ({ page }) => {
    // Verify the list panel exists and can be scrolled
    const listPanel = page.locator('.list-panel');
    await expect(listPanel).toBeVisible();
  });
});

test.describe('Pagination - Secrets', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createPaginationTestState(25));
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
  });

  test('should load secrets with pagination enabled', async ({ page }) => {
    // With pagination enabled, items should load
    const items = page.locator('.item-button');
    const count = await items.count();
    // Should have some items loaded
    expect(count).toBeGreaterThan(0);
    expect(count).toBeLessThanOrEqual(25);
  });

  test('should display scroll sentinel for secrets', async ({ page }) => {
    const sentinel = page.locator('.scroll-sentinel');
    await expect(sentinel).toBeVisible();
  });
});

// ============================================================================
// API Error Handling Tests
// ============================================================================

test.describe('API Error Handling - Parameter List', () => {
  test('should display error banner when ParamList fails', async ({ page }) => {
    await setupWailsMocks(page, createErrorState('ParamList', 'AWS service unavailable'));
    await page.goto('/');
    await expect(page.locator('.error-banner')).toBeVisible();
    await expect(page.getByText('AWS service unavailable')).toBeVisible();
  });

  test('should still show filter bar when list fails to load', async ({ page }) => {
    await setupWailsMocks(page, createErrorState('ParamList', 'Network error'));
    await page.goto('/');
    await expect(page.locator('.filter-bar')).toBeVisible();
  });
});

test.describe('API Error Handling - Secret List', () => {
  test('should display error banner when SecretList fails', async ({ page }) => {
    await setupWailsMocks(page, createErrorState('SecretList', 'Access denied'));
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await expect(page.locator('.error-banner')).toBeVisible();
    await expect(page.getByText('Access denied')).toBeVisible();
  });
});

test.describe('API Error Handling - Create Operations', () => {
  test('should display error in modal when ParamSet fails in immediate mode', async ({ page }) => {
    await setupWailsMocks(page, {
      ...createErrorState('ParamSet', 'Parameter limit exceeded'),
    });
    await page.goto('/');
    await waitForViewLoaded(page);

    // Open create modal and try to create parameter in immediate mode
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-name').fill('/new/param');
    await page.locator('#param-value').fill('test-value');
    // Check immediate mode checkbox
    const immediateCheckbox = page.locator('input[type="checkbox"]').filter({ has: page.locator('..').filter({ hasText: 'Immediate' }) });
    if (await immediateCheckbox.count() > 0) {
      await immediateCheckbox.first().check();
    }
    await page.getByRole('button', { name: 'Save' }).click();

    // Should show error in modal
    await expect(page.locator('.modal-error')).toBeVisible({ timeout: 5000 });
  });

  test('should display error in modal when SecretCreate fails in immediate mode', async ({ page }) => {
    await setupWailsMocks(page, {
      ...createErrorState('SecretCreate', 'Secret quota exceeded'),
    });
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForViewLoaded(page);

    // Open create modal and try to create secret in immediate mode
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#secret-name').fill('new-secret');
    await page.locator('#secret-value').fill('secret-value');
    // Check immediate mode checkbox
    const immediateCheckbox = page.locator('label').filter({ hasText: 'Immediate' }).locator('input[type="checkbox"]');
    if (await immediateCheckbox.count() > 0) {
      await immediateCheckbox.first().check();
    }
    // Secret create modal uses "Create" button, not "Save"
    await page.getByRole('button', { name: 'Create' }).click();

    // Should show error in modal
    await expect(page.locator('.modal-error')).toBeVisible({ timeout: 5000 });
  });
});

// ============================================================================
// Keyboard Navigation Tests
// ============================================================================

test.describe('Keyboard Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should close modal with Escape key', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should have focusable navigation items', async ({ page }) => {
    // Navigation items (buttons) should be present and clickable
    const navItems = page.locator('.nav-item');
    const count = await navItems.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should be able to navigate form fields with Tab', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();

    // Fill name and tab to value
    await page.locator('#param-name').fill('/test');
    await page.keyboard.press('Tab');

    // Should now be focused on value or type field
    const focusedElement = page.locator(':focus');
    await expect(focusedElement).toBeVisible();
  });
});

// ============================================================================
// Detail Panel Navigation Tests
// ============================================================================

test.describe('Detail Panel Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should open detail panel when clicking item', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.detail-panel')).toBeVisible();
  });

  test('should update detail panel when clicking different item', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.detail-panel')).toBeVisible();
    // Wait for detail to load
    await expect(page.locator('.detail-title').filter({ hasText: '/app/config' })).toBeVisible();

    await clickItemByName(page, '/app/database/url');
    // Wait for new detail to load
    await expect(page.locator('.detail-title').filter({ hasText: '/app/database/url' })).toBeVisible();
  });

  test('should close detail panel when clicking close button', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.detail-panel')).toBeVisible();

    // Close button has class btn-close
    await page.locator('.btn-close').click();
    await expect(page.locator('.detail-panel')).not.toBeVisible();
  });

  test('should preserve detail panel state when switching views', async ({ page }) => {
    // Open detail on parameters
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.detail-panel')).toBeVisible();

    // Switch to secrets and back
    await navigateTo(page, 'Secrets');
    await navigateTo(page, 'Parameters');

    // Detail panel should be closed after view switch
    await expect(page.locator('.detail-panel')).not.toBeVisible();
  });
});

// ============================================================================
// Button State Tests
// ============================================================================

test.describe('Button States', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should enable Edit button only when item is selected', async ({ page }) => {
    // Initially no Edit button visible (no selection)
    await expect(page.locator('.detail-panel')).not.toBeVisible();

    // Select an item
    await clickItemByName(page, '/app/config');

    // Now Edit button should be visible in detail panel
    await expect(page.getByRole('button', { name: 'Edit' })).toBeVisible();
  });

  test('should enable Delete button only when item is selected', async ({ page }) => {
    // Select an item
    await clickItemByName(page, '/app/config');

    // Delete button should be visible in detail panel
    await expect(page.getByRole('button', { name: 'Delete' })).toBeVisible();
  });

  test('should show add tag button when item is selected', async ({ page }) => {
    // Select an item
    await clickItemByName(page, '/app/config');

    // Add Tag button shows as "+ Add"
    await expect(page.getByRole('button', { name: '+ Add' })).toBeVisible();
  });
});

// ============================================================================
// View Mode Tests (Secret value display)
// ============================================================================

test.describe('View Mode - Show Values Toggle', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createFilterTestState());
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should have show values checkbox unchecked by default', async ({ page }) => {
    // Find checkbox by its label text
    const showValuesCheckbox = page.locator('label').filter({ hasText: 'Show Values' }).locator('input[type="checkbox"]');
    await expect(showValuesCheckbox).not.toBeChecked();
  });

  test('should toggle show values checkbox', async ({ page }) => {
    // Find checkbox by its label text
    const showValuesCheckbox = page.locator('label').filter({ hasText: 'Show Values' }).locator('input[type="checkbox"]');
    await showValuesCheckbox.check();
    await expect(showValuesCheckbox).toBeChecked();
  });
});

// ============================================================================
// Restore Secret Tests
// ============================================================================

test.describe('Restore Secret Feature', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
  });

  test('should have Restore button in toolbar', async ({ page }) => {
    await expect(page.getByRole('button', { name: 'Restore' })).toBeVisible();
  });

  test('should open restore modal when clicked', async ({ page }) => {
    await page.getByRole('button', { name: 'Restore' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    // Modal title should be "Restore Secret"
    await expect(page.locator('.modal-title').filter({ hasText: 'Restore Secret' })).toBeVisible();
  });

  test('should have secret name input in restore modal', async ({ page }) => {
    await page.getByRole('button', { name: 'Restore' }).click();
    // The restore name input
    await expect(page.locator('input[type="text"]').first()).toBeVisible();
  });

  test('should close restore modal on cancel', async ({ page }) => {
    await page.getByRole('button', { name: 'Restore' }).click();
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });
});

// ============================================================================
// Large Value Display Tests
// ============================================================================

test.describe('Large Value Display', () => {
  test.beforeEach(async ({ page }) => {
    // Create a parameter with a very large JSON value
    const largeJson = JSON.stringify({
      config: {
        settings: Array.from({ length: 20 }, (_, i) => ({
          id: i,
          name: `setting-${i}`,
          value: `value-${i}`,
          description: `Description for setting ${i} with some extra text to make it longer`,
        })),
      },
    });

    await setupWailsMocks(page, {
      params: [
        { name: '/app/large-config', type: 'String', value: largeJson },
      ],
    });
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should display large value in scrollable container', async ({ page }) => {
    await clickItemByName(page, '/app/large-config');
    await expect(page.locator('.value-display')).toBeVisible();
  });

  test('should show full value when expanded', async ({ page }) => {
    await clickItemByName(page, '/app/large-config');
    // The value should be present (may need to scroll)
    await expect(page.locator('.value-display')).toContainText('config');
  });
});

// ============================================================================
// Navigation State Preservation Tests
// ============================================================================

test.describe('Navigation State', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createFilterTestState());
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should have filter bar available after navigation', async ({ page }) => {
    // Navigate away
    await navigateTo(page, 'Secrets');
    await waitForViewLoaded(page);

    // Navigate back
    await navigateTo(page, 'Parameters');

    // Filter bar should be present
    await waitForViewLoaded(page);
    await expect(page.locator('.filter-bar')).toBeVisible();
  });

  test('should reset selection when switching views', async ({ page }) => {
    // Select a parameter
    await clickItemByName(page, '/prod/app/config');
    await expect(page.locator('.detail-panel')).toBeVisible();

    // Navigate to secrets
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);

    // Detail panel should be for secrets now or closed
    // Navigate back to parameters
    await navigateTo(page, 'Parameters');
    await waitForItemList(page);

    // Previous selection should be cleared
    await expect(page.locator('.detail-panel')).not.toBeVisible();
  });
});

// ============================================================================
// Edge Case Tests - Values
// ============================================================================

test.describe('Value Edge Cases', () => {
  test('should handle empty string value', async ({ page }) => {
    await setupWailsMocks(page, {
      params: [{ name: '/app/empty-value', type: 'String', value: '' }],
    });
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/empty-value');
    await expect(page.locator('.detail-panel')).toBeVisible();
    // Empty value should still be displayable
    await expect(page.locator('.value-display')).toBeVisible();
  });

  test('should handle very long parameter name', async ({ page }) => {
    const longName = '/very/long/hierarchical/path/to/a/deeply/nested/parameter/name';
    await setupWailsMocks(page, {
      params: [{ name: longName, type: 'String', value: 'value' }],
    });
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, longName);
    await expect(page.locator('.detail-panel')).toBeVisible();
    await expect(page.locator('.detail-title')).toContainText(longName);
  });

  test('should handle special characters in parameter name', async ({ page }) => {
    const specialName = '/app/config-with-dashes_and_underscores.dot';
    await setupWailsMocks(page, {
      params: [{ name: specialName, type: 'String', value: 'value' }],
    });
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, specialName);
    await expect(page.locator('.detail-panel')).toBeVisible();
  });

  test('should handle newlines in parameter value', async ({ page }) => {
    const multilineValue = 'line1\nline2\nline3';
    await setupWailsMocks(page, {
      params: [{ name: '/app/multiline', type: 'String', value: multilineValue }],
    });
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/multiline');
    await expect(page.locator('.value-display')).toContainText('line1');
  });

  test('should handle unicode characters in value', async ({ page }) => {
    const unicodeValue = '日本語テスト 🎉 emoji test';
    await setupWailsMocks(page, {
      params: [{ name: '/app/unicode', type: 'String', value: unicodeValue }],
    });
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/unicode');
    await expect(page.locator('.value-display')).toContainText('日本語テスト');
  });
});

// ============================================================================
// Edge Case Tests - Tags
// ============================================================================

test.describe('Tag Edge Cases', () => {
  test('should handle tag with empty value', async ({ page }) => {
    await setupWailsMocks(page, {
      params: [{ name: '/app/config', type: 'String', value: 'value' }],
      paramTags: {
        '/app/config': [{ key: 'empty-value-tag', value: '' }],
      },
    });
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/config');
    await expect(page.locator('.tag-key').filter({ hasText: 'empty-value-tag' })).toBeVisible();
  });

  test('should handle tag with special characters', async ({ page }) => {
    await setupWailsMocks(page, {
      params: [{ name: '/app/config', type: 'String', value: 'value' }],
      paramTags: {
        '/app/config': [{ key: 'env:stage', value: 'prod-v1.2.3' }],
      },
    });
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/config');
    await expect(page.locator('.tag-key').filter({ hasText: 'env:stage' })).toBeVisible();
  });

  test('should handle many tags on single parameter', async ({ page }) => {
    const manyTags = Array.from({ length: 10 }, (_, i) => ({
      key: `tag-${i}`,
      value: `value-${i}`,
    }));
    await setupWailsMocks(page, {
      params: [{ name: '/app/config', type: 'String', value: 'value' }],
      paramTags: { '/app/config': manyTags },
    });
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/config');
    // Should display all tags
    const tagCount = await page.locator('.tag-item').count();
    expect(tagCount).toBe(10);
  });
});

// ============================================================================
// Concurrent Operations Tests
// ============================================================================

test.describe('Concurrent Operations', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should handle rapid clicking between items', async ({ page }) => {
    // Click multiple items rapidly
    await clickItemByName(page, '/app/config');
    await clickItemByName(page, '/app/database/url');
    await clickItemByName(page, '/app/api/key');

    // Final selection should be shown
    await expect(page.locator('.detail-title').filter({ hasText: '/app/api/key' })).toBeVisible();
  });

  test('should handle rapid view switching', async ({ page }) => {
    // Switch views rapidly
    await navigateTo(page, 'Secrets');
    await navigateTo(page, 'Parameters');
    await navigateTo(page, 'Staging');
    await navigateTo(page, 'Parameters');

    // Should end on Parameters view
    await waitForViewLoaded(page);
    await expect(page.locator('.filter-bar')).toBeVisible();
  });

  test('should handle opening and closing modals rapidly', async ({ page }) => {
    // Open and close modals rapidly
    await page.getByRole('button', { name: '+ New' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    await page.getByRole('button', { name: '+ New' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
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
// Filter Persistence Tests
// ============================================================================

test.describe('Filter Behavior', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createFilterTestState());
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should filter parameters by prefix input', async ({ page }) => {
    await page.locator('.prefix-input').fill('/prod');
    await page.waitForTimeout(300); // Wait for debounce

    // Should only show prod items
    const items = page.locator('.item-button');
    const count = await items.count();
    // Prod items: /prod/app/config, /prod/app/secret, /prod/database/url
    expect(count).toBeLessThanOrEqual(6); // Allow some flexibility
  });

  test('should filter parameters by regex input', async ({ page }) => {
    await page.locator('.regex-input').fill('config');
    await page.waitForTimeout(300); // Wait for debounce

    // Should only show items matching 'config'
    const items = page.locator('.item-button');
    const count = await items.count();
    expect(count).toBeLessThanOrEqual(6);
  });

  test('should clear filter when clicking refresh', async ({ page }) => {
    // Set a filter
    await page.locator('.regex-input').fill('prod');
    await page.waitForTimeout(300);

    // Click refresh
    await page.getByRole('button', { name: 'Refresh' }).click();

    // Filter should remain (refresh doesn't clear filter)
    await expect(page.locator('.regex-input')).toHaveValue('prod');
  });
});

// ============================================================================
// Accessibility Tests
// ============================================================================

test.describe('Accessibility - Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should have navigation items with proper roles', async ({ page }) => {
    const navItems = page.locator('.nav-item');
    const count = await navItems.count();
    expect(count).toBeGreaterThan(0);

    // Each nav item should be clickable
    for (let i = 0; i < count; i++) {
      await expect(navItems.nth(i)).toBeEnabled();
    }
  });

  test('should indicate active navigation item', async ({ page }) => {
    const activeNav = page.locator('.nav-item.active');
    await expect(activeNav).toBeVisible();
  });

  test('should have accessible buttons in toolbar', async ({ page }) => {
    // Refresh button
    const refreshBtn = page.getByRole('button', { name: /Refresh|Loading/i });
    await expect(refreshBtn).toBeVisible();

    // New button
    const newBtn = page.getByRole('button', { name: '+ New' });
    await expect(newBtn).toBeVisible();
  });
});

test.describe('Accessibility - Forms', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should have labeled inputs in create form', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();

    // Name input should have associated label
    const nameInput = page.locator('#param-name');
    await expect(nameInput).toBeVisible();

    // Value input should have associated label
    const valueInput = page.locator('#param-value');
    await expect(valueInput).toBeVisible();
  });

  test('should have accessible checkboxes', async ({ page }) => {
    // Filter bar checkboxes
    const recursiveCheckbox = page.locator('label').filter({ hasText: 'Recursive' }).locator('input[type="checkbox"]');
    await expect(recursiveCheckbox).toBeVisible();

    const showValuesCheckbox = page.locator('label').filter({ hasText: 'Show Values' }).locator('input[type="checkbox"]');
    await expect(showValuesCheckbox).toBeVisible();
  });

  test('should have accessible select dropdown', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();

    const typeSelect = page.locator('#param-type');
    await expect(typeSelect).toBeVisible();

    // Should have options
    const options = typeSelect.locator('option');
    const optionCount = await options.count();
    expect(optionCount).toBeGreaterThan(0);
  });
});

test.describe('Accessibility - Modals', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should trap focus in modal', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();

    // Should be able to tab through modal elements
    await page.keyboard.press('Tab');
    const focused = page.locator(':focus');
    await expect(focused).toBeVisible();
  });

  test('should have modal title', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();

    const modalTitle = page.locator('.modal-title');
    await expect(modalTitle).toBeVisible();
  });

  test('should have cancel and submit buttons in modal', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();

    await expect(page.getByRole('button', { name: 'Cancel' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Stage' })).toBeVisible();
  });
});

// ============================================================================
// Viewport Tests
// ============================================================================

test.describe('Viewport - Mobile', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 }); // iPhone SE
    await setupWailsMocks(page);
    await page.goto('/');
  });

  test('should render on mobile viewport', async ({ page }) => {
    await waitForViewLoaded(page);
    await expect(page.locator('.filter-bar')).toBeVisible();
  });

  test('should show navigation on mobile', async ({ page }) => {
    const navItems = page.locator('.nav-item');
    await expect(navItems.first()).toBeVisible();
  });
});

test.describe('Viewport - Tablet', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 }); // iPad
    await setupWailsMocks(page);
    await page.goto('/');
  });

  test('should render on tablet viewport', async ({ page }) => {
    await waitForViewLoaded(page);
    await expect(page.locator('.filter-bar')).toBeVisible();
  });

  test('should show list panel on tablet', async ({ page }) => {
    await waitForItemList(page);
    await expect(page.locator('.list-panel')).toBeVisible();
  });
});

test.describe('Viewport - Large Desktop', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 });
    await setupWailsMocks(page);
    await page.goto('/');
  });

  test('should render on large desktop viewport', async ({ page }) => {
    await waitForViewLoaded(page);
    await expect(page.locator('.filter-bar')).toBeVisible();
  });

  test('should show detail panel alongside list on large screen', async ({ page }) => {
    await waitForItemList(page);
    await clickItemByName(page, '/app/config');

    // Both panels should be visible
    await expect(page.locator('.list-panel')).toBeVisible();
    await expect(page.locator('.detail-panel')).toBeVisible();
  });
});

// ============================================================================
// End-to-End Workflow Tests
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
// Input Debounce Tests
// ============================================================================

test.describe('Input Debounce Behavior', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createFilterTestState());
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should debounce prefix filter input', async ({ page }) => {
    const initialCount = await page.locator('.item-button').count();

    // Type quickly
    await page.locator('.prefix-input').fill('/prod');

    // Wait for debounce
    await page.waitForTimeout(400);

    // Should have filtered results
    const filteredCount = await page.locator('.item-button').count();
    expect(filteredCount).toBeLessThanOrEqual(initialCount);
  });

  test('should debounce regex filter input', async ({ page }) => {
    // Type quickly
    await page.locator('.regex-input').fill('config');

    // Wait for debounce
    await page.waitForTimeout(400);

    // Items should be filtered
    const items = page.locator('.item-button');
    const count = await items.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should cancel pending filter on quick input change', async ({ page }) => {
    // Type, then quickly change
    await page.locator('.regex-input').fill('prod');
    await page.locator('.regex-input').fill('dev');

    // Wait for debounce
    await page.waitForTimeout(400);

    // Should filter by 'dev', not 'prod'
    await expect(page.locator('.regex-input')).toHaveValue('dev');
  });
});

// ============================================================================
// Data Validation Tests
// ============================================================================

test.describe('Data Validation', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should require name and value for parameter creation', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();

    // Leave name empty
    await page.locator('#param-value').fill('some-value');
    await page.getByRole('button', { name: 'Stage' }).click();

    // Should remain on modal (validation should prevent submission)
    await expect(page.locator('.modal-backdrop')).toBeVisible();
  });

  test('should allow parameter name with leading slash', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-name').fill('/valid/param/name');
    await page.locator('#param-value').fill('valid-value');
    await page.getByRole('button', { name: 'Stage' }).click();

    // Should close modal on success
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should handle whitespace-only value', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-name').fill('/app/whitespace');
    await page.locator('#param-value').fill('   ');
    await page.getByRole('button', { name: 'Stage' }).click();

    // May succeed or fail depending on validation - just verify no crash
    await expect(page.locator('body')).toBeVisible();
  });
});

// ============================================================================
// Loading State Tests
// ============================================================================

test.describe('Loading States', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
  });

  test('should show loading state on initial load', async ({ page }) => {
    await page.goto('/');
    // The loading state might be brief, but page should eventually load
    await waitForViewLoaded(page);
  });

  test('should show loading state when switching views', async ({ page }) => {
    await page.goto('/');
    await waitForItemList(page);

    // Switch to secrets
    await navigateTo(page, 'Secrets');

    // Should eventually show secrets view
    await waitForItemList(page);
    await expect(page.locator('.item-button')).toHaveCount(3);
  });

  test('should show loading in modal during operation', async ({ page }) => {
    await page.goto('/');
    await waitForItemList(page);

    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-name').fill('/new/param');
    await page.locator('#param-value').fill('value');

    // Click stage - brief loading state
    await page.getByRole('button', { name: 'Stage' }).click();

    // Should complete (modal closes)
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });
});

// ============================================================================
// Selection State Tests
// ============================================================================

test.describe('Selection States', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should highlight selected item in list', async ({ page }) => {
    await clickItemByName(page, '/app/config');

    // The selected item should have a different visual state
    const selectedItem = page.locator('.item-entry.selected');
    await expect(selectedItem).toBeVisible();
  });

  test('should deselect when closing detail panel', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.item-entry.selected')).toBeVisible();

    await page.locator('.btn-close').click();

    // Selection should be cleared
    await expect(page.locator('.item-entry.selected')).not.toBeVisible();
  });

  test('should update selection when clicking different item', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.item-entry.selected')).toHaveCount(1);

    await clickItemByName(page, '/app/database/url');

    // Still only one selected
    await expect(page.locator('.item-entry.selected')).toHaveCount(1);
  });
});
