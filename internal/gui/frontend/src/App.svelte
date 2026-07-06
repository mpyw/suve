<script lang="ts">
  import { onMount } from 'svelte';
  import {
    Capabilities,
    DetectProviders,
    GetAWSIdentity,
    GetCurrentScope,
    InitialProvider,
    SelectScope,
    StagingStatus,
  } from '../wailsjs/go/gui/App';
  import type { gui } from '../wailsjs/go/models';
  import ParamView from './lib/ParamView.svelte';
  import { withRetry } from './lib/retry';
  import SecretView from './lib/SecretView.svelte';
  import Sidebar from './lib/Sidebar.svelte';
  import StagingView from './lib/StagingView.svelte';
  import { parseError } from './lib/viewUtils';

  type ViewKey = 'param' | 'secret' | 'staging';

  // ---- Provider / scope: single source of truth for the whole app ----------
  let capabilities = $state<gui.ProviderCapability[]>([]);
  let provider = $state(''); // '' until resolved/selected → selector prompt
  let scope = $state<gui.ScopeSelection | null>(null);
  let scopeReady = $state(false); // SelectScope resolved for the current provider
  let initializing = $state(true);
  let initError = $state('');
  let scopeError = $state('');

  // ---- View / sidebar state --------------------------------------------------
  let activeView: ViewKey = $state('param');
  let stagingCount = $state(0);
  let accountId = $state('');
  let region = $state('');
  let profile = $state('');

  // pendingProvider is a provider the user selected that still needs scope input:
  // its form shows in the sidebar while the previously-active provider stays
  // mounted, so a rejected/incomplete scope never tears down a working view.
  let pendingProvider = $state('');

  // ---- Derived capability lookups -------------------------------------------
  const activeProvider = $derived(capabilities.find((c) => c.provider === provider) ?? null);
  const allServices = $derived(activeProvider?.services ?? []);
  // Azure enables a service tab only when its scope name is set (vaultName → Key
  // Vault secret; storeName → App Configuration param), so a vault-only user
  // sees no param tab and vice versa. Other providers expose all their services.
  const services = $derived(
    provider === 'azure'
      ? allServices.filter((s) => (s.service === 'secret' ? !!scope?.vaultName : !!scope?.storeName))
      : allServices,
  );
  const hasAnyStaging = $derived(services.some((s) => s.hasStaging));

  // Which provider the sidebar scope form is for, and its prefill values
  // (localStorage-cached last scope, else the backend's current scope).
  const formProvider = $derived(pendingProvider || provider);
  const formPrefill = $derived(
    readCachedScope(formProvider) ?? (scope?.provider === formProvider ? scope : null),
  );

  // scopeKey drives the {#key} full remount: any provider/scope change swaps it,
  // so views re-initialize from scratch and no in-flight response from the old
  // scope can land in the new one.
  const scopeKey = $derived(
    scope
      ? [provider, scope.projectId, scope.subscriptionId, scope.resourceGroup, scope.vaultName, scope.storeName].join('|')
      : provider,
  );

  // effectiveView clamps activeView to a service the provider actually offers
  // (Google Cloud has no param; the Staging tab is hidden when no service supports it),
  // so switching providers never leaves a blank pane.
  const effectiveView = $derived.by((): ViewKey => {
    if (activeView === 'staging') {
      return hasAnyStaging ? 'staging' : ((services[0]?.service as ViewKey) ?? 'param');
    }
    if (services.some((s) => s.service === activeView)) return activeView;
    return (services[0]?.service as ViewKey) ?? 'param';
  });

  const paramCap = $derived(services.find((s) => s.service === 'param') ?? null);
  const secretCap = $derived(services.find((s) => s.service === 'secret') ?? null);

  // ---- Startup: gate before fetch -------------------------------------------
  // No GetAWSIdentity / StagingStatus until the provider is known AND is AWS —
  // this kills the ~5s STS retry storm in non-AWS environments.
  onMount(async () => {
    try {
      capabilities = await withRetry(() => Capabilities());
      scope = await withRetry(() => GetCurrentScope());

      let picked = await withRetry(() => InitialProvider());
      if (!picked) {
        const detected = await withRetry(() => DetectProviders());
        picked = uniqueActiveProvider(detected);
      }

      if (picked) {
        await handleSelectProvider(picked);
      }
    } catch (e) {
      initError = parseError(e);
    } finally {
      initializing = false;
    }
  });

  // uniqueActiveProvider returns the sole provider active across param+secret,
  // or '' when zero or two-plus are active (mirrors the backend rule).
  function uniqueActiveProvider(d: gui.DetectResult): string {
    const active = new Set<string>([...(d.paramActive ?? []), ...(d.secretActive ?? [])]);
    if (active.size !== 1) return '';
    const [only] = active;
    return only ?? '';
  }

  // buildSelection assembles the auto-apply candidate from the best prefill
  // source (localStorage-cached scope, else the backend's env-derived scope).
  function buildSelection(p: string): gui.ScopeSelection {
    const src = readCachedScope(p) ?? (scope?.provider === p ? scope : null);
    return {
      provider: p,
      projectId: p === 'googlecloud' ? (src?.projectId ?? '') : '',
      subscriptionId: p === 'azure' ? (src?.subscriptionId ?? '') : '',
      resourceGroup: p === 'azure' ? (src?.resourceGroup ?? '') : '',
      vaultName: p === 'azure' ? (src?.vaultName ?? '') : '',
      storeName: p === 'azure' ? (src?.storeName ?? '') : '',
    } as gui.ScopeSelection;
  }

  // ---- localStorage persistence of the last-applied scope per provider ------
  function scopeStorageKey(p: string): string {
    return `suve.scope.${p}`;
  }

  function readCachedScope(p: string): gui.ScopeSelection | null {
    if (!p || typeof localStorage === 'undefined') return null;
    try {
      const raw = localStorage.getItem(scopeStorageKey(p));
      return raw ? (JSON.parse(raw) as gui.ScopeSelection) : null;
    } catch {
      return null;
    }
  }

  function persistScope(sel: gui.ScopeSelection): void {
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(scopeStorageKey(sel.provider), JSON.stringify(sel));
    } catch {
      // localStorage may be unavailable (private mode); persistence is best-effort.
    }
  }

  function hasRequiredScope(sel: gui.ScopeSelection): boolean {
    switch (sel.provider) {
      case 'aws':
        return true;
      case 'googlecloud':
        return !!sel.projectId;
      case 'azure':
        return !!sel.vaultName || !!sel.storeName;
      default:
        return false;
    }
  }

  // handleSelectProvider is invoked from the provider dropdown (and at startup).
  // If the prefilled scope already satisfies the provider it applies at once;
  // otherwise it parks the choice in pendingProvider so the sidebar shows the
  // scope form WITHOUT tearing down the currently-active provider's views.
  async function handleSelectProvider(p: string) {
    scopeError = '';

    const sel = buildSelection(p);
    if (hasRequiredScope(sel)) {
      await applyScope(sel);
    } else {
      pendingProvider = p;
    }
  }

  // handleSelectScope is invoked when a scope form is submitted.
  async function handleSelectScope(sel: gui.ScopeSelection) {
    await applyScope(sel);
  }

  // handleCancelScope dismisses a pending scope form (Escape), returning to the
  // active provider (or the "select a provider" prompt when none is active).
  function handleCancelScope() {
    pendingProvider = '';
    scopeError = '';
  }

  // applyScope validates+commits the scope server-side, then (AWS only) loads
  // identity and the staging badge. On success it switches the active provider
  // and clears the pending form; on rejection it leaves the previous provider
  // active (its lists keep working) and surfaces the error in the form.
  async function applyScope(sel: gui.ScopeSelection): Promise<void> {
    scopeError = '';
    try {
      await SelectScope(sel);
      scope = await GetCurrentScope();
      provider = sel.provider;
      pendingProvider = '';
      scopeReady = true;
      persistScope(sel);
      resetIdentity();
      // AWS identity is AWS-only; the staging badge is scope-keyed for every
      // provider (StagingStatus resolves the scope without STS off-AWS).
      if (sel.provider === 'aws') {
        await loadAWSIdentity();
      }
      await loadStagingCount();
    } catch (e) {
      scopeError = parseError(e);
    }
  }

  function resetIdentity() {
    stagingCount = 0;
    accountId = '';
    region = '';
    profile = '';
  }

  function handleNavigate(view: ViewKey) {
    activeView = view;
  }

  function handleStagingCountChange(count: number) {
    stagingCount = count;
  }

  async function loadStagingCount() {
    // Steady-state, AWS-only: no retry wrapping (a real failure must not hammer).
    try {
      const staged = await StagingStatus();
      stagingCount = (staged?.param?.length ?? 0) + (staged?.secret?.length ?? 0);
    } catch {
      stagingCount = 0;
    }
  }

  function handleStagingChange() {
    if (scopeReady) loadStagingCount();
  }

  async function loadAWSIdentity() {
    try {
      const identity = await GetAWSIdentity();
      accountId = identity?.accountId ?? '';
      region = identity?.region ?? '';
      profile = identity?.profile ?? '';
    } catch {
      accountId = '';
      region = '';
      profile = '';
    }
  }
