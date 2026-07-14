// Shared monocart-coverage-reports options, used by both the Playwright
// global setup/teardown (cache clean + report generate) and the per-test
// fixture that feeds V8 coverage into the shared cache.
//
// Coverage is collected only when COVERAGE=1 (see the fixture and the global
// hooks); a normal `npm test` run never touches monocart.
//
// Mapping V8 coverage back to the app's own sources needs two adjustments for
// a Vite dev server:
//   * entryFilter keeps only the app modules Vite serves under `/src/` — this
//     drops the Svelte runtime (node_modules/.vite/deps/*, whose own source
//     maps resolve to `svelte/src/**`), @vite/client, Vite chunks, clsx, the
//     generated wailsjs bindings, and the extracted CSS — before unpacking.
//   * sourcePath rebuilds the real repo-relative path. Vite dev source maps
//     unpack each module to a bare basename (e.g. `Modal.svelte`), losing its
//     directory; since each dev entry is 1:1 with its source, the entry URL
//     (info.distFile) carries the true `src/**` path.
export const coverageOptions = {
  name: 'GUI Frontend Coverage',
  outputDir: './coverage',
  reports: ['lcovonly'],
  entryFilter: (entry: { url: string }) => {
    const url = entry.url;
    if (!url.includes('/src/')) {
      return false;
    }
    // Drop stylesheets and Svelte's extracted <style> blocks; only .svelte/.ts
    // logic is meaningful line coverage.
    return !/\.css(\?|$)|type=style|lang\.css/.test(url);
  },
  sourcePath: (filePath: string, info: { distFile?: string }) => {
    const dist = info?.distFile ?? filePath;
    const match = dist.match(/src\/.+$/);
    const resolved = match ? match[0] : filePath;
    // Strip any Vite query string (?t=..., ?svelte&...).
    return resolved.replace(/[?].*$/, '');
  },
};
