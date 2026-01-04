import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  waitForItemList,
  waitForViewLoaded,
  clickItemByName,
} from './fixtures/wails-mock';

// ============================================================================
// Viewport Tests - Mobile
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

// ============================================================================
// Viewport Tests - Tablet
// ============================================================================

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

// ============================================================================
// Viewport Tests - Large Desktop
// ============================================================================

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
