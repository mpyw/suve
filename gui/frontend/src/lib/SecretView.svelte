<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { SecretList, SecretShow, SecretLog, SecretCreate, SecretUpdate, SecretDelete, SecretDiff, SecretRestore, StagingAdd, StagingEdit, StagingDelete } from '../../wailsjs/go/main/App';
  import type { main } from '../../wailsjs/go/models';
  import CloseIcon from './icons/CloseIcon.svelte';
  import EyeIcon from './icons/EyeIcon.svelte';
  import EyeOffIcon from './icons/EyeOffIcon.svelte';
  import Modal from './Modal.svelte';
  import DiffDisplay from './DiffDisplay.svelte';
  import './common.css';

  const PAGE_SIZE = 50;

  let prefix = '';
  let filter = '';
  let withValue = false;
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;
  let nextToken = '';
  let loadingMore = false;

  // Reactive: auto-fetch when checkbox changes
  $: withValue, handleFilterChange();

  function handleFilterChange() {
    // Skip initial mount (handled by onMount)
    if (typeof window !== 'undefined' && entries !== undefined) {
      loadSecrets();
    }
  }

  function handlePrefixInput() {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      loadSecrets();
    }, 300);
  }

  function handleFilterInput() {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      loadSecrets();
    }, 300);
  }
  let loading = false;
  let error = '';

  let entries: main.SecretListEntry[] = [];
  let selectedSecret: string | null = null;
  let secretDetail: main.SecretShowResult | null = null;
  let secretLog: main.SecretLogEntry[] = [];
  let detailLoading = false;
  let showValue = false;

  // Modal states
  let showCreateModal = false;
  let showEditModal = false;
  let showDeleteModal = false;
  let showDiffModal = false;
  let showRestoreModal = false;
  let createForm = { name: '', value: '' };
  let editForm = { name: '', value: '' };
  let deleteTarget = '';
  let forceDelete = false;
  let modalLoading = false;
  let modalError = '';
  let immediateMode = false; // When false (default), changes are staged

  // Diff state
  let diffMode = false;
  let diffSelectedVersions: string[] = [];
  let diffResult: main.SecretDiffResult | null = null;

  // Restore state
  let restoreTarget = '';

  // Infinite scroll
  let sentinelElement: HTMLDivElement;
  let observer: IntersectionObserver | null = null;

  async function loadSecrets() {
    loading = true;
    error = '';
    nextToken = '';
    try {
      const result = await SecretList(prefix, withValue, filter, PAGE_SIZE, '');
      entries = result?.entries || [];
      nextToken = result?.nextToken || '';
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
      entries = [];
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    if (!nextToken || loadingMore || loading) return;

    loadingMore = true;
    try {
      const result = await SecretList(prefix, withValue, filter, PAGE_SIZE, nextToken);
      entries = [...entries, ...(result?.entries || [])];
      nextToken = result?.nextToken || '';
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loadingMore = false;
    }
  }

  function setupIntersectionObserver() {
    if (observer) observer.disconnect();

    observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && nextToken && !loadingMore && !loading) {
          loadMore();
        }
      },
      { rootMargin: '100px' }
    );

    if (sentinelElement) {
      observer.observe(sentinelElement);
    }
  }

  $: if (sentinelElement) {
    setupIntersectionObserver();
  }

  onDestroy(() => {
    if (observer) observer.disconnect();
  });

  async function selectSecret(name: string) {
    selectedSecret = name;
    detailLoading = true;
    showValue = false;
    try {
      const [detail, log] = await Promise.all([
        SecretShow(name),
        SecretLog(name, 10)
      ]);
      secretDetail = detail;
      secretLog = log?.entries || [];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      detailLoading = false;
    }
  }

  function closeDetail() {
    selectedSecret = null;
    secretDetail = null;
    secretLog = [];
    showValue = false;
  }

  function toggleShowValue() {
    showValue = !showValue;
  }

  function formatDate(dateStr: string | undefined): string {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleString();
  }

  function formatValue(value: string): string {
    try {
      const parsed = JSON.parse(value);
      return JSON.stringify(parsed, null, 2);
    } catch {
      return value;
    }
  }

  function maskValue(value: string): string {
    return '*'.repeat(Math.min(value.length, 32));
  }

  // Create modal
  function openCreateModal() {
    createForm = { name: prefix || '', value: '' };
    modalError = '';
    showCreateModal = true;
  }

  async function handleCreate() {
    if (!createForm.name || !createForm.value) {
      modalError = 'Name and value are required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      if (immediateMode) {
        await SecretCreate(createForm.name, createForm.value);
        await loadSecrets();
      } else {
        await StagingAdd('sm', createForm.name, createForm.value);
      }
      showCreateModal = false;
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
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

  async function handleEdit() {
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
          loadSecrets(),
          selectSecret(editForm.name)
        ]);
      } else {
        await StagingEdit('sm', editForm.name, editForm.value);
      }
      showEditModal = false;
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
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
        await loadSecrets();
      } else {
        // Stage delete with recovery window (default 30 days unless force)
        await StagingDelete('sm', deleteTarget, forceDelete, forceDelete ? 0 : 30);
      }
      showDeleteModal = false;
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
    } finally {
      modalLoading = false;
    }
  }

  // Diff functions
  function toggleDiffMode() {
    diffMode = !diffMode;
    diffSelectedVersions = [];
  }

  function toggleVersionSelection(versionId: string) {
    const idx = diffSelectedVersions.indexOf(versionId);
    if (idx >= 0) {
      diffSelectedVersions = diffSelectedVersions.filter(v => v !== versionId);
    } else if (diffSelectedVersions.length < 2) {
      diffSelectedVersions = [...diffSelectedVersions, versionId];
    }
  }

  async function executeDiff() {
    if (!selectedSecret || diffSelectedVersions.length !== 2) return;

    modalLoading = true;
    modalError = '';
    try {
      const spec1 = `${selectedSecret}#${diffSelectedVersions[0]}`;
      const spec2 = `${selectedSecret}#${diffSelectedVersions[1]}`;
      diffResult = await SecretDiff(spec1, spec2);
      showDiffModal = true;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      modalLoading = false;
    }
  }

  function closeDiffModal() {
    showDiffModal = false;
    diffResult = null;
    diffMode = false;
    diffSelectedVersions = [];
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
      await loadSecrets();
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
    } finally {
      modalLoading = false;
    }
  }

  onMount(() => {
    loadSecrets();
  });
