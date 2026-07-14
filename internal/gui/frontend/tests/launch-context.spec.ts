import { test, expect, type Page } from './fixtures/coverage';
import {
  setupWailsMocks,
  createAzureState,
  createAzureNamespaceState,
  getSelectScopeCalls,
  waitForItemList,
} from './fixtures/wails-mock';

// GUI launch-context (--gui) coverage:
//   Bug 1 — launch scope (flags + env) must win over a stale localStorage cache
//           per field, and the footer Namespace dropdown must seed from the
//           effective (launch) namespace on the initial connect.
//   Bug 2 — the launched service (param|secret, via InitialService) must select
//           the initial view.

const nav = (page: Page) => page.locator('.nav');
const nsSelect = (page: Page) => page.locator('.sidebar .namespace-select');

// Seed a localStorage-cached scope for a provider BEFORE the app boots, so
// buildSelection sees it as the cache source.
async function seedCachedScope(
  page: Page,
  provider: string,
  scope: Record<string, string>
): Promise<void> {
  await page.addInitScript(
    ({ key, value }) => {
      localStorage.setItem(key, value);
    },
    { key: `suve.scope.${provider}`, value: JSON.stringify({ provider, ...scope }) }
  );
}

test.describe('GUI launch context (--gui)', () => {
  test.describe('Bug 1 — launch scope wins over the cache', () => {
    test('namespace from the launch scope seeds the footer dropdown (no cache)', async ({
      page,
    }) => {
      // Launch scope carries namespace `dev` (as AZURE_APPCONFIG_NAMESPACE=dev /
      // --namespace dev would); no localStorage cache exists.
      await setupWailsMocks(
        page,
        createAzureNamespaceState({
          currentScope: {
            provider: 'azure',
            projectId: '',
            vaultName: 'my-vault',
            storeName: 'my-store',
            namespace: 'dev',
          },
        })
      );
      await page.goto('/');
      await waitForItemList(page);

      // The footer Namespace dropdown defaults to the launch namespace, not (NULL).
      await expect(nsSelect(page)).toHaveValue('dev');
    });

    test('launch scope beats a stale cache, per field', async ({ page }) => {
      // Stale cache points at different vault/store and an empty namespace.
      await seedCachedScope(page, 'azure', {
        projectId: '',
        vaultName: 'cached-vault',
        storeName: 'cached-store',
        namespace: '',
      });
      // Launch scope (backend) carries the authoritative values.
      await setupWailsMocks(
        page,
        createAzureNamespaceState({
          currentScope: {
            provider: 'azure',
            projectId: '',
            vaultName: 'my-vault',
            storeName: 'my-store',
            namespace: 'dev',
          },
        })
      );
      await page.goto('/');
      await waitForItemList(page);

      // The auto-applied SelectScope uses the launch values, field by field —
      // the cache did not override them.
      const calls = await getSelectScopeCalls(page);
      expect(calls.length).toBeGreaterThan(0);
      expect(calls[0]).toMatchObject({
        provider: 'azure',
        vaultName: 'my-vault',
        storeName: 'my-store',
        namespace: 'dev',
      });
      await expect(nsSelect(page)).toHaveValue('dev');
    });

    test('bare launch (empty launch fields) restores the cached scope', async ({ page }) => {
      // Cache holds the last-used Azure scope.
      await seedCachedScope(page, 'azure', {
        projectId: '',
        vaultName: 'cached-vault',
        storeName: 'cached-store',
        namespace: 'cached-ns',
      });
      // Bare `suve --gui`: backend scope has the provider but empty resource
      // fields (nothing on the command line, nothing in env).
      await setupWailsMocks(
        page,
        createAzureState({
          currentScope: {
            provider: 'azure',
            projectId: '',
            vaultName: '',
            storeName: '',
            namespace: '',
          },
        })
      );
      await page.goto('/');
      await waitForItemList(page);

      // The cache fills every gap the empty launch left.
      const calls = await getSelectScopeCalls(page);
      expect(calls.length).toBeGreaterThan(0);
      expect(calls[0]).toMatchObject({
        provider: 'azure',
        vaultName: 'cached-vault',
        storeName: 'cached-store',
        namespace: 'cached-ns',
      });
    });
  });

  test.describe('Bug 2 — InitialService selects the initial view', () => {
    test('InitialService=secret opens the Key Vault view', async ({ page }) => {
      await setupWailsMocks(page, createAzureState({ initialService: 'secret' }));
      await page.goto('/');
      await waitForItemList(page);

      const kvTab = nav(page).getByRole('button', { name: /Key Vault/i });
      await expect(kvTab).toHaveClass(/active/);
      // The App Configuration (param) tab exists but is NOT the active one.
      await expect(nav(page).getByRole('button', { name: /App Configuration/i })).not.toHaveClass(
        /active/
      );
    });

    test('InitialService=param opens the App Configuration view', async ({ page }) => {
      await setupWailsMocks(page, createAzureState({ initialService: 'param' }));
      await page.goto('/');
      await waitForItemList(page);

      const acTab = nav(page).getByRole('button', { name: /App Configuration/i });
      await expect(acTab).toHaveClass(/active/);
    });
  });
});
