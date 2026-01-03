<script lang="ts">
  import { onMount } from 'svelte';
  import { ParamList, ParamShow, ParamLog } from '../../wailsjs/go/main/App';
  import type { main } from '../../wailsjs/go/models';
  import CloseIcon from './icons/CloseIcon.svelte';
  import './common.css';

  let prefix = '';
  let recursive = true;
  let withValue = false;
  let loading = false;
  let error = '';

  let entries: main.ParamListEntry[] = [];
  let selectedParam: string | null = null;
  let paramDetail: main.ParamShowResult | null = null;
  let paramLog: main.ParamLogEntry[] = [];
  let detailLoading = false;

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
      on:keydown={(e) => e.key === 'Enter' && loadParams()}
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
          <button class="btn-close" on:click={closeDetail}>
            <CloseIcon />
          </button>
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
