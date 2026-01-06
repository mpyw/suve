import { test, expect, Locator } from '@playwright/test';
import {
  navigateTo,
  waitForItemList,
  clickItemByName,
} from './fixtures/wails-mock';

/**
 * GUI Demo Recording Script
 *
 * This test follows the same scenario as the CLI demo (demo/cli-demo.tape).
 * Uses real LocalStack backend via wails dev.
 *
 * Run: ./demo/gui-record.sh
 */

// Timing constants
const PAUSE_SHORT = 500;
const PAUSE_MEDIUM = 1000;
const PAUSE_LONG = 2000;
const TYPING_DELAY = 50; // ms per character

// Helper functions
const pause = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

async function slowType(locator: Locator, text: string) {
  await locator.pressSequentially(text, { delay: TYPING_DELAY });
}

async function slowBackspace(locator: Locator, count: number) {
  for (let i = 0; i < count; i++) {
    await locator.press('Backspace');
    await pause(TYPING_DELAY);
  }
}

test.describe('GUI Demo Recording', () => {
  test('Full staging workflow demo', async ({ page }) => {
    test.setTimeout(300000);

    // No mocks - using real LocalStack via wails dev
    await page.goto('/');
    await waitForItemList(page);

    // Wait for app to be fully rendered before starting demo
    // This minimizes white screen at the beginning of the recording
    await expect(page.locator('.item-list')).toBeVisible();
    // Wait for any initial animations/loading to complete
    await page.waitForLoadState('networkidle');

    // =========================================================================
    // 1. List existing parameters
    // =========================================================================
    console.log('Step 1: Viewing existing parameters');
    await pause(PAUSE_MEDIUM);

    // Enable "Show Values"
    await page.locator('input[type="checkbox"]').first().check();
    await pause(PAUSE_MEDIUM);

    // Click on /demo/api/url to show details with tags
    await clickItemByName(page, '/demo/api/url');
    await pause(PAUSE_LONG);

    // Close detail drawer
    await page.locator('.btn-close').click();
    await pause(PAUSE_SHORT);

    // =========================================================================
    // 2. Stage a new parameter (/demo/cache/ttl = 3600)
    // =========================================================================
    console.log('Step 2: Staging new parameter');
    await page.getByRole('button', { name: '+ New' }).click();
    await pause(PAUSE_SHORT); // Brief pause after opening

    await slowType(page.locator('#param-name'), '/demo/cache/ttl');
    await pause(PAUSE_SHORT);
    await slowType(page.locator('#param-value'), '3600');

    await pause(PAUSE_LONG); // Show filled form before closing
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    await pause(PAUSE_MEDIUM);

    // =========================================================================
    // 3. Check staging (briefly)
    // =========================================================================
    console.log('Step 3: Checking staging view');
    await navigateTo(page, 'Staging');
    await expect(page.locator('.staging-content')).toBeVisible();
    await pause(PAUSE_LONG);

    // =========================================================================
    // 4. Stage an update to existing parameter (/demo/api/url)
    // =========================================================================
    console.log('Step 4: Staging update to existing parameter');
    await navigateTo(page, 'Parameters');
    await waitForItemList(page);
    await pause(PAUSE_MEDIUM); // Wait for view transition to complete

    await clickItemByName(page, '/demo/api/url');
    await pause(PAUSE_MEDIUM);

    await page.getByRole('button', { name: 'Edit' }).click();
    await pause(PAUSE_SHORT); // Brief pause after opening

    // Vim-style edit: delete only the part that needs to change
    // https://api-v1.example.com -> https://api-v2.example.com
    // Delete from end: `.example.com` (12 chars) + `1` (1 char) = 13 backspaces
    const valueInput = page.locator('#param-value');
    await valueInput.focus();
    await slowBackspace(valueInput, 13);
    await slowType(valueInput, '2.example.com');

    await pause(PAUSE_LONG); // Show edited value before closing
    await page.getByRole('button', { name: 'Stage' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    await pause(PAUSE_MEDIUM);

    // =========================================================================
    // 5. Add tag to edited parameter (Env=Dev)
    // =========================================================================
    console.log('Step 5: Adding tag to edited parameter');
    await page.getByRole('button', { name: '+ Add' }).click();
    await pause(PAUSE_SHORT); // Brief pause after opening

    await slowType(page.locator('#tag-key'), 'Env');
    await pause(PAUSE_SHORT);
    await slowType(page.locator('#tag-value'), 'Dev');

    await pause(PAUSE_LONG); // Show filled tag form before closing
    await page.getByRole('button', { name: 'Stage Tag' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    await pause(PAUSE_MEDIUM);

    // =========================================================================
    // 6. Remove old tag (Version)
    // =========================================================================
    console.log('Step 6: Removing old tag');
    await page.locator('.btn-tag-remove').first().click();

    await pause(PAUSE_LONG); // Show remove confirmation before closing
    await page.getByRole('button', { name: 'Stage Remove' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    await pause(PAUSE_MEDIUM);

    // Close detail drawer before switching to another item
    await page.locator('.btn-close').click();
    await pause(PAUSE_SHORT);

    // =========================================================================
    // 7. Stage a deletion (/demo/legacy/endpoint)
    // =========================================================================
    console.log('Step 7: Staging deletion');
    await clickItemByName(page, '/demo/legacy/endpoint');
    await pause(PAUSE_MEDIUM);

    await page.getByRole('button', { name: 'Delete' }).click();

    await pause(PAUSE_LONG); // Show delete confirmation before closing
    await page.getByRole('button', { name: 'Stage Delete' }).click();
    await expect(page.locator('.modal-backdrop')).not.toBeVisible();
    await pause(PAUSE_MEDIUM);

    // Close detail drawer
    await page.locator('.btn-close').click();
    await pause(PAUSE_SHORT);

    // =========================================================================
    // 8. Check staging status
    // =========================================================================
    console.log('Step 8: Checking staging status');
    await navigateTo(page, 'Staging');
    await expect(page.locator('.staging-content')).toBeVisible();
    await pause(PAUSE_LONG);

    // =========================================================================
    // 9. Preview changes - toggle between Diff and Value modes
    // =========================================================================
    console.log('Step 9: Previewing changes');
    const diffToggle = page.locator('.view-toggle').getByText('Diff');
    const valueToggle = page.locator('.view-toggle').getByText('Value');

    // Show in Diff mode (default)
    await diffToggle.click();
    await pause(PAUSE_LONG);

    // Switch to Value mode
    await valueToggle.click();
    await pause(PAUSE_LONG);

    // Back to Diff mode
    await diffToggle.click();
    await pause(PAUSE_MEDIUM);

    // =========================================================================
    // 10. Apply all staged changes
    // =========================================================================
    console.log('Step 10: Applying staged changes');
    await page.getByRole('button', { name: /Apply/i }).first().click();

    await pause(PAUSE_LONG); // Show confirm dialog before closing
    await page.locator('.modal-content').getByRole('button').filter({ hasText: /Apply|Confirm/i }).click();

    await expect(page.getByRole('button', { name: 'Close' })).toBeVisible({ timeout: 10000 });
    await pause(PAUSE_LONG); // Show results before closing

    await page.getByRole('button', { name: 'Close' }).click();
    await pause(PAUSE_MEDIUM);

    // =========================================================================
    // 11. Verify changes
    // =========================================================================
    console.log('Step 11: Verifying changes');
    await navigateTo(page, 'Parameters');
    await waitForItemList(page);
    await pause(PAUSE_MEDIUM);

    // Refresh the list to show updated data
    await page.getByRole('button', { name: /Refresh/i }).click();
    await waitForItemList(page);
    await pause(PAUSE_MEDIUM);

    const showValues = page.locator('input[type="checkbox"]').first();
    if (!(await showValues.isChecked())) {
      await showValues.check();
    }
    await pause(PAUSE_MEDIUM);

    await clickItemByName(page, '/demo/api/url');
    await pause(PAUSE_LONG);

    // =========================================================================
    // 12. Check version history (if available)
    // =========================================================================
    console.log('Step 12: Checking version history');
    const historyBtn = page.getByRole('button', { name: /History|Log/i });
    if (await historyBtn.isVisible().catch(() => false)) {
      await historyBtn.click();
      await pause(PAUSE_LONG);
    }

    console.log('Demo complete!');
    await pause(PAUSE_LONG);
  });
});
