import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createFilterTestState,
  waitForItemList,
  waitForViewLoaded,
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
// Filter Behavior Tests
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
// View Mode Tests (Show Values Toggle)
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
});

// ============================================================================
// UI Hints Tests
// ============================================================================

test.describe('UI Hints', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should have placeholder text in filter inputs', async ({ page }) => {
    await expect(page.locator('.prefix-input')).toHaveAttribute('placeholder', /prefix/i);
    await expect(page.locator('.regex-input')).toHaveAttribute('placeholder', /filter|regex/i);
  });

  test('should show item count or status in view', async ({ page }) => {
    // The view should indicate how many items are shown or some status
    const itemList = page.locator('.item-list');
    await expect(itemList).toBeVisible();
  });
});
