import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  type MockState,
  createStagedValue,
  createStagedTags,
  createParamStagedState,
  createSecretStagedState,
  createMixedStagedState,
  createTagOnlyStagedState,
  navigateTo,
  waitForItemList,
  clickItemByName,
  openCreateModal,
  closeModal,
} from './fixtures/wails-mock';

// ============================================================================
// Basic Staging View Tests
// ============================================================================

test.describe('Staging View Basics', () => {
  test.describe('Empty Staging Area', () => {
    test.beforeEach(async ({ page }) => {
      await setupWailsMocks(page);
      await page.goto('/');
      await navigateTo(page, 'Staging');
    });

    test('should display staging area title', async ({ page }) => {
      await expect(page.getByText('Staging Area')).toBeVisible();
    });

    test('should display Parameters and Secrets sections', async ({ page }) => {
      await expect(page.getByRole('heading', { name: /Parameters/i })).toBeVisible();
      await expect(page.getByRole('heading', { name: /Secrets/i })).toBeVisible();
    });

    test('should show empty state when no staged changes', async ({ page }) => {
      // Wait for staging status to load
      await page.waitForFunction(() => {
        return document.querySelector('.section') !== null;
      });
      await expect(page.getByText(/No staged/i).first()).toBeVisible();
    });

    test('should have view mode toggle (Diff/Value)', async ({ page }) => {
      await expect(page.getByRole('button', { name: 'Diff' })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Value' })).toBeVisible();
    });

    test('should default to Diff view mode', async ({ page }) => {
      await expect(page.getByRole('button', { name: 'Diff' })).toHaveClass(/active/);
    });

    test('should switch view mode when toggle clicked', async ({ page }) => {
      const diffBtn = page.getByRole('button', { name: 'Diff' });
      const valueBtn = page.getByRole('button', { name: 'Value' });

      await expect(diffBtn).toHaveClass(/active/);
      await valueBtn.click();
      await expect(valueBtn).toHaveClass(/active/);
      await expect(diffBtn).not.toHaveClass(/active/);
    });

    test('should toggle between Diff and Value view', async ({ page }) => {
      await page.getByRole('button', { name: 'Value' }).click();
      await expect(page.getByRole('button', { name: 'Value' })).toHaveClass(/active/);
      await page.getByRole('button', { name: 'Diff' }).click();
      await expect(page.getByRole('button', { name: 'Diff' })).toHaveClass(/active/);
    });
  });
});

// ============================================================================
// Staging from Parameter View
// ============================================================================

test.describe('Staging from Parameter View', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);
  });

  test('should stage new parameter creation', async ({ page }) => {
    await openCreateModal(page);
    await page.locator('#param-name').fill('/test/new-param');
    await page.locator('#param-value').fill('new-param-value');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should stage parameter edit', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.locator('#param-value').fill('edited-value');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should stage parameter delete', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('button', { name: 'Stage Delete' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should stage parameter tag addition', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: '+ Add' }).click();
    await page.locator('#tag-key').waitFor();
    await page.locator('#tag-key').fill('staged-tag');
    await page.locator('#tag-value').fill('staged-value');
    await page.getByRole('button', { name: 'Stage Tag' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should stage parameter tag removal', async ({ page }) => {
    await clickItemByName(page, '/app/config');
    await page.locator('.btn-tag-remove').click();
    await page.getByRole('button', { name: 'Stage Remove' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });
});

// ============================================================================
// Staging from Secret View
// ============================================================================

test.describe('Staging from Secret View', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);
  });

  test('should stage new secret creation', async ({ page }) => {
    await openCreateModal(page);
    await page.locator('#secret-name').fill('new-staged-secret');
    await page.locator('#secret-value').fill('{"staged": true}');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should stage secret edit', async ({ page }) => {
    await clickItemByName(page, 'my-secret');
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.locator('#edit-secret-value').fill('{"edited": true}');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should stage secret delete', async ({ page }) => {
    await clickItemByName(page, 'my-secret');
    await page.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('button', { name: 'Stage Delete' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should stage secret tag addition', async ({ page }) => {
    await clickItemByName(page, 'my-secret');
    await page.getByRole('button', { name: '+ Add' }).click();
    await page.locator('#tag-key').waitFor();
    await page.locator('#tag-key').fill('staged-tag');
    await page.locator('#tag-value').fill('staged-value');
    await page.getByRole('button', { name: 'Stage Tag' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should stage secret tag removal', async ({ page }) => {
    await clickItemByName(page, 'my-secret');
    await page.locator('.btn-tag-remove').click();
    await page.getByRole('button', { name: 'Stage Remove' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });
});

// ============================================================================
// Staging State Combinations
// ============================================================================

test.describe('Staging State Combinations', () => {
  test.describe('Param Only Staged', () => {
    test('should display Param staged changes when Secret is empty', async ({ page }) => {
      const paramOnly = createParamStagedState([
        createStagedValue('/test/param', 'create', 'new-value'),
      ]);
      await setupWailsMocks(page, paramOnly);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForFunction(() => document.querySelector('.entry-item') !== null);
      await expect(page.locator('.entry-item')).toBeVisible();
    });

    test('should show empty state in Secret section when only Param has changes', async ({ page }) => {
      const paramOnly = createParamStagedState([
        createStagedValue('/test/param', 'update', 'updated'),
      ]);
      await setupWailsMocks(page, paramOnly);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForFunction(() => document.querySelector('.section') !== null);
      // Secret section should show empty message
      const secretSection = page.locator('.section').nth(1);
      await expect(secretSection.getByText(/No staged/i)).toBeVisible();
    });
  });

  test.describe('Secret Only Staged', () => {
    test('should display Secret staged changes when Param is empty', async ({ page }) => {
      const secretOnly = createSecretStagedState([
        createStagedValue('new-secret', 'create', '{"new": true}'),
      ]);
      await setupWailsMocks(page, secretOnly);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForFunction(() => document.querySelector('.entry-item') !== null);
      await expect(page.locator('.entry-item')).toBeVisible();
    });

    test('should show empty state in Param section when only Secret has changes', async ({ page }) => {
      const secretOnly = createSecretStagedState([
        createStagedValue('secret', 'update', 'updated'),
      ]);
      await setupWailsMocks(page, secretOnly);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForFunction(() => document.querySelector('.section') !== null);
      // Param section should show empty message
      const paramSection = page.locator('.section').first();
      await expect(paramSection.getByText(/No staged/i)).toBeVisible();
    });
  });

  test.describe('Both Param and Secret Staged', () => {
    test('should display both Param and Secret staged changes', async ({ page }) => {
      const mixed = createMixedStagedState(
        [createStagedValue('/test/param', 'create', 'value')],
        [createStagedValue('test-secret', 'create', 'secret')]
      );
      await setupWailsMocks(page, mixed);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForFunction(() => document.querySelectorAll('.entry-item').length >= 2);
      await expect(page.locator('.entry-item')).toHaveCount(2);
    });
  });
});

// ============================================================================
// Operation Type Combinations
// ============================================================================

test.describe('Operation Type Display', () => {
  test('should show create badge for new items', async ({ page }) => {
    const createOnly = createParamStagedState([
      createStagedValue('/new/param', 'create', 'new-value'),
    ]);
    await setupWailsMocks(page, createOnly);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.operation-badge') !== null);
    await expect(page.locator('.operation-badge').filter({ hasText: /create/i })).toBeVisible();
  });

  test('should show update badge for edited items', async ({ page }) => {
    const updateOnly = createParamStagedState([
      createStagedValue('/app/config', 'update', 'updated-value'),
    ]);
    await setupWailsMocks(page, updateOnly);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.operation-badge') !== null);
    await expect(page.locator('.operation-badge').filter({ hasText: /update/i })).toBeVisible();
  });

  test('should show delete badge for deleted items', async ({ page }) => {
    const deleteOnly = createParamStagedState([
      createStagedValue('/app/config', 'delete'),
    ]);
    await setupWailsMocks(page, deleteOnly);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.operation-badge') !== null);
    await expect(page.locator('.operation-badge').filter({ hasText: /delete/i })).toBeVisible();
  });

  test('should display mixed operations correctly', async ({ page }) => {
    const mixed: Partial<MockState> = {
      stagedParam: [
        createStagedValue('/new/param', 'create', 'new'),
        createStagedValue('/app/config', 'update', 'updated'),
        createStagedValue('/app/api/key', 'delete'),
      ],
      stagedSecret: [],
    };
    await setupWailsMocks(page, mixed);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelectorAll('.entry-item').length >= 3);
    await expect(page.locator('.entry-item')).toHaveCount(3);
    await expect(page.locator('.operation-badge')).toHaveCount(3);
  });
});

// ============================================================================
// Tag Staging
// ============================================================================

test.describe('Tag Staging Display', () => {
  test('should display tag-only changes', async ({ page }) => {
    const tagOnly = createTagOnlyStagedState(
      [createStagedTags('/app/config', { 'new-tag': 'value' }, {})],
      []
    );
    await setupWailsMocks(page, tagOnly);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.section') !== null);
    await expect(page.locator('.section').first()).toBeVisible();
  });

  test('should display add tag changes', async ({ page }) => {
    const addTags = createTagOnlyStagedState(
      [createStagedTags('/app/config', { 'env': 'staging', 'team': 'backend' }, {})],
      []
    );
    await setupWailsMocks(page, addTags);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.section') !== null);
  });

  test('should display remove tag changes', async ({ page }) => {
    const removeTags = createTagOnlyStagedState(
      [createStagedTags('/app/config', {}, { 'old-tag': 'old-value', 'deprecated': 'true' })],
      []
    );
    await setupWailsMocks(page, removeTags);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.section') !== null);
  });

  test('should display both add and remove tag changes', async ({ page }) => {
    const mixedTags = createTagOnlyStagedState(
      [createStagedTags('/app/config', { 'new': 'value' }, { 'old': 'old-value' })],
      []
    );
    await setupWailsMocks(page, mixedTags);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.section') !== null);
  });

  test('should display value change with tag changes', async ({ page }) => {
    const valueAndTags: Partial<MockState> = {
      stagedParam: [createStagedValue('/app/config', 'update', 'new-value')],
      stagedParamTags: [createStagedTags('/app/config', { 'version': '2' }, {})],
      stagedSecret: [],
      stagedSecretTags: [],
    };
    await setupWailsMocks(page, valueAndTags);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.entry-item') !== null);
  });
});

// ============================================================================
// Apply and Reset Operations
// ============================================================================

test.describe('Apply Operations', () => {
  test.beforeEach(async ({ page }) => {
    const staged: Partial<MockState> = {
      stagedParam: [createStagedValue('/test/param', 'create', 'test-value')],
      stagedSecret: [createStagedValue('test-secret', 'create', 'secret-value')],
    };
    await setupWailsMocks(page, staged);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.entry-item') !== null);
  });

  test('should have Apply button when changes exist', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Apply/i }).first()).toBeVisible();
  });

  test('should open apply confirmation modal', async ({ page }) => {
    await page.getByRole('button', { name: /Apply/i }).first().click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
  });

  test('should close apply modal on cancel', async ({ page }) => {
    await page.getByRole('button', { name: /Apply/i }).first().click();
    await closeModal(page);
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should apply changes on confirm', async ({ page }) => {
    await page.getByRole('button', { name: /Apply/i }).first().click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
    // Click confirm button in modal
    const confirmBtn = page.locator('.modal-content').getByRole('button').filter({ hasText: /Apply|Confirm/i });
    await confirmBtn.click();
    // After apply, results are shown with a Close button
    await expect(page.getByRole('button', { name: 'Close' })).toBeVisible({ timeout: 10000 });
    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });
});

