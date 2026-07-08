<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { SecretAddTag, SecretCreate, SecretDelete, SecretDiff, SecretList, SecretLog, SecretRemoveTag, SecretRestore, SecretShow, SecretUpdate, StagingAdd, StagingAddTag, StagingCheckStatus, StagingDelete, StagingEdit, StagingRemoveTag } from '../../wailsjs/go/gui/App';
  import type { gui } from '../../wailsjs/go/models';
  import DiffDisplay from './DiffDisplay.svelte';
  import CloseIcon from './icons/CloseIcon.svelte';
  import EyeIcon from './icons/EyeIcon.svelte';
  import EyeOffIcon from './icons/EyeOffIcon.svelte';
  import Modal from './Modal.svelte';
  import StagingBanner from './StagingBanner.svelte';
  import TagList from './TagList.svelte';
  import { createDiffMode } from './useDiffMode.svelte';
  import { createDebouncer, formatDate, formatJsonValue, maskValue, parseError } from './viewUtils';
  import './common.css';

  interface Props {
    capability?: gui.ServiceCapability;
    provider?: string;
    onnavigatetostaging?: () => void;
    onstagingchange?: () => void;
  }

  let { capability, provider = '', onnavigatetostaging, onstagingchange }: Props = $props();

  // Capability-driven visibility. Absent capability defaults to AWS-like (true).
  const stagingEnabled = $derived(capability?.hasStaging ?? true);
  const tagsEnabled = $derived(capability?.hasTags ?? true);
  const historyEnabled = $derived(capability?.hasVersionHistory ?? true);
  const restoreEnabled = $derived(capability?.hasRestore ?? true);
  const forceDeleteEnabled = $derived(capability?.hasForceDelete ?? true);
  const recoveryWindowEnabled = $derived(capability?.hasRecoveryWindow ?? true);

  // Staging labels and per-version state are two independent concepts (#419), so
  // render each from the field that actually carries it rather than guessing
  // from the provider string. AWS Secrets Manager populates stagingLabels
  // (AWSCURRENT/AWSPENDING/...); Google Cloud + Azure Key Vault populate state
  // (enabled/disabled/destroyed). A version never has both.

  const PAGE_SIZE = 50;
  const debounce = createDebouncer(300);
  const diffMode = createDiffMode<string>();

  // Cancel a pending filter debounce on unmount so it can't fire against a new
  // provider after a {#key} remount.
  onDestroy(() => debounce.cancel());

  let prefix = $state('');
  let filter = $state('');
  let withValue = $state(false);
  let nextToken = $state('');
  let loadingMore = $state(false);

  // Track if initial load has happened
  let initialLoadDone = $state(false);

  interface LoadSecretsOptions {
    prefix: string;
    filter: string;
    withValue: boolean;
  }

  // Reload when filter options change
  $effect(() => {
    const opts = { prefix, filter, withValue }; // read values to create dependencies
    if (initialLoadDone) {
      loadSecrets(opts);
    }
  });

  function handlePrefixInput() {
    debounce(() => loadSecrets({ prefix, filter, withValue }));
  }

  function handleFilterInput() {
    debounce(() => loadSecrets({ prefix, filter, withValue }));
  }
  let loading = $state(false);
  let error = $state('');

  let entries: gui.SecretListEntry[] = $state([]);
  let selectedSecret: string | null = $state(null);
  let secretDetail: gui.SecretShowResult | null = $state(null);
  let secretLog: gui.SecretLogEntry[] = $state([]);
  let detailLoading = $state(false);
  let showValue = $state(false);

  // Staging status for the selected item
  let stagingStatus: { hasEntry: boolean; hasTags: boolean } | null = $state(null);

  // Modal states
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let showDeleteModal = $state(false);
  let showDiffModal = $state(false);
  let showRestoreModal = $state(false);
  let createForm = $state({ name: '', value: '' });
  let editForm = $state({ name: '', value: '' });
  let deleteTarget = $state('');
  let forceDelete = $state(false);
  let modalLoading = $state(false);
  let modalError = $state('');
  let immediateMode = $state(false); // When false (default), changes are staged
  // When staging is unavailable, every write is immediate (no staging toggle).
  const immediate = $derived(immediateMode || !stagingEnabled);

  // Diff state
  let diffResult: gui.SecretDiffResult | null = $state(null);

  // Restore state
  let restoreTarget = $state('');

  // Tag state
  let showTagModal = $state(false);
  let tagForm = $state({ key: '', value: '' });
  let tagLoading = $state(false);
  let tagError = $state('');

  // Tag remove state
  let showRemoveTagModal = $state(false);
  let removeTagTarget = $state('');
  let removeTagLoading = $state(false);
  let removeTagError = $state('');

  // Infinite scroll
  let sentinelElement: HTMLDivElement | undefined = $state(undefined);
  let observer: IntersectionObserver | null = null;

  async function loadSecrets(opts: LoadSecretsOptions) {
    loading = true;
    error = '';
    nextToken = '';
    try {
      const result = await SecretList(opts.prefix, opts.withValue, opts.filter, PAGE_SIZE, '');
      entries = result?.entries || [];
      nextToken = result?.nextToken || '';
    } catch (e) {
      error = parseError(e);
      entries = [];
    } finally {
      loading = false;
    }
  }

  async function loadMore(opts: LoadSecretsOptions) {
    if (!nextToken || loadingMore || loading) return;

    loadingMore = true;
    try {
      const result = await SecretList(opts.prefix, opts.withValue, opts.filter, PAGE_SIZE, nextToken);
      entries = [...entries, ...(result?.entries || [])];
      nextToken = result?.nextToken || '';
    } catch (e) {
      error = parseError(e);
    } finally {
      loadingMore = false;
    }
  }

  function setupIntersectionObserver() {
    if (observer) observer.disconnect();

    observer = new IntersectionObserver(
      (observedEntries) => {
        if (observedEntries[0]?.isIntersecting && nextToken && !loadingMore && !loading) {
          loadMore({ prefix, filter, withValue });
        }
      },
      { rootMargin: '100px' }
    );

    if (sentinelElement) {
      observer.observe(sentinelElement);
    }
  }

  $effect(() => {
    if (sentinelElement) {
      setupIntersectionObserver();
    }
    return () => {
      if (observer) observer.disconnect();
    };
  });

  async function selectSecret(name: string) {
    selectedSecret = name;
    detailLoading = true;
    showValue = withValue;
    stagingStatus = null;
    error = '';
    // History is best-effort: fetch it only when supported, and use allSettled
    // so a failed SecretLog never blanks out the value shown by SecretShow.
    const [detailResult, logResult] = await Promise.allSettled([
      SecretShow(name),
      historyEnabled ? SecretLog(name, 10) : Promise.resolve(null),
    ]);
    if (detailResult.status === 'fulfilled') {
      secretDetail = detailResult.value;
    } else {
      error = parseError(detailResult.reason);
    }
    secretLog = logResult.status === 'fulfilled' ? (logResult.value?.entries ?? []) : [];
    detailLoading = false;

    // Decoupled staging status: only for staging-capable providers; a failure
    // just means no banner and never breaks the detail pane.
    if (stagingEnabled) {
      try {
        stagingStatus = await StagingCheckStatus('secret', name, '');
      } catch {
        stagingStatus = null;
      }
    }
  }

  function closeDetail() {
    selectedSecret = null;
    secretDetail = null;
    secretLog = [];
    showValue = false;
    stagingStatus = null;
  }

  function toggleShowValue() {
    showValue = !showValue;
  }

  // Create modal
  function openCreateModal() {
    createForm = { name: prefix || '', value: '' };
    modalError = '';
    showCreateModal = true;
  }

  async function handleCreate(e: SubmitEvent) {
    e.preventDefault();
    if (!createForm.name || !createForm.value) {
      modalError = 'Name and value are required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      if (immediate) {
        await SecretCreate(createForm.name, createForm.value);
        await loadSecrets({ prefix, filter, withValue });
      } else {
        await StagingAdd('secret', createForm.name, createForm.value, '');
        onstagingchange?.();
      }
      showCreateModal = false;
    } catch (err) {
      modalError = parseError(err);
    } finally {
      modalLoading = false;
    }
  }

  // Edit modal
  function openEditModal() {
    if (secretDetail) {
      editForm = { name: secretDetail.name, value: secretDetail.value };
    }
    modalError = '';
    showEditModal = true;
  }

  async function handleEdit(e: SubmitEvent) {
    e.preventDefault();
    if (!editForm.value) {
      modalError = 'Value is required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      if (immediate) {
        await SecretUpdate(editForm.name, editForm.value);
        await Promise.all([
          loadSecrets({ prefix, filter, withValue }),
          selectSecret(editForm.name)
        ]);
      } else {
        await StagingEdit('secret', editForm.name, editForm.value, '');
        onstagingchange?.();
        // Refresh staging status to update the indicator
        await selectSecret(editForm.name);
      }
      showEditModal = false;
    } catch (err) {
      modalError = parseError(err);
    } finally {
      modalLoading = false;
    }
  }

  // Delete modal
  function openDeleteModal(name: string) {
    deleteTarget = name;
    forceDelete = false;
    modalError = '';
    showDeleteModal = true;
  }

  async function handleDelete() {
    modalLoading = true;
    modalError = '';
    try {
      if (immediate) {
        await SecretDelete(deleteTarget, forceDelete);
        if (selectedSecret === deleteTarget) {
          closeDetail();
        }
        await loadSecrets({ prefix, filter, withValue });
      } else {
        // Stage delete with recovery window (default 30 days unless force)
        await StagingDelete('secret', deleteTarget, forceDelete, forceDelete ? 0 : 30, '');
        onstagingchange?.();
      }
      showDeleteModal = false;
    } catch (err) {
      modalError = parseError(err);
    } finally {
      modalLoading = false;
    }
  }

  // Diff functions
  async function executeDiff() {
    if (!selectedSecret || !diffMode.canCompare) return;

    modalLoading = true;
    modalError = '';
    try {
      const spec1 = `${selectedSecret}#${diffMode.selectedVersions[0]}`;
      const spec2 = `${selectedSecret}#${diffMode.selectedVersions[1]}`;
      diffResult = await SecretDiff(spec1, spec2);
      showDiffModal = true;
    } catch (err) {
      error = parseError(err);
    } finally {
      modalLoading = false;
    }
  }

  function closeDiffModal() {
    showDiffModal = false;
    diffResult = null;
    diffMode.reset();
  }

  // Restore functions
  function openRestoreModal(name: string) {
    restoreTarget = name;
    modalError = '';
    showRestoreModal = true;
  }

  async function handleRestore() {
    modalLoading = true;
    modalError = '';
    try {
      await SecretRestore(restoreTarget);
      showRestoreModal = false;
      await loadSecrets({ prefix, filter, withValue });
    } catch (err) {
      modalError = parseError(err);
    } finally {
      modalLoading = false;
    }
  }

  // Tag functions
  function openTagModal() {
    tagForm = { key: '', value: '' };
    tagError = '';
    showTagModal = true;
  }

  async function handleAddTag(e: SubmitEvent) {
    e.preventDefault();
    if (!selectedSecret || !tagForm.key) {
      tagError = 'Key is required';
      return;
    }
    tagLoading = true;
    tagError = '';
    try {
      if (immediate) {
        await SecretAddTag(selectedSecret, tagForm.key, tagForm.value);
      } else {
        await StagingAddTag('secret', selectedSecret, tagForm.key, tagForm.value, '');
        onstagingchange?.();
      }
      showTagModal = false;
      await selectSecret(selectedSecret);
    } catch (err) {
      tagError = parseError(err);
    } finally {
      tagLoading = false;
    }
  }

  function openRemoveTagModal(key: string) {
    removeTagTarget = key;
    removeTagError = '';
    showRemoveTagModal = true;
  }

  async function handleRemoveTag() {
    if (!selectedSecret || !removeTagTarget) return;
    removeTagLoading = true;
    removeTagError = '';
    try {
      if (immediate) {
        await SecretRemoveTag(selectedSecret, removeTagTarget);
      } else {
        await StagingRemoveTag('secret', selectedSecret, removeTagTarget, '');
        onstagingchange?.();
      }
      showRemoveTagModal = false;
      await selectSecret(selectedSecret);
    } catch (err) {
      removeTagError = parseError(err);
    } finally {
      removeTagLoading = false;
    }
  }

  onMount(async () => {
    await loadSecrets({ prefix, filter, withValue });
    initialLoadDone = true;
  });
