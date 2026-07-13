<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte';
  import { ParamAddTag, ParamDelete, ParamDiff, ParamList, ParamLog, ParamRemoveTag, ParamSet, ParamShow, ParamTypeOptions, StagingAdd, StagingAddTag, StagingCheckStatus, StagingDelete, StagingEdit, StagingRemoveTag } from '../../wailsjs/go/gui/App';
  import type { gui } from '../../wailsjs/go/models';
  import DiffDisplay from './DiffDisplay.svelte';
  import CloseIcon from './icons/CloseIcon.svelte';
  import EyeIcon from './icons/EyeIcon.svelte';
  import EyeOffIcon from './icons/EyeOffIcon.svelte';
  import Modal from './Modal.svelte';
  import StagingBanner from './StagingBanner.svelte';
  import TagList from './TagList.svelte';
  import { createDiffMode } from './useDiffMode.svelte';
  import { createDebouncer, formatDate, maskValue, NS_ALL, NS_NULL, parseError } from './viewUtils';
  import './common.css';

  interface Props {
    capability?: gui.ServiceCapability;
    provider?: string;
    // selectedNamespace is the App Configuration namespace filter, owned by App
    // (the dropdown lives in the sidebar footer). (NULL) → null/default rows, a
    // name → that namespace, * → all. Only meaningful for Azure App Configuration.
    selectedNamespace?: string;
    // onnamespaces reports the distinct namespaces present in the loaded rows
    // (sorted) so App can build the footer dropdown's options.
    onnamespaces?: (ns: string[]) => void;
    onnavigatetostaging?: () => void;
    onstagingchange?: () => void;
  }

  let { capability, provider = '', selectedNamespace = NS_NULL, onnamespaces, onnavigatetostaging, onstagingchange }: Props = $props();

  // Capability-driven visibility. Absent capability defaults to AWS-like (true)
  // so the component degrades safely if mounted without one.
  const stagingEnabled = $derived(capability?.hasStaging ?? true);
  const tagsEnabled = $derived(capability?.hasTags ?? true);
  const historyEnabled = $derived(capability?.hasVersionHistory ?? true);
  // The Description input is shown only where the provider persists it (AWS
  // param); Azure App Configuration ignores it, so it stays hidden there.
  // Default false so a capability object missing the field hides the input.
  const descriptionEnabled = $derived(capability?.hasDescription ?? false);

  // The namespace axis (Azure calls it a "label") exists only for Azure App
  // Configuration; ParamView under the Azure provider is always App Config
  // (Key Vault is a secret store, rendered by SecretView).
  const isAppConfig = $derived(provider === 'azure');

  // The namespace of the currently selected row, shown in the detail panel.
  let selectedEntryNamespace = $state('');

  const PAGE_SIZE = 50;
  const debounce = createDebouncer(300);
  const diffMode = createDiffMode<number>();

  // A pending filter debounce must not fire a stray List after a {#key} remount
  // into a different provider.
  onDestroy(() => debounce.cancel());

  let prefix = $state('');
  let filter = $state('');
  let recursive = $state(true);
  let withValue = $state(false);
  let loading = $state(false);
  let loadingMore = $state(false);
  let error = $state('');
  let nextToken = $state('');

  // Monotonic request id. Filter/prefix input is debounced, but debounce only
  // delays dispatch — it does not serialize in-flight requests. A slow broader
  // query resolving after a fast narrower one would otherwise overwrite the list
  // with stale rows. Each loadParams bumps the id and only the latest run may
  // assign entries/nextToken; loadMore captures the current id and appends only
  // while it is still current (#539).
  let loadSeq = 0;

  let entries: gui.ParamListEntry[] = $state([]);
  let selectedParam: string | null = $state(null);
  let paramDetail: gui.ParamShowResult | null = $state(null);
  let paramLog: gui.ParamLogEntry[] = $state([]);
  let detailLoading = $state(false);
  let showValue = $state(false);

  // Distinct namespaces present in the loaded rows (sorted). Feeds both the App
  // footer dropdown (via onnamespaces) and the create form's suggestions.
  const discoveredNamespaces = $derived.by(() => {
    if (!isAppConfig) return [] as string[];
    const discovered = new Set<string>();
    for (const e of entries) {
      if (e.namespace) discovered.add(e.namespace);
    }
    return [...discovered].sort();
  });

  // Report them up to App (App Config only), which owns the footer Namespace
  // dropdown and its options.
  $effect(() => {
    if (!isAppConfig) return;
    onnamespaces?.(discoveredNamespaces);
  });

  // A create must target exactly one (key, namespace). When the namespace filter
  // shows all (*) or multiple (a ,-list), there is no single target, so creation
  // is blocked until the user narrows the filter (the backend guard is the
  // backstop). Only meaningful for App Config.
  const createNamespaceBlocked = $derived(
    isAppConfig && (selectedNamespace === NS_ALL || selectedNamespace.includes(',')),
  );

  // The namespace a new setting is created under, derived from the current
  // filter: a concrete namespace prefills itself; (NULL)/all/multi prefill the
  // null namespace ("") — for all/multi the form is blocked anyway.
  function defaultCreateNamespace(): string {
    if (!isAppConfig || createNamespaceBlocked || selectedNamespace === NS_NULL) return '';
    return selectedNamespace;
  }

  // Rows actually displayed. Non-App-Config providers show every row; App Config
  // narrows by the selected namespace client-side (like the prefix/regex filters).
  const visibleEntries = $derived.by(() => {
    if (!isAppConfig || selectedNamespace === NS_ALL) return entries;
    if (selectedNamespace === NS_NULL) return entries.filter((e) => !e.namespace);
    return entries.filter((e) => e.namespace === selectedNamespace);
  });

  // Staging status for the selected item
  let stagingStatus: { hasEntry: boolean; hasTags: boolean } | null = $state(null);

  // Modal states
  let showSetModal = $state(false);
  let showDeleteModal = $state(false);
  let showDiffModal = $state(false);
  // Parameter type options are provider-driven (fetched from the backend), so
  // the UI never hardcodes SSM type strings.
  let paramTypeOptions: string[] = $state([]);
  let setForm = $state({ name: '', value: '', type: '', namespace: '', description: '' });
  let isEditMode = $state(false);
  let deleteTarget = $state('');
  let modalLoading = $state(false);
  let modalError = $state('');
  let immediateMode = $state(false);
  // When staging is unavailable, every write is immediate (no staging toggle).
  const immediate = $derived(immediateMode || !stagingEnabled);
  // The Type dropdown is scope-aware: empty options (Azure App Config) hide it.
  const typeEnabled = $derived(paramTypeOptions.length > 0);

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

  // Reload when the checkboxes change. prefix/filter are intentionally NOT
  // dependencies — the debounced oninput handlers own text-input reloads, so
  // typing doesn't fire a backend list on every keystroke (AWS lists everything
  // then filters client-side).
  $effect(() => {
    const trackedRecursive = recursive;
    const trackedWithValue = withValue;
    if (initialLoadDone) {
      loadParams(untrack(() => ({ prefix, filter, recursive: trackedRecursive, withValue: trackedWithValue })));
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
    const seq = ++loadSeq;
    loading = true;
    error = '';
    nextToken = '';
    try {
      const result = await ParamList(opts.prefix, opts.recursive, opts.withValue, opts.filter, PAGE_SIZE, '');
      if (seq !== loadSeq) return; // superseded by a newer query
      entries = result?.entries || [];
      nextToken = result?.nextToken || '';
    } catch (e) {
      if (seq !== loadSeq) return; // superseded by a newer query
      error = parseError(e);
      entries = [];
    } finally {
      if (seq === loadSeq) loading = false;
    }
  }

  async function loadMore(opts: LoadParamsOptions) {
    if (!nextToken || loadingMore || loading) return;

    // Continuation of the current query: capture the id without bumping so a
    // loadParams starting mid-flight supersedes this append.
    const seq = loadSeq;
    loadingMore = true;
    try {
      const result = await ParamList(opts.prefix, opts.recursive, opts.withValue, opts.filter, PAGE_SIZE, nextToken);
      if (seq !== loadSeq) return; // superseded by a newer query
      entries = [...entries, ...(result?.entries || [])];
      nextToken = result?.nextToken || '';
    } catch (e) {
      if (seq !== loadSeq) return; // superseded by a newer query
      error = parseError(e);
    } finally {
      loadingMore = false;
    }
  }

  function setupIntersectionObserver() {
    if (observer) observer.disconnect();

    observer = new IntersectionObserver(
      (observerEntries) => {
        if (observerEntries[0]?.isIntersecting && nextToken && !loadingMore && !loading) {
          loadMore({ prefix, filter, recursive, withValue });
        }
      },
      { rootMargin: '100px' }
    );

    if (sentinelElement) {
      observer.observe(sentinelElement);
    }
  }

  async function selectParam(name: string, namespace = '') {
    selectedParam = name;
    selectedEntryNamespace = namespace;
    detailLoading = true;
    showValue = withValue;
    stagingStatus = null;
    error = '';
    // History is best-effort and unsupported by some providers (Azure App
    // Configuration): fetch it only when supported, and use allSettled so a
    // failed ParamLog never blanks out the value shown by ParamShow.
    const [detailResult, logResult] = await Promise.allSettled([
      ParamShow(name, namespace),
      historyEnabled ? ParamLog(name, 10, namespace) : Promise.resolve(null),
    ]);
    if (detailResult.status === 'fulfilled') {
      paramDetail = detailResult.value;
    } else {
      error = parseError(detailResult.reason);
    }
    paramLog = logResult.status === 'fulfilled' ? (logResult.value?.entries ?? []) : [];
    detailLoading = false;

    // Staging status is decoupled from the detail fetch: only for
    // staging-capable providers, and a failure just means no banner — it must
    // never break the detail pane.
    if (stagingEnabled) {
      try {
        stagingStatus = await StagingCheckStatus('param', name, namespace);
      } catch {
        stagingStatus = null;
      }
    }
  }

  function closeDetail() {
    selectedParam = null;
    paramDetail = null;
    paramLog = [];
    showValue = false;
    stagingStatus = null;
  }

  function toggleShowValue() {
    showValue = !showValue;
  }

  // Set modal
  function openSetModal(name?: string) {
    if (name && paramDetail) {
      // Edit targets the selected entry's own namespace (read-only in the form).
      // Pre-fill the current description so an unchanged edit round-trips it.
      setForm = { name, value: paramDetail.value, type: paramDetail.type, namespace: selectedEntryNamespace, description: paramDetail.description ?? '' };
      isEditMode = true;
    } else {
      setForm = { name: prefix || '', value: '', type: paramTypeOptions[0] ?? '', namespace: defaultCreateNamespace(), description: '' };
      isEditMode = false;
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
      // App Config: create targets the form's namespace; edit targets the
      // existing entry's namespace. Other providers ignore this argument.
      const targetNamespace = isEdit ? selectedEntryNamespace : setForm.namespace;
      // Only send a description where the provider supports it; the binding also
      // drops it server-side for providers that ignore it (#767).
      const description = descriptionEnabled ? setForm.description : '';
      if (immediate) {
        await ParamSet(setForm.name, setForm.value, setForm.type, targetNamespace, description);
        await loadParams({ prefix, filter, recursive, withValue });
        if (isEdit) {
          await selectParam(setForm.name, selectedEntryNamespace);
        }
      } else {
        if (isEdit) {
          await StagingEdit('param', setForm.name, setForm.value, targetNamespace);
        } else {
          await StagingAdd('param', setForm.name, setForm.value, targetNamespace);
        }
        onstagingchange?.();
        // Refresh staging status to update the indicator
        if (isEdit) {
          await selectParam(setForm.name, selectedEntryNamespace);
        }
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
      if (immediate) {
        await ParamDelete(deleteTarget, selectedEntryNamespace);
        if (selectedParam === deleteTarget) {
          closeDetail();
        }
        await loadParams({ prefix, filter, recursive, withValue });
      } else {
        await StagingDelete('param', deleteTarget, false, 0, selectedEntryNamespace);
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
      diffResult = await ParamDiff(spec1, spec2, selectedEntryNamespace);
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
      if (immediate) {
        await ParamAddTag(selectedParam, tagForm.key, tagForm.value, selectedEntryNamespace);
      } else {
        await StagingAddTag('param', selectedParam, tagForm.key, tagForm.value, selectedEntryNamespace);
        onstagingchange?.();
      }
      showTagModal = false;
      await selectParam(selectedParam, selectedEntryNamespace);
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
      if (immediate) {
        await ParamRemoveTag(selectedParam, removeTagTarget, selectedEntryNamespace);
      } else {
        await StagingRemoveTag('param', selectedParam, removeTagTarget, selectedEntryNamespace);
        onstagingchange?.();
      }
      showRemoveTagModal = false;
      await selectParam(selectedParam, selectedEntryNamespace);
    } catch (err) {
      removeTagError = parseError(err);
    } finally {
      removeTagLoading = false;
    }
  }

  onMount(async () => {
    try {
      paramTypeOptions = await ParamTypeOptions();
    } catch {
      paramTypeOptions = [];
    }
    await loadParams({ prefix, filter, recursive, withValue });
    initialLoadDone = true;
  });
</script>

<div class="view-container">
  <div class="filter-bar">
    <input
      type="text"
      class="filter-input prefix-input"
      placeholder="Filter by prefix"
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
      {#if visibleEntries.length === 0 && !loading}
        <div class="empty-state">
          No parameters found. Try adjusting the prefix filter.
        </div>
      {:else}
        <ul class="item-list">
          {#each visibleEntries as entry}
            <li class="item-entry" class:selected={selectedParam === entry.name && selectedEntryNamespace === entry.namespace}>
              <button class="item-button" onclick={() => selectParam(entry.name, entry.namespace)}>
                <span class="item-name param">{entry.name}</span>
                {#if isAppConfig}
                  <span class="namespace-badge">{entry.namespace || NS_NULL}</span>
                {/if}
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
            {#if historyEnabled && paramLog.length >= 2}
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
        {:else if paramDetail}
          <div class="detail-content">
            <div class="detail-section">
              <div class="section-header-value">
                <h4>Current Value</h4>
                {#if paramDetail.secret}
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
              <pre class="value-display" class:masked={paramDetail.secret && !showValue}>{paramDetail.secret && !showValue ? maskValue(paramDetail.value) : paramDetail.value}</pre>
            </div>

            <div class="detail-meta">
              {#if historyEnabled}
                <div class="meta-item">
                  <span class="meta-label">Version</span>
                  <span class="meta-value">{paramDetail.version}</span>
                </div>
              {/if}
              {#if isAppConfig}
                <div class="meta-item">
                  <span class="meta-label">Namespace</span>
                  <span class="meta-value namespace-value">{selectedEntryNamespace || NS_NULL}</span>
                </div>
              {/if}
              {#if typeEnabled}
                <div class="meta-item">
                  <span class="meta-label">Type</span>
                  <span class="meta-value">{paramDetail.type}</span>
                </div>
              {/if}
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

            {#if tagsEnabled}
              <TagList tags={paramDetail.tags} serviceClass="param" {provider} onadd={openTagModal} onremove={openRemoveTagModal} />
            {/if}

            {#if historyEnabled && paramLog.length > 0}
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
                        <pre class="history-value" class:masked={logEntry.secret && !showValue}>{logEntry.secret && !showValue ? maskValue(logEntry.value) : logEntry.value}</pre>
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
<Modal title={isEditMode ? 'Edit Parameter' : 'New Parameter'} show={showSetModal} onclose={() => showSetModal = false}>
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
    {#if isAppConfig}
      {#if !isEditMode && createNamespaceBlocked}
        <div class="modal-warning" data-testid="ns-blocked">
          Pick a single namespace to create under — currently viewing all namespaces.
        </div>
      {:else}
        <div class="form-group">
          <label for="param-namespace">Namespace</label>
          <input
            id="param-namespace"
            type="text"
            class="form-input"
            list="param-namespace-suggestions"
            bind:value={setForm.namespace}
            placeholder="(NULL) — default namespace"
            disabled={isEditMode}
          />
          <datalist id="param-namespace-suggestions">
            {#each discoveredNamespaces as ns}
              <option value={ns}></option>
            {/each}
          </datalist>
        </div>
      {/if}
    {/if}
    {#if typeEnabled}
      <div class="form-group">
        <label for="param-type">Type</label>
        <select id="param-type" class="form-input" bind:value={setForm.type}>
          {#each paramTypeOptions as typeOption}
            <option value={typeOption}>{typeOption}</option>
          {/each}
        </select>
      </div>
    {/if}
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
    {#if descriptionEnabled}
      <div class="form-group">
        <label for="param-description">Description</label>
        <input
          id="param-description"
          type="text"
          class="form-input"
          data-testid="param-description-input"
          bind:value={setForm.description}
          placeholder="Optional description"
        />
      </div>
    {/if}
    {#if stagingEnabled}
      <label class="checkbox-label immediate-checkbox">
        <input type="checkbox" bind:checked={immediateMode} />
        <span>Apply immediately (skip staging)</span>
      </label>
    {/if}
    <div class="form-actions">
      <button type="button" class="btn-secondary" onclick={() => showSetModal = false}>Cancel</button>
      <button type="submit" class="btn-primary" disabled={modalLoading || (!isEditMode && createNamespaceBlocked)}>
        {modalLoading ? (immediate ? 'Saving...' : 'Staging...') : (immediate ? 'Save' : 'Stage')}
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
    <p class="warning">{immediate ? 'This action cannot be undone.' : 'This will stage a delete operation.'}</p>
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

<!-- Remove Tag Modal -->
<Modal title="Remove Tag" show={showRemoveTagModal} onclose={() => showRemoveTagModal = false}>
  <div class="modal-confirm">
    {#if removeTagError}
      <div class="modal-error">{removeTagError}</div>
    {/if}
    <p>Are you sure you want to remove this tag?</p>
    <code class="delete-target param">{removeTagTarget}</code>
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

<!-- Diff Modal -->
<Modal title="Version Comparison" show={showDiffModal} onclose={closeDiffModal}>
  {#if diffResult}
    <DiffDisplay
      oldValue={diffResult.oldValue}
      newValue={diffResult.newValue}
      secret={diffResult.secret}
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

  /* Per-row namespace badge in the list. */
  .namespace-badge {
    display: inline-block;
    margin-left: 8px;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 11px;
    background: rgba(120, 120, 120, 0.18);
    color: #888;
    white-space: nowrap;
  }
</style>