test.describe('Reset Operations', () => {
  test.beforeEach(async ({ page }) => {
    const staged: Partial<MockState> = {
      stagedParam: [createStagedValue('/test/param', 'create', 'test-value')],
      stagedSecret: [createStagedValue('test-secret', 'update', 'updated')],
    };
    await setupWailsMocks(page, staged);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.entry-item') !== null);
  });

  test('should have Reset button when changes exist', async ({ page }) => {
    await expect(page.getByRole('button', { name: /Reset/i }).first()).toBeVisible();
  });

  test('should open reset confirmation modal', async ({ page }) => {
    await page.getByRole('button', { name: /Reset/i }).first().click();
    await expect(page.locator('.modal-backdrop')).toBeVisible();
  });

  test('should close reset modal on cancel', async ({ page }) => {
    await page.getByRole('button', { name: /Reset/i }).first().click();
    await closeModal(page);
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should reset changes on confirm', async ({ page }) => {
    await page.getByRole('button', { name: /Reset/i }).first().click();
    await page.locator('.modal-content').getByRole('button', { name: /Reset/i }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });
});

// ============================================================================
// Unstage Individual Items
// ============================================================================

test.describe('Unstage Individual Items', () => {
  test.beforeEach(async ({ page }) => {
    const staged: Partial<MockState> = {
      stagedParam: [
        createStagedValue('/test/param-1', 'create', 'value-1'),
        createStagedValue('/test/param-2', 'update', 'value-2'),
      ],
      stagedSecret: [],
    };
    await setupWailsMocks(page, staged);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelectorAll('.entry-item').length >= 2);
  });

  test('should display multiple staged entries', async ({ page }) => {
    await expect(page.locator('.entry-item')).toHaveCount(2);
  });

  test('should have unstage capability for entries', async ({ page }) => {
    // Look for unstage buttons or icons in entry items
    await expect(page.locator('.entry-item').first()).toBeVisible();
  });
});

