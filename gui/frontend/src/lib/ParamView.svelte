<script lang="ts">
  import { onMount } from 'svelte';
  import { ParamList, ParamShow, ParamLog, ParamSet, ParamDelete } from '../../wailsjs/go/main/App';
  import type { main } from '../../wailsjs/go/models';
  import CloseIcon from './icons/CloseIcon.svelte';
  import Modal from './Modal.svelte';
  import './common.css';

  let prefix = '';
  let recursive = true;
  let withValue = false;
  let loading = false;
  let error = '';
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Reactive: auto-fetch when checkbox changes
  $: recursive, withValue, handleFilterChange();

  function handleFilterChange() {
    // Skip initial mount (handled by onMount)
    if (typeof window !== 'undefined' && entries !== undefined) {
      loadParams();
    }
  }

  function handlePrefixInput() {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      loadParams();
    }, 300);
  }

  let entries: main.ParamListEntry[] = [];
  let selectedParam: string | null = null;
  let paramDetail: main.ParamShowResult | null = null;
  let paramLog: main.ParamLogEntry[] = [];
  let detailLoading = false;

  // Modal states
  let showSetModal = false;
  let showDeleteModal = false;
  let setForm = { name: '', value: '', type: 'String' };
  let deleteTarget = '';
  let modalLoading = false;
  let modalError = '';

  async function loadParams() {
    loading = true;
    error = '';
    try {
      const result = await ParamList(prefix, recursive, withValue);
      entries = result?.entries || [];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
      entries = [];
    } finally {
      loading = false;
    }
  }

  async function selectParam(name: string) {
    selectedParam = name;
    detailLoading = true;
    try {
      const [detail, log] = await Promise.all([
        ParamShow(name),
        ParamLog(name, 10)
      ]);
      paramDetail = detail;
      paramLog = log?.entries || [];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      detailLoading = false;
    }
  }

  function closeDetail() {
    selectedParam = null;
    paramDetail = null;
    paramLog = [];
  }

  function formatDate(dateStr: string | undefined): string {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleString();
  }

  // Set modal
  function openSetModal(name?: string) {
    if (name && paramDetail) {
      setForm = { name, value: paramDetail.value, type: paramDetail.type };
    } else {
      setForm = { name: prefix || '', value: '', type: 'String' };
    }
    modalError = '';
    showSetModal = true;
  }

  async function handleSet() {
    if (!setForm.name || !setForm.value) {
      modalError = 'Name and value are required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      await ParamSet(setForm.name, setForm.value, setForm.type);
      showSetModal = false;
      await loadParams();
      if (selectedParam === setForm.name) {
        await selectParam(setForm.name);
      }
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
    } finally {
      modalLoading = false;
    }
  }

  // Delete modal
  function openDeleteModal(name: string) {
    deleteTarget = name;
    modalError = '';
    showDeleteModal = true;
  }

  async function handleDelete() {
    modalLoading = true;
    modalError = '';
    try {
      await ParamDelete(deleteTarget);
      showDeleteModal = false;
      if (selectedParam === deleteTarget) {
        closeDetail();
      }
      await loadParams();
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
    } finally {
      modalLoading = false;
    }
  }

  onMount(() => {
    loadParams();
  });
</script>

