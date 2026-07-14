import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  createParamStagedState,
  createStagedValue,
  navigateTo,
} from './fixtures/wails-mock';

// Regression: the Staging Area must scroll when the staged list is taller than
// the viewport. `.staging-content` is `flex: 1; overflow-y: auto` inside the
// flex-column `.view-container`; without `min-height: 0` the flex item refuses
// to shrink below its content, so it overflows `.main-content` (overflow:hidden)
// and is clipped instead of scrolling.

// Enough staged entries to overflow any reasonable window height.
function manyStagedState() {
  const entries = Array.from({ length: 40 }, (_, i) =>
    createStagedValue(`/app/param-${String(i).padStart(2, '0')}`, 'create', `value-${i}`),
  );
  return createParamStagedState(entries);
}

const stagingContent = (page: Page) => page.locator('.staging-content');

test.describe('Staging Area scrolling', () => {
  test('the staged list scrolls when it overflows the viewport', async ({ page }) => {
    await page.setViewportSize({ width: 1000, height: 600 });
    await setupWailsMocks(page, manyStagedState());
    await page.goto('/');
    await navigateTo(page, 'Staging');

    const content = stagingContent(page);
    await expect(content).toBeVisible();

    // The content must be a bounded scroll container: its scrollable height
    // exceeds its visible height (otherwise nothing can scroll).
    const { scrollHeight, clientHeight } = await content.evaluate((el) => ({
      scrollHeight: el.scrollHeight,
      clientHeight: el.clientHeight,
    }));
    expect(scrollHeight).toBeGreaterThan(clientHeight);

    // And it actually scrolls: setting scrollTop moves it (before the fix the
    // element grew to full height and was clipped, so scrollTop stayed 0).
    await content.evaluate((el) => {
      el.scrollTop = 300;
    });
    expect(await content.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  });
});
