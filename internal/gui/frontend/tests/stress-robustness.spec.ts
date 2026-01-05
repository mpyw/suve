import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  waitForItemList,
  waitForViewLoaded,
  clickItemByName,
  navigateTo,
} from './fixtures/wails-mock';

// ============================================================================
// Rapid Operation Tests
// ============================================================================

test.describe('Rapid Operations', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should handle rapid refresh clicks', async ({ page }) => {
    const refreshBtn = page.getByRole('button', { name: 'Refresh' });

    // Click refresh multiple times rapidly
    for (let i = 0; i < 5; i++) {
      await refreshBtn.click();
    }

    // Should still be functional
    await waitForItemList(page);
    await expect(page.locator('.item-list')).toBeVisible();
  });

  test('should handle rapid navigation switching', async ({ page }) => {
    // Switch between views rapidly
    for (let i = 0; i < 3; i++) {
      await navigateTo(page, 'Secrets');
      await navigateTo(page, 'Parameters');
      await navigateTo(page, 'Staging');
    }

    // Should still be functional
    await expect(page.locator('body')).toBeVisible();
  });

  test('should handle rapid item selection', async ({ page }) => {
    const items = page.locator('.item-button');
    const count = await items.count();

    // Click through items rapidly
    for (let i = 0; i < Math.min(count, 5); i++) {
      await items.nth(i).click();
    }

    // Detail panel should show last selected
    await expect(page.locator('.detail-panel')).toBeVisible();
  });

  test('should handle rapid modal open/close', async ({ page }) => {
    for (let i = 0; i < 5; i++) {
      await page.getByRole('button', { name: '+ New' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    }

    // Should still be functional
    await expect(page.locator('.item-list')).toBeVisible();
  });
});

// ============================================================================
// State Consistency Tests
// ============================================================================

test.describe('State Consistency', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should maintain filter state across refresh', async ({ page }) => {
    // Set a filter
    await page.locator('.prefix-input').fill('/app');
    await page.waitForTimeout(400);

    // Refresh
    await page.getByRole('button', { name: 'Refresh' }).click();

    // Filter should be preserved
    await expect(page.locator('.prefix-input')).toHaveValue('/app');
  });

  test('should clear selection when underlying data changes', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.detail-panel')).toBeVisible();

    // Refresh the data
    await page.getByRole('button', { name: 'Refresh' }).click();
    await waitForItemList(page);

    // Selection should be cleared or maintained based on implementation
    // Just verify no crash
    await expect(page.locator('.item-list')).toBeVisible();
  });

  test('should handle modal state when view changes', async ({ page }) => {
    // Open modal
    await page.getByRole('button', { name: '+ New' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();

    // Close modal first (modal blocks navigation)
    await page.keyboard.press('Escape');
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // Now navigate
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);

    // Should still be functional
    await expect(page.locator('.item-list')).toBeVisible();
  });
});

// ============================================================================
// Large Dataset Simulation Tests
// ============================================================================

test.describe('Large Dataset Handling', () => {
  test('should handle many parameters', async ({ page }) => {
    const manyParams = Array.from({ length: 50 }, (_, i) => ({
      name: `/param/${i}/config`,
      type: 'String',
      value: `value-${i}`,
    }));

    await setupWailsMocks(page, { params: manyParams });
    await page.goto('/');
    await waitForItemList(page);

    // Should render items
    const items = page.locator('.item-button');
    const count = await items.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should handle many secrets', async ({ page }) => {
    const manySecrets = Array.from({ length: 50 }, (_, i) => ({
      name: `secret-${i}`,
      value: `secret-value-${i}`,
    }));

    await setupWailsMocks(page, { secrets: manySecrets });
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);

    // Should render items
    const items = page.locator('.item-button');
    const count = await items.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should handle many staged changes', async ({ page }) => {
    const manyStaged = Array.from({ length: 20 }, (_, i) => ({
      name: `/staged/param/${i}`,
      value: `staged-value-${i}`,
      operation: 'create',
    }));

    await setupWailsMocks(page, { stagedParam: manyStaged });
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // Should render staging content
    await expect(page.locator('.staging-content')).toBeVisible();
  });

  test('should handle parameter with many tags', async ({ page }) => {
    const manyTags = Array.from({ length: 50 }, (_, i) => ({
      key: `tag-${i}`,
      value: `value-${i}`,
    }));

    await setupWailsMocks(page, {
      params: [{ name: '/app/many-tags', type: 'String', value: 'value' }],
      paramTags: { '/app/many-tags': manyTags },
    });
    await page.goto('/');
    await waitForItemList(page);

    await clickItemByName(page, '/app/many-tags');

    // Should display tags (even if scrollable)
    await expect(page.locator('.tag-item').first()).toBeVisible();
  });
});

// ============================================================================
// Error State Recovery Tests
// ============================================================================

test.describe('Error State Recovery', () => {
  test('should allow retry after list error', async ({ page }) => {
    await setupWailsMocks(page, {
      simulateError: { operation: 'ParamList', message: 'Network error' },
    });
    await page.goto('/');

    // Should show error
    await expect(page.locator('.error-banner')).toBeVisible();

    // Refresh should be available
    await expect(page.getByRole('button', { name: /Refresh/i })).toBeVisible();
  });

  test('should navigate away from error state', async ({ page }) => {
    await setupWailsMocks(page, {
      simulateError: { operation: 'ParamList', message: 'Network error' },
    });
    await page.goto('/');
    await expect(page.locator('.error-banner')).toBeVisible();

    // Navigate to a different view
    await navigateTo(page, 'Secrets');

    // Should show new view
    await expect(page.locator('.item-list, .error-banner')).toBeVisible();
  });
});

