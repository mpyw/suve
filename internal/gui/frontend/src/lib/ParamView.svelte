<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { ParamList, ParamShow, ParamLog, ParamSet, ParamDelete, ParamDiff, ParamAddTag, ParamRemoveTag, StagingAdd, StagingEdit, StagingDelete, StagingAddTag, StagingRemoveTag } from '../../wailsjs/go/gui/App';
  import type { gui } from '../../wailsjs/go/models';
  import CloseIcon from './icons/CloseIcon.svelte';
  import EyeIcon from './icons/EyeIcon.svelte';
  import EyeOffIcon from './icons/EyeOffIcon.svelte';
  import Modal from './Modal.svelte';
  import DiffDisplay from './DiffDisplay.svelte';
  import './common.css';

  const PAGE_SIZE = 50;

  let prefix = $state('');
  let filter = $state('');
  let recursive = $state(true);
  let withValue = $state(false);
  let loading = $state(false);
  let loadingMore = $state(false);
  let error = $state('');
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;
  let nextToken = $state('');

  let entries: gui.ParamListEntry[] = $state([]);
  let selectedParam: string | null = $state(null);
  let paramDetail: gui.ParamShowResult | null = $state(null);
  let paramLog: gui.ParamLogEntry[] = $state([]);
  let detailLoading = $state(false);
  let showValue = $state(false);

  // Modal states
  let showSetModal = $state(false);
  let showDeleteModal = $state(false);
  let showDiffModal = $state(false);
  let setForm = $state({ name: '', value: '', type: 'String' });
  let deleteTarget = $state('');
  let modalLoading = $state(false);
  let modalError = $state('');
  let immediateMode = $state(false);

  // Diff state
  let diffMode = $state(false);
  let diffSelectedVersions: number[] = $state([]);
  let diffResult: gui.ParamDiffResult | null = $state(null);

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

  // Track previous values for effect
  let prevRecursive = recursive;
  let prevWithValue = withValue;
  let mounted = false;

  $effect(() => {
    // Watch recursive and withValue changes
    if (mounted && (recursive !== prevRecursive || withValue !== prevWithValue)) {
      prevRecursive = recursive;
      prevWithValue = withValue;
      loadParams();
    }
  });

  $effect(() => {
    if (sentinelElement) {
      setupIntersectionObserver();
    }
  });

  function handlePrefixInput() {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      loadParams();
    }, 300);
  }

  function handleFilterInput() {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      loadParams();
    }, 300);
  }

  async function loadParams() {
    loading = true;
    error = '';
    nextToken = '';
    try {
      const result = await ParamList(prefix, recursive, withValue, filter, PAGE_SIZE, '');
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
      const result = await ParamList(prefix, recursive, withValue, filter, PAGE_SIZE, nextToken);
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
      (observerEntries) => {
        if (observerEntries[0].isIntersecting && nextToken && !loadingMore && !loading) {
          loadMore();
        }
      },
      { rootMargin: '100px' }
    );

    if (sentinelElement) {
      observer.observe(sentinelElement);
    }
  }

  onDestroy(() => {
    if (observer) observer.disconnect();
  });

  async function selectParam(name: string) {
    selectedParam = name;
    detailLoading = true;
    showValue = withValue;
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
    showValue = false;
  }

  function toggleShowValue() {
    showValue = !showValue;
  }

  function maskValue(value: string): string {
    return '*'.repeat(Math.min(value.length, 32));
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

  async function handleSet(e: SubmitEvent) {
    e.preventDefault();
    if (!setForm.name || !setForm.value) {
      modalError = 'Name and value are required';
      return;
    }
    modalLoading = true;
    modalError = '';
    try {
      const isEdit = paramDetail && selectedParam === setForm.name;
      if (immediateMode) {
        await ParamSet(setForm.name, setForm.value, setForm.type);
        await loadParams();
        if (isEdit) {
          await selectParam(setForm.name);
        }
      } else {
        if (isEdit) {
          await StagingEdit('param', setForm.name, setForm.value);
        } else {
          await StagingAdd('param', setForm.name, setForm.value);
        }
      }
      showSetModal = false;
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
      if (immediateMode) {
        await ParamDelete(deleteTarget);
        if (selectedParam === deleteTarget) {
          closeDetail();
        }
        await loadParams();
      } else {
        await StagingDelete('param', deleteTarget, false, 0);
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

  function toggleVersionSelection(version: number) {
    const idx = diffSelectedVersions.indexOf(version);
    if (idx >= 0) {
      diffSelectedVersions = diffSelectedVersions.filter(v => v !== version);
    } else if (diffSelectedVersions.length < 2) {
      diffSelectedVersions = [...diffSelectedVersions, version];
    }
  }

  async function executeDiff() {
    if (!selectedParam || diffSelectedVersions.length !== 2) return;

    modalLoading = true;
    modalError = '';
    try {
      const sorted = [...diffSelectedVersions].sort((a, b) => a - b);
      const spec1 = `${selectedParam}#${sorted[0]}`;
      const spec2 = `${selectedParam}#${sorted[1]}`;
      diffResult = await ParamDiff(spec1, spec2);
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

  // Tag functions
  function openTagModal() {
    tagForm = { key: '', value: '' };
    tagError = '';
    showTagModal = true;
  }

  async function handleAddTag(e: SubmitEvent) {
    e.preventDefault();
    if (!selectedParam || !tagForm.key) {
      tagError = 'Key is required';
      return;
    }
    tagLoading = true;
    tagError = '';
    try {
      if (immediateMode) {
        await ParamAddTag(selectedParam, tagForm.key, tagForm.value);
      } else {
        await StagingAddTag('param', selectedParam, tagForm.key, tagForm.value);
      }
      showTagModal = false;
      await selectParam(selectedParam);
    } catch (e) {
      tagError = e instanceof Error ? e.message : String(e);
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
    if (!selectedParam || !removeTagTarget) return;
    removeTagLoading = true;
    removeTagError = '';
    try {
      if (immediateMode) {
        await ParamRemoveTag(selectedParam, removeTagTarget);
      } else {
        await StagingRemoveTag('param', selectedParam, removeTagTarget);
      }
      showRemoveTagModal = false;
      await selectParam(selectedParam);
    } catch (e) {
      removeTagError = e instanceof Error ? e.message : String(e);
    } finally {
      removeTagLoading = false;
    }
  }

  onMount(() => {
    mounted = true;
    loadParams();
  });
</script>

<div class="view-container">
  <div class="filter-bar">
    <input
      type="text"
      class="filter-input prefix-input"
      placeholder="Prefix (e.g., /prod/)"
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
      <input type="checkbox" bind:checked={recursive} />
      Recursive
    </label>
    <label class="checkbox-label">
      <input type="checkbox" bind:checked={withValue} />
      Show Values
    </label>
    <button class="btn-primary" onclick={loadParams} disabled={loading}>
      {loading ? 'Loading...' : 'Refresh'}
    </button>
    <button class="btn-secondary" onclick={() => openSetModal()}>
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
              <button class="item-button" onclick={() => selectParam(entry.name)}>
                <span class="item-name param">{entry.name}</span>
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

    {#if selectedParam}
      <div class="detail-panel">
        <div class="detail-header">
          <h3 class="detail-title param">{selectedParam}</h3>
          <div class="detail-actions">
            <button class="btn-action-sm" onclick={() => selectedParam && openSetModal(selectedParam)}>Edit</button>
            <button class="btn-action-sm btn-danger" onclick={() => selectedParam && openDeleteModal(selectedParam)}>Delete</button>
            {#if paramLog.length >= 2}
              <button class="btn-action-sm" class:active={diffMode} onclick={toggleDiffMode}>
                {diffMode ? 'Cancel' : 'Compare'}
              </button>
            {/if}
            <button class="btn-close" onclick={closeDetail}>
              <CloseIcon />
            </button>
          </div>
        </div>

        {#if detailLoading}
          <div class="loading">Loading...</div>
        {:else if paramDetail}
          <div class="detail-content">
            <div class="detail-section">
              <div class="section-header-value">
                <h4>Current Value</h4>
                {#if paramDetail.type === 'SecureString'}
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
                {/if}
              </div>
              <pre class="value-display" class:masked={paramDetail.type === 'SecureString' && !showValue}>{paramDetail.type === 'SecureString' && !showValue ? maskValue(paramDetail.value) : paramDetail.value}</pre>
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

            {#if paramDetail.description}
              <div class="detail-section">
                <h4>Description</h4>
                <p class="description-text">{paramDetail.description}</p>
              </div>
            {/if}

            <div class="detail-section">
              <div class="section-header">
                <h4>Tags</h4>
                <button class="btn-action-sm" onclick={openTagModal}>+ Add</button>
              </div>
              {#if paramDetail.tags && paramDetail.tags.length > 0}
                <div class="tags-list">
                  {#each paramDetail.tags as tag}
                    <div class="tag-item">
                      <span class="tag-key">{tag.key}</span>
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

            {#if paramLog.length > 0}
              <div class="detail-section">
                <div class="section-header">
                  <h4>Version History</h4>
                  {#if diffMode && diffSelectedVersions.length === 2}
                    <button class="btn-action-sm btn-compare" onclick={executeDiff} disabled={modalLoading}>
                      {modalLoading ? 'Comparing...' : 'Show Diff'}
                    </button>
                  {/if}
                </div>
                {#if diffMode}
                  <p class="diff-hint">Select 2 versions to compare</p>
                {/if}
                <ul class="history-list">
                  {#each paramLog as logEntry}
                    <li
                      class="history-item"
                      class:current={logEntry.isCurrent}
                      class:selectable={diffMode}
                      class:selected={diffSelectedVersions.includes(logEntry.version)}
                    >
                      {#if diffMode}
                        <label class="diff-checkbox">
                          <input
                            type="checkbox"
                            checked={diffSelectedVersions.includes(logEntry.version)}
                            disabled={!diffSelectedVersions.includes(logEntry.version) && diffSelectedVersions.length >= 2}
                            onchange={() => toggleVersionSelection(logEntry.version)}
                          />
                        </label>
                      {/if}
                      <div class="history-content">
                        <div class="history-header">
                          <span class="history-version">v{logEntry.version}</span>
                          {#if logEntry.isCurrent}
                            <span class="badge badge-current">current</span>
                          {/if}
                          <span class="history-date">{formatDate(logEntry.lastModified)}</span>
                        </div>
                        <pre class="history-value" class:masked={logEntry.type === 'SecureString' && !showValue}>{logEntry.type === 'SecureString' && !showValue ? maskValue(logEntry.value) : logEntry.value}</pre>
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

<!-- Set Modal -->
<Modal title={setForm.name ? 'Edit Parameter' : 'New Parameter'} show={showSetModal} onclose={() => showSetModal = false}>
  <form class="modal-form" onsubmit={handleSet}>
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
    <label class="checkbox-label immediate-checkbox">
      <input type="checkbox" bind:checked={immediateMode} />
      <span>Apply immediately (skip staging)</span>
    </label>
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showSetModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading}>
        {modalLoading ? (immediateMode ? 'Saving...' : 'Staging...') : (immediateMode ? 'Save' : 'Stage')}
      </button>
    </div>
  </form>
</Modal>

<!-- Delete Modal -->
<Modal title="Delete Parameter" show={showDeleteModal} onclose={() => showDeleteModal = false}>
  <div class="modal-confirm">
    {#if modalError}
      <div class="modal-error">{modalError}</div>
    {/if}
    <p>Are you sure you want to delete this parameter?</p>
    <code class="delete-target">{deleteTarget}</code>
    <p class="warning">{immediateMode ? 'This action cannot be undone.' : 'This will stage a delete operation.'}</p>
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

<!-- Remove Tag Modal -->
<Modal title="Remove Tag" show={showRemoveTagModal} onclose={() => showRemoveTagModal = false}>
  <div class="modal-confirm">
    {#if removeTagError}
      <div class="modal-error">{removeTagError}</div>
    {/if}
    <p>Are you sure you want to remove this tag?</p>
    <code class="delete-target">{removeTagTarget}</code>
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

<!-- Diff Modal -->
<Modal title="Version Comparison" show={showDiffModal} onclose={closeDiffModal}>
  {#if diffResult}
    <DiffDisplay
      oldValue={diffResult.oldValue}
      newValue={diffResult.newValue}
      oldLabel="Old"
      newLabel="New"
      oldSubLabel={`v${diffSelectedVersions[0]}`}
      newSubLabel={`v${diffSelectedVersions[1]}`}
    />
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={closeDiffModal}>Close</button>
    </div>
  {/if}
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

  /* Diff mode styles */
  .btn-action-sm.active {
    background: #e94560;
  }

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

  .section-header-value {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
  }

  .section-header-value h4 {
    margin: 0;
    font-size: 12px;
    text-transform: uppercase;
    color: #888;
    letter-spacing: 0.5px;
  }

  .btn-toggle {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 8px;
    background: #2d2d44;
    color: #fff;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 12px;
  }

  .btn-toggle:hover {
    background: #3d3d54;
  }

  .btn-toggle.active {
    background: #e94560;
  }

  .value-display.masked {
    color: #888;
    font-style: italic;
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
    color: #4fc3f7;
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

  /* Tag styles */
  .btn-tag-remove {
    margin-left: auto;
    padding: 2px 8px;
    background: transparent;
    border: 1px solid #444;
    border-radius: 4px;
    color: #888;
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
  }

  .btn-tag-remove:hover {
    background: #f44336;
    border-color: #f44336;
    color: #fff;
  }

  .no-tags {
    color: #666;
    font-size: 13px;
    margin: 0;
  }
</style>