<div class="view-container">
  <div class="filter-bar">
    <input
      type="text"
      class="filter-input"
      placeholder="Prefix filter (e.g., /prod/)"
      bind:value={prefix}
      on:input={handlePrefixInput}
    />
    <label class="checkbox-label">
      <input type="checkbox" bind:checked={recursive} />
      Recursive
    </label>
    <label class="checkbox-label">
      <input type="checkbox" bind:checked={withValue} />
      Show Values
    </label>
    <button class="btn-primary" on:click={loadParams} disabled={loading}>
      {loading ? 'Loading...' : 'Refresh'}
    </button>
    <button class="btn-secondary" on:click={() => openSetModal()}>
      + New
    </button>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <div class="content">
    <div class="list-panel" class:collapsed={selectedParam !== null}>
      {#if entries.length === 0 && !loading}
        <div class="empty-state">
          No parameters found. Try adjusting the prefix filter.
        </div>
      {:else}
        <ul class="item-list">
          {#each entries as entry}
            <li class="item-entry" class:selected={selectedParam === entry.name}>
              <button class="item-button" on:click={() => selectParam(entry.name)}>
                <span class="item-name param">{entry.name}</span>
                {#if entry.value !== undefined}
                  <span class="item-value">{entry.value}</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    {#if selectedParam}
      <div class="detail-panel">
        <div class="detail-header">
          <h3 class="detail-title param">{selectedParam}</h3>
          <div class="detail-actions">
            <button class="btn-action-sm" on:click={() => selectedParam && openSetModal(selectedParam)}>Edit</button>
            <button class="btn-action-sm btn-danger" on:click={() => selectedParam && openDeleteModal(selectedParam)}>Delete</button>
            <button class="btn-close" on:click={closeDetail}>
              <CloseIcon />
            </button>
          </div>
        </div>

        {#if detailLoading}
          <div class="loading">Loading...</div>
        {:else if paramDetail}
          <div class="detail-content">
            <div class="detail-section">
              <h4>Current Value</h4>
              <pre class="value-display">{paramDetail.value}</pre>
            </div>

            <div class="detail-meta">
              <div class="meta-item">
                <span class="meta-label">Version</span>
                <span class="meta-value">{paramDetail.version}</span>
              </div>
              <div class="meta-item">
                <span class="meta-label">Type</span>
                <span class="meta-value">{paramDetail.type}</span>
              </div>
              <div class="meta-item">
                <span class="meta-label">Last Modified</span>
                <span class="meta-value">{formatDate(paramDetail.lastModified)}</span>
              </div>
            </div>

            {#if paramLog.length > 0}
              <div class="detail-section">
                <h4>Version History</h4>
                <ul class="history-list">
                  {#each paramLog as logEntry}
                    <li class="history-item" class:current={logEntry.isCurrent}>
                      <div class="history-header">
                        <span class="history-version">v{logEntry.version}</span>
                        {#if logEntry.isCurrent}
                          <span class="badge badge-current">current</span>
                        {/if}
                        <span class="history-date">{formatDate(logEntry.lastModified)}</span>
                      </div>
                      <pre class="history-value">{logEntry.value}</pre>
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

<!-- Set Modal -->
<Modal title={setForm.name ? 'Edit Parameter' : 'New Parameter'} show={showSetModal} on:close={() => showSetModal = false}>
  <form class="modal-form" on:submit|preventDefault={handleSet}>
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <div class="form-group">
      <label for="param-name">Name</label>
      <input
        id="param-name"
        type="text"
        class="form-input"
        bind:value={setForm.name}
        placeholder="/path/to/parameter"
        disabled={!!paramDetail && selectedParam === setForm.name}
      />
    </div>
    <div class="form-group">
      <label for="param-type">Type</label>
      <select id="param-type" class="form-input" bind:value={setForm.type}>
        <option value="String">String</option>
        <option value="SecureString">SecureString</option>
        <option value="StringList">StringList</option>
      </select>
    </div>
    <div class="form-group">
      <label for="param-value">Value</label>
      <textarea
        id="param-value"
        class="form-input form-textarea"
        bind:value={setForm.value}
        placeholder="Parameter value"
        rows="5"
      ></textarea>
    </div>
    <div class="form-actions">
      <button type="button" class="btn-secondary" on:click={() => showSetModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? 'Saving...' : 'Save'}
      </button>
    </div>
  </form>
</Modal>

<!-- Delete Modal -->
<Modal title="Delete Parameter" show={showDeleteModal} on:close={() => showDeleteModal = false}>
  <div class="modal-confirm">
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <p>Are you sure you want to delete this parameter?</p>
    <code class="delete-target">{deleteTarget}</code>
    <p class="warning">This action cannot be undone.</p>
    <div class="form-actions">
      <button type="button" class="btn-secondary" on:click={() => showDeleteModal = false}>Cancel</button>
      <button type="button" class="btn-danger" on:click={handleDelete} disabled={modalLoading}>
        {modalLoading ? 'Deleting...' : 'Delete'}
      </button>
    </div>
  </div>
</Modal>

<style>
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
    color: #4fc3f7;
    font-size: 14px;
    margin-bottom: 16px;
  }

  .warning {
    color: #ff9800 !important;
    font-size: 13px;
  }
</style>
