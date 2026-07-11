<script lang="ts">
  import { onMount } from 'svelte';
  import { InspectImportFile, PickExportPath, PickImportPath, StagingAddTag, StagingApply, StagingCancelAddTag, StagingCancelRemoveTag, StagingDiff, StagingEdit, StagingExport, StagingImport, StagingReset, StagingUnstage } from '../../wailsjs/go/gui/App';
  import { gui } from '../../wailsjs/go/models';
  import Modal from './Modal.svelte';
  import PassphraseModal from './PassphraseModal.svelte';
  import { withRetry } from './retry';
  import StagingSection from './StagingSection.svelte';
  import { parseError } from './viewUtils';
  import './common.css';

  interface Props {
    services?: gui.ServiceCapability[];
    oncountchange?: (count: number) => void;
  }

  let { services = [], oncountchange }: Props = $props();

  // Only render/fetch sections for services the active provider offers (Google
  // Cloud has no param); headings use the capability display names.
  const paramSvc = $derived(services.find((s) => s.service === 'param') ?? null);
  const secretSvc = $derived(services.find((s) => s.service === 'secret') ?? null);
  const paramLabel = $derived(paramSvc?.displayName ?? 'Parameters');
  const secretLabel = $derived(secretSvc?.displayName ?? 'Secrets');

  let loading = $state(false);
  let error = $state('');
  let paramEntries: gui.StagingDiffEntry[] = $state([]);
  let secretEntries: gui.StagingDiffEntry[] = $state([]);
  let paramTagEntries: gui.StagingDiffTagEntry[] = $state([]);
  let secretTagEntries: gui.StagingDiffTagEntry[] = $state([]);

  // View mode: 'diff' (default) or 'value'
  let viewMode: 'diff' | 'value' = $state('diff');

  // Modal states
  let showApplyModal = $state(false);
  let showResetModal = $state(false);
  let showEditModal = $state(false);
  let showEditTagModal = $state(false);
  let applyService = $state('');
  let resetService = $state('');
  let ignoreConflicts = $state(false);
  let modalLoading = $state(false);
  let modalError = $state('');
  let applyResult: gui.StagingApplyResult | null = $state(null);

  // Edit form
  let editService = $state('');
  let editName = $state('');
  let editValue = $state('');
  // App Configuration namespace of the entry being edited (empty otherwise).
  let editNamespace = $state('');

  // Tag edit form
  let tagEditService = $state('');
  let tagEditEntryName = $state('');
  let tagEditNamespace = $state('');
  let tagEditKey = $state('');
  let tagEditValue = $state('');
  let tagEditIsNew = $state(false);

  // Export / Import: per-service transfer to/from a file. Files are per-service
  // (one service per envelope), so every flow carries a concrete service — never
  // the combined scope — which keeps Azure App Configuration (param) and Key
  // Vault (secret) in their own on-disk buckets (#445).
  let showTransferDropdown = $state(false);

  // Export flow
  let showExportPassphrase = $state(false);
  let exportService = $state('');
  let exportPath = $state('');
  let exportKeep = $state(false);
  let exportLoading = $state(false);
  let exportError = $state('');
  let exportResult: gui.StagingExportResult | null = $state(null);

  // Import flow
  let importService = $state('');
  let importPath = $state('');
  let importInfo: gui.EnvelopeInfoResult | null = $state(null);
  let importPassphrase = $state('');
  let importMode: 'merge' | 'overwrite' = $state('merge');
  let showImportWarnModal = $state(false);
  let showImportModeModal = $state(false);
  let showImportPassphrase = $state(false);
  let showImportErrorModal = $state(false);
  let importLoading = $state(false);
  let importError = $state('');
  let importResult: gui.StagingImportResult | null = $state(null);

  async function loadStatus() {
    loading = true;
    error = '';
    try {
      const [paramResult, secretResult] = await Promise.all([
        paramSvc ? withRetry(() => StagingDiff('param', '')) : Promise.resolve(null),
        secretSvc ? withRetry(() => StagingDiff('secret', '')) : Promise.resolve(null),
      ]);
      paramEntries = paramResult?.entries?.filter(e => e.type !== 'autoUnstaged') || [];
      secretEntries = secretResult?.entries?.filter(e => e.type !== 'autoUnstaged') || [];
      paramTagEntries = paramResult?.tagEntries || [];
      secretTagEntries = secretResult?.tagEntries || [];
      // Emit count change for sidebar badge
      const totalCount = paramEntries.length + secretEntries.length + paramTagEntries.length + secretTagEntries.length;
      oncountchange?.(totalCount);
    } catch (e) {
      error = parseError(e);
      paramEntries = [];
      secretEntries = [];
      paramTagEntries = [];
      secretTagEntries = [];
      oncountchange?.(0);
    } finally {
      loading = false;
    }
  }

  // Apply modal
  function openApplyModal(service: string) {
    applyService = service;
    ignoreConflicts = false;
    modalError = '';
    applyResult = null;
    showApplyModal = true;
  }

  async function handleApply() {
    modalLoading = true;
    modalError = '';
    applyResult = null;
    try {
      // "Apply All" (applyService === 'all') applies every service with staged
      // changes in one click — each service is a separate backend call (Azure
      // App Configuration and Key Vault have independent scopes), so aggregate
      // the per-service results into one. Per-section apply passes a concrete
      // service and applies just that one.
      const targets = applyService === 'all' ? stagedServices() : [applyService];

      const merged = new gui.StagingApplyResult({
        serviceName: getServiceName(applyService),
        entryResults: [],
        tagResults: [],
        conflicts: [],
        entrySucceeded: 0,
        entryFailed: 0,
        tagSucceeded: 0,
        tagFailed: 0,
      });

      for (const svc of targets) {
        const r = await StagingApply(svc, ignoreConflicts);
        merged.entrySucceeded += r.entrySucceeded;
        merged.entryFailed += r.entryFailed;
        merged.tagSucceeded += r.tagSucceeded;
        merged.tagFailed += r.tagFailed;
        // The backend marshals empty slices as null, so guard every spread.
        merged.conflicts = [...(merged.conflicts ?? []), ...(r.conflicts ?? [])];
        merged.entryResults = [...merged.entryResults, ...(r.entryResults ?? [])];
        merged.tagResults = [...merged.tagResults, ...(r.tagResults ?? [])];
      }

      applyResult = merged;
    } catch (e) {
      modalError = parseError(e);
    } finally {
      // Reconcile the view with the backend regardless of a mid-loop rejection.
      // "Apply All" applies each service in a separate call, and every success
      // unstages that service's entries on disk. If a later service rejects
      // (conflict / per-entry failure), the already-applied entries must not keep
      // rendering as pending with a stale badge until a manual Refresh (#477).
      modalLoading = false;
      await loadStatus();
    }
  }

  function closeApplyModal() {
    showApplyModal = false;
    if (applyResult && applyResult.entryFailed === 0 && applyResult.tagFailed === 0) {
      loadStatus();
    }
  }

  // Reset modal
  function openResetModal(service: string) {
    resetService = service;
    modalError = '';
    showResetModal = true;
  }

  async function handleReset() {
    modalLoading = true;
    modalError = '';
    try {
      // "Reset All" (resetService === 'all') resets every service with staged
      // changes — each is a separate backend call (Azure App Configuration and
      // Key Vault have independent scopes), mirroring "Apply All". Per-section
      // reset passes a concrete service.
      const targets = resetService === 'all' ? stagedServices() : [resetService];
      for (const svc of targets) {
        await StagingReset(svc);
      }
      showResetModal = false;
    } catch (e) {
      modalError = parseError(e);
    } finally {
      // Reconcile with the backend regardless of a mid-loop rejection: a later
      // service failing must not leave already-reset services rendering as
      // pending with a stale badge (#477).
      modalLoading = false;
      await loadStatus();
    }
  }

  function getServiceName(service: string): string {
    if (service === 'all') return 'all services';
    return service === 'param' ? paramLabel : secretLabel;
  }

  // Services that currently have staged changes (entries or tags), in display
  // order. Drives "Apply All" so a single click applies every service, not just
  // the first non-empty one.
  function stagedServices(): string[] {
    const targets: string[] = [];
    if (paramEntries.length > 0 || paramTagEntries.length > 0) targets.push('param');
    if (secretEntries.length > 0 || secretTagEntries.length > 0) targets.push('secret');
    return targets;
  }

  // Edit modal
  function openEditModal(service: string, entry: gui.StagingDiffEntry) {
    editService = service;
    editName = entry.name;
    editNamespace = entry.namespace;
    editValue = entry.stagedValue || '';
    modalError = '';
    showEditModal = true;
  }

  async function handleEdit(e: SubmitEvent) {
    e.preventDefault();
    if (!editValue) {
      modalError = 'Value is required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      await StagingEdit(editService, editName, editValue, editNamespace);
      showEditModal = false;
      await loadStatus();
    } catch (e) {
      modalError = parseError(e);
    } finally {
      modalLoading = false;
    }
  }

  // Unstage
  async function handleUnstage(service: string, name: string, namespace: string) {
    try {
      await StagingUnstage(service, name, namespace);
      await loadStatus();
    } catch (e) {
      error = parseError(e);
    }
  }

  // Tag edit modal
  function openAddTagModal(service: string, entryName: string, namespace: string) {
    tagEditService = service;
    tagEditEntryName = entryName;
    tagEditNamespace = namespace;
    tagEditKey = '';
    tagEditValue = '';
    tagEditIsNew = true;
    modalError = '';
    showEditTagModal = true;
  }

  function openEditTagModal(service: string, entryName: string, namespace: string, key: string, value: string) {
    tagEditService = service;
    tagEditEntryName = entryName;
    tagEditNamespace = namespace;
    tagEditKey = key;
    tagEditValue = value;
    tagEditIsNew = false;
    modalError = '';
    showEditTagModal = true;
  }

  async function handleSaveTag(e: SubmitEvent) {
    e.preventDefault();
    if (!tagEditKey) {
      modalError = 'Tag key is required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      await StagingAddTag(tagEditService, tagEditEntryName, tagEditKey, tagEditValue, tagEditNamespace);
      showEditTagModal = false;
      await loadStatus();
    } catch (e) {
      modalError = parseError(e);
    } finally {
      modalLoading = false;
    }
  }

  async function handleRemoveTag(service: string, entryName: string, namespace: string, key: string) {
    // Cancel a staged tag addition (remove from Tags only, don't add to UntagKeys)
    try {
      await StagingCancelAddTag(service, entryName, key, namespace);
      await loadStatus();
    } catch (e) {
      error = parseError(e);
    }
  }

  async function handleCancelUntag(service: string, entryName: string, namespace: string, key: string) {
    // Cancel a staged tag removal (remove from UntagKeys only, don't add to Tags)
    try {
      await StagingCancelRemoveTag(service, entryName, key, namespace);
      await loadStatus();
    } catch (e) {
      error = parseError(e);
    }
  }

  // Does the given service currently have staged changes (entries or tags)?
  // Drives the import merge/overwrite prompt and the export enable state.
  function serviceHasChanges(service: string): boolean {
    if (service === 'param') return paramEntries.length > 0 || paramTagEntries.length > 0;
    return secretEntries.length > 0 || secretTagEntries.length > 0;
  }

  function serviceLabel(service: string): string {
    return service === 'param' ? paramLabel : secretLabel;
  }

  // ---- Export flow -------------------------------------------------------
  // Pick a destination file (native Save dialog), then collect a passphrase
  // (optional) before writing a single-service envelope.
  async function startExport(service: string) {
    showTransferDropdown = false;
    exportError = '';
    exportResult = null;
    exportKeep = false;
    exportService = service;
    try {
      const path = await PickExportPath(`${service}.json`);
      if (!path) return; // dialog cancelled
      exportPath = path;
      showExportPassphrase = true;
    } catch (e) {
      error = parseError(e);
    }
  }

  async function handleExport(passphrase: string) {
    exportLoading = true;
    exportError = '';
    try {
      const result = await StagingExport(exportPath, exportService, passphrase, exportKeep);
      exportResult = result;
      showExportPassphrase = false;
      await loadStatus();
    } catch (e) {
      exportError = parseError(e);
    } finally {
      exportLoading = false;
    }
  }

  function closeExportModal() {
    showExportPassphrase = false;
  }

  // ---- Import flow -------------------------------------------------------
  // Pick a source file (native Open dialog), inspect its plaintext header
  // (no passphrase yet), then chain: service-mismatch refusal → scope warning
  // → merge/overwrite (only when the target already has changes) → passphrase
  // (only when encrypted) → import.
  async function startImport(service: string) {
    showTransferDropdown = false;
    importError = '';
    importResult = null;
    importInfo = null;
    importPassphrase = '';
    importMode = 'merge';
    importService = service;
    try {
      const path = await PickImportPath();
      if (!path) return; // dialog cancelled
      importPath = path;

      const info = await InspectImportFile(path);
      importInfo = info;

      if (info.service !== service) {
        importError = `This file holds ${info.service} data, not ${serviceLabel(service)}. Choose the matching Import action.`;
        showImportErrorModal = true;
        return;
      }

      if (!info.scopeMatches) {
        showImportWarnModal = true;
        return;
      }

      proceedImportAfterWarn();
    } catch (e) {
      importError = parseError(e);
      showImportErrorModal = true;
    }
  }

  function proceedImportAfterWarn() {
    showImportWarnModal = false;
    // Prompt for merge/overwrite only when the working area already holds
    // changes for this service. The signal comes from the import metadata
    // (InspectImportFile) rather than the loaded view state, which may be stale
    // or not yet loaded — matching the CLI, which prompts from the working area.
    if (importInfo?.workingHasChanges) {
      importMode = 'merge';
      showImportModeModal = true;
      return;
    }
    afterModeChosen();
  }

  function afterModeChosen() {
    showImportModeModal = false;
    if (importInfo?.encrypted) {
      showImportPassphrase = true;
      return;
    }
    runImport('');
  }

  function handleImportPassphrase(passphrase: string) {
    // Keep the passphrase modal open across the attempt so a wrong passphrase can
    // be re-entered inline (via the modal's error prop); runImport closes it on
    // success. Mirrors the export flow.
    runImport(passphrase);
  }

  async function runImport(passphrase: string) {
    importLoading = true;
    importError = '';
    try {
      // Pass force=true only for a confirmed scope mismatch: reaching runImport
      // with importInfo.scopeMatches === false means the user clicked through the
      // Scope Mismatch modal, so the backend scope guard should be overridden.
      const force = importInfo ? !importInfo.scopeMatches : false;
      const result = await StagingImport(importPath, importService, passphrase, importMode, force);
      importResult = result;
      showImportModeModal = false;
      showImportPassphrase = false;
      await loadStatus();
    } catch (e) {
      importError = parseError(e);
      // When the passphrase prompt is open (encrypted import), keep it open and
      // show the error inline via the modal's error prop so a wrong passphrase can
      // be retried without rebuilding the whole import chain (mirrors export).
      // Only non-passphrase errors (the prompt is closed) fall back to the
      // standalone Import Failed modal.
      if (!showImportPassphrase) {
        showImportErrorModal = true;
      }
    } finally {
      importLoading = false;
    }
  }

  function closeImportModals() {
    showImportWarnModal = false;
    showImportModeModal = false;
    showImportPassphrase = false;
    showImportErrorModal = false;
  }

  onMount(() => {
    loadStatus();
  });
