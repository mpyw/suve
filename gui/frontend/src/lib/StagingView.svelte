<script lang="ts">
  import { onMount } from 'svelte';
  import { StagingStatus, StagingApply, StagingReset, StagingAdd, StagingEdit, StagingUnstage, StagingDelete } from '../../wailsjs/go/main/App';
  import type { main } from '../../wailsjs/go/models';
  import Modal from './Modal.svelte';
  import CloseIcon from './icons/CloseIcon.svelte';
  import './common.css';

  let loading = false;
  let error = '';
  let ssmEntries: main.StagingEntry[] = [];
  let smEntries: main.StagingEntry[] = [];

  // Modal states
  let showApplyModal = false;
  let showResetModal = false;
  let showAddModal = false;
  let showEditModal = false;
  let showDeleteModal = false;
  let applyService = '';
  let resetService = '';
  let ignoreConflicts = false;
  let modalLoading = false;
  let modalError = '';
  let applyResult: main.StagingApplyResult | null = null;

  // Add/Edit form
  let addEditService = 'ssm';
  let addEditName = '';
  let addEditValue = '';
  let isEditMode = false;

  // Delete form
  let deleteService = '';
  let deleteName = '';
  let deleteForce = false;
  let deleteRecoveryWindow = 7;

  async function loadStatus() {
    loading = true;
    error = '';
    try {
      const result = await StagingStatus();
      ssmEntries = result?.ssm || [];
      smEntries = result?.sm || [];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
      ssmEntries = [];
      smEntries = [];
    } finally {
      loading = false;
    }
  }

  function formatDate(dateStr: string): string {
    return new Date(dateStr).toLocaleString();
  }

  function getOperationColor(op: string): string {
    switch (op.toLowerCase()) {
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
      if (result.failed === 0 && result.conflicts?.length === 0) {
        await loadStatus();
      }
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
    } finally {
      modalLoading = false;
    }
  }

  function closeApplyModal() {
    showApplyModal = false;
    if (applyResult && applyResult.failed === 0) {
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
      modalError = e instanceof Error ? e.message : String(e);
    } finally {
      modalLoading = false;
    }
  }

  function getServiceName(service: string): string {
    return service === 'ssm' ? 'Parameters (SSM)' : 'Secrets (SM)';
  }

  // Add modal
  function openAddModal(service: string) {
    addEditService = service;
    addEditName = '';
    addEditValue = '';
    isEditMode = false;
    modalError = '';
    showAddModal = true;
  }

  async function handleAdd() {
    if (!addEditName || !addEditValue) {
      modalError = 'Name and value are required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      await StagingAdd(addEditService, addEditName, addEditValue);
      showAddModal = false;
      await loadStatus();
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
    } finally {
      modalLoading = false;
    }
  }

  // Edit modal
  function openEditModal(service: string, entry: main.StagingEntry) {
    addEditService = service;
    addEditName = entry.name;
    addEditValue = entry.value || '';
    isEditMode = true;
    modalError = '';
    showEditModal = true;
  }

  async function handleEdit() {
    if (!addEditValue) {
      modalError = 'Value is required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      await StagingEdit(addEditService, addEditName, addEditValue);
      showEditModal = false;
      await loadStatus();
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
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
      error = e instanceof Error ? e.message : String(e);
    }
  }

  // Stage delete modal
  function openDeleteModal(service: string) {
    deleteService = service;
    deleteName = '';
    deleteForce = false;
    deleteRecoveryWindow = 7;
    modalError = '';
    showDeleteModal = true;
  }

  async function handleStageDelete() {
    if (!deleteName) {
      modalError = 'Name is required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      await StagingDelete(deleteService, deleteName, deleteForce, deleteRecoveryWindow);
      showDeleteModal = false;
      await loadStatus();
    } catch (e) {
      modalError = e instanceof Error ? e.message : String(e);
    } finally {
      modalLoading = false;
    }
  }

  onMount(() => {
    loadStatus();
  });
</script>

