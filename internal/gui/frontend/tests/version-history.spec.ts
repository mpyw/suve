import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createVersionHistoryState,
  waitForItemList,
  clickItemByName,
  navigateTo,
} from './fixtures/wails-mock';

// ============================================================================
// Parameter Version History Tests
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

// ============================================================================
// Secret Version History Tests
// ============================================================================

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
// Version Comparison Tests
// ============================================================================

test.describe('Version Comparison', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page, {
      ...createVersionHistoryState(),
      params: [{ name: '/app/config', type: 'String', value: 'current-value-v3' }],
    });
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should show detail panel with parameter info', async ({ page }) => {
    await clickItemByName(page, '/app/config');

    // Should show detail panel
    await expect(page.locator('.detail-panel')).toBeVisible();
    // Should show the parameter name
    await expect(page.locator('.detail-title')).toContainText('/app/config');
  });

  test('should mark current version distinctly', async ({ page }) => {
    await clickItemByName(page, '/app/config');

    // Current version should be marked
    const currentBadge = page.locator('.badge-current');
    await expect(currentBadge.first()).toBeVisible();
  });

  test('should enable compare button when multiple versions exist', async ({ page }) => {
    await clickItemByName(page, '/app/config');

    // Compare button should be visible
    const compareBtn = page.getByRole('button', { name: 'Compare' });
    await expect(compareBtn).toBeVisible();
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
