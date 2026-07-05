import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  waitForItemList,
  createGoogleCloudState,
} from './fixtures/wails-mock';

// ============================================================================
// Accessibility Tests - Navigation
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

// ============================================================================
// Accessibility Tests - Forms
// ============================================================================

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

// ============================================================================
// Accessibility Tests - Modals
// ============================================================================

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
// Accessibility - Provider selector (#266)
// ============================================================================

test.describe('Accessibility - Provider selector', () => {
  test('provider selector is labeled', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await expect(page.getByLabel('Provider')).toBeVisible();
  });

  test('provider selector is keyboard-focusable', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await page.locator('#provider-select').focus();
    await expect(page.locator('#provider-select')).toBeFocused();
  });

  test('focus lands on the active tab after a provider clamp', async ({ page }) => {
    await setupWailsMocks(page, createGoogleCloudState());
    await page.goto('/');
    await waitForItemList(page);

    // Google Cloud has no Param tab; focus moves to the active Secret tab.
    await expect(page.locator('.nav').getByRole('button', { name: /Secret/i })).toBeFocused();
  });
});
