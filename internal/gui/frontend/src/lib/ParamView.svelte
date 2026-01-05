<script lang="ts">
  import { onMount } from 'svelte';
  import { ParamList, ParamShow, ParamLog, ParamSet, ParamDelete, ParamDiff, ParamAddTag, ParamRemoveTag, StagingAdd, StagingEdit, StagingDelete, StagingAddTag, StagingRemoveTag, StagingCheckStatus } from '../../wailsjs/go/gui/App';
  import type { gui } from '../../wailsjs/go/models';
  import CloseIcon from './icons/CloseIcon.svelte';
  import EyeIcon from './icons/EyeIcon.svelte';
  import EyeOffIcon from './icons/EyeOffIcon.svelte';
  import Modal from './Modal.svelte';
  import DiffDisplay from './DiffDisplay.svelte';
  import { maskValue, formatDate, parseError, createDebouncer } from './viewUtils';
  import { createDiffMode } from './useDiffMode.svelte';
  import './common.css';

  interface Props {
    onnavigatetostaging?: () => void;
    onstagingchange?: () => void;
  }

  let { onnavigatetostaging, onstagingchange }: Props = $props();

  const PAGE_SIZE = 50;
  const debounce = createDebouncer(300);
  const diffMode = createDiffMode<number>();

  let prefix = $state('');
  let filter = $state('');
  let recursive = $state(true);
  let withValue = $state(false);
  let loading = $state(false);
  let loadingMore = $state(false);
  let error = $state('');
  let nextToken = $state('');

  let entries: gui.ParamListEntry[] = $state([]);
  let selectedParam: string | null = $state(null);
  let paramDetail: gui.ParamShowResult | null = $state(null);
  let paramLog: gui.ParamLogEntry[] = $state([]);
  let detailLoading = $state(false);
  let showValue = $state(false);

  // Staging status for the selected item
  let stagingStatus: { hasEntry: boolean; hasTags: boolean } | null = $state(null);

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

  // Track if initial load has happened
  let initialLoadDone = $state(false);

  interface LoadParamsOptions {
    prefix: string;
    filter: string;
    recursive: boolean;
    withValue: boolean;
  }

  // Reload when filter options change
  $effect(() => {
    const opts = { prefix, filter, recursive, withValue }; // read values to create dependencies
    if (initialLoadDone) {
      loadParams(opts);
    }
  });

  $effect(() => {
    if (sentinelElement) {
      setupIntersectionObserver();
    }
    return () => {
      if (observer) observer.disconnect();
    };
  });

  function handlePrefixInput() {
    debounce(() => loadParams({ prefix, filter, recursive, withValue }));
  }

  function handleFilterInput() {
    debounce(() => loadParams({ prefix, filter, recursive, withValue }));
  }

  async function loadParams(opts: LoadParamsOptions) {
    loading = true;
    error = '';
    nextToken = '';
    try {
      const result = await ParamList(opts.prefix, opts.recursive, opts.withValue, opts.filter, PAGE_SIZE, '');
      entries = result?.entries || [];
      nextToken = result?.nextToken || '';
    } catch (e) {
      error = parseError(e);
      entries = [];
    } finally {
      loading = false;
    }
  }

  async function loadMore(opts: LoadParamsOptions) {
    if (!nextToken || loadingMore || loading) return;

    loadingMore = true;
    try {
      const result = await ParamList(opts.prefix, opts.recursive, opts.withValue, opts.filter, PAGE_SIZE, nextToken);
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
      (observerEntries) => {
        if (observerEntries[0].isIntersecting && nextToken && !loadingMore && !loading) {
          loadMore({ prefix, filter, recursive, withValue });
        }
      },
      { rootMargin: '100px' }
    );

    if (sentinelElement) {
      observer.observe(sentinelElement);
    }
  }

  async function selectParam(name: string) {
    selectedParam = name;
    detailLoading = true;
    showValue = withValue;
    stagingStatus = null;
    try {
      const [detail, log, staging] = await Promise.all([
        ParamShow(name),
        ParamLog(name, 10),
        StagingCheckStatus('param', name)
      ]);
      paramDetail = detail;
      paramLog = log?.entries || [];
      stagingStatus = staging;
    } catch (e) {
      error = parseError(e);
    } finally {
      detailLoading = false;
    }
  }

  function closeDetail() {
    selectedParam = null;
    paramDetail = null;
    paramLog = [];
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
        await loadParams({ prefix, filter, recursive, withValue });
        if (isEdit) {
          await selectParam(setForm.name);
        }
      } else {
        if (isEdit) {
          await StagingEdit('param', setForm.name, setForm.value);
        } else {
          await StagingAdd('param', setForm.name, setForm.value);
        }
        onstagingchange?.();
      }
      showSetModal = false;
    } catch (err) {
      modalError = parseError(err);
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
        await loadParams({ prefix, filter, recursive, withValue });
      } else {
        await StagingDelete('param', deleteTarget, false, 0);
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
    if (!selectedParam || !diffMode.canCompare) return;

    modalLoading = true;
    modalError = '';
    try {
      const sorted = [...diffMode.selectedVersions].sort((a, b) => a - b);
      const spec1 = `${selectedParam}#${sorted[0]}`;
      const spec2 = `${selectedParam}#${sorted[1]}`;
      diffResult = await ParamDiff(spec1, spec2);
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
        onstagingchange?.();
      }
      showTagModal = false;
      await selectParam(selectedParam);
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
    if (!selectedParam || !removeTagTarget) return;
    removeTagLoading = true;
    removeTagError = '';
    try {
      if (immediateMode) {
        await ParamRemoveTag(selectedParam, removeTagTarget);
      } else {
        await StagingRemoveTag('param', selectedParam, removeTagTarget);
        onstagingchange?.();
      }
      showRemoveTagModal = false;
      await selectParam(selectedParam);
    } catch (err) {
      removeTagError = parseError(err);
    } finally {
      removeTagLoading = false;
    }
  }

  onMount(async () => {
    await loadParams({ prefix, filter, recursive, withValue });
    initialLoadDone = true;
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
    <button class="btn-primary" onclick={() => loadParams({ prefix, filter, recursive, withValue })} disabled={loading}>
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
          <button class="staging-banner" onclick={onnavigatetostaging}>
            <span class="staging-icon">⚠</span>
            <span class="staging-text">{getStagingMessage()}</span>
            <span class="staging-link">View in Staging →</span>
          </button>
        {/if}

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
                      <span class="tag-key param">{tag.key}</span>
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
                  {#each paramLog as logEntry}
                    <li
                      class="history-item"
                      class:current={logEntry.isCurrent}
                      class:selectable={diffMode.active}
                      class:selected={diffMode.isSelected(logEntry.version)}
                    >
                      {#if diffMode.active}
                        <label class="diff-checkbox">
                          <input
                            type="checkbox"
                            checked={diffMode.isSelected(logEntry.version)}
                            disabled={diffMode.isDisabled(logEntry.version)}
                            onchange={() => diffMode.toggleSelection(logEntry.version)}
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
    <code class="delete-target param">{deleteTarget}</code>
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
    <code class="delete-target param">{removeTagTarget}</code>
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
      oldSubLabel={`v${diffMode.selectedVersions[0]}`}
      newSubLabel={`v${diffMode.selectedVersions[1]}`}
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
  /* ParamView-specific styles - shared styles are in common.css */

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

  .value-display.masked {
    font-style: italic;
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
