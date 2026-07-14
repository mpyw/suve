import { CoverageReport } from 'monocart-coverage-reports';
import { coverageOptions } from './config';

// Playwright globalTeardown: merge every worker's cached V8 coverage into the
// configured reports (lcov.info under outputDir). Playwright runs each worker in
// its own process, so the per-test fixture's `add()` calls write to monocart's
// shared on-disk cache; this single `generate()` merges them. No-op unless
// COVERAGE=1.
export default async function globalTeardown() {
  if (process.env.COVERAGE !== '1') {
    return;
  }
  const report = new CoverageReport(coverageOptions);
  await report.generate();
}
