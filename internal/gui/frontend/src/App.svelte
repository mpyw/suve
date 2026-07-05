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

  // ---- Derived capability lookups -------------------------------------------
  const activeProvider = $derived(capabilities.find((c) => c.provider === provider) ?? null);
  const services = $derived(activeProvider?.services ?? []);
  const hasAnyStaging = $derived(services.some((s) => s.hasStaging));
  const isAWS = $derived(provider === 'aws');

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
        await selectProvider(picked);
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

  function buildSelection(p: string): gui.ScopeSelection {
    return {
      provider: p,
      projectId: p === 'googlecloud' ? (scope?.projectId ?? '') : '',
      subscriptionId: p === 'azure' ? (scope?.subscriptionId ?? '') : '',
      resourceGroup: p === 'azure' ? (scope?.resourceGroup ?? '') : '',
      vaultName: p === 'azure' ? (scope?.vaultName ?? '') : '',
      storeName: p === 'azure' ? (scope?.storeName ?? '') : '',
    } as gui.ScopeSelection;
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

  // selectProvider switches the active provider. When the (env-prefilled) scope
  // already has the required fields it applies immediately; otherwise it leaves
  // scopeReady=false so the sidebar shows the scope form.
  async function selectProvider(p: string) {
    provider = p;
    scopeReady = false;
    scopeError = '';
    resetIdentity();

    const sel = buildSelection(p);
    if (hasRequiredScope(sel)) {
      await applyScope(sel);
    }
  }

  // applyScope validates+commits the scope server-side, then (AWS only) loads
  // identity and the staging badge.
  async function applyScope(sel: gui.ScopeSelection) {
    scopeError = '';
    try {
      await SelectScope(sel);
      scope = await GetCurrentScope();
      provider = sel.provider;
      scopeReady = true;
      resetIdentity();
      if (sel.provider === 'aws') {
        await loadAWSIdentity();
        await loadStagingCount();
      }
    } catch (e) {
      scopeReady = false;
      scopeError = parseError(e);
    }
  }

  function resetIdentity() {
    stagingCount = 0;
    accountId = '';
    region = '';
    profile = '';
  }

  function handleSelectProvider(p: string) {
    selectProvider(p);
  }

  function handleSelectScope(sel: gui.ScopeSelection) {
    applyScope(sel);
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
    if (isAWS) loadStagingCount();
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
    {services}
    {hasAnyStaging}
    {scope}
    {scopeReady}
    activeView={effectiveView}
    {stagingCount}
    {accountId}
    {region}
    {profile}
    onnavigate={handleNavigate}
    onselectprovider={handleSelectProvider}
    onselectscope={handleSelectScope}
  />

  <main class="main-content">
    {#if initializing}
      <div class="app-status">Loading…</div>
    {:else if initError}
      <div class="app-status app-error">Failed to initialize: {initError}</div>
    {:else if !provider}
      <div class="app-status">Select a provider to begin.</div>
    {:else if !scopeReady}
      <div class="app-status">
        {scopeError || 'Enter the required scope in the sidebar to continue.'}
      </div>
    {:else}
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
          <StagingView oncountchange={handleStagingCountChange} />
        {/if}
      {/key}
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
