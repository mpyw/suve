<script lang="ts">
  import { onMount } from 'svelte';
  import { StagingDiff, StagingApply, StagingReset, StagingEdit, StagingUnstage, StagingAddTag, StagingCancelAddTag, StagingCancelRemoveTag, StagingFileStatus, StagingDrain, StagingPersist, StagingDrop } from '../../wailsjs/go/gui/App';
  import type { gui } from '../../wailsjs/go/models';
  import Modal from './Modal.svelte';
  import PassphraseModal from './PassphraseModal.svelte';
  import DiffDisplay from './DiffDisplay.svelte';
  import { formatDate, parseError } from './viewUtils';
  import './common.css';

  interface Props {
    oncountchange?: (count: number) => void;
  }

  let { oncountchange }: Props = $props();

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

  // Tag edit form
  let tagEditService = $state('');
  let tagEditEntryName = $state('');
  let tagEditKey = $state('');
  let tagEditValue = $state('');
  let tagEditIsNew = $state(false);

  // Persist/Drain modal states
  let showPersistModal = $state(false);
  let showDrainModal = $state(false);
  let showDrainOptionsModal = $state(false);
  let persistLoading = $state(false);
  let drainLoading = $state(false);
  let persistError = $state('');
  let drainError = $state('');
  let persistResult: gui.StagingPersistResult | null = $state(null);
  let drainResult: gui.StagingDrainResult | null = $state(null);
  let fileStatus: gui.StagingFileStatusResult | null = $state(null);

  // Drain options
  let drainKeep = $state(false);
  let drainForce = $state(false);
  let drainMerge = $state(false);

  // Push options modal (when file exists)
  let showPushOptionsModal = $state(false);
  let pushPassphrase = $state('');
  let pushMode: 'overwrite' | 'merge' = $state('overwrite');

  // Drop confirmation modal
  let showDropModal = $state(false);
  let dropLoading = $state(false);
  let dropError = $state('');

  // Stash dropdown state
  let showStashDropdown = $state(false);

  async function loadStatus() {
    loading = true;
    error = '';
    try {
      // Load diff data and file status in parallel
      const [ssmResult, smResult, fileStatusResult] = await Promise.all([
        StagingDiff('param', ''),
        StagingDiff('secret', ''),
        StagingFileStatus().catch(() => null) // Don't fail if file status check fails
      ]);
      paramEntries = ssmResult?.entries?.filter(e => e.type !== 'autoUnstaged') || [];
      secretEntries = smResult?.entries?.filter(e => e.type !== 'autoUnstaged') || [];
      paramTagEntries = ssmResult?.tagEntries || [];
      secretTagEntries = smResult?.tagEntries || [];
      fileStatus = fileStatusResult;
      // Emit count change for sidebar badge
      const totalCount = paramEntries.length + secretEntries.length + paramTagEntries.length + secretTagEntries.length;
      oncountchange?.(totalCount);
    } catch (e) {
      error = parseError(e);
      paramEntries = [];
      secretEntries = [];
      paramTagEntries = [];
      secretTagEntries = [];
      fileStatus = null;
      oncountchange?.(0);
    } finally {
      loading = false;
    }
  }

  function getOperationColor(op: string): string {
    switch (op?.toLowerCase()) {
      case 'set':
      case 'create':
        return '#4caf50';
      case 'update':
        return '#ff9800';
      case 'delete':
        return '#f44336';
      default:
        return '#888';
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
      const result = await StagingApply(applyService, ignoreConflicts);
      applyResult = result;
      if (result.entryFailed === 0 && result.tagFailed === 0 && result.conflicts?.length === 0) {
        await loadStatus();
      }
    } catch (e) {
      modalError = parseError(e);
    } finally {
      modalLoading = false;
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
      await StagingReset(resetService);
      showResetModal = false;
      await loadStatus();
    } catch (e) {
      modalError = parseError(e);
    } finally {
      modalLoading = false;
    }
  }

  function getServiceName(service: string): string {
    return service === 'param' ? 'Parameters' : 'Secrets';
  }

  // Edit modal
  function openEditModal(service: string, entry: gui.StagingDiffEntry) {
    editService = service;
    editName = entry.name;
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
      await StagingEdit(editService, editName, editValue);
      showEditModal = false;
      await loadStatus();
    } catch (e) {
      modalError = parseError(e);
    } finally {
      modalLoading = false;
    }
  }

  // Unstage
  async function handleUnstage(service: string, name: string) {
    try {
      await StagingUnstage(service, name);
      await loadStatus();
    } catch (e) {
      error = parseError(e);
    }
  }

  // Tag edit modal
  function openAddTagModal(service: string, entryName: string) {
    tagEditService = service;
    tagEditEntryName = entryName;
    tagEditKey = '';
    tagEditValue = '';
    tagEditIsNew = true;
    modalError = '';
    showEditTagModal = true;
  }

  function openEditTagModal(service: string, entryName: string, key: string, value: string) {
    tagEditService = service;
    tagEditEntryName = entryName;
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
      await StagingAddTag(tagEditService, tagEditEntryName, tagEditKey, tagEditValue);
      showEditTagModal = false;
      await loadStatus();
    } catch (e) {
      modalError = parseError(e);
    } finally {
      modalLoading = false;
    }
  }

  async function handleRemoveTag(service: string, entryName: string, key: string) {
    // Cancel a staged tag addition (remove from Tags only, don't add to UntagKeys)
    try {
      await StagingCancelAddTag(service, entryName, key);
      await loadStatus();
    } catch (e) {
      error = parseError(e);
    }
  }

  async function handleCancelUntag(service: string, entryName: string, key: string) {
    // Cancel a staged tag removal (remove from UntagKeys only, don't add to Tags)
    try {
      await StagingCancelRemoveTag(service, entryName, key);
      await loadStatus();
    } catch (e) {
      error = parseError(e);
    }
  }

  // Persist modal handlers
  async function openPersistModal() {
    persistError = '';
    persistResult = null;
    pushPassphrase = '';
    pushMode = 'overwrite';

    try {
      // Check if file exists first
      fileStatus = await StagingFileStatus();
      if (fileStatus?.exists) {
        // File exists, show options modal for merge/overwrite
        showPushOptionsModal = true;
      } else {
        // No existing file, go straight to passphrase modal
        showPersistModal = true;
      }
    } catch (e) {
      // If can't check, just show passphrase modal
      showPersistModal = true;
    }
  }

  async function handlePersist(passphrase: string) {
    persistLoading = true;
    persistError = '';
    try {
      const result = await StagingPersist('', passphrase, false, pushMode); // service='', keep=false
      persistResult = result;
      showPersistModal = false;
      await loadStatus();
    } catch (e) {
      persistError = parseError(e);
    } finally {
      persistLoading = false;
    }
  }

  async function handlePersistWithOptions() {
    persistLoading = true;
    persistError = '';
    try {
      const result = await StagingPersist('', pushPassphrase, false, pushMode);
      persistResult = result;
      showPushOptionsModal = false;
      await loadStatus();
    } catch (e) {
      persistError = parseError(e);
    } finally {
      persistLoading = false;
    }
  }

  function closePersistModal() {
    showPersistModal = false;
    showPushOptionsModal = false;
    persistResult = null;
  }

  // Drain modal handlers
  async function openDrainModal() {
    drainError = '';
    drainResult = null;
    drainKeep = false;
    drainForce = false;
    drainMerge = false;

    try {
      // Check file status first
      fileStatus = await StagingFileStatus();
      if (!fileStatus.exists) {
        error = 'No staging file found. Nothing to drain.';
        return;
      }

      if (fileStatus.encrypted) {
        // File is encrypted, show passphrase modal
        showDrainModal = true;
      } else {
        // File is not encrypted, show options modal directly
        showDrainOptionsModal = true;
      }
    } catch (e) {
      error = parseError(e);
    }
  }

  async function handleDrainWithPassphrase(passphrase: string) {
    drainLoading = true;
    drainError = '';
    try {
      const result = await StagingDrain('', passphrase, drainKeep, drainForce, drainMerge);
      drainResult = result;
      showDrainModal = false;
      await loadStatus();
    } catch (e) {
      drainError = parseError(e);
    } finally {
      drainLoading = false;
    }
  }

  async function handleDrainWithOptions() {
    drainLoading = true;
    drainError = '';
    try {
      const result = await StagingDrain('', '', drainKeep, drainForce, drainMerge);
      drainResult = result;
      showDrainOptionsModal = false;
      await loadStatus();
    } catch (e) {
      drainError = parseError(e);
    } finally {
      drainLoading = false;
    }
  }

  function closeDrainModal() {
    showDrainModal = false;
    showDrainOptionsModal = false;
    drainResult = null;
  }

  // Drop modal handlers
  function openDropModal() {
    dropError = '';
    showDropModal = true;
  }

  async function handleDrop() {
    dropLoading = true;
    dropError = '';
    try {
      await StagingDrop();
      showDropModal = false;
      await loadStatus();
    } catch (e) {
      dropError = parseError(e);
    } finally {
      dropLoading = false;
    }
  }

  function closeDropModal() {
    showDropModal = false;
  }

  // Computed helpers for entry display logic
  function hasValueChange(entry: gui.StagingDiffEntry): boolean {
    return entry.stagedValue !== undefined && entry.stagedValue !== '';
  }

  function showEditButton(entry: gui.StagingDiffEntry): boolean {
    return entry.operation !== 'delete' && hasValueChange(entry);
  }

  // Find tag entry for a given name
  function findTagEntry(service: string, name: string): gui.StagingDiffTagEntry | undefined {
    const tagEntries = service === 'param' ? paramTagEntries : secretTagEntries;
    return tagEntries.find(t => t.name === name);
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
    <div class="section">
      <div class="section-header">
        <h3 class="section-title">
          <span class="section-icon">P</span>
          Parameters
        </h3>
        <span class="count-badge">{paramEntries.length}</span>
        <div class="section-actions">
          {#if paramEntries.length > 0}
            <button class="btn-section btn-apply-sm" onclick={() => openApplyModal('param')}>Apply</button>
            <button class="btn-section btn-reset-sm" onclick={() => openResetModal('param')}>Reset</button>
          {/if}
        </div>
      </div>

      {#if paramEntries.length === 0}
        <div class="empty-state">No staged parameter changes</div>
      {:else}
        <ul class="entry-list">
          {#each paramEntries as entry}
            {@const tagEntry = findTagEntry('param', entry.name)}
            <li class="entry-item">
              <div class="entry-header">
                <span class="operation-badge" style="background: {getOperationColor(entry.operation || '')}">
                  {entry.operation}
                </span>
                <span class="entry-name">{entry.name}</span>
                <div class="entry-actions">
                  {#if showEditButton(entry)}
                    <button class="btn-entry" onclick={() => openEditModal('param', entry)}>Edit</button>
                  {/if}
                  <button class="btn-entry btn-unstage" onclick={() => handleUnstage('param', entry.name)}>Unstage</button>
                </div>
              </div>
              <div class="entry-tags">
                {#if tagEntry?.addTags && Object.keys(tagEntry.addTags).length > 0}
                  <div class="tag-changes tag-add">
                    <span class="tag-label">+ Tags:</span>
                    {#each Object.entries(tagEntry.addTags) as [key, value]}
                      <button class="tag-item tag-item-editable" type="button" onclick={() => openEditTagModal('param', entry.name, key, value)}>
                        {key}={value}
                        <span class="tag-delete-btn" role="button" tabindex="0" onclick={(e: MouseEvent) => { e.stopPropagation(); handleRemoveTag('param', entry.name, key); }} onkeydown={(e: KeyboardEvent) => { e.stopPropagation(); if (e.key === 'Enter') handleRemoveTag('param', entry.name, key); }}>×</span>
                      </button>
                    {/each}
                  </div>
                {/if}
                {#if tagEntry?.removeTags && tagEntry.removeTags.length > 0}
                  <div class="tag-changes tag-remove">
                    <span class="tag-label">- Tags:</span>
                    {#each tagEntry.removeTags as key}
                      <span class="tag-item">
                        {key}
                        <button class="tag-cancel-btn" onclick={() => handleCancelUntag('param', entry.name, key)} title="Cancel untag">↩</button>
                      </span>
                    {/each}
                  </div>
                {/if}
                {#if entry.operation !== 'delete'}
                  <button class="btn-add-tag" onclick={() => openAddTagModal('param', entry.name)}>+ Add Tag</button>
                {/if}
              </div>
              {#if entry.operation === 'delete'}
                {#if viewMode === 'diff'}
                  <div class="entry-diff">
                    <DiffDisplay
                      oldValue={entry.awsValue || ''}
                      newValue="(deleted)"
                      oldLabel="AWS"
                      newLabel="Staged"
                      oldSubLabel={entry.awsIdentifier || ''}
                    />
                  </div>
                {:else}
                  <pre class="entry-value entry-value-delete">(will be deleted)</pre>
                {/if}
              {:else if entry.stagedValue !== undefined && entry.stagedValue !== ''}
                {#if viewMode === 'diff' && entry.operation !== 'create'}
                  <div class="entry-diff">
                    <DiffDisplay
                      oldValue={entry.awsValue || ''}
                      newValue={entry.stagedValue}
                      oldLabel="AWS"
                      newLabel="Staged"
                      oldSubLabel={entry.awsIdentifier || ''}
                    />
                  </div>
                {:else}
                  <pre class="entry-value">{entry.stagedValue}</pre>
                {/if}
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="section">
      <div class="section-header">
        <h3 class="section-title">
          <span class="section-icon">S</span>
          Secrets
        </h3>
        <span class="count-badge">{secretEntries.length}</span>
        <div class="section-actions">
          {#if secretEntries.length > 0}
            <button class="btn-section btn-apply-sm" onclick={() => openApplyModal('secret')}>Apply</button>
            <button class="btn-section btn-reset-sm" onclick={() => openResetModal('secret')}>Reset</button>
          {/if}
        </div>
      </div>

      {#if secretEntries.length === 0}
        <div class="empty-state">No staged secret changes</div>
      {:else}
        <ul class="entry-list">
          {#each secretEntries as entry}
            {@const tagEntry = findTagEntry('secret', entry.name)}
            <li class="entry-item">
              <div class="entry-header">
                <span class="operation-badge" style="background: {getOperationColor(entry.operation || '')}">
                  {entry.operation}
                </span>
                <span class="entry-name">{entry.name}</span>
                <div class="entry-actions">
                  {#if showEditButton(entry)}
                    <button class="btn-entry" onclick={() => openEditModal('secret', entry)}>Edit</button>
                  {/if}
                  <button class="btn-entry btn-unstage" onclick={() => handleUnstage('secret', entry.name)}>Unstage</button>
                </div>
              </div>
              <div class="entry-tags">
                {#if tagEntry?.addTags && Object.keys(tagEntry.addTags).length > 0}
                  <div class="tag-changes tag-add">
                    <span class="tag-label">+ Tags:</span>
                    {#each Object.entries(tagEntry.addTags) as [key, value]}
                      <button class="tag-item tag-item-editable" type="button" onclick={() => openEditTagModal('secret', entry.name, key, value)}>
                        {key}={value}
                        <span class="tag-delete-btn" role="button" tabindex="0" onclick={(e: MouseEvent) => { e.stopPropagation(); handleRemoveTag('secret', entry.name, key); }} onkeydown={(e: KeyboardEvent) => { e.stopPropagation(); if (e.key === 'Enter') handleRemoveTag('secret', entry.name, key); }}>×</span>
                      </button>
                    {/each}
                  </div>
                {/if}
                {#if tagEntry?.removeTags && tagEntry.removeTags.length > 0}
                  <div class="tag-changes tag-remove">
                    <span class="tag-label">- Tags:</span>
                    {#each tagEntry.removeTags as key}
                      <span class="tag-item">
                        {key}
                        <button class="tag-cancel-btn" onclick={() => handleCancelUntag('secret', entry.name, key)} title="Cancel untag">↩</button>
                      </span>
                    {/each}
                  </div>
                {/if}
                {#if entry.operation !== 'delete'}
                  <button class="btn-add-tag" onclick={() => openAddTagModal('secret', entry.name)}>+ Add Tag</button>
                {/if}
              </div>
              {#if entry.operation === 'delete'}
                {#if viewMode === 'diff'}
                  <div class="entry-diff">
                    <DiffDisplay
                      oldValue={entry.awsValue || ''}
                      newValue="(deleted)"
                      oldLabel="AWS"
                      newLabel="Staged"
                      oldSubLabel={entry.awsIdentifier || ''}
                    />
                  </div>
                {:else}
                  <pre class="entry-value entry-value-delete">(will be deleted)</pre>
                {/if}
              {:else if entry.stagedValue !== undefined && entry.stagedValue !== ''}
                {#if viewMode === 'diff' && entry.operation !== 'create'}
                  <div class="entry-diff">
                    <DiffDisplay
                      oldValue={entry.awsValue || ''}
                      newValue={entry.stagedValue}
                      oldLabel="AWS"
                      newLabel="Staged"
                      oldSubLabel={entry.awsIdentifier || ''}
                    />
                  </div>
                {:else}
                  <pre class="entry-value">{entry.stagedValue}</pre>
                {/if}
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>

  <div class="actions">
    <div class="actions-center">
      <button
        class="btn-action btn-apply"
        onclick={() => openApplyModal(paramEntries.length > 0 ? 'param' : 'secret')}
        disabled={paramEntries.length === 0 && secretEntries.length === 0}
      >
        Apply All
      </button>
      <button
        class="btn-action btn-reset"
        onclick={() => {
          if (paramEntries.length > 0) openResetModal('param');
          if (secretEntries.length > 0) openResetModal('secret');
        }}
        disabled={paramEntries.length === 0 && secretEntries.length === 0}
      >
        Reset All
      </button>
    </div>
    <div class="stash-dropdown">
      <button
        class="btn-stash"
        class:has-file={fileStatus?.exists}
        onclick={() => showStashDropdown = !showStashDropdown}
      >
        Stash
        {#if fileStatus?.exists}
          <span class="stash-indicator">●</span>
        {/if}
        <span class="dropdown-arrow">▾</span>
      </button>
      {#if showStashDropdown}
        <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
        <div class="dropdown-backdrop" onclick={() => showStashDropdown = false}></div>
        <div class="dropdown-menu">
          <button
            class="dropdown-item"
            onclick={() => { showStashDropdown = false; openPersistModal(); }}
            disabled={loading || (paramEntries.length === 0 && secretEntries.length === 0)}
          >
            <span class="dropdown-icon">📤</span>
            Push <span class="dropdown-desc">(Persist)</span>
          </button>
          <button
            class="dropdown-item"
            onclick={() => { showStashDropdown = false; openDrainModal(); }}
            disabled={loading || !fileStatus?.exists}
          >
            <span class="dropdown-icon">📥</span>
            Pop <span class="dropdown-desc">(Drain)</span>
          </button>
          <div class="dropdown-divider"></div>
          <button
            class="dropdown-item dropdown-item-danger"
            onclick={() => { showStashDropdown = false; openDropModal(); }}
            disabled={loading || !fileStatus?.exists}
          >
            <span class="dropdown-icon">🗑️</span>
            Drop <span class="dropdown-desc">(Trash)</span>
          </button>
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
                  <span class="result-name">{result.name}</span>
                  <span class="result-status" class:status-created={result.status === 'created'} class:status-updated={result.status === 'updated'} class:status-deleted={result.status === 'deleted'} class:status-failed={result.status === 'failed'}>
                    {result.status}
                  </span>
                  {#if result.error}
                    <span class="result-error">{result.error}</span>
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
                  <span class="result-name">{result.name}</span>
                  <span class="result-status status-updated">tags</span>
                  {#if result.error}
                    <span class="result-error">{result.error}</span>
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
      <p class="info">This will push all staged changes to AWS.</p>
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

<!-- Persist Modal (uses PassphraseModal for encryption) -->
<PassphraseModal
  show={showPersistModal}
  mode="encrypt"
  title="Persist to File"
  onsubmit={handlePersist}
  oncancel={closePersistModal}
  loading={persistLoading}
  error={persistError}
/>

<!-- Drain Modal (uses PassphraseModal for decryption when encrypted) -->
<PassphraseModal
  show={showDrainModal}
  mode="decrypt"
  title="Drain from File"
  onsubmit={handleDrainWithPassphrase}
  oncancel={closeDrainModal}
  loading={drainLoading}
  error={drainError}
/>

<!-- Drain Options Modal (for unencrypted files) -->
<Modal title="Drain from File" show={showDrainOptionsModal} onclose={closeDrainModal}>
  <div class="drain-options">
    {#if drainError}
      <div class="modal-error">{drainError}</div>
    {/if}

    {#if drainResult}
      <div class="drain-result">
        <div class="result-icon success">✓</div>
        <h4>Successfully loaded from file</h4>
        <div class="result-stats">
          <span class="stat">{drainResult.entryCount} entries</span>
          <span class="stat">{drainResult.tagCount} tag changes</span>
          {#if drainResult.merged}
            <span class="stat merged">(merged)</span>
          {/if}
        </div>
        <div class="form-actions">
          <button type="button" class="btn-primary" onclick={closeDrainModal}>Close</button>
        </div>
      </div>
    {:else}
      <p>Load staged changes from file into memory?</p>
      <p class="info">This will load changes from the staging file ({fileStatus?.encrypted ? 'encrypted' : 'plain text'}).</p>

      <div class="options-group">
        <label class="checkbox-label">
          <input type="checkbox" bind:checked={drainKeep} />
          <span>Keep file after loading</span>
        </label>
        <label class="checkbox-label">
          <input type="checkbox" bind:checked={drainMerge} />
          <span>Merge with existing changes</span>
        </label>
        <label class="checkbox-label">
          <input type="checkbox" bind:checked={drainForce} />
          <span>Force overwrite existing changes</span>
        </label>
      </div>

      <div class="form-actions">
        <button type="button" class="btn-secondary" onclick={closeDrainModal}>Cancel</button>
        <button type="button" class="btn-drain-action" onclick={handleDrainWithOptions} disabled={drainLoading}>
          {drainLoading ? 'Loading...' : 'Load from File'}
        </button>
      </div>
    {/if}
  </div>
</Modal>

<!-- Persist Result Modal -->
{#if persistResult}
<Modal title="Push Complete" show={true} onclose={() => persistResult = null}>
  <div class="persist-result">
    <div class="result-icon success">✓</div>
    <h4>Successfully saved to file</h4>
    <div class="result-stats">
      <span class="stat">{persistResult.entryCount} entries</span>
      <span class="stat">{persistResult.tagCount} tag changes</span>
    </div>
    <div class="form-actions">
      <button type="button" class="btn-primary" onclick={() => persistResult = null}>Close</button>
    </div>
  </div>
</Modal>
{/if}

<!-- Push Options Modal (when stash file already exists) -->
<Modal title="Stash File Exists" show={showPushOptionsModal} onclose={closePersistModal}>
  <div class="push-options">
    {#if persistError}
      <div class="modal-error">{persistError}</div>
    {/if}

    <p>A stash file already exists. How do you want to proceed?</p>

    <div class="options-group">
      <label class="radio-label">
        <input type="radio" name="pushMode" value="overwrite" bind:group={pushMode} />
        <span>Overwrite</span>
        <span class="option-desc">Replace existing stash file</span>
      </label>
      <label class="radio-label">
        <input type="radio" name="pushMode" value="merge" bind:group={pushMode} />
        <span>Merge</span>
        <span class="option-desc">Merge with existing stash file</span>
      </label>
    </div>

    <div class="form-group">
      <label for="push-passphrase">Passphrase (optional, for encryption)</label>
      <input
        id="push-passphrase"
        type="password"
        class="form-input"
        bind:value={pushPassphrase}
        placeholder="Leave empty for no encryption"
      />
    </div>

    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={closePersistModal}>Cancel</button>
      <button type="button" class="btn-push-action" onclick={handlePersistWithOptions} disabled={persistLoading}>
        {persistLoading ? 'Pushing...' : 'Push'}
      </button>
    </div>
  </div>
</Modal>

<!-- Drop Confirmation Modal -->
<Modal title="Drop Stash" show={showDropModal} onclose={closeDropModal}>
  <div class="modal-confirm">
    {#if dropError}
      <div class="modal-error">{dropError}</div>
    {/if}
    <p>Delete the stash file without loading?</p>
    <p class="warning">This will permanently delete the stash file. This action cannot be undone.</p>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={closeDropModal}>Cancel</button>
      <button type="button" class="btn-danger" onclick={handleDrop} disabled={dropLoading}>
        {dropLoading ? 'Dropping...' : 'Drop'}
      </button>
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
    overflow-y: auto;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 24px;
  }

  .section {
    background: #1a1a2e;
    border-radius: 8px;
    overflow: hidden;
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px;
    border-bottom: 1px solid #2d2d44;
  }

  .section-title {
    margin: 0;
    font-size: 14px;
    color: #fff;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .section-icon {
    width: 24px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 4px;
    font-weight: bold;
    font-size: 12px;
  }

  .count-badge {
    font-size: 12px;
    padding: 2px 8px;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 10px;
    color: #888;
  }

  .section-actions {
    margin-left: auto;
    display: flex;
    gap: 8px;
  }

  .btn-section {
    padding: 4px 10px;
    border: none;
    border-radius: 4px;
    font-size: 12px;
    cursor: pointer;
  }

  .btn-apply-sm {
    background: #4caf50;
    color: #fff;
  }

  .btn-apply-sm:hover {
    background: #43a047;
  }

  .btn-reset-sm {
    background: #f44336;
    color: #fff;
  }

  .btn-reset-sm:hover {
    background: #e53935;
  }

  .entry-actions {
    display: flex;
    gap: 6px;
    margin-left: auto;
  }

  .btn-entry {
    padding: 2px 8px;
    font-size: 11px;
    border: none;
    border-radius: 3px;
    cursor: pointer;
    background: #2d2d44;
    color: #fff;
  }

  .btn-entry:hover {
    background: #3d3d54;
  }

  .btn-unstage {
    background: #666;
  }

  .btn-unstage:hover {
    background: #888;
  }

  .entry-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .entry-item {
    padding: 12px 16px;
    border-bottom: 1px solid #2d2d44;
  }

  .entry-item:last-child {
    border-bottom: none;
  }

  .entry-header {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .operation-badge {
    font-size: 10px;
    padding: 2px 8px;
    border-radius: 3px;
    color: #fff;
    text-transform: uppercase;
    font-weight: bold;
  }

  .entry-name {
    flex: 1;
    font-family: monospace;
    font-size: 13px;
    color: #4fc3f7;
  }

  .entry-diff {
    margin-top: 12px;
  }

  .entry-value {
    margin: 8px 0 0 0;
    padding: 8px;
    background: #0f0f1a;
    border-radius: 4px;
    font-family: monospace;
    font-size: 12px;
    color: #a5d6a7;
    white-space: pre-wrap;
    word-break: break-all;
  }

  .entry-value-delete {
    color: #ef9a9a;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    align-items: center;
    gap: 12px 24px;
    padding: 16px;
    background: #1a1a2e;
    border-top: 1px solid #2d2d44;
  }

  .actions-center {
    display: flex;
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

  /* Stash dropdown */
  .stash-dropdown {
    position: relative;
  }

  .btn-stash {
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

  .btn-stash:hover {
    background: #3d3d54;
  }

  .btn-stash.has-file {
    background: #3d3d54;
  }

  .stash-indicator {
    color: #4fc3f7;
    font-size: 10px;
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

  .dropdown-item-danger:hover:not(:disabled) {
    background: rgba(244, 67, 54, 0.2);
  }

  .dropdown-icon {
    font-size: 16px;
  }

  .dropdown-desc {
    font-size: 12px;
    font-weight: 400;
    color: #888;
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

  /* Tag display styles */
  .entry-tags {
    margin-top: 8px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .tag-changes {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    font-size: 12px;
    padding: 4px 8px;
    border-radius: 4px;
  }

  .tag-add {
    background: rgba(76, 175, 80, 0.1);
    border: 1px solid rgba(76, 175, 80, 0.3);
  }

  .tag-remove {
    background: rgba(244, 67, 54, 0.1);
    border: 1px solid rgba(244, 67, 54, 0.3);
  }

  .tag-label {
    font-weight: bold;
  }

  .tag-add .tag-label {
    color: #4caf50;
  }

  .tag-remove .tag-label {
    color: #f44336;
  }

  .tag-item {
    font-family: monospace;
    padding: 2px 6px;
    border-radius: 3px;
    background: rgba(255, 255, 255, 0.1);
    color: #ddd;
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .tag-item-editable {
    cursor: pointer;
    transition: background 0.2s;
  }

  .tag-item-editable:hover {
    background: rgba(255, 255, 255, 0.2);
  }

  .tag-delete-btn,
  .tag-cancel-btn {
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
    padding: 0 2px;
    margin-left: 2px;
  }

  .tag-delete-btn:hover {
    color: #f44336;
  }

  .tag-cancel-btn:hover {
    color: #4caf50;
  }

  .btn-add-tag {
    padding: 2px 8px;
    font-size: 11px;
    border: 1px dashed #4caf50;
    border-radius: 3px;
    background: transparent;
    color: #4caf50;
    cursor: pointer;
    margin-top: 4px;
    transition: all 0.2s;
  }

  .btn-add-tag:hover {
    background: rgba(76, 175, 80, 0.1);
    border-style: solid;
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

  /* Drain options modal styles */
  .drain-options {
    text-align: center;
  }

  .drain-options p {
    color: #ccc;
    margin: 0 0 12px 0;
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

  .btn-drain-action {
    padding: 8px 16px;
    background: #9c27b0;
    color: #fff;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
  }

  .btn-drain-action:hover:not(:disabled) {
    background: #7b1fa2;
  }

  .btn-drain-action:disabled {
    opacity: 0.6;
    cursor: not-allowed;
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
