<script lang="ts">
  import type { gui } from '../../wailsjs/go/models';

  type ViewKey = 'param' | 'secret' | 'staging';

  interface Props {
    capabilities?: gui.ProviderCapability[];
    provider?: string;
    pendingProvider?: string;
    services?: gui.ServiceCapability[];
    hasAnyStaging?: boolean;
    scope?: gui.ScopeSelection | null;
    scopeReady?: boolean;
    formScope?: gui.ScopeSelection | null;
    scopeError?: string;
    activeView?: ViewKey;
    stagingCount?: number;
    accountId?: string;
    region?: string;
    profile?: string;
    onnavigate?: (view: ViewKey) => void;
    onselectprovider?: (provider: string) => void;
    onselectscope?: (sel: gui.ScopeSelection) => void;
    oncancelscope?: () => void;
    onchangescope?: () => void;
  }

  let {
    capabilities = [],
    provider = '',
    pendingProvider = '',
    services = [],
    hasAnyStaging = false,
    scope = null,
    scopeReady = false,
    formScope = null,
    scopeError = '',
    activeView = 'param',
    stagingCount = 0,
    accountId = '',
    region = '',
    profile = '',
    onnavigate,
    onselectprovider,
    onselectscope,
    oncancelscope,
    onchangescope,
  }: Props = $props();

  // Service key → stable nav icon/letter (labels come from capability names).
  const NAV_ICON: Record<string, string> = { param: 'P', secret: 'S' };

  // ---- Scope-form inputs (seeded from the prefill for the pending provider) --
  let projectInput = $state('');
  let vaultInput = $state('');
  let storeInput = $state('');
  let namespaceInput = $state('');
  let formError = $state('');
  let firstFieldEl: HTMLInputElement | undefined = $state();

  $effect(() => {
    // Re-seed the inputs whenever the pending provider (or its prefill) changes.
    const s = formScope;
    projectInput = pendingProvider === 'googlecloud' ? (s?.projectId ?? '') : '';
    vaultInput = pendingProvider === 'azure' ? (s?.vaultName ?? '') : '';
    storeInput = pendingProvider === 'azure' ? (s?.storeName ?? '') : '';
    namespaceInput = pendingProvider === 'azure' ? (s?.namespace ?? '') : '';
    formError = '';
  });

  // Focus the first field when a scope form opens (a11y).
  $effect(() => {
    if (pendingProvider && firstFieldEl) {
      firstFieldEl.focus();
    }
  });

  // Move keyboard focus to the active tab after a provider switch, so a clamped
  // view (e.g. Google Cloud dropping the Param tab) doesn't strand focus on a gone tab.
  let navEl: HTMLElement | undefined = $state();
  let lastProvider = '';
  $effect(() => {
    if (provider !== lastProvider && scopeReady && navEl) {
      lastProvider = provider;
      const active = navEl.querySelector<HTMLButtonElement>('.nav-item.active');
      active?.focus();
    }
  });

  function navigate(view: ViewKey) {
    onnavigate?.(view);
  }

  function handleProviderChange(e: Event) {
    const value = (e.currentTarget as HTMLSelectElement).value;
    onselectprovider?.(value);
  }

  // Escape while a scope form is open cancels the pending selection.
  function handleWindowKeydown(e: KeyboardEvent) {
    if (pendingProvider && e.key === 'Escape') {
      e.preventDefault();
      oncancelscope?.();
    }
  }

  // Submitting empty is intentional: the parent treats a no-scope submission as
  // "disconnect + clear", so Connect stays enabled and there is no required-field
  // guard here.
  function submitProject(e: SubmitEvent) {
    e.preventDefault();
    onselectscope?.({
      provider: 'googlecloud',
      projectId: projectInput.trim(),
      vaultName: '',
      storeName: '',
      namespace: '',
    } as gui.ScopeSelection);
  }

  function submitAzure(e: SubmitEvent) {
    e.preventDefault();
    onselectscope?.({
      provider: 'azure',
      projectId: '',
      vaultName: vaultInput.trim(),
      storeName: storeInput.trim(),
      namespace: namespaceInput.trim(),
    } as gui.ScopeSelection);
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<aside class="sidebar">
  <div class="logo">
    <span class="logo-text">suve</span>
    <span class="logo-sub">Secret Unified Versioning Explorer</span>
  </div>

  <!-- Provider selector -->
  <div class="provider-select">
    <label class="provider-label" for="provider-select">Provider</label>
    <select id="provider-select" class="provider-dropdown" value={pendingProvider || provider} onchange={handleProviderChange}>
      {#if !provider && !pendingProvider}
        <option value="" disabled selected>Select provider…</option>
      {/if}
      {#each capabilities as cap}
        <option value={cap.provider}>{cap.displayName}</option>
      {/each}
    </select>
  </div>

  <!-- Scope form: shown while a selected provider still needs input -->
  {#if pendingProvider === 'googlecloud'}
    <form class="scope-form" onsubmit={submitProject}>
      <label class="scope-label" for="gcloud-project">Project ID</label>
      <input
        id="gcloud-project"
        class="scope-input"
        type="text"
        placeholder="my-project"
        bind:value={projectInput}
        bind:this={firstFieldEl}
      />
      {#if formError || scopeError}
        <div class="scope-error">{formError || scopeError}</div>
      {/if}
      <button type="submit" class="scope-submit">Connect</button>
    </form>
  {:else if pendingProvider === 'azure'}
    <form class="scope-form" onsubmit={submitAzure}>
      <label class="scope-label" for="azure-vault">Key Vault name</label>
      <input
        id="azure-vault"
        class="scope-input"
        type="text"
        placeholder="my-vault (secrets)"
        bind:value={vaultInput}
        bind:this={firstFieldEl}
      />
      <label class="scope-label" for="azure-store">App Configuration store</label>
      <input id="azure-store" class="scope-input" type="text" placeholder="my-store (params)" bind:value={storeInput} />
      <label class="scope-label" for="azure-namespace">Namespace</label>
      <input
        id="azure-namespace"
        class="scope-input"
        type="text"
        placeholder="empty = default"
        bind:value={namespaceInput}
      />
      <p class="scope-hint">Azure calls this a label; empty = default. Applies to the App Configuration store only.</p>
      {#if formError || scopeError}
        <div class="scope-error">{formError || scopeError}</div>
      {/if}
      <button type="submit" class="scope-submit">Connect</button>
    </form>
  {/if}

  <!-- Navigation tabs: capability-driven, only once a scope is active -->
  {#if scopeReady}
    <nav class="nav" bind:this={navEl}>
      {#each services as svc}
        <button
          class="nav-item"
          class:active={activeView === svc.service}
          onclick={() => navigate(svc.service as ViewKey)}
        >
          <span class="nav-icon">{NAV_ICON[svc.service] ?? svc.displayName.charAt(0)}</span>
          <span class="nav-label" title={svc.displayName}>{svc.displayName}</span>
        </button>
      {/each}

      {#if hasAnyStaging}
        <button
          class="nav-item"
          class:active={activeView === 'staging'}
          onclick={() => navigate('staging')}
        >
          <span class="nav-icon">*</span>
          <span class="nav-label">Staging</span>
          {#if stagingCount > 0}
            <span class="staging-count">{stagingCount}</span>
          {/if}
        </button>
      {/if}
    </nav>
  {/if}

  <!-- Identity / scope info (hidden while a scope form is pending) -->
  {#if provider === 'aws'}
    <!-- Gated by provider only, symmetric with Google Cloud / Azure: every row
         is always rendered, showing "?" when unset (e.g. identity unavailable).
         AWS has no editable scope form (region/creds come from the ambient AWS
         config), so there is no Change scope button. -->
    <div class="aws-info">
      <div class="aws-info-row">
        <span class="aws-info-label">Profile</span>
        <span class="aws-info-value aws-info-profile" title={profile || '?'}>{profile || '?'}</span>
      </div>
      <div class="aws-info-row">
        <span class="aws-info-label">Account</span>
        <span class="aws-info-value" title={accountId || '?'}>{accountId || '?'}</span>
      </div>
      <div class="aws-info-row">
        <span class="aws-info-label">Region</span>
        <span class="aws-info-value" title={region || '?'}>{region || '?'}</span>
      </div>
    </div>
  {:else if provider === 'googlecloud'}
    <!-- Gated by provider only: every row is always rendered ("?" when unset).
         Change scope is always present and only disabled while a form is pending
         (so it stays reachable in an errored/partial state). -->
    <div class="aws-info scope-info">
      <div class="aws-info-row">
        <span class="aws-info-label">Project</span>
        <span class="aws-info-value" title={scope?.projectId || '?'}>{scope?.projectId || '?'}</span>
      </div>
      <button type="button" class="scope-change" disabled={!!pendingProvider} onclick={() => onchangescope?.()}>Change scope</button>
    </div>
  {:else if provider === 'azure'}
    <div class="aws-info scope-info">
      <div class="aws-info-row">
        <span class="aws-info-label">Key Vault</span>
        <span class="aws-info-value" title={scope?.vaultName || '?'}>{scope?.vaultName || '?'}</span>
      </div>
      <div class="aws-info-row">
        <span class="aws-info-label">App Config</span>
        <span class="aws-info-value" title={scope?.storeName || '?'}>{scope?.storeName || '?'}</span>
      </div>
      {#if scope?.storeName}
        <div class="aws-info-row">
          <span class="aws-info-label">Namespace</span>
          <span class="aws-info-value" title={scope?.namespace || 'default'}>{scope?.namespace || 'default'}</span>
        </div>
      {/if}
      <button type="button" class="scope-change" disabled={!!pendingProvider} onclick={() => onchangescope?.()}>Change scope</button>
    </div>
  {/if}
</aside>

<style>
  .sidebar {
    width: 200px;
    height: 100%;
    background: #1a1a2e;
    display: flex;
    flex-direction: column;
    border-right: 1px solid #2d2d44;
  }

  .logo {
    padding: 20px 16px;
    border-bottom: 1px solid #2d2d44;
  }

  .logo-text {
    font-size: 24px;
    font-weight: bold;
    color: #e94560;
    display: block;
  }

  .logo-sub {
    font-size: 10px;
    color: #666;
    display: block;
    margin-top: 2px;
  }

  .provider-select {
    padding: 12px 16px 4px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .provider-label {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #666;
  }

  .provider-dropdown {
    width: 100%;
    box-sizing: border-box;
    padding: 6px 8px;
    background: #252542;
    color: #fff;
    border: 1px solid #2d2d44;
    border-radius: 6px;
    font-size: 13px;
  }

  .scope-form {
    padding: 8px 16px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .scope-label {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #666;
  }


  .scope-input {
    width: 100%;
    box-sizing: border-box;
    padding: 6px 8px;
    background: #252542;
    color: #fff;
    border: 1px solid #2d2d44;
    border-radius: 6px;
    font-size: 13px;
  }

  .scope-submit {
    padding: 6px 8px;
    background: #e94560;
    color: #fff;
    border: none;
    border-radius: 6px;
    font-size: 13px;
    cursor: pointer;
  }

  .scope-submit:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .scope-error {
    font-size: 11px;
    color: #e94560;
    line-height: 1.4;
  }

  .scope-hint {
    margin: 0;
    font-size: 10px;
    color: #666;
    line-height: 1.4;
  }

  .scope-change {
    margin-top: 8px;
    padding: 4px 8px;
    background: transparent;
    color: #8a8aa0;
    border: 1px solid #2d2d44;
    border-radius: 6px;
    font-size: 11px;
    cursor: pointer;
  }

  .scope-change:hover:not(:disabled) {
    color: #fff;
    border-color: #3d3d5c;
  }

  .scope-change:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .nav {
    padding: 12px 8px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 12px;
    border: none;
    background: transparent;
    color: #a0a0a0;
    cursor: pointer;
    border-radius: 6px;
    transition: all 0.2s;
    text-align: left;
    font-size: 14px;
  }

  .nav-item:hover {
    background: #252542;
    color: #fff;
  }

  .nav-item.active {
    background: #e94560;
    color: #fff;
  }

  .nav-icon {
    width: 24px;
    height: 24px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 4px;
    font-weight: bold;
    font-size: 12px;
  }

  .nav-label {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .staging-count {
    min-width: 18px;
    height: 18px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 10px;
    font-weight: bold;
    background: #e94560;
    color: #fff;
    border-radius: 50%;
    padding: 0 4px;
  }

  .nav-item.active .staging-count {
    background: #fff;
    color: #e94560;
  }

  .aws-info {
    margin-top: auto;
    padding: 12px 16px;
    border-top: 1px solid #2d2d44;
    font-size: 11px;
  }

  .aws-info-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    padding: 4px 0;
  }

  .aws-info-label {
    color: #666;
    flex-shrink: 0;
  }

  .aws-info-value {
    color: #a0a0a0;
    font-family: monospace;
    font-size: 10px;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .aws-info-profile {
    color: #e94560;
    font-weight: bold;
  }
</style>