</script>

<div class="view-container">
  <div class="filter-bar">
    <input
      type="text"
      class="filter-input prefix-input"
      placeholder="Prefix"
      bind:value={prefix}
      on:input={handlePrefixInput}
    />
    <input
      type="text"
      class="filter-input regex-input"
      placeholder="Filter (regex)"
      bind:value={filter}
      on:input={handleFilterInput}
    />
    <label class="checkbox-label">
      <input type="checkbox" bind:checked={withValue} />
      Show Values
    </label>
    <button class="btn-primary" on:click={loadSecrets} disabled={loading}>
      {loading ? 'Loading...' : 'Refresh'}
    </button>
    <button class="btn-secondary" on:click={openCreateModal}>
      + New
    </button>
    <button class="btn-secondary btn-restore" on:click={() => openRestoreModal('')}>
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
              <button class="item-button" on:click={() => selectSecret(entry.name)}>
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
            <button class="btn-action-sm" on:click={openEditModal}>Edit</button>
            <button class="btn-action-sm btn-danger" on:click={() => selectedSecret && openDeleteModal(selectedSecret)}>Delete</button>
            {#if secretLog.length >= 2}
              <button class="btn-action-sm" class:active={diffMode} on:click={toggleDiffMode}>
                {diffMode ? 'Cancel' : 'Compare'}
              </button>
            {/if}
            <button class="btn-close" on:click={closeDetail}>
              <CloseIcon />
            </button>
          </div>
        </div>

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
                  on:click={toggleShowValue}
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
              <pre class="value-display" class:masked={!showValue}>{showValue ? formatValue(secretDetail.value) : maskValue(secretDetail.value)}</pre>
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

            {#if secretDetail.tags && secretDetail.tags.length > 0}
              <div class="detail-section">
                <h4>Tags</h4>
                <div class="tags-list">
                  {#each secretDetail.tags as tag}
                    <div class="tag-item">
                      <span class="tag-key">{tag.key}</span>
                      <span class="tag-separator">=</span>
                      <span class="tag-value">{tag.value}</span>
                    </div>
                  {/each}
                </div>
              </div>
            {/if}

            {#if secretLog.length > 0}
              <div class="detail-section">
                <div class="section-header-history">
                  <h4>Version History</h4>
                  {#if diffMode && diffSelectedVersions.length === 2}
                    <button class="btn-action-sm btn-compare" on:click={executeDiff} disabled={modalLoading}>
                      {modalLoading ? 'Comparing...' : 'Show Diff'}
                    </button>
                  {/if}
                </div>
                {#if diffMode}
                  <p class="diff-hint">Select 2 versions to compare</p>
                {/if}
                <ul class="history-list">
                  {#each secretLog as logEntry}
                    <li
                      class="history-item"
                      class:current-secret={logEntry.stages?.includes('AWSCURRENT')}
                      class:selectable={diffMode}
                      class:selected={diffSelectedVersions.includes(logEntry.versionId)}
                    >
                      {#if diffMode}
                        <label class="diff-checkbox">
                          <input
                            type="checkbox"
                            checked={diffSelectedVersions.includes(logEntry.versionId)}
                            disabled={!diffSelectedVersions.includes(logEntry.versionId) && diffSelectedVersions.length >= 2}
                            on:change={() => toggleVersionSelection(logEntry.versionId)}
                          />
                        </label>
                      {/if}
                      <div class="history-content">
                        <div class="history-header">
                          <span class="history-version mono" title={logEntry.versionId}>{logEntry.versionId}</span>
                          <span class="history-date">{formatDate(logEntry.created)}</span>
                        </div>
                        <div class="history-labels">
                          {#each logEntry.stages || [] as stage}
                            <span class="badge badge-stage small">{stage}</span>
                          {/each}
                        </div>
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
<Modal title="New Secret" show={showCreateModal} on:close={() => showCreateModal = false}>
  <form class="modal-form" on:submit|preventDefault={handleCreate}>
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
      <button type="button" class="btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? (immediateMode ? 'Creating...' : 'Staging...') : (immediateMode ? 'Create' : 'Stage')}
      </button>
    </div>
  </form>
</Modal>

<!-- Edit Modal -->
<Modal title="Edit Secret" show={showEditModal} on:close={() => showEditModal = false}>
  <form class="modal-form" on:submit|preventDefault={handleEdit}>
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
      <button type="button" class="btn-secondary" on:click={() => showEditModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? (immediateMode ? 'Saving...' : 'Staging...') : (immediateMode ? 'Save' : 'Stage')}
      </button>
    </div>
  </form>
</Modal>

<!-- Delete Modal -->
<Modal title="Delete Secret" show={showDeleteModal} on:close={() => showDeleteModal = false}>
  <div class="modal-confirm">
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <p>Are you sure you want to delete this secret?</p>
    <code class="delete-target">{deleteTarget}</code>
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
      <button type="button" class="btn-secondary" on:click={() => showDeleteModal = false}>Cancel</button>
      <button type="button" class="btn-danger" on:click={handleDelete} disabled={modalLoading}>
        {modalLoading ? (immediateMode ? 'Deleting...' : 'Staging...') : (immediateMode ? 'Delete' : 'Stage Delete')}
      </button>
    </div>
  </div>
</Modal>

<!-- Diff Modal -->
<Modal title="Version Comparison" show={showDiffModal} on:close={closeDiffModal}>
  {#if diffResult}
    <DiffDisplay
      oldValue={formatValue(diffResult.oldValue)}
      newValue={formatValue(diffResult.newValue)}
      oldLabel="Old"
      newLabel="New"
      oldSubLabel={diffResult.oldVersionId}
      newSubLabel={diffResult.newVersionId}
    />
    <div class="form-actions">
      <button type="button" class="btn-secondary" on:click={closeDiffModal}>Close</button>
    </div>
  {/if}
</Modal>

<!-- Restore Modal -->
<Modal title="Restore Secret" show={showRestoreModal} on:close={() => showRestoreModal = false}>
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
      <button type="button" class="btn-secondary" on:click={() => showRestoreModal = false}>Cancel</button>
      <button type="button" class="btn-primary btn-restore-confirm" on:click={handleRestore} disabled={modalLoading || !restoreTarget}>
        {modalLoading ? 'Restoring...' : 'Restore'}
      </button>
    </div>
  </div>
</Modal>

<style>
  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
  }

  .section-header h4 {
    margin: 0;
    font-size: 12px;
    text-transform: uppercase;
    color: #888;
    letter-spacing: 0.5px;
  }

  .detail-actions {
    display: flex;
    gap: 8px;
    align-items: center;
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

  .btn-action-sm {
    padding: 6px 12px;
    background: #2d2d44;
    color: #fff;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 12px;
  }

  .btn-action-sm:hover {
    background: #3d3d54;
  }

  .btn-danger {
    background: #f44336;
  }

  .btn-danger:hover {
    background: #e53935;
  }

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

  .form-actions {
    display: flex;
    gap: 12px;
    justify-content: flex-end;
    margin-top: 8px;
  }

  .modal-error {
    padding: 10px 12px;
    background: rgba(244, 67, 54, 0.2);
    border: 1px solid #f44336;
    border-radius: 4px;
    color: #f44336;
    font-size: 13px;
  }

  .modal-confirm {
    text-align: center;
  }

  .modal-confirm p {
    color: #ccc;
    margin: 0 0 16px 0;
  }

  .delete-target {
    display: block;
    padding: 12px;
    background: #0f0f1a;
    border-radius: 4px;
    color: #ffb74d;
    font-size: 14px;
    margin-bottom: 16px;
  }

  .force-delete {
    justify-content: center;
    margin-bottom: 12px;
  }

  .warning {
    color: #ff9800 !important;
    font-size: 13px;
  }

  /* Restore button */
  .btn-restore {
    background: #4caf50;
  }

  .btn-restore:hover {
    background: #43a047;
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

  /* Diff mode styles */
  .btn-action-sm.active {
    background: #e94560;
  }

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

  .btn-compare {
    background: #4caf50;
  }

  .btn-compare:hover {
    background: #43a047;
  }

  .diff-hint {
    color: #888;
    font-size: 12px;
    margin: 0 0 12px 0;
  }

  .history-item.selectable {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    cursor: pointer;
  }

  .history-item.selectable:hover {
    background: #1a1a2e;
  }

  .history-item.selected {
    background: rgba(233, 69, 96, 0.15);
    border-left: 3px solid #e94560;
  }

  .diff-checkbox {
    display: flex;
    align-items: center;
    padding-top: 2px;
  }

  .diff-checkbox input {
    width: 16px;
    height: 16px;
    cursor: pointer;
  }

  .history-content {
    flex: 1;
    min-width: 0;
  }

  /* Filter inputs */
  .prefix-input {
    flex: 0.4;
  }

  .regex-input {
    flex: 0.6;
  }

  .immediate-checkbox {
    margin-top: 8px;
    padding: 8px 12px;
    background: rgba(255, 152, 0, 0.1);
    border: 1px solid rgba(255, 152, 0, 0.3);
    border-radius: 4px;
  }

  /* Infinite scroll styles */
  .scroll-sentinel {
    padding: 16px;
    text-align: center;
    min-height: 50px;
  }

  .loading-more {
    color: #888;
    font-size: 14px;
  }

  .load-more-hint {
    color: #555;
    font-size: 12px;
  }

  /* Tags styles */
  .tags-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .tag-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    background: #1a1a2e;
    border-radius: 4px;
    font-family: monospace;
    font-size: 13px;
  }

  .tag-key {
    color: #ffb74d;
    font-weight: 600;
  }

  .tag-separator {
    color: #666;
  }

  .tag-value {
    color: #a5d6a7;
  }

  /* Description styles */
  .description-text {
    margin: 0;
    padding: 12px;
    background: #1a1a2e;
    border-radius: 4px;
    color: #ccc;
    font-size: 14px;
    line-height: 1.5;
    white-space: pre-wrap;
  }
</style>
