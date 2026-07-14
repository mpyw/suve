import { CoverageReport } from 'monocart-coverage-reports';
import { test as base, expect, type Page } from '@playwright/test';
import { coverageOptions } from '../coverage/config';

// Every spec imports `test`/`expect` from here rather than from
// `@playwright/test`, so a single shared fixture wraps the `page` for the whole
// suite. When COVERAGE=1 (and only on Chromium, since V8 coverage via CDP is
// Chromium-only), the wrapped `page` collects JS coverage across the test and
// hands it to monocart's shared cache; a plain `npm test` run is unaffected.
const COLLECT = process.env.COVERAGE === '1';

export const test = base.extend({
  page: async ({ page, browserName }, use) => {
    const collect = COLLECT && browserName === 'chromium';
    if (collect) {
      await page.coverage.startJSCoverage({ resetOnNavigation: false });
    }

    await use(page);

    if (collect) {
      const coverage = await page.coverage.stopJSCoverage();
      const report = new CoverageReport(coverageOptions);
      await report.add(coverage);
    }
  },
});

export { expect, type Page };
