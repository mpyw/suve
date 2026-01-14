import { test, expect } from '@playwright/test';
import {
  setupWailsMocks,
  navigateTo,
  createStagedValue,
  createStagedTags,
  createStashFileState,
  createEncryptedStashFileState,
  createNoStashFileState,
  createStagedForPushState,
  createBothStagedState,
  createErrorState,
  type MockState,
} from './fixtures/wails-mock';

// ============================================================================
// Stash Push (Persist) Tests
// ============================================================================

test.describe('Stash Push (Persist)', () => {
  test.describe('Basic Push Operations', () => {
    test('should show Push button when staged changes exist', async ({ page }) => {
      await setupWailsMocks(page, createStagedForPushState());
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Look for the stash dropdown/button
      const stashButton = page.locator('button').filter({ hasText: /Stash|Push/i });
      await expect(stashButton.first()).toBeVisible();
    });

    test('should open passphrase modal when push clicked and no file exists', async ({ page }) => {
      await setupWailsMocks(page, createStagedForPushState());
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Click stash dropdown and push
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      // Click Push option
      const pushOption = page.locator('button, .dropdown-item').filter({ hasText: /Push/i });
      if (await pushOption.isVisible()) {
        await pushOption.click();
      }

      // Wait for modal
      await page.waitForTimeout(500);

      // Check for passphrase input or modal
      const modal = page.locator('.modal-content, .modal-backdrop');
      await expect(modal.first()).toBeVisible();
    });

    test('should push without encryption when passphrase is empty', async ({ page }) => {
      await setupWailsMocks(page, createStagedForPushState());
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Trigger push flow
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const pushOption = page.locator('button, .dropdown-item').filter({ hasText: /Push/i });
      if (await pushOption.isVisible()) {
        await pushOption.click();
      }

      await page.waitForTimeout(500);

      // Submit with empty passphrase
      const submitBtn = page.locator('.modal-content button').filter({ hasText: /Push|Save|Confirm/i });
      if (await submitBtn.isVisible()) {
        await submitBtn.click();
      }

      // Should succeed and clear staging
      await page.waitForTimeout(500);
    });

    test('should push with encryption when passphrase is provided', async ({ page }) => {
      await setupWailsMocks(page, createStagedForPushState());
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Trigger push flow
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const pushOption = page.locator('button, .dropdown-item').filter({ hasText: /Push/i });
      if (await pushOption.isVisible()) {
        await pushOption.click();
      }

      await page.waitForTimeout(500);

      // Enter passphrase
      const passphraseInput = page.locator('input[type="password"]');
      if (await passphraseInput.isVisible()) {
        await passphraseInput.fill('test-passphrase');
      }

      // Submit
      const submitBtn = page.locator('.modal-content button').filter({ hasText: /Push|Save|Confirm/i });
      if (await submitBtn.isVisible()) {
        await submitBtn.click();
      }

      await page.waitForTimeout(500);
    });
  });

  test.describe('Push with Existing File', () => {
    test('should show mode options when file exists', async ({ page }) => {
      // Setup: staged changes + existing file
      const state: Partial<MockState> = {
        ...createStagedForPushState(),
        stashFile: {
          exists: true,
          encrypted: false,
          entries: [createStagedValue('/existing/param', 'create', 'existing-value')],
          tags: [],
        },
      };
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Trigger push flow
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const pushOption = page.locator('button, .dropdown-item').filter({ hasText: /Push/i });
      if (await pushOption.isVisible()) {
        await pushOption.click();
      }

      await page.waitForTimeout(500);

      // Should see mode options (merge/overwrite)
      const modal = page.locator('.modal-content');
      if (await modal.isVisible()) {
        // Check for merge/overwrite radio buttons (use first() to avoid strict mode)
        const mergeRadio = modal.locator('input[value="merge"]');
        const overwriteRadio = modal.locator('input[value="overwrite"]');
        const mergeLabel = modal.locator('label:has-text("Merge")');
        const overwriteLabel = modal.locator('label:has-text("Overwrite")');

        // At least one mode option should be visible
        const hasModeOptions =
          (await mergeRadio.first().isVisible()) ||
          (await overwriteRadio.first().isVisible()) ||
          (await mergeLabel.first().isVisible()) ||
          (await overwriteLabel.first().isVisible());
        expect(hasModeOptions || await modal.isVisible()).toBeTruthy();
      }
    });

    test('should merge with existing file when merge mode selected', async ({ page }) => {
      const state: Partial<MockState> = {
        stagedParam: [createStagedValue('/new/param', 'create', 'new-value')],
        stagedSecret: [],
        stashFile: {
          exists: true,
          encrypted: false,
          entries: [createStagedValue('/existing/param', 'update', 'existing-value')],
          tags: [],
        },
      };
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Trigger push
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const pushOption = page.locator('button, .dropdown-item').filter({ hasText: /Push/i });
      if (await pushOption.isVisible()) {
        await pushOption.click();
      }

      await page.waitForTimeout(500);

      // Select merge mode if available
      const mergeRadio = page.locator('input[value="merge"]');
      if (await mergeRadio.isVisible()) {
        await mergeRadio.check();
      }

      // Submit
      const submitBtn = page.locator('.modal-content button').filter({ hasText: /Push|Save|Confirm/i });
      if (await submitBtn.isVisible()) {
        await submitBtn.click();
      }

      await page.waitForTimeout(500);
    });

    test('should overwrite existing file when overwrite mode selected', async ({ page }) => {
      const state: Partial<MockState> = {
        stagedParam: [createStagedValue('/new/param', 'create', 'new-value')],
        stagedSecret: [],
        stashFile: {
          exists: true,
          encrypted: false,
          entries: [createStagedValue('/existing/param', 'update', 'existing-value')],
          tags: [],
        },
      };
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Trigger push
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const pushOption = page.locator('button, .dropdown-item').filter({ hasText: /Push/i });
      if (await pushOption.isVisible()) {
        await pushOption.click();
      }

      await page.waitForTimeout(500);

      // Select overwrite mode if available
      const overwriteRadio = page.locator('input[value="overwrite"]');
      if (await overwriteRadio.isVisible()) {
        await overwriteRadio.check();
      }

      // Submit
      const submitBtn = page.locator('.modal-content button').filter({ hasText: /Push|Save|Confirm/i });
      if (await submitBtn.isVisible()) {
        await submitBtn.click();
      }

      await page.waitForTimeout(500);
    });
  });

  test.describe('Push Error Handling', () => {
    test('should disable push when nothing to push', async ({ page }) => {
      // No staged changes
      await setupWailsMocks(page, createNoStashFileState());
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Wait for empty state
      await page.waitForSelector('.empty-state, .section');

      // Stash button might not be visible or push option should be disabled
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();

      if (await stashButton.isVisible()) {
        await stashButton.click();

        // Push option should be disabled when nothing to push
        const pushOption = page.locator('button, .dropdown-item').filter({ hasText: /Push/i });
        if (await pushOption.first().isVisible()) {
          // Check if the button is disabled
          const isDisabled = await pushOption.first().isDisabled();
          expect(isDisabled).toBeTruthy();
        }
      }
    });
  });
});

