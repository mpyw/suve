<script lang="ts">
  import type { gui } from '../../wailsjs/go/models';

  type ViewKey = 'param' | 'secret' | 'staging';

  interface Props {
    capabilities?: gui.ProviderCapability[];
    provider?: string;
    services?: gui.ServiceCapability[];
    hasAnyStaging?: boolean;
    scope?: gui.ScopeSelection | null;
    scopeReady?: boolean;
    activeView?: ViewKey;
    stagingCount?: number;
    accountId?: string;
    region?: string;
    profile?: string;
    onnavigate?: (view: ViewKey) => void;
    onselectprovider?: (provider: string) => void;
    onselectscope?: (sel: gui.ScopeSelection) => void;
  }

  let {
    capabilities = [],
    provider = '',
    services = [],
    hasAnyStaging = false,
    scope = null,
    scopeReady = false,
    activeView = 'param',
    stagingCount = 0,
    accountId = '',
    region = '',
    profile = '',
    onnavigate,
    onselectprovider,
    onselectscope,
  }: Props = $props();

  // Service key → stable nav icon/letter (labels come from capability names).
  const NAV_ICON: Record<string, string> = { param: 'P', secret: 'S' };

  // Google Cloud project form state (prefilled from the backend's current scope).
  let projectInput = $state('');
  $effect(() => {
    // Re-seed the project field whenever the provider or prefilled scope changes.
    projectInput = provider === 'googlecloud' ? (scope?.projectId ?? '') : '';
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

  function submitProject(e: SubmitEvent) {
    e.preventDefault();
    const projectId = projectInput.trim();
    if (!projectId) return;
    onselectscope?.({
      provider: 'googlecloud',
      projectId,
      subscriptionId: '',
      resourceGroup: '',
      vaultName: '',
      storeName: '',
    } as gui.ScopeSelection);
  }
</script>

<aside class="sidebar">
  <div class="logo">
    <span class="logo-text">suve</span>
    <span class="logo-sub">Secret Unified Versioning Explorer</span>
  </div>

  <!-- Provider selector -->
  <div class="provider-select">
    <label class="provider-label" for="provider-select">Provider</label>
    <select id="provider-select" class="provider-dropdown" value={provider} onchange={handleProviderChange}>
      {#if !provider}
        <option value="" disabled selected>Select provider…</option>
      {/if}
      {#each capabilities as cap}
        <option value={cap.provider} disabled={cap.provider === 'azure'}>
          {cap.displayName}{cap.provider === 'azure' ? ' (coming soon)' : ''}
        </option>
      {/each}
    </select>
  </div>

  <!-- Scope form: only shown when the selected provider still needs input -->
  {#if provider === 'googlecloud' && !scopeReady}
    <form class="scope-form" onsubmit={submitProject}>
      <label class="scope-label" for="gcloud-project">Project ID</label>
      <input
        id="gcloud-project"
        class="scope-input"
        type="text"
        placeholder="my-project"
        bind:value={projectInput}
      />
      <button type="submit" class="scope-submit" disabled={!projectInput.trim()}>Connect</button>
    </form>
  {:else if provider === 'azure' && !scopeReady}
    <div class="scope-hint">Azure scope setup ships next.</div>
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

  <!-- Identity / scope info -->
  {#if scopeReady && provider === 'aws' && accountId && region}
    <div class="aws-info">
      {#if profile}
        <div class="aws-info-row">
          <span class="aws-info-label">Profile</span>
          <span class="aws-info-value aws-info-profile" title={profile}>{profile}</span>
        </div>
      {/if}
      <div class="aws-info-row">
        <span class="aws-info-label">Account</span>
        <span class="aws-info-value" title={accountId}>{accountId}</span>
      </div>
      <div class="aws-info-row">
        <span class="aws-info-label">Region</span>
        <span class="aws-info-value" title={region}>{region}</span>
      </div>
    </div>
  {:else if scopeReady && provider === 'googlecloud' && scope?.projectId}
    <div class="aws-info scope-info">
      <div class="aws-info-row">
        <span class="aws-info-label">Project</span>
        <span class="aws-info-value" title={scope.projectId}>{scope.projectId}</span>
      </div>
    </div>
  {:else if scopeReady && provider === 'azure'}
    <div class="aws-info scope-info">
      {#if scope?.subscriptionId}
        <div class="aws-info-row">
          <span class="aws-info-label">Subscription</span>
          <span class="aws-info-value" title={scope.subscriptionId}>{scope.subscriptionId}</span>
        </div>
      {/if}
      {#if scope?.resourceGroup}
        <div class="aws-info-row">
          <span class="aws-info-label">Resource Group</span>
          <span class="aws-info-value" title={scope.resourceGroup}>{scope.resourceGroup}</span>
        </div>
      {/if}
      {#if scope?.vaultName}
        <div class="aws-info-row">
          <span class="aws-info-label">Key Vault</span>
          <span class="aws-info-value" title={scope.vaultName}>{scope.vaultName}</span>
        </div>
      {/if}
      {#if scope?.storeName}
        <div class="aws-info-row">
          <span class="aws-info-label">App Config</span>
          <span class="aws-info-value" title={scope.storeName}>{scope.storeName}</span>
        </div>
      {/if}
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

  .scope-hint {
    padding: 8px 16px;
    font-size: 11px;
    color: #888;
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
