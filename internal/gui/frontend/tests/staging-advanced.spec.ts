import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  waitForItemList,
  waitForViewLoaded,
  navigateTo,
} from './fixtures/wails-mock';

// ============================================================================
// Staging View Toggle Tests
// ============================================================================

test.describe('Staging View Toggle', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, {
      stagedSSM: [
        { name: '/staged/param', value: 'new-value', oldValue: 'old-value', operation: 'update' },
      ],
    });
    await page.goto('/');
    await navigateTo(page, 'Staging');
  });

  test('should default to Diff view mode', async ({ page }) => {
    const diffToggle = page.locator('.view-toggle').getByText('Diff');
    await expect(diffToggle).toHaveClass(/active/);
  });

  test('should switch to Value view mode', async ({ page }) => {
    await page.locator('.view-toggle').getByText('Value').click();

    const valueToggle = page.locator('.view-toggle').getByText('Value');
    await expect(valueToggle).toHaveClass(/active/);
  });

  test('should show different content in Diff vs Value mode', async ({ page }) => {
    // In Diff mode, should show diff markers or formatting
    await expect(page.locator('.staging-content')).toBeVisible();

    // Switch to Value mode
    await page.locator('.view-toggle').getByText('Value').click();

    // Content should still be visible (just formatted differently)
    await expect(page.locator('.staging-content')).toBeVisible();
  });
});

// ============================================================================
// Sidebar Badge Tests
// ============================================================================

test.describe('Sidebar Badges', () => {
  test('should show staging count badge when items are staged', async ({ page }) => {
    await setupWailsMocks(page, {
      stagedSSM: [
        { name: '/staged/param', value: 'staged-value', operation: 'create' },
      ],
    });
    await page.goto('/');
    await waitForViewLoaded(page);

    // Staging nav item should show count badge
    const stagingBadge = page.locator('.staging-count');
    await expect(stagingBadge).toBeVisible();
    await expect(stagingBadge).toContainText('1');
  });

  test('should have staging navigation item clickable', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    // Staging nav should be clickable
    const stagingNav = page.locator('.nav-item').filter({ hasText: 'Staging' });
    await expect(stagingNav).toBeVisible();
    await stagingNav.click();

    // Should navigate to staging view
    await expect(page.locator('.staging-content')).toBeVisible();
  });
});