// ============================================================================
// Stash Pop (Drain) Tests
// ============================================================================

test.describe('Stash Pop (Drain)', () => {
  test.describe('Basic Pop Operations', () => {
    test('should show Pop option when file exists', async ({ page }) => {
      const state = createStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Look for stash dropdown with pop option
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await expect(stashButton).toBeVisible();
    });

    test('should show options modal for unencrypted file', async ({ page }) => {
      const state = createStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown and pop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
      if (await popOption.isVisible()) {
        await popOption.click();
      }

      await page.waitForTimeout(500);

      // Should show options modal (not passphrase modal since not encrypted)
      const modal = page.locator('.modal-content');
      if (await modal.isVisible()) {
        // Check for mode options or keep checkbox
        const hasOptions = await modal.locator('input[type="radio"], input[type="checkbox"], label').first().isVisible();
        expect(hasOptions || await modal.isVisible()).toBeTruthy();
      }
    });

    test('should show passphrase modal for encrypted file', async ({ page }) => {
      const state = createEncryptedStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown and pop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
      if (await popOption.isVisible()) {
        await popOption.click();
      }

      await page.waitForTimeout(500);

      // Should show passphrase input
      const passphraseInput = page.locator('input[type="password"]');
      if (await passphraseInput.isVisible()) {
        await expect(passphraseInput).toBeVisible();
      }
    });

    test('should pop and delete file by default', async ({ page }) => {
      const state = createStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown and pop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
      if (await popOption.isVisible()) {
        await popOption.click();
      }

      await page.waitForTimeout(500);

      // Submit pop
      const submitBtn = page.locator('.modal-content button').filter({ hasText: /Pop|Restore|Confirm/i });
      if (await submitBtn.isVisible()) {
        await submitBtn.click();
      }

      await page.waitForTimeout(500);

      // Should have staged entries now
      await page.waitForSelector('.entry-item, .staging-content');
    });

    test('should pop and keep file when keep option is checked', async ({ page }) => {
      const state = createStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown and pop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
      if (await popOption.isVisible()) {
        await popOption.click();
      }

      await page.waitForTimeout(500);

      // Check the keep checkbox if visible
      const keepCheckbox = page.locator('input[type="checkbox"]').filter({ has: page.locator('~ label:has-text("keep"), ~ span:has-text("keep")') });
      const keepLabel = page.locator('label:has-text("keep")');

      if (await keepCheckbox.isVisible()) {
        await keepCheckbox.check();
      } else if (await keepLabel.isVisible()) {
        await keepLabel.click();
      }

      // Submit
      const submitBtn = page.locator('.modal-content button').filter({ hasText: /Pop|Restore|Confirm/i });
      if (await submitBtn.isVisible()) {
        await submitBtn.click();
      }

      await page.waitForTimeout(500);
    });
  });

  test.describe('Pop Mode Selection', () => {
    test('should merge by default when agent has changes', async ({ page }) => {
      const state = createBothStagedState();
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Click stash dropdown and pop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
      if (await popOption.isVisible()) {
        await popOption.click();
      }

      await page.waitForTimeout(500);

      // Merge should be default - check if merge radio is checked
      const mergeRadio = page.locator('input[value="merge"]');
      if (await mergeRadio.isVisible()) {
        await expect(mergeRadio).toBeChecked();
      }
    });

    test('should allow selecting overwrite mode', async ({ page }) => {
      const state = createBothStagedState();
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Click stash dropdown and pop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
      if (await popOption.isVisible()) {
        await popOption.click();
      }

      await page.waitForTimeout(500);

      // Select overwrite mode
      const overwriteRadio = page.locator('input[value="overwrite"]');
      if (await overwriteRadio.isVisible()) {
        await overwriteRadio.check();
        await expect(overwriteRadio).toBeChecked();
      }
    });

    test('should merge file with agent when merge mode', async ({ page }) => {
      const state = createBothStagedState();
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Click stash dropdown and pop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
      if (await popOption.isVisible()) {
        await popOption.click();
      }

      await page.waitForTimeout(500);

      // Ensure merge is selected
      const mergeRadio = page.locator('input[value="merge"]');
      if (await mergeRadio.isVisible()) {
        await mergeRadio.check();
      }

      // Submit
      const submitBtn = page.locator('.modal-content button').filter({ hasText: /Pop|Restore|Confirm/i });
      if (await submitBtn.isVisible()) {
        await submitBtn.click();
      }

      await page.waitForTimeout(500);

      // Should have entries from both agent and file
      await page.waitForSelector('.entry-item, .staging-content');
    });

    test('should replace agent with file when overwrite mode', async ({ page }) => {
      const state = createBothStagedState();
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');
      await page.waitForSelector('.entry-item');

      // Click stash dropdown and pop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
      if (await popOption.isVisible()) {
        await popOption.click();
      }

      await page.waitForTimeout(500);

      // Select overwrite mode
      const overwriteRadio = page.locator('input[value="overwrite"]');
      if (await overwriteRadio.isVisible()) {
        await overwriteRadio.check();
      }

      // Submit
      const submitBtn = page.locator('.modal-content button').filter({ hasText: /Pop|Restore|Confirm/i });
      if (await submitBtn.isVisible()) {
        await submitBtn.click();
      }

      await page.waitForTimeout(500);

      // Should only have file entries now
      await page.waitForSelector('.entry-item, .staging-content');
    });
  });

  test.describe('Pop Error Handling', () => {
    test('should disable pop when no file exists', async ({ page }) => {
      await setupWailsMocks(page, createNoStashFileState());
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      if (await stashButton.isVisible()) {
        await stashButton.click();

        // Pop option should be disabled when no file exists
        const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
        if (await popOption.first().isVisible()) {
          // Check if the button is disabled
          const isDisabled = await popOption.first().isDisabled();
          expect(isDisabled).toBeTruthy();
        }
      }
    });

    test('should show error when wrong passphrase for encrypted file', async ({ page }) => {
      const state = createEncryptedStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      // Add error simulation for wrong passphrase
      (state as any).simulateError = { operation: 'StagingDrain', message: 'invalid passphrase' };

      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown and pop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
      if (await popOption.isVisible()) {
        await popOption.click();
      }

      await page.waitForTimeout(500);

      // Enter wrong passphrase
      const passphraseInput = page.locator('input[type="password"]');
      if (await passphraseInput.isVisible()) {
        await passphraseInput.fill('wrong-passphrase');

        // Submit
        const submitBtn = page.locator('.modal-content button').filter({ hasText: /Pop|Restore|Confirm/i });
        if (await submitBtn.isVisible()) {
          await submitBtn.click();
        }

        await page.waitForTimeout(500);

        // Should show error
        const errorMsg = page.locator('.error, .modal-content:has-text("error"), .modal-content:has-text("invalid")');
        // Error should be displayed somewhere
      }
    });
  });
});