</script>

<div class="app">
  <Sidebar
    {capabilities}
    {provider}
    {pendingProvider}
    {services}
    {hasAnyStaging}
    {scope}
    {scopeReady}
    formScope={formPrefill}
    {scopeError}
    activeView={effectiveView}
    {stagingCount}
    {accountId}
    {region}
    {profile}
    onnavigate={handleNavigate}
    onselectprovider={handleSelectProvider}
    onselectscope={handleSelectScope}
    oncancelscope={handleCancelScope}
  />

  <main class="main-content">
    {#if initializing}
      <div class="app-status">Loading…</div>
    {:else if initError}
      <div class="app-status app-error">Failed to initialize: {initError}</div>
    {:else if provider && scopeReady}
      {#key scopeKey}
        {#if effectiveView === 'param' && paramCap}
          <ParamView
            capability={paramCap}
            onnavigatetostaging={() => handleNavigate('staging')}
            onstagingchange={handleStagingChange}
          />
        {:else if effectiveView === 'secret' && secretCap}
          <SecretView
            capability={secretCap}
            onnavigatetostaging={() => handleNavigate('staging')}
            onstagingchange={handleStagingChange}
          />
        {:else if effectiveView === 'staging' && hasAnyStaging}
          <StagingView {services} oncountchange={handleStagingCountChange} />
        {/if}
      {/key}
    {:else if pendingProvider}
      <div class="app-status">
        {scopeError || 'Enter the required scope in the sidebar to continue.'}
      </div>
    {:else}
      <div class="app-status">Select a provider to begin.</div>
    {/if}
  </main>
</div>

<style>
  .app {
    display: flex;
    height: 100vh;
    width: 100vw;
    overflow: hidden;
  }

  .main-content {
    flex: 1;
    overflow: hidden;
  }

  .app-status {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    padding: 24px;
    color: #a0a0a0;
    font-size: 14px;
    text-align: center;
  }

  .app-error {
    color: #e94560;
  }
</style>
