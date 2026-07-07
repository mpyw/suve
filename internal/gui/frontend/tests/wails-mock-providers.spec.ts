import { test, expect } from '@playwright/test';
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

  test('stash drain routes Google Cloud/Azure names by explicit service, not name shape', async ({ page }) => {
    // Google Cloud/Azure secret names have no leading '/', so the legacy name-shape
    // heuristic would misroute them to secrets. With an explicit service field
    // they route correctly.
    await setupWailsMocks(page, {
      stashFile: {
        exists: true,
        encrypted: false,
        entries: [
          { name: 'gcloud-param-like', operation: 'create', value: 'v', service: 'param' },
          { name: 'gcloud-secret', operation: 'create', value: 'v', service: 'secret' },
        ],
        tags: [],
      },
    });
    await page.goto('/');

    const result = await page.evaluate(async () => {
      const app = (window as any).go.gui.App;
      await app.StagingDrain('', '', false, 'overwrite');
      return await app.StagingStatus();
    });

    expect(result.param.map((e: any) => e.name)).toEqual(['gcloud-param-like']);
    expect(result.secret.map((e: any) => e.name)).toEqual(['gcloud-secret']);
  });
});