<div class="view-container">
  <div class="header">
    <h2 class="title">Staging Area</h2>
    <button class="btn-primary" on:click={loadStatus} disabled={loading}>
      {loading ? 'Loading...' : 'Refresh'}
    </button>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <div class="staging-content">
    <div class="section">
      <div class="section-header">
        <h3 class="section-title">
          <span class="section-icon">P</span>
          Parameters (SSM)
        </h3>
        <span class="count-badge">{ssmEntries.length}</span>
        <div class="section-actions">
          <button class="btn-section btn-add" on:click={() => openAddModal('ssm')}>+ Add</button>
          <button class="btn-section btn-delete-stage" on:click={() => openDeleteModal('ssm')}>Stage Delete</button>
          {#if ssmEntries.length > 0}
            <button class="btn-section btn-apply-sm" on:click={() => openApplyModal('ssm')}>Apply</button>
            <button class="btn-section btn-reset-sm" on:click={() => openResetModal('ssm')}>Reset</button>
          {/if}
        </div>
      </div>

      {#if ssmEntries.length === 0}
        <div class="empty-state">No staged parameter changes</div>
      {:else}
        <ul class="entry-list">
          {#each ssmEntries as entry}
            <li class="entry-item">
              <div class="entry-header">
                <span class="operation-badge" style="background: {getOperationColor(entry.operation)}">
                  {entry.operation}
                </span>
                <span class="entry-name">{entry.name}</span>
                <span class="entry-date">{formatDate(entry.stagedAt)}</span>
                <div class="entry-actions">
                  {#if entry.operation !== 'delete'}
                    <button class="btn-entry" on:click={() => openEditModal('ssm', entry)}>Edit</button>
                  {/if}
                  <button class="btn-entry btn-unstage" on:click={() => handleUnstage('ssm', entry.name)}>Unstage</button>
                </div>
              </div>
              {#if entry.value !== undefined}
                <pre class="entry-value">{entry.value}</pre>
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
          Secrets (SM)
        </h3>
        <span class="count-badge">{smEntries.length}</span>
        <div class="section-actions">
          <button class="btn-section btn-add" on:click={() => openAddModal('sm')}>+ Add</button>
          <button class="btn-section btn-delete-stage" on:click={() => openDeleteModal('sm')}>Stage Delete</button>
          {#if smEntries.length > 0}
            <button class="btn-section btn-apply-sm" on:click={() => openApplyModal('sm')}>Apply</button>
            <button class="btn-section btn-reset-sm" on:click={() => openResetModal('sm')}>Reset</button>
          {/if}
        </div>
      </div>

      {#if smEntries.length === 0}
        <div class="empty-state">No staged secret changes</div>
      {:else}
        <ul class="entry-list">
          {#each smEntries as entry}
            <li class="entry-item">
              <div class="entry-header">
                <span class="operation-badge" style="background: {getOperationColor(entry.operation)}">
                  {entry.operation}
                </span>
                <span class="entry-name">{entry.name}</span>
                <span class="entry-date">{formatDate(entry.stagedAt)}</span>
                <div class="entry-actions">
                  {#if entry.operation !== 'delete'}
                    <button class="btn-entry" on:click={() => openEditModal('sm', entry)}>Edit</button>
                  {/if}
                  <button class="btn-entry btn-unstage" on:click={() => handleUnstage('sm', entry.name)}>Unstage</button>
                </div>
              </div>
              {#if entry.value !== undefined}
                <pre class="entry-value">{entry.value}</pre>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>

  {#if ssmEntries.length > 0 || smEntries.length > 0}
    <div class="actions">
      <button
        class="btn-action btn-apply"
        on:click={() => openApplyModal(ssmEntries.length > 0 ? 'ssm' : 'sm')}
      >
        Apply All Changes
      </button>
      <button
        class="btn-action btn-reset"
        on:click={() => {
          if (ssmEntries.length > 0) openResetModal('ssm');
          if (smEntries.length > 0) openResetModal('sm');
        }}
      >
        Reset All
      </button>
    </div>
  {/if}
</div>

<!-- Apply Modal -->
<Modal title="Apply Staged Changes" show={showApplyModal} on:close={closeApplyModal}>
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
          <span class="result-success">Succeeded: {applyResult.succeeded}</span>
          <span class="result-failed">Failed: {applyResult.failed}</span>
        </div>
        {#if applyResult.results && applyResult.results.length > 0}
          <ul class="result-list">
            {#each applyResult.results as result}
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
        {/if}
        <div class="form-actions">
          <button type="button" class="btn-primary" on:click={closeApplyModal}>Close</button>
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
        <button type="button" class="btn-secondary" on:click={closeApplyModal}>Cancel</button>
        <button type="button" class="btn-apply" on:click={handleApply} disabled={modalLoading}>
          {modalLoading ? 'Applying...' : 'Apply'}
        </button>
      </div>
    {/if}
  </div>
</Modal>

<!-- Reset Modal -->
<Modal title="Reset Staged Changes" show={showResetModal} on:close={() => showResetModal = false}>
  <div class="modal-confirm">
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <p>Reset all staged changes for {getServiceName(resetService)}?</p>
    <p class="warning">This will discard all staged changes without applying them.</p>
    <div class="form-actions">
      <button type="button" class="btn-secondary" on:click={() => showResetModal = false}>Cancel</button>
      <button type="button" class="btn-danger" on:click={handleReset} disabled={modalLoading}>
        {modalLoading ? 'Resetting...' : 'Reset'}
      </button>
    </div>
  </div>
</Modal>

<!-- Add Modal -->
<Modal title="Stage New {addEditService === 'ssm' ? 'Parameter' : 'Secret'}" show={showAddModal} on:close={() => showAddModal = false}>
  <form class="modal-form" on:submit|preventDefault={handleAdd}>
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <div class="form-group">
      <label for="add-name">Name</label>
      <input
        id="add-name"
        type="text"
        class="form-input"
        bind:value={addEditName}
        placeholder={addEditService === 'ssm' ? '/path/to/parameter' : 'my-secret-name'}
      />
    </div>
    <div class="form-group">
      <label for="add-value">Value</label>
      <textarea
        id="add-value"
        class="form-input form-textarea"
        bind:value={addEditValue}
        placeholder="Value"
        rows="5"
      ></textarea>
    </div>
    <div class="form-actions">
      <button type="button" class="btn-secondary" on:click={() => showAddModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? 'Staging...' : 'Stage'}
      </button>
    </div>
  </form>
</Modal>

<!-- Edit Modal -->
<Modal title="Edit Staged {addEditService === 'ssm' ? 'Parameter' : 'Secret'}" show={showEditModal} on:close={() => showEditModal = false}>
  <form class="modal-form" on:submit|preventDefault={handleEdit}>
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <div class="form-group">
      <label for="edit-name">Name</label>
      <input
        id="edit-name"
        type="text"
        class="form-input"
        value={addEditName}
        disabled
      />
    </div>
    <div class="form-group">
      <label for="edit-value">Value</label>
      <textarea
        id="edit-value"
        class="form-input form-textarea"
        bind:value={addEditValue}
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

<!-- Stage Delete Modal -->
<Modal title="Stage Delete {deleteService === 'ssm' ? 'Parameter' : 'Secret'}" show={showDeleteModal} on:close={() => showDeleteModal = false}>
  <form class="modal-form" on:submit|preventDefault={handleStageDelete}>
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <div class="form-group">
      <label for="delete-name">Name</label>
      <input
        id="delete-name"
        type="text"
        class="form-input"
        bind:value={deleteName}
        placeholder={deleteService === 'ssm' ? '/path/to/parameter' : 'my-secret-name'}
      />
    </div>
    {#if deleteService === 'sm'}
      <div class="form-group">
        <label class="checkbox-label">
          <input type="checkbox" bind:checked={deleteForce} />
          <span>Force delete (skip recovery window)</span>
        </label>
      </div>
      {#if !deleteForce}
        <div class="form-group">
          <label for="recovery-window">Recovery Window (days)</label>
          <input
            id="recovery-window"
            type="number"
            class="form-input"
            bind:value={deleteRecoveryWindow}
            min="7"
            max="30"
          />
        </div>
      {/if}
    {/if}
    <div class="form-actions">
      <button type="button" class="btn-secondary" on:click={() => showDeleteModal = false}>Cancel</button>
      <button type="submit" class="btn-danger" disabled={modalLoading}>
        {modalLoading ? 'Staging...' : 'Stage Delete'}
      </button>
    </div>
  </form>
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

  .btn-add {
    background: #2196f3;
    color: #fff;
  }

  .btn-add:hover {
    background: #1976d2;
  }

  .btn-delete-stage {
    background: #ff9800;
    color: #fff;
  }

  .btn-delete-stage:hover {
    background: #f57c00;
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

  .entry-date {
    font-size: 12px;
    color: #666;
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

  .actions {
    display: flex;
    gap: 12px;
    padding: 16px;
    background: #1a1a2e;
    border-top: 1px solid #2d2d44;
  }

  .btn-action {
    flex: 1;
    padding: 12px;
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
</style>