</script>

<div class="view-container">
  <div class="header">
    <h2 class="title">Staging Area</h2>
    <div class="header-actions">
      <div class="view-toggle">
        <button
          class="toggle-btn"
          class:active={viewMode === 'diff'}
          onclick={() => viewMode = 'diff'}
        >
          Diff
        </button>
        <button
          class="toggle-btn"
          class:active={viewMode === 'value'}
          onclick={() => viewMode = 'value'}
        >
          Value
        </button>
      </div>
      <button class="btn-primary" onclick={loadStatus} disabled={loading}>
        {loading ? 'Loading...' : 'Refresh'}
      </button>
    </div>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <div class="staging-content">
    {#if paramSvc}
      <StagingSection
        icon="P"
        title={paramLabel}
        emptyText="No staged {paramLabel} changes"
        entries={paramEntries}
        tagEntries={paramTagEntries}
        hasTags={paramSvc?.hasTags ?? true}
        showNamespace={paramSvc?.hasNamespaces ?? false}
        {viewMode}
        onapply={() => openApplyModal('param')}
        onreset={() => openResetModal('param')}
        onedit={(entry) => openEditModal('param', entry)}
        onunstage={(name, namespace) => handleUnstage('param', name, namespace)}
        onaddtag={(entryName, namespace) => openAddTagModal('param', entryName, namespace)}
        onedittag={(entryName, namespace, key, value) => openEditTagModal('param', entryName, namespace, key, value)}
        onremovetag={(entryName, namespace, key) => handleRemoveTag('param', entryName, namespace, key)}
        oncanceluntag={(entryName, namespace, key) => handleCancelUntag('param', entryName, namespace, key)}
      />
    {/if}

    {#if secretSvc}
      <StagingSection
        icon="S"
        title={secretLabel}
        emptyText="No staged {secretLabel} changes"
        entries={secretEntries}
        tagEntries={secretTagEntries}
        hasTags={secretSvc?.hasTags ?? true}
        {viewMode}
        onapply={() => openApplyModal('secret')}
        onreset={() => openResetModal('secret')}
        onedit={(entry) => openEditModal('secret', entry)}
        onunstage={(name, namespace) => handleUnstage('secret', name, namespace)}
        onaddtag={(entryName, namespace) => openAddTagModal('secret', entryName, namespace)}
        onedittag={(entryName, namespace, key, value) => openEditTagModal('secret', entryName, namespace, key, value)}
        onremovetag={(entryName, namespace, key) => handleRemoveTag('secret', entryName, namespace, key)}
        oncanceluntag={(entryName, namespace, key) => handleCancelUntag('secret', entryName, namespace, key)}
      />
    {/if}
  </div>

  <div class="actions">
    <div class="actions-center">
      <button
        class="btn-action btn-apply"
        onclick={() => openApplyModal('all')}
        disabled={paramEntries.length === 0 && secretEntries.length === 0 && paramTagEntries.length === 0 && secretTagEntries.length === 0}
      >
        Apply All
      </button>
      <button
        class="btn-action btn-reset"
        onclick={() => openResetModal('all')}
        disabled={paramEntries.length === 0 && secretEntries.length === 0 && paramTagEntries.length === 0 && secretTagEntries.length === 0}
      >
        Reset All
      </button>
    </div>
    <div class="transfer-dropdown">
      <button
        class="btn-transfer"
        data-testid="transfer-menu"
        onclick={() => showTransferDropdown = !showTransferDropdown}
      >
        Export / Import
        <span class="dropdown-arrow">▾</span>
      </button>
      {#if showTransferDropdown}
        <button type="button" class="dropdown-backdrop" aria-label="Dismiss menu" onclick={() => showTransferDropdown = false}></button>
        <div class="dropdown-menu">
          <div class="dropdown-heading">Export</div>
          {#if paramSvc}
            <button
              class="dropdown-item"
              data-testid="export-param"
              onclick={() => startExport('param')}
              disabled={loading || !serviceHasChanges('param')}
            >
              <span class="dropdown-icon">⏏️</span>
              Export {paramLabel}
            </button>
          {/if}
          {#if secretSvc}
            <button
              class="dropdown-item"
              data-testid="export-secret"
              onclick={() => startExport('secret')}
              disabled={loading || !serviceHasChanges('secret')}
            >
              <span class="dropdown-icon">⏏️</span>
              Export {secretLabel}
            </button>
          {/if}
          <div class="dropdown-divider"></div>
          <div class="dropdown-heading">Import</div>
          {#if paramSvc}
            <button
              class="dropdown-item"
              data-testid="import-param"
              onclick={() => startImport('param')}
              disabled={loading}
            >
              <span class="dropdown-icon">▶️</span>
              Import {paramLabel}
            </button>
          {/if}
          {#if secretSvc}
            <button
              class="dropdown-item"
              data-testid="import-secret"
              onclick={() => startImport('secret')}
              disabled={loading}
            >
              <span class="dropdown-icon">▶️</span>
              Import {secretLabel}
            </button>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>

<!-- Apply Modal -->
<Modal title="Apply Staged Changes" show={showApplyModal} onclose={closeApplyModal}>
  <div class="modal-apply">
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}

    {#if applyResult}
      <div class="apply-result">
        <h4>Apply Results</h4>
        {#if applyResult.conflicts && applyResult.conflicts.length > 0}
          <div class="conflicts">
            <p class="warning">Conflicts detected:</p>
            <ul>
              {#each applyResult.conflicts as conflict}
                <li>{conflict}</li>
              {/each}
            </ul>
          </div>
        {/if}
        <div class="result-summary">
          <span class="result-success">Entries: {applyResult.entrySucceeded} succeeded</span>
          {#if applyResult.entryFailed > 0}
            <span class="result-failed">{applyResult.entryFailed} failed</span>
          {/if}
          {#if applyResult.tagSucceeded > 0 || applyResult.tagFailed > 0}
            <span class="result-divider">|</span>
            <span class="result-success">Tags: {applyResult.tagSucceeded} succeeded</span>
            {#if applyResult.tagFailed > 0}
              <span class="result-failed">{applyResult.tagFailed} failed</span>
            {/if}
          {/if}
        </div>
        {#if applyResult.entryResults && applyResult.entryResults.length > 0}
          <div class="result-section">
            <h5 class="result-section-title">Entry Changes</h5>
            <ul class="result-list">
              {#each applyResult.entryResults as result}
                <li class="result-item" class:failed={result.status === 'failed'}>
                  {#if result.namespace}
                    <span class="result-namespace">{result.namespace}</span>
                  {/if}
                  <span class="result-name">{result.name}</span>
                  <span class="result-status" class:status-created={result.status === 'created'} class:status-updated={result.status === 'updated'} class:status-deleted={result.status === 'deleted'} class:status-failed={result.status === 'failed'}>
                    {result.status}
                  </span>
                  {#if result.error}
                    <span class="result-error">{result.error}</span>
                  {/if}
                  {#if result.unstageError}
                    <span class="result-warning" data-testid="unstage-warning">
                      Applied, but still staged (failed to clear): {result.unstageError}
                    </span>
                  {/if}
                </li>
              {/each}
            </ul>
          </div>
        {/if}
        {#if applyResult.tagResults && applyResult.tagResults.length > 0}
          <div class="result-section">
            <h5 class="result-section-title">Tag Changes</h5>
            <ul class="result-list">
              {#each applyResult.tagResults as result}
                <li class="result-item" class:failed={!!result.error}>
                  {#if result.namespace}
                    <span class="result-namespace">{result.namespace}</span>
                  {/if}
                  <span class="result-name">{result.name}</span>
                  <span class="result-status status-updated">tags</span>
                  {#if result.error}
                    <span class="result-error">{result.error}</span>
                  {/if}
                  {#if result.unstageError}
                    <span class="result-warning" data-testid="unstage-warning">
                      Applied, but still staged (failed to clear): {result.unstageError}
                    </span>
                  {/if}
                </li>
              {/each}
            </ul>
          </div>
        {/if}
        <div class="form-actions">
          <button type="button" class="btn-primary" onclick={closeApplyModal}>Close</button>
        </div>
      </div>
    {:else}
      <p>Apply staged changes to {getServiceName(applyService)}?</p>
      <p class="info">This will push all staged changes to the remote store.</p>
      <label class="checkbox-label">
        <input type="checkbox" bind:checked={ignoreConflicts} />
        <span>Ignore conflicts</span>
      </label>
      <div class="form-actions">
        <button type="button" class="btn-secondary" onclick={closeApplyModal}>Cancel</button>
        <button type="button" class="btn-apply" onclick={handleApply} disabled={modalLoading}>
          {modalLoading ? 'Applying...' : 'Apply'}
        </button>
      </div>
    {/if}
  </div>
</Modal>

<!-- Reset Modal -->
<Modal title="Reset Staged Changes" show={showResetModal} onclose={() => showResetModal = false}>
  <div class="modal-confirm">
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <p>Reset all staged changes for {getServiceName(resetService)}?</p>
    <p class="warning">This will discard all staged changes without applying them.</p>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showResetModal = false}>Cancel</button>
      <button type="button" class="btn-danger" onclick={handleReset} disabled={modalLoading}>
        {modalLoading ? 'Resetting...' : 'Reset'}
      </button>
    </div>
  </div>
</Modal>

<!-- Edit Modal -->
<Modal title="Edit Staged {editService === 'param' ? 'Parameter' : 'Secret'}" show={showEditModal} onclose={() => showEditModal = false}>
  <form class="modal-form" onsubmit={handleEdit}>
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <div class="form-group">
      <label for="edit-name">Name</label>
      <input
        id="edit-name"
        type="text"
        class="form-input"
        value={editName}
        disabled
      />
    </div>
    <div class="form-group">
      <label for="edit-value">Value</label>
      <textarea
        id="edit-value"
        class="form-input form-textarea"
        bind:value={editValue}
        rows="8"
      ></textarea>
    </div>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showEditModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? 'Saving...' : 'Save'}
      </button>
    </div>
  </form>
</Modal>

<!-- Edit Tag Modal -->
<Modal title={tagEditIsNew ? 'Add Tag' : 'Edit Tag'} show={showEditTagModal} onclose={() => showEditTagModal = false}>
  <form class="modal-form" onsubmit={handleSaveTag}>
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <div class="form-group">
      <label for="tag-key">Key</label>
      <input
        id="tag-key"
        type="text"
        class="form-input"
        bind:value={tagEditKey}
        disabled={!tagEditIsNew}
        placeholder="e.g., environment"
      />
    </div>
    <div class="form-group">
      <label for="tag-value">Value</label>
      <input
        id="tag-value"
        type="text"
        class="form-input"
        bind:value={tagEditValue}
        placeholder="e.g., production"
      />
    </div>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showEditTagModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? 'Saving...' : 'Save'}
      </button>
    </div>
  </form>
</Modal>

<!-- Export Passphrase Modal (encrypt; empty passphrase = plain text) -->
<PassphraseModal
  show={showExportPassphrase}
  mode="encrypt"
  title="Export {serviceLabel(exportService)}"
  keepOption={true}
  bind:keep={exportKeep}
  onsubmit={handleExport}
  oncancel={closeExportModal}
  loading={exportLoading}
  error={exportError}
/>

<!-- Export Result Modal -->
{#if exportResult}
<Modal title="Export Complete" show={true} onclose={() => exportResult = null}>
  <div class="persist-result">
    <div class="result-icon success">✓</div>
    <h4>Exported {serviceLabel(exportService)} to file</h4>
    <div class="result-stats">
      <span class="stat">{exportResult.entryCount} entries</span>
      <span class="stat">{exportResult.tagCount} tag changes</span>
    </div>
    <div class="form-actions">
      <button type="button" class="btn-primary" onclick={() => exportResult = null}>Close</button>
    </div>
  </div>
</Modal>
{/if}

<!-- Import Scope-Mismatch Warning Modal -->
<Modal title="Scope Mismatch" show={showImportWarnModal} onclose={closeImportModals}>
  <div class="modal-confirm">
    <p>The file was exported from a different scope.</p>
    <p class="warning">
      File scope: <code>{importInfo?.scope}</code><br />
      Importing it here may not match the current provider/scope.
    </p>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={closeImportModals}>Cancel</button>
      <button type="button" class="btn-warning" data-testid="import-warn-continue" onclick={proceedImportAfterWarn}>
        Import anyway
      </button>
    </div>
  </div>
</Modal>

<!-- Import Mode Modal (merge/overwrite; only when the target has changes) -->
<Modal title="Import {serviceLabel(importService)}" show={showImportModeModal} onclose={closeImportModals}>
  <div class="push-options">
    {#if importError}
      <div class="modal-error">{importError}</div>
    {/if}
    <p>The staging area already has {serviceLabel(importService)} changes. How do you want to proceed?</p>
    <div class="options-group">
      <label class="radio-label">
        <input type="radio" name="importMode" value="merge" bind:group={importMode} />
        <span>Merge</span>
        <span class="option-desc">Combine imported changes with existing</span>
      </label>
      <label class="radio-label">
        <input type="radio" name="importMode" value="overwrite" bind:group={importMode} />
        <span>Overwrite</span>
        <span class="option-desc">Replace existing with imported changes</span>
      </label>
    </div>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={closeImportModals}>Cancel</button>
      <button type="button" class="btn-push-action" data-testid="import-mode-continue" onclick={afterModeChosen}>
        Continue
      </button>
    </div>
  </div>
</Modal>

<!-- Import Passphrase Modal (decrypt; only when the payload is encrypted) -->
<PassphraseModal
  show={showImportPassphrase}
  mode="decrypt"
  title="Import {serviceLabel(importService)}"
  onsubmit={handleImportPassphrase}
  oncancel={closeImportModals}
  loading={importLoading}
  error={importError}
/>

<!-- Import Busy Modal: progress feedback + input blocking for the import paths
     that show no passphrase modal (plaintext, and plaintext into a service with
     no existing changes). The encrypted path already surfaces progress via the
     passphrase modal's loading state, so this is suppressed while that is open.
     It has no onclose, so its backdrop/Escape/× cannot dismiss it mid-flight. -->
<Modal title="Importing {serviceLabel(importService)}" show={importLoading && !showImportPassphrase}>
  <div class="modal-confirm">
    <p>Importing staged changes…</p>
  </div>
</Modal>

<!-- Import Result Modal -->
{#if importResult}
<Modal title="Import Complete" show={true} onclose={() => importResult = null}>
  <div class="drain-result">
    <div class="result-icon success">✓</div>
    <h4>Imported {serviceLabel(importService)} into the staging area</h4>
    <div class="result-stats">
      <span class="stat">{importResult.entryCount} entries</span>
      <span class="stat">{importResult.tagCount} tag changes</span>
      {#if importResult.merged}
        <span class="stat merged">(merged)</span>
      {/if}
    </div>
    <div class="form-actions">
      <button type="button" class="btn-primary" onclick={() => importResult = null}>Close</button>
    </div>
  </div>
</Modal>
{/if}

<!-- Import Error Modal (service mismatch, inspect failure, backend error) -->
<Modal title="Import Failed" show={showImportErrorModal} onclose={closeImportModals}>
  <div class="modal-confirm">
    <div class="modal-error" data-testid="import-error">{importError}</div>
    <div class="form-actions">
      <button type="button" class="btn-primary" onclick={closeImportModals}>Close</button>
    </div>
  </div>
</Modal>

<style>
  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px;
    background: #1a1a2e;
    border-bottom: 1px solid #2d2d44;
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .view-toggle {
    display: flex;
    background: #0f0f1a;
    border-radius: 4px;
    overflow: hidden;
  }

  .toggle-btn {
    padding: 6px 12px;
    border: none;
    background: transparent;
    color: #888;
    font-size: 12px;
    cursor: pointer;
    transition: all 0.2s;
  }

  .toggle-btn:hover {
    color: #fff;
  }

  .toggle-btn.active {
    background: #e94560;
    color: #fff;
  }

  .title {
    margin: 0;
    font-size: 18px;
    color: #fff;
  }

  .staging-content {
    flex: 1;
    /* min-height: 0 lets this flex child shrink below its content height so its
       own overflow-y scrolls, instead of growing past .view-container and being
       clipped by .main-content's overflow:hidden (no scroll at all). */
    min-height: 0;
    overflow-y: auto;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 24px;
  }

  .actions {
    position: relative;
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    align-items: center;
    gap: 12px;
    padding: 16px;
    background: #1a1a2e;
    border-top: 1px solid #2d2d44;
  }

  .actions-center {
    display: flex;
    justify-content: center;
    gap: 12px;
  }

  .btn-action {
    min-width: 200px;
    padding: 12px 40px;
    border: none;
    border-radius: 4px;
    font-size: 14px;
    font-weight: bold;
    cursor: pointer;
  }

  .btn-apply {
    background: #4caf50;
    color: #fff;
  }

  .btn-apply:hover:not(:disabled) {
    background: #43a047;
  }

  .btn-reset {
    background: #f44336;
    color: #fff;
  }

  .btn-reset:hover:not(:disabled) {
    background: #e53935;
  }

  .btn-action:disabled {
    background: #444;
    color: #888;
    cursor: not-allowed;
  }

  /* Export / Import dropdown */
  .transfer-dropdown {
    position: absolute;
    right: 16px;
    top: 50%;
    transform: translateY(-50%);
  }

  /* Narrow viewport: the Export / Import menu wraps below and aligns right */
  @media (max-width: 800px) {
    .actions {
      flex-direction: column;
      gap: 12px;
    }

    .transfer-dropdown {
      position: static;
      transform: none;
      align-self: flex-end;
    }
  }

  .btn-transfer {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px 16px;
    background: #2d2d44;
    color: #fff;
    border: none;
    border-radius: 4px;
    font-size: 14px;
    cursor: pointer;
    transition: background 0.2s;
  }

  .btn-transfer:hover {
    background: #3d3d54;
  }

  .dropdown-arrow {
    font-size: 10px;
    color: #888;
  }

  .dropdown-backdrop {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 99;
    background: transparent;
    border: none;
    padding: 0;
    margin: 0;
    cursor: default;
  }

  .dropdown-menu {
    position: absolute;
    bottom: 100%;
    right: 0;
    margin-bottom: 8px;
    background: #1a1a2e;
    border: 1px solid #2d2d44;
    border-radius: 8px;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
    min-width: 200px;
    z-index: 100;
    overflow: hidden;
  }

  .dropdown-heading {
    padding: 8px 16px 4px;
    font-size: 11px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #888;
  }

  .btn-warning {
    padding: 8px 16px;
    background: #ff9800;
    color: #000;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
    font-weight: 500;
  }

  .btn-warning:hover {
    background: #f57c00;
  }

  .dropdown-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 10px 16px;
    background: transparent;
    border: none;
    color: #fff;
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    text-align: left;
    white-space: nowrap;
    transition: background 0.2s;
  }

  .dropdown-item:hover:not(:disabled) {
    background: #2d2d44;
  }

  .dropdown-item:disabled {
    color: #666;
    cursor: not-allowed;
  }

  .dropdown-icon {
    font-size: 16px;
  }

  .dropdown-divider {
    height: 1px;
    background: #2d2d44;
    margin: 4px 0;
  }

  /* Modal styles */
  .modal-apply,
  .modal-confirm {
    text-align: center;
  }

  .modal-apply p,
  .modal-confirm p {
    color: #ccc;
    margin: 0 0 16px 0;
  }

  .info {
    font-size: 13px;
    color: #888 !important;
  }

  .warning {
    color: #ff9800 !important;
    font-size: 13px;
  }

  .btn-secondary {
    padding: 8px 16px;
    background: #2d2d44;
    color: #fff;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
  }

  .btn-secondary:hover {
    background: #3d3d54;
  }

  .btn-danger {
    background: #f44336;
    color: #fff;
    padding: 8px 16px;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
  }

  .btn-danger:hover {
    background: #e53935;
  }

  .form-actions {
    display: flex;
    gap: 12px;
    justify-content: center;
    margin-top: 16px;
  }

  .modal-error {
    padding: 10px 12px;
    background: rgba(244, 67, 54, 0.2);
    border: 1px solid #f44336;
    border-radius: 4px;
    color: #f44336;
    font-size: 13px;
    margin-bottom: 16px;
  }

  .apply-result h4 {
    margin: 0 0 16px 0;
    color: #fff;
  }

  .conflicts {
    background: rgba(255, 152, 0, 0.1);
    border: 1px solid #ff9800;
    border-radius: 4px;
    padding: 12px;
    margin-bottom: 16px;
    text-align: left;
  }

  .conflicts ul {
    margin: 8px 0 0 0;
    padding-left: 20px;
  }

  .conflicts li {
    color: #ff9800;
    font-family: monospace;
    font-size: 13px;
  }

  .result-summary {
    display: flex;
    gap: 24px;
    justify-content: center;
    margin-bottom: 16px;
  }

  .result-success {
    color: #4caf50;
    font-weight: bold;
  }

  .result-failed {
    color: #f44336;
    font-weight: bold;
  }

  .result-divider {
    color: #666;
    margin: 0 4px;
  }

  .result-section {
    margin-bottom: 16px;
  }

  .result-section-title {
    margin: 0 0 8px 0;
    font-size: 12px;
    color: #888;
    text-transform: uppercase;
  }

  .result-list {
    list-style: none;
    margin: 0;
    padding: 0;
    text-align: left;
  }

  .result-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 12px;
    background: #0f0f1a;
    border-radius: 4px;
    margin-bottom: 8px;
  }

  .result-item.failed {
    border-left: 3px solid #f44336;
  }

  .result-name {
    flex: 1;
    font-family: monospace;
    font-size: 13px;
    color: #fff;
  }

  /* App Configuration namespace badge on an apply-result row (empty otherwise). */
  .result-namespace {
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 11px;
    background: rgba(120, 120, 120, 0.18);
    color: #888;
    white-space: nowrap;
  }

  .result-status {
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 3px;
    text-transform: uppercase;
    font-weight: bold;
  }

  .status-created {
    background: #4caf50;
    color: #fff;
  }

  .status-updated {
    background: #ff9800;
    color: #fff;
  }

  .status-deleted {
    background: #f44336;
    color: #fff;
  }

  .status-failed {
    background: #666;
    color: #fff;
  }

  .result-error {
    font-size: 12px;
    color: #f44336;
    width: 100%;
    margin-top: 4px;
  }

  .result-warning {
    font-size: 12px;
    color: #ff9800;
    width: 100%;
    margin-top: 4px;
  }

  /* Form styles for modals */
  .modal-form {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .form-group label {
    font-size: 12px;
    color: #888;
    text-transform: uppercase;
  }

  .form-input {
    padding: 10px 12px;
    background: #0f0f1a;
    border: 1px solid #2d2d44;
    border-radius: 4px;
    color: #fff;
    font-size: 14px;
  }

  .form-input:focus {
    outline: none;
    border-color: #e94560;
  }

  .form-input:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .form-textarea {
    font-family: monospace;
    resize: vertical;
    min-height: 100px;
  }

  .btn-primary {
    padding: 8px 16px;
    background: #e94560;
    color: #fff;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
  }

  .btn-primary:hover {
    background: #d63050;
  }

  .btn-primary:disabled {
    background: #666;
    cursor: not-allowed;
  }

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    color: #ccc;
    cursor: pointer;
  }

  .checkbox-label input {
    cursor: pointer;
  }

  /* Modal action buttons */
  .btn-push-action {
    padding: 8px 16px;
    background: #2196f3;
    color: #fff;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
  }

  .btn-push-action:hover:not(:disabled) {
    background: #1976d2;
  }

  .btn-push-action:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  /* Push options modal styles */
  .push-options {
    text-align: center;
  }

  .push-options p {
    color: #ccc;
    margin: 0 0 12px 0;
  }

  .radio-label {
    display: flex;
    align-items: center;
    gap: 8px;
    color: #ccc;
    cursor: pointer;
    padding: 8px;
    border-radius: 4px;
    transition: background 0.2s;
  }

  .radio-label:hover {
    background: rgba(255, 255, 255, 0.05);
  }

  .option-desc {
    font-size: 12px;
    color: #888;
    margin-left: auto;
  }

  .options-group {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin: 20px 0;
    text-align: left;
    background: #0f0f1a;
    padding: 16px;
    border-radius: 4px;
  }

  /* Result modal styles */
  .drain-result,
  .persist-result {
    text-align: center;
    padding: 16px 0;
  }

  .result-icon {
    width: 48px;
    height: 48px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0 auto 16px;
    font-size: 24px;
  }

  .result-icon.success {
    background: rgba(76, 175, 80, 0.2);
    border: 2px solid #4caf50;
    color: #4caf50;
  }

  .drain-result h4,
  .persist-result h4 {
    margin: 0 0 12px;
    color: #fff;
    font-size: 16px;
  }

  .result-stats {
    display: flex;
    gap: 16px;
    justify-content: center;
    margin-bottom: 20px;
  }

  .result-stats .stat {
    font-size: 13px;
    color: #888;
  }

  .result-stats .stat.merged {
    color: #ff9800;
  }
</style>
