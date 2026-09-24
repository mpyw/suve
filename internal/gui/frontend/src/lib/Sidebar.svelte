<script lang="ts">
  import type { capability, gui } from '../../wailsjs/go/models';
  import { scopeFieldSpecs, scopeFieldValue, selectionFor } from './scopeFields';

  type ViewKey = 'param' | 'secret' | 'staging';

  interface Props {
    capabilities?: capability.ProviderCapability[];
    provider?: string;
    pendingProvider?: string;
    services?: capability.ServiceCapability[];
    hasAnyStaging?: boolean;
    scope?: gui.ScopeSelection | null;
    scopeReady?: boolean;
    formScope?: gui.ScopeSelection | null;
    scopeError?: string;
    activeView?: ViewKey;
    stagingCount?: number;
    // What the active scope points at, in display order (see GetScopeTarget).
    target?: gui.ScopeTarget | null;
    // Azure App Configuration namespace filter (footer dropdown). namespaceOptions
    // are the choices ((NULL), discovered, *); selectedNamespace is the current
    // value; onchangenamespace reports a new selection. Client-side only — this
    // does NOT re-scope.
    namespaceOptions?: string[];
    selectedNamespace?: string;
    onnavigate?: (view: ViewKey) => void;
    onselectprovider?: (provider: string) => void;
    onselectscope?: (sel: gui.ScopeSelection) => void;
    oncancelscope?: () => void;
    onchangescope?: () => void;
    onchangenamespace?: (ns: string) => void;
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
    target = null,
    namespaceOptions = [],
    selectedNamespace = '',
    onnavigate,
    onselectprovider,
    onselectscope,
    oncancelscope,
    onchangescope,
    onchangenamespace,
  }: Props = $props();

  // A provider with scope inputs (a scope form) offers "Change scope"; one that
  // reads its scope from the ambient config (AWS) has nothing to change.
  const activeCap = $derived(capabilities.find((c) => c.provider === provider) ?? null);
  const hasScopeForm = $derived((activeCap?.scopeFields?.length ?? 0) > 0);
  // The namespace filter row shows for a service with a namespace axis.
  const hasNamespaces = $derived(services.some((s) => s.hasNamespaces));

  // Service key → stable nav icon/letter (labels come from capability names).
  const NAV_ICON: Record<string, string> = { param: 'P', secret: 'S' };

  // ---- Scope form: one input per capability scope field of the pending provider
  const pendingCap = $derived(capabilities.find((c) => c.provider === pendingProvider) ?? null);
  const formFields = $derived(pendingProvider ? scopeFieldSpecs(pendingCap) : []);
  let fieldInputs = $state<Record<string, string>>({});
  let formError = $state('');
  let fieldEls: (HTMLInputElement | undefined)[] = $state([]);

  $effect(() => {
    // Re-seed the inputs whenever the pending provider (or its prefill) changes.
    const s = formScope;
    fieldInputs = Object.fromEntries(formFields.map(([field]) => [field, scopeFieldValue(s, field)]));
    formError = '';
  });

  // Focus the first field when a scope form opens (a11y).
  $effect(() => {
    const first = fieldEls[0];
    if (pendingProvider && first) {
      first.focus();
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

  function handleNamespaceChange(e: Event) {
    onchangenamespace?.((e.currentTarget as HTMLSelectElement).value);
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
  function submitScope(e: SubmitEvent) {
    e.preventDefault();
    onselectscope?.(selectionFor(pendingCap, pendingProvider, (field) => (fieldInputs[field] ?? '').trim()));
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
  {#if formFields.length > 0}
    <form class="scope-form" onsubmit={submitScope}>
      {#each formFields as [field, spec], i (field)}
        <label class="scope-label" for={spec.id}>{spec.label}</label>
        <input
          id={spec.id}
          class="scope-input"
          type="text"
          placeholder={spec.placeholder}
          bind:value={fieldInputs[field]}
          bind:this={fieldEls[i]}
        />
        {#if spec.hint}
          <p class="scope-hint">{spec.hint}</p>
        {/if}
      {/each}
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

  <!-- Scope target: the same segments the TUI status bar and the CLI prompts
       show. Every segment is always rendered, showing "…" while a lookup is
       pending and "?" when unset (e.g. identity unavailable). -->
  {#if provider}
    <div class="scope-info">
      {#each target?.segments ?? [] as seg, i}
        <div class="scope-info-row">
          <span class="scope-info-label">{seg.label}</span>
          <span
            class="scope-info-value"
            class:scope-info-primary={i === 0}
            title={seg.value || '?'}>{seg.value || (target?.pending ? '…' : '?')}</span
          >
        </div>
      {/each}
      {#if hasNamespaces}
        <!-- The Namespace row is a client-side filter dropdown. Changing it
             filters ParamView's rows without re-scoping; the "Change scope"
             button below re-points the scope. -->
        <div class="scope-info-row">
          <span class="scope-info-label">namespace</span>
          <select
            class="scope-info-value namespace-select"
            value={selectedNamespace}
            onchange={handleNamespaceChange}
            aria-label="Namespace"
          >
            {#each namespaceOptions as ns}
              <option value={ns}>{ns}</option>
            {/each}
          </select>
        </div>
      {/if}
      {#if hasScopeForm}
        <!-- Always present and only disabled while a form is pending (so it
             stays reachable in an errored/partial state). -->
        <button type="button" class="scope-change" disabled={!!pendingProvider} onclick={() => onchangescope?.()}>Change scope</button>
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

  .scope-info {
    margin-top: auto;
    padding: 12px 16px;
    border-top: 1px solid #2d2d44;
    font-size: 11px;
  }

  .scope-info-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    padding: 4px 0;
  }

  .scope-info-label {
    color: #666;
    flex-shrink: 0;
    text-transform: capitalize;
  }

  .scope-info-value {
    color: #a0a0a0;
    font-family: monospace;
    font-size: 10px;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .scope-info-primary {
    color: #e94560;
    font-weight: bold;
  }

  /* Namespace filter dropdown in the scope-info footer (App Configuration only).
     Styled to sit inline with the info rows like the read-only values it
     replaced. */
  .namespace-select {
    max-width: 60%;
    box-sizing: border-box;
    padding: 2px 4px;
    background: #252542;
    color: #a0a0a0;
    border: 1px solid #2d2d44;
    border-radius: 4px;
    font-family: monospace;
    font-size: 10px;
    cursor: pointer;
  }
</style>
