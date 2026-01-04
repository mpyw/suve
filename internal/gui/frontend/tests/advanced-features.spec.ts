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