</script>

<div class="view-container">
  <div class="filter-bar">
    <input
      type="text"
      class="filter-input prefix-input"
      placeholder="Prefix"
      bind:value={prefix}
      oninput={handlePrefixInput}
    />
    <input
      type="text"
      class="filter-input regex-input"
      placeholder="Filter (regex)"
      bind:value={filter}
      oninput={handleFilterInput}
    />
    <label class="checkbox-label">
      <input type="checkbox" bind:checked={withValue} />
      Show Values
    </label>
    <button class="btn-primary" onclick={() => loadSecrets({ prefix, filter, withValue })} disabled={loading}>
      {loading ? 'Loading...' : 'Refresh'}
    </button>
    <button class="btn-secondary" onclick={openCreateModal}>
      + New
    </button>
    {#if restoreEnabled}
      <button class="btn-secondary btn-restore" onclick={() => openRestoreModal('')}>
        Restore
      </button>
    {/if}
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <div class="content">
    <div class="list-panel" class:collapsed={selectedSecret !== null}>
      {#if entries.length === 0 && !loading}
        <div class="empty-state">
          No secrets found. Try adjusting the prefix filter.
        </div>
      {:else}
        <ul class="item-list">
          {#each entries as entry}
            <li class="item-entry" class:selected={selectedSecret === entry.name}>
              <button class="item-button" onclick={() => selectSecret(entry.name)}>
                <span class="item-name secret">{entry.name}</span>
                {#if entry.value !== undefined}
                  <span class="item-value">{entry.value}</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
        <!-- Sentinel for infinite scroll -->
        <div bind:this={sentinelElement} class="scroll-sentinel">
          {#if loadingMore}
            <div class="loading-more">Loading more...</div>
          {:else if nextToken}
            <div class="load-more-hint">Scroll for more</div>
          {/if}
        </div>
      {/if}
    </div>

    {#if selectedSecret}
      <div class="detail-panel">
        <div class="detail-header">
          <h3 class="detail-title secret">{selectedSecret}</h3>
          <div class="detail-actions">
            <button class="btn-action-sm" onclick={openEditModal}>Edit</button>
            <button class="btn-action-sm btn-danger" onclick={() => selectedSecret && openDeleteModal(selectedSecret)}>Delete</button>
            {#if secretLog.length >= 2}
              <button class="btn-action-sm" class:active={diffMode.active} onclick={diffMode.toggle}>
                {diffMode.active ? 'Cancel' : 'Compare'}
              </button>
            {/if}
            <button class="btn-close" onclick={closeDetail}>
              <CloseIcon />
            </button>
          </div>
        </div>

        {#if stagingEnabled}
          <StagingBanner {stagingStatus} onnavigate={onnavigatetostaging} />
        {/if}

        {#if detailLoading}
          <div class="loading">Loading...</div>
        {:else if secretDetail}
          <div class="detail-content">
            <div class="detail-section">
              <div class="section-header">
                <h4>Current Value</h4>
                <button
                  class="btn-toggle"
                  class:active={showValue}
                  onclick={toggleShowValue}
                  title={showValue ? 'Hide value' : 'Show value'}
                >
                  {#if showValue}
                    <EyeOffIcon />
                    Hide
                  {:else}
                    <EyeIcon />
                    Show
                  {/if}
                </button>
              </div>
              <pre class="value-display" class:masked={!showValue}>{showValue ? formatJsonValue(secretDetail.value) : maskValue(secretDetail.value)}</pre>
            </div>

            <div class="detail-meta">
              <div class="meta-item">
                <span class="meta-label">Version ID</span>
                <span class="meta-value mono">{secretDetail.versionId}</span>
              </div>
              {#if secretDetail.state}
                <div class="meta-item">
                  <span class="meta-label">State</span>
                  <span class="meta-value">
                    <span class="badge badge-stage">{secretDetail.state}</span>
                  </span>
                </div>
              {:else if (secretDetail.stagingLabels || []).length > 0}
                <div class="meta-item">
                  <span class="meta-label">Staging labels</span>
                  <span class="meta-value">
                    {#each secretDetail.stagingLabels || [] as label}
                      <span class="badge badge-stage">{label}</span>
                    {/each}
                  </span>
                </div>
              {/if}
              <div class="meta-item">
                <span class="meta-label">Created</span>
                <span class="meta-value">{formatDate(secretDetail.createdDate)}</span>
              </div>
            </div>

            {#if secretDetail.description}
              <div class="detail-section">
                <h4>Description</h4>
                <p class="description-text">{secretDetail.description}</p>
              </div>
            {/if}

            {#if secretDetail.arn}
              <div class="detail-section">
                <h4>ARN</h4>
                <code class="arn-display">{secretDetail.arn}</code>
              </div>
            {/if}

            {#if tagsEnabled}
              <TagList tags={secretDetail.tags} serviceClass="secret" {provider} onadd={openTagModal} onremove={openRemoveTagModal} />
            {/if}

            {#if historyEnabled && secretLog.length > 0}
              <div class="detail-section">
                <div class="section-header-history">
                  <h4>Version History</h4>
                  {#if diffMode.canCompare}
                    <button class="btn-action-sm btn-compare" onclick={executeDiff} disabled={modalLoading}>
                      {modalLoading ? 'Comparing...' : 'Show Diff'}
                    </button>
                  {/if}
                </div>
                {#if diffMode.active}
                  <p class="diff-hint">Select 2 versions to compare</p>
                {/if}
                <ul class="history-list">
                  {#each secretLog as logEntry}
                    <li
                      class="history-item"
                      class:current-secret={logEntry.isCurrent}
                      class:selectable={diffMode.active}
                      class:selected={diffMode.isSelected(logEntry.versionId)}
                    >
                      {#if diffMode.active}
                        <label class="diff-checkbox">
                          <input
                            type="checkbox"
                            checked={diffMode.isSelected(logEntry.versionId)}
                            disabled={diffMode.isDisabled(logEntry.versionId)}
                            onchange={() => diffMode.toggleSelection(logEntry.versionId)}
                          />
                        </label>
                      {/if}
                      <div class="history-content">
                        <div class="history-header">
                          <span class="history-version mono" title={logEntry.versionId}>{logEntry.versionId}</span>
                          {#if logEntry.isCurrent}
                            <span class="badge badge-current">current</span>
                          {/if}
                          <span class="history-date">{formatDate(logEntry.created)}</span>
                        </div>
                        {#if logEntry.state}
                          <div class="history-labels">
                            <span class="badge badge-stage small">{logEntry.state}</span>
                          </div>
                        {:else if (logEntry.stagingLabels || []).length > 0}
                          <div class="history-labels">
                            {#each logEntry.stagingLabels || [] as label}
                              <span class="badge badge-stage small">{label}</span>
                            {/each}
                          </div>
                        {/if}
                        <pre class="history-value" class:masked={!showValue}>{showValue ? formatJsonValue(logEntry.value) : maskValue(logEntry.value)}</pre>
                      </div>
                    </li>
                  {/each}
                </ul>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

<!-- Create Modal -->
<Modal title="New Secret" show={showCreateModal} onclose={() => showCreateModal = false}>
  <form class="modal-form" onsubmit={handleCreate}>
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <div class="form-group">
      <label for="secret-name">Name</label>
      <input
        id="secret-name"
        type="text"
        class="form-input"
        bind:value={createForm.name}
        placeholder="my-secret-name"
      />
    </div>
    <div class="form-group">
      <label for="secret-value">Value</label>
      <textarea
        id="secret-value"
        class="form-input form-textarea"
        bind:value={createForm.value}
        placeholder={'{"username": "admin", "password": "secret"}'}
        rows="5"
      ></textarea>
    </div>
    {#if stagingEnabled}
      <label class="checkbox-label immediate-checkbox">
        <input type="checkbox" bind:checked={immediateMode} />
        <span>Apply immediately (skip staging)</span>
      </label>
    {/if}
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showCreateModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? (immediate ? 'Creating...' : 'Staging...') : (immediate ? 'Create' : 'Stage')}
      </button>
    </div>
  </form>
</Modal>

<!-- Edit Modal -->
<Modal title="Edit Secret" show={showEditModal} onclose={() => showEditModal = false}>
  <form class="modal-form" onsubmit={handleEdit}>
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <div class="form-group">
      <label for="edit-secret-name">Name</label>
      <input
        id="edit-secret-name"
        type="text"
        class="form-input"
        value={editForm.name}
        disabled
      />
    </div>
    <div class="form-group">
      <label for="edit-secret-value">Value</label>
      <textarea
        id="edit-secret-value"
        class="form-input form-textarea"
        bind:value={editForm.value}
        rows="8"
      ></textarea>
    </div>
    {#if stagingEnabled}
      <label class="checkbox-label immediate-checkbox">
        <input type="checkbox" bind:checked={immediateMode} />
        <span>Apply immediately (skip staging)</span>
      </label>
    {/if}
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showEditModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? (immediate ? 'Saving...' : 'Staging...') : (immediate ? 'Save' : 'Stage')}
      </button>
    </div>
  </form>
</Modal>

<!-- Delete Modal -->
<Modal title="Delete Secret" show={showDeleteModal} onclose={() => showDeleteModal = false}>
  <div class="modal-confirm">
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <p>Are you sure you want to delete this secret?</p>
    <code class="delete-target secret">{deleteTarget}</code>
    {#if forceDeleteEnabled}
      <label class="checkbox-label force-delete">
        <input type="checkbox" bind:checked={forceDelete} />
        <span>Force delete (skip recovery window)</span>
      </label>
    {/if}
    <p class="warning">
      {#if immediate}
        {#if forceDelete}
          This will permanently delete the secret immediately!
        {:else if recoveryWindowEnabled}
          The secret will be scheduled for deletion with a recovery window.
        {:else}
          This will delete the secret.
        {/if}
      {:else}
        This will stage a delete operation.
      {/if}
    </p>
    {#if stagingEnabled}
      <label class="checkbox-label immediate-checkbox">
        <input type="checkbox" bind:checked={immediateMode} />
        <span>Apply immediately (skip staging)</span>
      </label>
    {/if}
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showDeleteModal = false}>Cancel</button>
      <button type="button" class="btn-danger" onclick={handleDelete} disabled={modalLoading}>
        {modalLoading ? (immediate ? 'Deleting...' : 'Staging...') : (immediate ? 'Delete' : 'Stage Delete')}
      </button>
    </div>
  </div>
</Modal>

<!-- Diff Modal -->
<Modal title="Version Comparison" show={showDiffModal} onclose={closeDiffModal}>
  {#if diffResult}
    <DiffDisplay
      oldValue={formatJsonValue(diffResult.oldValue)}
      newValue={formatJsonValue(diffResult.newValue)}
      oldLabel="Old"
      newLabel="New"
      oldSubLabel={diffResult.oldVersionId}
      newSubLabel={diffResult.newVersionId}
    />
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={closeDiffModal}>Close</button>
    </div>
  {/if}
</Modal>

<!-- Restore Modal -->
<Modal title="Restore Secret" show={showRestoreModal} onclose={() => showRestoreModal = false}>
  <div class="modal-form">
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <p class="restore-info">Restore a previously deleted secret that is still within its recovery window.</p>
    <div class="form-group">
      <label for="restore-name">Secret Name</label>
      <input
        id="restore-name"
        type="text"
        class="form-input"
        bind:value={restoreTarget}
        placeholder="my-deleted-secret"
      />
    </div>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showRestoreModal = false}>Cancel</button>
      <button type="button" class="btn-primary btn-restore-confirm" onclick={handleRestore} disabled={modalLoading || !restoreTarget}>
        {modalLoading ? 'Restoring...' : 'Restore'}
      </button>
    </div>
  </div>
</Modal>

<!-- Tag Modal -->
<Modal title="Add Tag" show={showTagModal} onclose={() => showTagModal = false}>
  <form class="modal-form" onsubmit={handleAddTag}>
    {#if tagError}
      <div class="modal-error">{tagError}</div>
    {/if}
    <div class="form-group">
      <label for="tag-key">Key</label>
      <input
        id="tag-key"
        type="text"
        class="form-input"
        bind:value={tagForm.key}
        placeholder="tag-key"
      />
    </div>
    <div class="form-group">
      <label for="tag-value">Value</label>
      <input
        id="tag-value"
        type="text"
        class="form-input"
        bind:value={tagForm.value}
        placeholder="tag-value"
      />
    </div>
    {#if stagingEnabled}
      <label class="checkbox-label immediate-checkbox">
        <input type="checkbox" bind:checked={immediateMode} />
        <span>Apply immediately (skip staging)</span>
      </label>
    {/if}
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showTagModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={tagLoading}>
        {tagLoading ? (immediate ? 'Adding...' : 'Staging...') : (immediate ? 'Add Tag' : 'Stage Tag')}
      </button>
    </div>
  </form>
</Modal>

<!-- Remove Tag Modal -->
<Modal title="Remove Tag" show={showRemoveTagModal} onclose={() => showRemoveTagModal = false}>
  <div class="modal-confirm">
    {#if removeTagError}
      <div class="modal-error">{removeTagError}</div>
    {/if}
    <p>Are you sure you want to remove this tag?</p>
    <code class="delete-target secret">{removeTagTarget}</code>
    <p class="warning">{immediate ? 'This action will take effect immediately.' : 'This will stage a tag removal operation.'}</p>
    {#if stagingEnabled}
      <label class="checkbox-label immediate-checkbox">
        <input type="checkbox" bind:checked={immediateMode} />
        <span>Apply immediately (skip staging)</span>
      </label>
    {/if}
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showRemoveTagModal = false}>Cancel</button>
      <button type="button" class="btn-danger" onclick={handleRemoveTag} disabled={removeTagLoading}>
        {removeTagLoading ? (immediate ? 'Removing...' : 'Staging...') : (immediate ? 'Remove' : 'Stage Remove')}
      </button>
    </div>
  </div>
</Modal>

<style>
  /* SecretView-specific styles - shared styles are in common.css */

  .section-header-history {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
  }

  .section-header-history h4 {
    margin: 0;
    font-size: 12px;
    text-transform: uppercase;
    color: #888;
    letter-spacing: 0.5px;
  }

  .restore-info {
    color: #888;
    font-size: 13px;
    margin: 0 0 16px 0;
  }

  .btn-restore-confirm {
    background: #4caf50;
  }

  .btn-restore-confirm:hover {
    background: #43a047;
  }
</style>
