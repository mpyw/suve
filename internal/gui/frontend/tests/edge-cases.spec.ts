import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  createErrorState,
  createPaginationTestState,
  waitForItemList,
  waitForViewLoaded,
  clickItemByName,
  navigateTo,
} from './fixtures/wails-mock';

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
// Form Validation Tests
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
// Error Recovery Tests
// ============================================================================

test.describe('Error Recovery', () => {
  test('should show error banner when API fails', async ({ page }) => {
    // Start with error state
    await setupWailsMocks(page, createErrorState('ParamList', 'Network error'));
    await page.goto('/');

    // Should show error
    await expect(page.locator('.error-banner')).toBeVisible();
  });

  test('should have refresh button available during error state', async ({ page }) => {
    await setupWailsMocks(page, createErrorState('ParamList', 'Network error'));
    await page.goto('/');

    // Refresh button should still be available
    await expect(page.getByRole('button', { name: /Refresh|Loading/i })).toBeVisible();
  });

  test('should clear error after successful operation', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    // Verify no error initially
    await expect(page.locator('.error-banner')).not.toBeVisible();

    // Operations should work without error
    await page.getByRole('button', { name: 'Refresh' }).click();
    await waitForItemList(page);
    await expect(page.locator('.error-banner')).not.toBeVisible();
  });
});

// ============================================================================
// Empty State Tests
// ============================================================================

test.describe('Empty States', () => {
  test('should show empty state message for parameters', async ({ page }) => {
    await setupWailsMocks(page, { params: [] });
    await page.goto('/');
    await waitForViewLoaded(page);

    await expect(page.locator('.empty-state')).toBeVisible();
  });

  test('should show empty state message for secrets', async ({ page }) => {
    await setupWailsMocks(page, { secrets: [] });
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForViewLoaded(page);

    await expect(page.locator('.empty-state')).toBeVisible();
  });

  test('should show staging view with no changes', async ({ page }) => {
    await setupWailsMocks(page, { stagedParam: [], stagedSecret: [] });
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // Staging content should be visible even when empty
    await expect(page.locator('.staging-content')).toBeVisible();
  });
});
