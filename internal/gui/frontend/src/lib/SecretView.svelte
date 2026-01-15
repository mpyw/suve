<script lang="ts">
  import { onMount } from 'svelte';
  import { SecretList, SecretShow, SecretLog, SecretCreate, SecretUpdate, SecretDelete, SecretDiff, SecretRestore, SecretAddTag, SecretRemoveTag, StagingAdd, StagingEdit, StagingDelete, StagingAddTag, StagingRemoveTag, StagingCheckStatus } from '../../wailsjs/go/gui/App';
  import type { gui } from '../../wailsjs/go/models';
  import { withRetry } from './retry';
  import CloseIcon from './icons/CloseIcon.svelte';
  import EyeIcon from './icons/EyeIcon.svelte';
  import EyeOffIcon from './icons/EyeOffIcon.svelte';
  import Modal from './Modal.svelte';
  import DiffDisplay from './DiffDisplay.svelte';
  import { maskValue, formatDate, formatJsonValue, parseError, createDebouncer } from './viewUtils';
  import { createDiffMode } from './useDiffMode.svelte';
  import './common.css';

  interface Props {
    onnavigatetostaging?: () => void;
    onstagingchange?: () => void;
  }

  let { onnavigatetostaging, onstagingchange }: Props = $props();

  const PAGE_SIZE = 50;
  const debounce = createDebouncer(300);
  const diffMode = createDiffMode<string>();

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
        if (observedEntries[0].isIntersecting && nextToken && !loadingMore && !loading) {
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
    try {
      const [detail, log, staging] = await Promise.all([
        SecretShow(name),
        SecretLog(name, 10),
        withRetry(() => StagingCheckStatus('secret', name))
      ]);
      secretDetail = detail;
      secretLog = log?.entries || [];
      stagingStatus = staging;
    } catch (e) {
      error = parseError(e);
    } finally {
      detailLoading = false;
    }
  }

  function closeDetail() {
    selectedSecret = null;
    secretDetail = null;
    secretLog = [];
    showValue = false;
    stagingStatus = null;
  }

  function getStagingMessage(): string | null {
    if (!stagingStatus || (!stagingStatus.hasEntry && !stagingStatus.hasTags)) {
      return null;
    }
    if (stagingStatus.hasEntry && stagingStatus.hasTags) {
      return 'This item has staged value and tag changes';
    }
    if (stagingStatus.hasEntry) {
      return 'This item has staged value changes';
    }
    return 'This item has staged tag changes';
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
      if (immediateMode) {
        await SecretCreate(createForm.name, createForm.value);
        await loadSecrets({ prefix, filter, withValue });
      } else {
        await StagingAdd('secret', createForm.name, createForm.value);
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
      if (immediateMode) {
        await SecretUpdate(editForm.name, editForm.value);
        await Promise.all([
          loadSecrets({ prefix, filter, withValue }),
          selectSecret(editForm.name)
        ]);
      } else {
        await StagingEdit('secret', editForm.name, editForm.value);
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
      if (immediateMode) {
        await SecretDelete(deleteTarget, forceDelete);
        if (selectedSecret === deleteTarget) {
          closeDetail();
        }
        await loadSecrets({ prefix, filter, withValue });
      } else {
        // Stage delete with recovery window (default 30 days unless force)
        await StagingDelete('secret', deleteTarget, forceDelete, forceDelete ? 0 : 30);
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
      if (immediateMode) {
        await SecretAddTag(selectedSecret, tagForm.key, tagForm.value);
      } else {
        await StagingAddTag('secret', selectedSecret, tagForm.key, tagForm.value);
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
      if (immediateMode) {
        await SecretRemoveTag(selectedSecret, removeTagTarget);
      } else {
        await StagingRemoveTag('secret', selectedSecret, removeTagTarget);
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
    <button class="btn-secondary btn-restore" onclick={() => openRestoreModal('')}>
      Restore
    </button>
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

        {#if getStagingMessage()}
          <!-- Using div instead of button to avoid conflicts with Playwright button selectors -->
          <div class="staging-banner" role="link" tabindex="0" onclick={onnavigatetostaging} onkeydown={(e) => e.key === 'Enter' && onnavigatetostaging?.()}>
            <span class="staging-icon">⚠</span>
            <span class="staging-text">{getStagingMessage()}</span>
            <span class="staging-link">View in Staging →</span>
          </div>
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
              <div class="meta-item">
                <span class="meta-label">Labels</span>
                <span class="meta-value">
                  {#each secretDetail.versionStage || [] as stage}
                    <span class="badge badge-stage">{stage}</span>
                  {/each}
                </span>
              </div>
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

            <div class="detail-section">
              <h4>ARN</h4>
              <code class="arn-display">{secretDetail.arn}</code>
            </div>

            <div class="detail-section">
              <div class="section-header">
                <h4>Tags</h4>
                <button class="btn-action-sm" onclick={openTagModal}>+ Add</button>
              </div>
              {#if secretDetail.tags && secretDetail.tags.length > 0}
                <div class="tags-list">
                  {#each secretDetail.tags as tag}
                    <div class="tag-item">
                      <span class="tag-key secret">{tag.key}</span>
                      <span class="tag-separator">=</span>
                      <span class="tag-value">{tag.value}</span>
                      <button class="btn-tag-remove" onclick={() => openRemoveTagModal(tag.key)} title="Remove tag">×</button>
                    </div>
                  {/each}
                </div>
              {:else}
                <p class="no-tags">No tags</p>
              {/if}
            </div>

            {#if secretLog.length > 0}
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
                        <div class="history-labels">
                          {#each logEntry.stages || [] as stage}
                            <span class="badge badge-stage small">{stage}</span>
                          {/each}
                        </div>
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
    <label class="checkbox-label immediate-checkbox">
      <input type="checkbox" bind:checked={immediateMode} />
      <span>Apply immediately (skip staging)</span>
    </label>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showCreateModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? (immediateMode ? 'Creating...' : 'Staging...') : (immediateMode ? 'Create' : 'Stage')}
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
    <label class="checkbox-label immediate-checkbox">
      <input type="checkbox" bind:checked={immediateMode} />
      <span>Apply immediately (skip staging)</span>
    </label>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showEditModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? (immediateMode ? 'Saving...' : 'Staging...') : (immediateMode ? 'Save' : 'Stage')}
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
    <label class="checkbox-label force-delete">
      <input type="checkbox" bind:checked={forceDelete} />
      <span>Force delete (skip recovery window)</span>
    </label>
    <p class="warning">
      {#if immediateMode}
        {#if forceDelete}
          This will permanently delete the secret immediately!
        {:else}
          The secret will be scheduled for deletion with a recovery window.
        {/if}
      {:else}
        This will stage a delete operation.
      {/if}
    </p>
    <label class="checkbox-label immediate-checkbox">
      <input type="checkbox" bind:checked={immediateMode} />
      <span>Apply immediately (skip staging)</span>
    </label>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showDeleteModal = false}>Cancel</button>
      <button type="button" class="btn-danger" onclick={handleDelete} disabled={modalLoading}>
        {modalLoading ? (immediateMode ? 'Deleting...' : 'Staging...') : (immediateMode ? 'Delete' : 'Stage Delete')}
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
    <label class="checkbox-label immediate-checkbox">
      <input type="checkbox" bind:checked={immediateMode} />
      <span>Apply immediately (skip staging)</span>
    </label>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showTagModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={tagLoading}>
        {tagLoading ? (immediateMode ? 'Adding...' : 'Staging...') : (immediateMode ? 'Add Tag' : 'Stage Tag')}
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
    <p class="warning">{immediateMode ? 'This action will take effect immediately.' : 'This will stage a tag removal operation.'}</p>
    <label class="checkbox-label immediate-checkbox">
      <input type="checkbox" bind:checked={immediateMode} />
      <span>Apply immediately (skip staging)</span>
    </label>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showRemoveTagModal = false}>Cancel</button>
      <button type="button" class="btn-danger" onclick={handleRemoveTag} disabled={removeTagLoading}>
        {removeTagLoading ? (immediateMode ? 'Removing...' : 'Staging...') : (immediateMode ? 'Remove' : 'Stage Remove')}
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

  .staging-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 10px 12px;
    margin: 8px 0 0 0;
    background: rgba(255, 193, 7, 0.15);
    border: 1px solid rgba(255, 193, 7, 0.4);
    border-radius: 6px;
    cursor: pointer;
    transition: background-color 0.2s, border-color 0.2s;
    text-align: left;
  }

  .staging-banner:hover {
    background: rgba(255, 193, 7, 0.25);
    border-color: rgba(255, 193, 7, 0.6);
  }

  .staging-icon {
    color: #ffc107;
    font-size: 14px;
    flex-shrink: 0;
  }

  .staging-text {
    color: #e0e0e0;
    font-size: 13px;
    flex: 1;
  }

  .staging-link {
    color: #ffc107;
    font-size: 12px;
    font-weight: 500;
    flex-shrink: 0;
  }
</style>
