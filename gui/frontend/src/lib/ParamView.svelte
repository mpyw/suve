<script lang="ts">
  import { onMount } from 'svelte';
  import { ParamList, ParamShow, ParamLog, ParamDiff } from '../../wailsjs/go/main/App';
  import type { main } from '../../wailsjs/go/models';

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

<div class="param-view">
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
    <button class="btn-refresh" on:click={loadParams} disabled={loading}>
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
        <ul class="param-list">
          {#each entries as entry}
            <li class="param-item" class:selected={selectedParam === entry.name}>
              <button class="param-button" on:click={() => selectParam(entry.name)}>
                <span class="param-name">{entry.name}</span>
                {#if entry.value !== undefined}
                  <span class="param-value">{entry.value}</span>
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
          <h3 class="detail-title">{selectedParam}</h3>
          <button class="btn-close" on:click={closeDetail}>x</button>
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
                          <span class="badge-current">current</span>
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

<style>
  .param-view {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: #0f0f1a;
  }

  .filter-bar {
    display: flex;
    gap: 12px;
    padding: 16px;
    background: #1a1a2e;
    border-bottom: 1px solid #2d2d44;
    align-items: center;
  }

  .filter-input {
    flex: 1;
    padding: 8px 12px;
    border: 1px solid #2d2d44;
    border-radius: 4px;
    background: #0f0f1a;
    color: #fff;
    font-size: 14px;
  }

  .filter-input:focus {
    outline: none;
    border-color: #e94560;
  }

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 6px;
    color: #a0a0a0;
    font-size: 14px;
    cursor: pointer;
  }

  .btn-refresh {
    padding: 8px 16px;
    background: #e94560;
    color: #fff;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
  }

  .btn-refresh:hover {
    background: #d63050;
  }

  .btn-refresh:disabled {
    background: #666;
    cursor: not-allowed;
  }

  .error-banner {
    padding: 12px 16px;
    background: #ff4444;
    color: #fff;
    font-size: 14px;
  }

  .content {
    flex: 1;
    display: flex;
    overflow: hidden;
  }

  .list-panel {
    flex: 1;
    overflow-y: auto;
    transition: flex 0.3s;
  }

  .list-panel.collapsed {
    flex: 0.4;
    min-width: 250px;
  }

  .empty-state {
    padding: 40px;
    text-align: center;
    color: #666;
  }

  .param-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .param-item {
    border-bottom: 1px solid #1a1a2e;
  }

  .param-item.selected {
    background: #1a1a2e;
  }

  .param-button {
    display: flex;
    flex-direction: column;
    gap: 4px;
    width: 100%;
    padding: 12px 16px;
    background: transparent;
    border: none;
    text-align: left;
    cursor: pointer;
    color: #fff;
  }

  .param-button:hover {
    background: #252542;
  }

  .param-name {
    font-family: monospace;
    font-size: 13px;
    color: #4fc3f7;
  }

  .param-value {
    font-family: monospace;
    font-size: 12px;
    color: #888;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .detail-panel {
    flex: 0.6;
    border-left: 1px solid #2d2d44;
    display: flex;
    flex-direction: column;
    background: #1a1a2e;
  }

  .detail-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px;
    border-bottom: 1px solid #2d2d44;
  }

  .detail-title {
    margin: 0;
    font-size: 14px;
    font-family: monospace;
    color: #4fc3f7;
  }

  .btn-close {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 1px solid #2d2d44;
    border-radius: 4px;
    color: #888;
    cursor: pointer;
    font-size: 16px;
  }

  .btn-close:hover {
    background: #e94560;
    border-color: #e94560;
    color: #fff;
  }

  .loading {
    padding: 40px;
    text-align: center;
    color: #666;
  }

  .detail-content {
    flex: 1;
    overflow-y: auto;
    padding: 16px;
  }

  .detail-section {
    margin-bottom: 24px;
  }

  .detail-section h4 {
    margin: 0 0 12px 0;
    font-size: 12px;
    text-transform: uppercase;
    color: #888;
    letter-spacing: 0.5px;
  }

  .value-display {
    margin: 0;
    padding: 12px;
    background: #0f0f1a;
    border-radius: 4px;
    font-family: monospace;
    font-size: 13px;
    color: #a5d6a7;
    white-space: pre-wrap;
    word-break: break-all;
  }

  .detail-meta {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin-bottom: 24px;
  }

  .meta-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .meta-label {
    font-size: 11px;
    text-transform: uppercase;
    color: #666;
  }

  .meta-value {
    font-size: 14px;
    color: #fff;
  }

  .history-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .history-item {
    padding: 12px;
    background: #0f0f1a;
    border-radius: 4px;
    margin-bottom: 8px;
  }

  .history-item.current {
    border-left: 3px solid #e94560;
  }

  .history-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }

  .history-version {
    font-weight: bold;
    color: #fff;
  }

  .badge-current {
    font-size: 10px;
    padding: 2px 6px;
    background: #e94560;
    color: #fff;
    border-radius: 3px;
  }

  .history-date {
    font-size: 12px;
    color: #666;
    margin-left: auto;
  }

  .history-value {
    margin: 0;
    font-family: monospace;
    font-size: 12px;
    color: #888;
    white-space: pre-wrap;
    word-break: break-all;
  }
</style>