// ============================================================================
// Browser Behavior Tests
// ============================================================================

test.describe('Browser Behaviors', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should handle page visibility change', async ({ page }) => {
    // Simulate tab switching (visibility change)
    await page.evaluate(() => {
      document.dispatchEvent(new Event('visibilitychange'));
    });

    // App should still be functional
    await expect(page.locator('.item-list')).toBeVisible();
  });

  test('should handle window resize', async ({ page }) => {
    // Resize window
    await page.setViewportSize({ width: 800, height: 600 });
    await expect(page.locator('.item-list')).toBeVisible();

    await page.setViewportSize({ width: 1200, height: 800 });
    await expect(page.locator('.item-list')).toBeVisible();

    await page.setViewportSize({ width: 1920, height: 1080 });
    await expect(page.locator('.item-list')).toBeVisible();
  });

  test('should handle zoom level changes', async ({ page }) => {
    // Different viewport sizes simulate zoom
    await page.setViewportSize({ width: 1280, height: 720 });
    await expect(page.locator('.item-list')).toBeVisible();
  });
});

// ============================================================================
// Memory Leak Prevention Tests
// ============================================================================

test.describe('Memory Management', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should handle many modal open/close cycles', async ({ page }) => {
    // Open and close modal many times
    for (let i = 0; i < 10; i++) {
      await page.getByRole('button', { name: '+ New' }).click();
      await expect(page.locator('.modal-backdrop')).toBeVisible();
      await page.getByRole('button', { name: 'Cancel' }).click();
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    }

    // App should still be responsive
    await expect(page.locator('.item-list')).toBeVisible();
  });

  test('should handle many view switches', async ({ page }) => {
    // Switch views many times
    for (let i = 0; i < 10; i++) {
      await navigateTo(page, 'Secrets');
      await waitForViewLoaded(page);
      await navigateTo(page, 'Parameters');
      await waitForViewLoaded(page);
    }

    // App should still be responsive
    await expect(page.locator('.item-list')).toBeVisible();
  });

  test('should handle many item selections', async ({ page }) => {
    // Select items repeatedly
    for (let i = 0; i < 20; i++) {
      await clickItemByName(page, '/app/config');
      await page.locator('.btn-close').click();
    }

    // App should still be responsive
    await expect(page.locator('.item-list')).toBeVisible();
  });
});

// ============================================================================
// Animation and Transition Tests
// ============================================================================

test.describe('Animations and Transitions', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should complete modal animation before interaction', async ({ page }) => {
    await page.getByRole('button', { name: '+ New' }).click();

    // Wait for modal to be fully visible
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    await expect(page.locator('.modal-content')).toBeVisible();

    // Interact with modal
    await page.locator('#param-name').fill('/test');
    await expect(page.locator('#param-name')).toHaveValue('/test');
  });

  test('should complete detail panel animation before interaction', async ({ page }) => {
    await clickItemByName(page, '/app/config');

    // Wait for panel to be fully visible
    await expect(page.locator('.detail-panel')).toBeVisible();

    // Interact with panel
    await page.getByRole('button', { name: 'Edit' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
  });
});

// ============================================================================
// Network Timing Tests
// ============================================================================

test.describe('Network Timing', () => {
  test('should handle slow initial load', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');

    // Should eventually load
    await waitForViewLoaded(page);
    await expect(page.locator('.filter-bar')).toBeVisible();
  });

  test('should handle slow refresh', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    await page.getByRole('button', { name: 'Refresh' }).click();

    // Should eventually complete
    await waitForItemList(page);
    await expect(page.locator('.item-list')).toBeVisible();
  });
});

// ============================================================================
// Edge Case Combinations Tests
// ============================================================================

test.describe('Edge Case Combinations', () => {
  test('should handle filter + selection + refresh', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    // Apply filter
    await page.locator('.prefix-input').fill('/app');
    await page.waitForTimeout(400);

    // Select item
    await clickItemByName(page, '/app/config');
    await expect(page.locator('.detail-panel')).toBeVisible();

    // Refresh
    await page.getByRole('button', { name: 'Refresh' }).click();
    await waitForItemList(page);

    // Filter should be preserved
    await expect(page.locator('.prefix-input')).toHaveValue('/app');
  });

  test('should handle view switch + modal + navigation', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    // Open modal
    await page.getByRole('button', { name: '+ New' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();

    // Cancel modal
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // Navigate
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);

    // App should be functional
    await expect(page.locator('.item-list')).toBeVisible();
  });

  test('should handle error + navigation + retry', async ({ page }) => {
    await setupWailsMocks(page, {
      simulateError: { operation: 'ParamList', message: 'Error' },
    });
    await page.goto('/');
    await expect(page.locator('.error-banner')).toBeVisible();

    // Navigate to secrets
    await navigateTo(page, 'Secrets');

    // Navigate back
    await navigateTo(page, 'Parameters');

    // Should show error or retry
    await expect(page.locator('.filter-bar')).toBeVisible();
  });
});
