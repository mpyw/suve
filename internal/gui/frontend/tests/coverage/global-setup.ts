import { CoverageReport } from 'monocart-coverage-reports';
import { coverageOptions } from './config';

// Playwright globalSetup: clear monocart's shared V8 cache before the run so a
// previous run's coverage can't leak into this one. No-op unless COVERAGE=1.
export default async function globalSetup() {
  if (process.env.COVERAGE !== '1') {
    return;
  }
  const report = new CoverageReport(coverageOptions);
  report.cleanCache();
}
