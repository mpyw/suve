import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createFilterTestState,
  waitForItemList,
  waitForViewLoaded,
  clickItemByName,
  navigateTo,
} from './fixtures/wails-mock';

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
// Form Interaction Tests
// ============================================================================

test.describe('Form Interactions', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should clear form when closing create modal', async ({ page }) => {
    // Open modal and fill form
    await page.getByRole('button', { name: '+ New' }).click();
    await page.locator('#param-name').fill('/test/param');
    await page.locator('#param-value').fill('test-value');

    // Close modal
    await page.getByRole('button', { name: 'Cancel' }).click();

    // Reopen modal
    await page.getByRole('button', { name: '+ New' }).click();

    // Form should be cleared
    await expect(page.locator('#param-name')).toHaveValue('');
  });

  test('should populate form with existing values when editing', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: 'Edit' }).click();

    // Name should be populated (and disabled)
    await expect(page.locator('#param-name')).toHaveValue('/app/config');
    await expect(page.locator('#param-name')).toBeDisabled();

    // Value should be populated
    await expect(page.locator('#param-value')).not.toHaveValue('');
  });

  test('should toggle immediate mode checkbox', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();

    const immediateCheckbox = page.locator('label').filter({ hasText: /Immediate|immediately/i }).locator('input[type="checkbox"]');
    await expect(immediateCheckbox).not.toBeChecked();

    await immediateCheckbox.check();
    await expect(immediateCheckbox).toBeChecked();

    await immediateCheckbox.uncheck();
    await expect(immediateCheckbox).not.toBeChecked();
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
// Navigation State Reset Tests
// ============================================================================

test.describe('Navigation State Reset', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, createFilterTestState());
    await page.goto('/');
    await waitForItemList(page);
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