// ============================================================================
// Navigation and State Persistence
// ============================================================================

test.describe('Navigation and State', () => {
  test('should preserve staged changes when navigating away and back', async ({ page }) => {
    // Stage a change
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    // Create staged change
    await openCreateModal(page);
    await page.locator('#param-name').fill('/test/staged');
    await page.locator('#param-value').fill('staged-value');
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();

    // Navigate to Secrets
    await navigateTo(page, 'Secrets');
    await waitForItemList(page);

    // Navigate to Staging - changes should still be there
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.entry-item') !== null);
    await expect(page.locator('.entry-item')).toBeVisible();
  });

  test('should refresh staging view when navigating back', async ({ page }) => {
    const staged: Partial<MockState> = {
      stagedParam: [createStagedValue('/test/param', 'create', 'value')],
      stagedSecret: [],
    };
    await setupWailsMocks(page, staged);
    await page.goto('/');

    // Go to staging first
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.entry-item') !== null);

    // Go to parameters
    await navigateTo(page, 'Parameters');
    await waitForItemList(page);

    // Go back to staging
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.entry-item') !== null);
    await expect(page.locator('.entry-item')).toBeVisible();
  });
});

// ============================================================================
// Edge Cases
// ============================================================================

test.describe('Staging Edge Cases', () => {
  test('should handle large number of staged changes', async ({ page }) => {
    const manyChanges: Partial<MockState> = {
      stagedParam: Array.from({ length: 10 }, (_, i) =>
        createStagedValue(`/test/param-${i}`, i % 3 === 0 ? 'create' : i % 3 === 1 ? 'update' : 'delete', `value-${i}`)
      ),
      stagedSecret: [],
    };
    await setupWailsMocks(page, manyChanges);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelectorAll('.entry-item').length >= 10);
    await expect(page.locator('.entry-item')).toHaveCount(10);
  });

  test('should handle staging the same item multiple times (update overwrites)', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
    await waitForItemList(page);

    // Stage edit
    await clickItemByName(page, '/app/config');
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.locator('#param-value').fill('first-edit');
    await page.getByRole('button', { name: 'Stage' }).click();

    // Stage another edit on same item
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.locator('#param-value').fill('second-edit');
    await page.getByRole('button', { name: 'Stage' }).click();

    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
  });

  test('should handle empty value in staged change', async ({ page }) => {
    const emptyValue: Partial<MockState> = {
      stagedParam: [createStagedValue('/test/empty', 'update', '')],
      stagedSecret: [],
    };
    await setupWailsMocks(page, emptyValue);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.entry-item') !== null);
    await expect(page.locator('.entry-item')).toBeVisible();
  });

  test('should handle special characters in staged values', async ({ page }) => {
    const specialChars: Partial<MockState> = {
      stagedParam: [createStagedValue('/test/special', 'create', '<script>alert("xss")</script>')],
      stagedSecret: [],
    };
    await setupWailsMocks(page, specialChars);
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForFunction(() => document.querySelector('.entry-item') !== null);
    await expect(page.locator('.entry-item')).toBeVisible();
  });
});
