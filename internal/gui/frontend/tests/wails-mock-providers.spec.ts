import { test, expect } from './fixtures/coverage';
import { setupWailsMocks } from './fixtures/wails-mock';

// Infrastructure test (not a feature spec): it exercises the provider-aware
// mock bindings added for the multi-cloud GUI work (#269), so a regression in
// the fixture is caught before the selector UI (#266) depends on them. It calls
// the mock directly via page.evaluate and does not touch the UI, so it stays
// valid regardless of whether App.svelte calls these bindings yet.
test.describe('wails-mock provider bindings', () => {
  test('new bindings return AWS-only defaults', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');

    const result = await page.evaluate(async () => {
      const app = (window as any).go.gui.App;
      return {
        initial: await app.InitialProvider(),
        scope: await app.GetCurrentScope(),
        detect: await app.DetectProviders(),
        caps: await app.Capabilities(),
        types: await app.ParamTypeOptions(),
      };
    });

    expect(result.initial).toBe('aws');
    expect(result.scope.provider).toBe('aws');
    expect(result.detect.secret).toBe('aws');
    expect(result.detect.secretActive).toEqual(['aws']);
    expect(result.caps.map((c: any) => c.provider)).toEqual(['aws', 'googlecloud', 'azure']);
    // Staging is AWS-only until multi-provider staging lands.
    const awsSecret = result.caps[0].services.find((s: any) => s.service === 'secret');
    expect(awsSecret.hasStaging).toBe(true);
    expect(awsSecret.hasRecoveryWindow).toBe(true);
    const gcloudSecret = result.caps[1].services[0];
    expect(gcloudSecret.hasStaging).toBe(true); // multi-provider staging (#270)
    expect(gcloudSecret.hasForceDelete).toBe(false); // force-delete stays AWS-only
    expect(result.types).toContain('SecureString');
  });

  test('SelectScope validates required fields and updates current scope', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');

    const result = await page.evaluate(async () => {
      const app = (window as any).go.gui.App;
      const errors: string[] = [];
      try {
        await app.SelectScope({ provider: 'googlecloud', projectId: '' });
      } catch (e: any) {
        errors.push(e.message);
      }
      try {
        await app.SelectScope({ provider: 'azure' });
      } catch (e: any) {
        errors.push(e.message);
      }
      await app.SelectScope({ provider: 'googlecloud', projectId: 'p1' });
      const scope = await app.GetCurrentScope();
      const types = await app.ParamTypeOptions();

      return { errors, scope, types };
    });

    expect(result.errors).toHaveLength(2);
    expect(result.errors[0]).toContain('Google Cloud project ID is required');
    expect(result.errors[1]).toContain('Azure requires');
    expect(result.scope.provider).toBe('googlecloud');
    expect(result.scope.projectId).toBe('p1');
    // Non-AWS providers expose no SSM value types.
    expect(result.types).toEqual([]);
  });

  test('SelectScope accepts a one-sided Azure scope (only one service available)', async ({ page }) => {
    await setupWailsMocks(page);
    await page.goto('/');

    const result = await page.evaluate(async () => {
      const app = (window as any).go.gui.App;

      // Key Vault (secret) only — App Configuration (param) absent.
      await app.SelectScope({ provider: 'azure', vaultName: 'v' });
      const vaultOnly = await app.GetCurrentScope();

      // App Configuration (param) only — Key Vault (secret) absent.
      await app.SelectScope({ provider: 'azure', storeName: 'c' });
      const storeOnly = await app.GetCurrentScope();

      return { vaultOnly, storeOnly };
    });

    expect(result.vaultOnly.vaultName).toBe('v');
    expect(result.vaultOnly.storeName).toBe('');
    expect(result.storeOnly.storeName).toBe('c');
    expect(result.storeOnly.vaultName).toBe('');
  });

  test('export/import round-trips a single concrete service through the virtual file system', async ({ page }) => {
    // Export writes a per-service envelope keyed by path; import reads it back
    // into the same service. Each call carries a concrete service (never the
    // combined scope), so a secret cannot leak into the param bucket.
    await setupWailsMocks(page, {
      stagedParam: [{ name: '/p1', operation: 'create', value: 'v' }],
      stagedSecret: [{ name: 's1', operation: 'create', value: 'v' }],
      savePath: '/mock/secret.json',
    });
    await page.goto('/');

    const result = await page.evaluate(async () => {
      const app = (window as any).go.gui.App;
      // Export only the secret service; the param bucket is untouched.
      const exp = await app.StagingExport('/mock/secret.json', 'secret', '', false);
      const info = await app.InspectImportFile('/mock/secret.json');
      const afterExport = await app.StagingStatus();
      // Re-import the secret file back into the secret service.
      const imp = await app.StagingImport('/mock/secret.json', 'secret', '', 'merge');
      const afterImport = await app.StagingStatus();
      return { exp, info, afterExport, imp, afterImport };
    });

    expect(result.exp.entryCount).toBe(1);
    expect(result.info.service).toBe('secret');
    expect(result.info.scopeMatches).toBe(true);
    // Export cleared the secret bucket but left param staged.
    expect(result.afterExport.secret.map((e: any) => e.name)).toEqual([]);
    expect(result.afterExport.param.map((e: any) => e.name)).toEqual(['/p1']);
    // Import restored the secret; it stayed a secret (never routed to param).
    expect(result.afterImport.secret.map((e: any) => e.name)).toEqual(['s1']);
    expect(result.afterImport.param.map((e: any) => e.name)).toEqual(['/p1']);
  });

  test('service mismatch on import is a hard error', async ({ page }) => {
    await setupWailsMocks(page, {
      files: {
        '/mock/secret.json': {
          provider: 'aws',
          scope: 'aws/123456789012/ap-northeast-1',
          service: 'secret',
          encrypted: false,
          entries: [{ name: 's1', operation: 'create', value: 'v' }],
          tags: [],
        },
      },
    });
    await page.goto('/');

    const err = await page.evaluate(async () => {
      const app = (window as any).go.gui.App;
      try {
        await app.StagingImport('/mock/secret.json', 'param', '', 'merge');
        return null;
      } catch (e: any) {
        return e.message as string;
      }
    });

    expect(err).toContain('secret');
    expect(err).toContain('param');
  });
});