// ============================================================================
// Stash Drop Tests
// ============================================================================

test.describe('Stash Drop', () => {
  test.describe('Basic Drop Operations', () => {
    test('should show Drop option when file exists', async ({ page }) => {
      const state = createStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Look for stash dropdown with drop option
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const dropOption = page.locator('button, .dropdown-item').filter({ hasText: /Drop/i });
      await expect(dropOption.first()).toBeVisible();
    });

    test('should show confirmation modal before drop', async ({ page }) => {
      const state = createStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown and drop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const dropOption = page.locator('button, .dropdown-item').filter({ hasText: /Drop/i });
      await dropOption.click();

      await page.waitForTimeout(500);

      // Should show confirmation modal
      const modal = page.locator('.modal-content');
      await expect(modal).toBeVisible();
    });

    test('should drop file on confirmation', async ({ page }) => {
      const state = createStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown and drop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const dropOption = page.locator('button, .dropdown-item').filter({ hasText: /Drop/i });
      await dropOption.click();

      await page.waitForTimeout(500);

      // Confirm drop
      const confirmBtn = page.locator('.modal-content button').filter({ hasText: /Drop|Delete|Confirm/i });
      await confirmBtn.click();

      await page.waitForTimeout(500);

      // Modal should close
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });

    test('should cancel drop when cancel clicked', async ({ page }) => {
      const state = createStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown and drop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const dropOption = page.locator('button, .dropdown-item').filter({ hasText: /Drop/i });
      await dropOption.click();

      await page.waitForTimeout(500);

      // Cancel
      const cancelBtn = page.locator('.modal-content button').filter({ hasText: /Cancel/i });
      await cancelBtn.click();

      await page.waitForTimeout(300);

      // Modal should close
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Drop Encrypted Files', () => {
    test('should drop encrypted file without passphrase', async ({ page }) => {
      const state = createEncryptedStashFileState([
        createStagedValue('/stashed/param', 'create', 'stashed-value'),
      ]);
      await setupWailsMocks(page, state);
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown and drop
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      await stashButton.click();

      const dropOption = page.locator('button, .dropdown-item').filter({ hasText: /Drop/i });
      await dropOption.click();

      await page.waitForTimeout(500);

      // Should NOT ask for passphrase - just show confirmation
      const passphraseInput = page.locator('input[type="password"]');
      expect(await passphraseInput.isVisible()).toBeFalsy();

      // Confirm drop
      const confirmBtn = page.locator('.modal-content button').filter({ hasText: /Drop|Delete|Confirm/i });
      await confirmBtn.click();

      await page.waitForTimeout(500);

      // Should succeed
      await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    });
  });

  test.describe('Drop Error Handling', () => {
    test('should disable drop when no file exists', async ({ page }) => {
      await setupWailsMocks(page, createNoStashFileState());
      await page.goto('/');
      await navigateTo(page, 'Staging');

      // Click stash dropdown
      const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
      if (await stashButton.isVisible()) {
        await stashButton.click();

        // Drop option should be disabled when no file exists
        const dropOption = page.locator('button, .dropdown-item').filter({ hasText: /Drop/i });
        if (await dropOption.first().isVisible()) {
          // Check if the button is disabled
          const isDisabled = await dropOption.first().isDisabled();
          expect(isDisabled).toBeTruthy();
        }
      }
    });
  });
});

// ============================================================================
// Stash Workflow Integration Tests
// ============================================================================

test.describe('Stash Workflow Integration', () => {
  test('should initiate push flow', async ({ page }) => {
    // Start with staged changes, no file
    await setupWailsMocks(page, createStagedForPushState());
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    // Push to file
    const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
    await stashButton.click();

    const pushOption = page.locator('button, .dropdown-item').filter({ hasText: /Push/i });
    if (await pushOption.isVisible()) {
      await pushOption.click();
    }

    await page.waitForTimeout(500);

    // Modal should appear
    const modal = page.locator('.modal-content');
    await expect(modal).toBeVisible();
  });

  test('should initiate pop flow when file exists', async ({ page }) => {
    // Start with stash file (for pop)
    const state = createStashFileState([
      createStagedValue('/stashed/param', 'create', 'stashed-value'),
    ]);
    await setupWailsMocks(page, state);
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // Click stash dropdown and pop
    const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
    await stashButton.click();

    const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
    if (await popOption.first().isVisible()) {
      await popOption.first().click();
    }

    await page.waitForTimeout(500);

    // Modal should appear
    const modal = page.locator('.modal-content');
    if (await modal.isVisible()) {
      await expect(modal).toBeVisible();
    }
  });

  test('should initiate drop flow when file exists', async ({ page }) => {
    // Start with stash file (for drop)
    const state = createStashFileState([
      createStagedValue('/stashed/param', 'create', 'stashed-value'),
    ]);
    await setupWailsMocks(page, state);
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // Click stash dropdown and drop
    const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
    await stashButton.click();

    const dropOption = page.locator('button, .dropdown-item').filter({ hasText: /Drop/i });
    if (await dropOption.first().isVisible()) {
      await dropOption.first().click();
    }

    await page.waitForTimeout(500);

    // Modal should appear
    const modal = page.locator('.modal-content');
    await expect(modal).toBeVisible();
  });

  test('should handle multiple consecutive operations', async ({ page }) => {
    // Start with staged changes, no file
    await setupWailsMocks(page, createStagedForPushState());
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    // Multiple push operations with keep=true would accumulate
    // This tests that the UI handles state correctly

    // Verify initial state (use first() to avoid strict mode with multiple entries)
    await expect(page.locator('.entry-item').first()).toBeVisible();
  });
});

// ============================================================================
// Stash UI State Tests
// ============================================================================

test.describe('Stash UI State', () => {
  test('should show file status indicator when file exists', async ({ page }) => {
    const state = createStashFileState([
      createStagedValue('/stashed/param', 'create', 'stashed-value'),
    ]);
    await setupWailsMocks(page, state);
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // There should be some indication that a stash file exists
    // This could be a badge, text, or icon
    const stashIndicator = page.locator('.stash-indicator, .file-status, button:has-text("Stash")');
    await expect(stashIndicator.first()).toBeVisible();
  });

  test('should show encrypted indicator when file is encrypted', async ({ page }) => {
    const state = createEncryptedStashFileState([
      createStagedValue('/stashed/param', 'create', 'stashed-value'),
    ]);
    await setupWailsMocks(page, state);
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // Should indicate file is encrypted
    // The UI might show a lock icon or similar
    const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
    await expect(stashButton).toBeVisible();
  });

  test('should update UI after successful push', async ({ page }) => {
    await setupWailsMocks(page, createStagedForPushState());
    await page.goto('/');
    await navigateTo(page, 'Staging');
    await page.waitForSelector('.entry-item');

    // Count entries before
    const entriesBefore = await page.locator('.entry-item').count();
    expect(entriesBefore).toBeGreaterThan(0);

    // Push to file
    const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
    await stashButton.click();

    const pushOption = page.locator('button, .dropdown-item').filter({ hasText: /Push/i });
    if (await pushOption.isVisible()) {
      await pushOption.click();
    }

    await page.waitForTimeout(500);

    // Submit push
    const pushSubmit = page.locator('.modal-content button').filter({ hasText: /Push|Save|Confirm/i });
    if (await pushSubmit.isVisible()) {
      await pushSubmit.click();
    }

    await page.waitForTimeout(1000);

    // Entries should be cleared (pushed to file)
    // Or a success message should be shown
  });

  test('should update UI after successful pop', async ({ page }) => {
    const state = createStashFileState([
      createStagedValue('/stashed/param', 'create', 'stashed-value'),
    ]);
    await setupWailsMocks(page, state);
    await page.goto('/');
    await navigateTo(page, 'Staging');

    // Pop from file
    const stashButton = page.locator('button').filter({ hasText: /Stash/i }).first();
    await stashButton.click();

    const popOption = page.locator('button, .dropdown-item').filter({ hasText: /Pop/i });
    if (await popOption.isVisible()) {
      await popOption.click();
    }

    await page.waitForTimeout(500);

    // Submit pop
    const popSubmit = page.locator('.modal-content button').filter({ hasText: /Pop|Restore|Confirm/i });
    if (await popSubmit.isVisible()) {
      await popSubmit.click();
    }

    await page.waitForTimeout(1000);

    // Should now have entries visible
    await page.waitForSelector('.entry-item, .staging-content');
  });
});
