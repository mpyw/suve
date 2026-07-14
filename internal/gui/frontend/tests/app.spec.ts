import { test, expect } from './fixtures/coverage';
import { setupWailsMocks, type MockState, createAWSIdentityState, createNoAWSIdentityState } from './fixtures/wails-mock';

test.describe('App Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');
  });

  test('should display the app logo', async ({ page }) => {
    await expect(page.locator('.logo-text')).toContainText('suve');
  });

  test('should show sidebar with navigation items', async ({ page }) => {
    await expect(page.locator('.nav').getByRole('button', { name: /Param/i })).toBeVisible();
    await expect(page.locator('.nav').getByRole('button', { name: /Secret/i })).toBeVisible();
    await expect(page.locator('.nav').getByRole('button', { name: /Staging/i })).toBeVisible();
  });

  test('should highlight active navigation item', async ({ page }) => {
    // Parameters should be active by default
    const paramsBtn = page.locator('.nav').getByRole('button', { name: /Param/i });
    await expect(paramsBtn).toHaveClass(/active/);

    // Click on Secrets and verify it becomes active
    await page.locator('.nav').getByRole('button', { name: /Secret/i }).click();
    await expect(page.locator('.nav').getByRole('button', { name: /Secret/i })).toHaveClass(/active/);

    // Click on Staging and verify it becomes active
    await page.locator('.nav').getByRole('button', { name: /Staging/i }).click();
    await expect(page.locator('.nav').getByRole('button', { name: /Staging/i })).toHaveClass(/active/);
  });

  test('should persist active view when refreshing data', async ({ page }) => {
    // Navigate to Secrets
    await page.locator('.nav').getByRole('button', { name: /Secret/i }).click();
    await expect(page.locator('.nav').getByRole('button', { name: /Secret/i })).toHaveClass(/active/);

    // Click refresh - view should stay on Secrets
    const refreshBtn = page.getByRole('button', { name: /Refresh/i });
    await refreshBtn.click();
    await expect(page.locator('.nav').getByRole('button', { name: /Secret/i })).toHaveClass(/active/);
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
    await page.locator('.nav').getByRole('button', { name: /Secret/i }).click();
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
    await page.locator('.nav').getByRole('button', { name: /Secret/i }).click();
    await page.waitForSelector('.filter-bar');

    await expect(page.locator('.item-button')).toHaveCount(0);
  });
});

test.describe('AWS Identity Display', () => {
  test('should display AWS identity with profile in sidebar', async ({ page }) => {
    await setupWailsMocks(page, createAWSIdentityState('123456789012', 'ap-northeast-1', 'production'));
    await page.goto('/');

    // Wait for sidebar to be visible
    await expect(page.locator('.sidebar')).toBeVisible();

    // Check AWS info section is displayed
    const awsInfo = page.locator('.aws-info');
    await expect(awsInfo).toBeVisible();

    // Check profile is displayed
    await expect(awsInfo.locator('.aws-info-profile')).toContainText('production');

    // Check account is displayed
    await expect(awsInfo).toContainText('123456789012');

    // Check region is displayed
    await expect(awsInfo).toContainText('ap-northeast-1');
  });

  test('renders "?" for the profile when none is set', async ({ page }) => {
    await setupWailsMocks(page, createAWSIdentityState('987654321098', 'us-east-1', ''));
    await page.goto('/');

    const awsInfo = page.locator('.aws-info');
    await expect(awsInfo).toBeVisible();

    // Profile is optional; it renders as "?" instead of being hidden.
    await expect(awsInfo.locator('.aws-info-profile')).toHaveText('?');
    // Account and region still show their values.
    await expect(awsInfo).toContainText('987654321098');
    await expect(awsInfo).toContainText('us-east-1');
  });

  test('renders the AWS panel with "?" everywhere when identity is unavailable', async ({ page }) => {
    await setupWailsMocks(page, createNoAWSIdentityState());
    await page.goto('/');

    const awsInfo = page.locator('.aws-info');
    // Symmetric with Google Cloud / Azure: the panel always renders, with "?"
    // for every unresolved value rather than being hidden.
    await expect(awsInfo).toBeVisible();
    await expect(awsInfo.getByText('?', { exact: true })).toHaveCount(3);
  });

  test('renders the AWS panel with "?" when GetAWSIdentity fails', async ({ page }) => {
    await setupWailsMocks(page, {
      simulateError: { operation: 'GetAWSIdentity', message: 'No credentials found' },
    });
    await page.goto('/');

    const awsInfo = page.locator('.aws-info');
    await expect(awsInfo).toBeVisible();
    await expect(awsInfo.getByText('?', { exact: true })).toHaveCount(3);
  });

  test('renders "?" for region when only the account id is available', async ({ page }) => {
    await setupWailsMocks(page, createAWSIdentityState('123456789012', '', ''));
    await page.goto('/');

    const awsInfo = page.locator('.aws-info');
    await expect(awsInfo).toBeVisible();
    await expect(awsInfo).toContainText('123456789012'); // account shown
    await expect(awsInfo.getByText('?', { exact: true })).toHaveCount(2); // region + profile
  });

  test('renders "?" for the account id when only the region is available', async ({ page }) => {
    await setupWailsMocks(page, createAWSIdentityState('', 'ap-northeast-1', ''));
    await page.goto('/');

    const awsInfo = page.locator('.aws-info');
    await expect(awsInfo).toBeVisible();
    await expect(awsInfo).toContainText('ap-northeast-1'); // region shown
    await expect(awsInfo.getByText('?', { exact: true })).toHaveCount(2); // account + profile
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
    await page.locator('.nav').getByRole('button', { name: /Secret/i }).click();
    await expect(page.locator('.nav').getByRole('button', { name: /Secret/i })).toHaveClass(/active/);
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
    await page.locator('.nav').getByRole('button', { name: /Staging/i }).click();
    await expect(page.locator('.nav').getByRole('button', { name: /Staging/i })).toHaveClass(/active/);
  });
});
