<script lang="ts">
  import { onMount } from 'svelte';
  import { SecretList, SecretShow, SecretLog, SecretCreate, SecretUpdate, SecretDelete } from '../../wailsjs/go/main/App';
  import type { main } from '../../wailsjs/go/models';
  import CloseIcon from './icons/CloseIcon.svelte';
  import EyeIcon from './icons/EyeIcon.svelte';
  import EyeOffIcon from './icons/EyeOffIcon.svelte';
  import Modal from './Modal.svelte';
  import './common.css';

  let prefix = '';
  let withValue = false;
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
  let createForm = { name: '', value: '' };
  let editForm = { name: '', value: '' };
  let deleteTarget = '';
  let forceDelete = false;
  let modalLoading = false;
  let modalError = '';

  async function loadSecrets() {
    loading = true;
    error = '';
    try {
      const result = await SecretList(prefix, withValue);
      entries = result?.entries || [];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
      entries = [];
    } finally {
      loading = false;
    }
  }

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
      await SecretCreate(createForm.name, createForm.value);
      showCreateModal = false;
      await loadSecrets();
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
      await SecretUpdate(editForm.name, editForm.value);
      showEditModal = false;
      await selectSecret(editForm.name);
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
      await SecretDelete(deleteTarget, forceDelete);
      showDeleteModal = false;
      if (selectedSecret === deleteTarget) {
        closeDetail();
      }
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
      class="filter-input"
      placeholder="Prefix filter"
      bind:value={prefix}
      on:keydown={(e) => e.key === 'Enter' && loadSecrets()}
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
      {/if}
    </div>

    {#if selectedSecret}
      <div class="detail-panel">
        <div class="detail-header">
          <h3 class="detail-title secret">{selectedSecret}</h3>
          <div class="detail-actions">
            <button class="btn-action-sm" on:click={openEditModal}>Edit</button>
            <button class="btn-action-sm btn-danger" on:click={() => selectedSecret && openDeleteModal(selectedSecret)}>Delete</button>
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
              <pre class="value-display" class:masked={!showValue}>
                {showValue ? formatValue(secretDetail.value) : maskValue(secretDetail.value)}
              </pre>
            </div>

            <div class="detail-meta">
              <div class="meta-item">
                <span class="meta-label">Version ID</span>
                <span class="meta-value mono">{secretDetail.versionId}</span>
              </div>
              <div class="meta-item">
                <span class="meta-label">Stages</span>
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

            <div class="detail-section">
              <h4>ARN</h4>
              <code class="arn-display">{secretDetail.arn}</code>
            </div>

            {#if secretLog.length > 0}
              <div class="detail-section">
                <h4>Version History</h4>
                <ul class="history-list">
                  {#each secretLog as logEntry}
                    <li class="history-item" class:current-secret={logEntry.stages?.includes('AWSCURRENT')}>
                      <div class="history-header">
                        <span class="history-version">{logEntry.versionId?.substring(0, 8)}...</span>
                        {#each logEntry.stages || [] as stage}
                          <span class="badge badge-stage small">{stage}</span>
                        {/each}
                        <span class="history-date">{formatDate(logEntry.created)}</span>
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
    <div class="form-actions">
      <button type="button" class="btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? 'Creating...' : 'Create'}
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
    <div class="form-actions">
      <button type="button" class="btn-secondary" on:click={() => showEditModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? 'Saving...' : 'Save'}
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
      {#if forceDelete}
        This will permanently delete the secret immediately!
      {:else}
        The secret will be scheduled for deletion with a recovery window.
      {/if}
    </p>
    <div class="form-actions">
      <button type="button" class="btn-secondary" on:click={() => showDeleteModal = false}>Cancel</button>
      <button type="button" class="btn-danger" on:click={handleDelete} disabled={modalLoading}>
        {modalLoading ? 'Deleting...' : 'Delete'}
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
</style>
