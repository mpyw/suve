import { test, expect } from '@playwright/test';
import { setupWailsMocks, type MockState } from './fixtures/wails-mock';

test.describe('App Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
  });

  test('should display the app logo', async ({ page }) => {
    await expect(page.locator('.logo-text')).toContainText('suve');
  });

  test('should show sidebar with navigation items', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Parameters/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /Secrets/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /Staging/i })).toBeVisible();
  });

  test('should highlight active navigation item', async ({ page }) => {
    // Parameters should be active by default
    const paramsBtn = page.getByRole('button', { name: /Parameters/i });
    await expect(paramsBtn).toHaveClass(/active/);

    // Click on Secrets and verify it becomes active
    await page.getByRole('button', { name: /Secrets/i }).click();
    await expect(page.getByRole('button', { name: /Secrets/i })).toHaveClass(/active/);

    // Click on Staging and verify it becomes active
    await page.getByRole('button', { name: /Staging/i }).click();
    await expect(page.getByRole('button', { name: /Staging/i })).toHaveClass(/active/);
  });

  test('should persist active view when refreshing data', async ({ page }) => {
    // Navigate to Secrets
    await page.getByRole('button', { name: /Secrets/i }).click();
    await expect(page.getByRole('button', { name: /Secrets/i })).toHaveClass(/active/);

    // Click refresh - view should stay on Secrets
    const refreshBtn = page.getByRole('button', { name: /Refresh/i });
    await refreshBtn.click();
    await expect(page.getByRole('button', { name: /Secrets/i })).toHaveClass(/active/);
  });
});

test.describe('Parameters View Initial State', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
  });

  test('should have a refresh button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Refresh/i })).toBeVisible();
  });

  test('should have "+ New" button', async ({ page }) => {
    await expect(page.getByRole('button', { name: '+ New' })).toBeVisible();
  });

  test('should display parameter list after load', async ({ page }) => {
    await page.waitForSelector('.item-list');
    await expect(page.locator('.item-list')).toBeVisible();
  });
});

test.describe('Secrets View Initial State', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await page.getByRole('button', { name: /Secrets/i }).click();
  });

  test('should have a refresh button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Refresh/i })).toBeVisible();
  });

  test('should have "+ New" button', async ({ page }) => {
    await expect(page.getByRole('button', { name: '+ New' })).toBeVisible();
  });

  test('should have "Restore" button (unique to secrets)', async ({ page }) => {
    await expect(page.getByRole('button', { name: 'Restore' })).toBeVisible();
  });

  test('should display secret list after load', async ({ page }) => {
    await page.waitForSelector('.item-list');
    await expect(page.locator('.item-list')).toBeVisible();
  });
});

test.describe('Empty State Handling', () => {
  test('should show empty state for parameters when list is empty', async ({ page }) => {
    const emptyState: Partial<MockState> = {
      params: [],
      secrets: [],
    };
    await setupWailsMocks(page, emptyState);
    await page.goto('/');
    // Wait for filter bar (always present even when list is empty)
    await page.waitForSelector('.filter-bar');

    // Should show empty list (no items)
    await expect(page.locator('.item-button')).toHaveCount(0);
  });

  test('should show empty state for secrets when list is empty', async ({ page }) => {
    const emptyState: Partial<MockState> = {
      params: [],
      secrets: [],
    };
    await setupWailsMocks(page, emptyState);
    await page.goto('/');
    await page.getByRole('button', { name: /Secrets/i }).click();
    await page.waitForSelector('.filter-bar');

    await expect(page.locator('.item-button')).toHaveCount(0);
  });
});

test.describe('Error Recovery', () => {
  test('should allow navigation after closing detail panel', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await page.waitForSelector('.item-list');

    // Open detail panel
    await page.locator('.item-button').first().click();
    await expect(page.locator('.detail-panel')).toBeVisible();

    // Close it
    await page.locator('.btn-close').click();
    await expect(page.locator('.detail-panel')).not.toBeVisible();

    // Should be able to navigate to another view
    await page.getByRole('button', { name: /Secrets/i }).click();
    await expect(page.getByRole('button', { name: /Secrets/i })).toHaveClass(/active/);
  });

  test('should allow navigation after cancelling modal', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await page.waitForSelector('.item-list');

    // Open create modal
    await page.getByRole('button', { name: '+ New' }).click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();

    // Cancel it
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // Should be able to navigate
    await page.getByRole('button', { name: /Staging/i }).click();
    await expect(page.getByRole('button', { name: /Staging/i })).toHaveClass(/active/);
  });
});
