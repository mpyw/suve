<script lang="ts">
  import { onMount } from 'svelte';
  import { StagingStatus } from '../../wailsjs/go/main/App';
  import type { main } from '../../wailsjs/go/models';

  let loading = false;
  let error = '';
  let ssmEntries: main.StagingEntry[] = [];
  let smEntries: main.StagingEntry[] = [];

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

  onMount(() => {
    loadStatus();
  });
</script>

<div class="staging-view">
  <div class="header">
    <h2 class="title">Staging Area</h2>
    <button class="btn-refresh" on:click={loadStatus} disabled={loading}>
      {loading ? 'Loading...' : 'Refresh'}
    </button>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <div class="content">
    <div class="section">
      <div class="section-header">
        <h3 class="section-title">
          <span class="section-icon">P</span>
          Parameters (SSM)
        </h3>
        <span class="count-badge">{ssmEntries.length}</span>
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
      <button class="btn-action btn-apply" disabled>
        Apply Changes
      </button>
      <button class="btn-action btn-reset" disabled>
        Reset All
      </button>
    </div>
  {/if}
</div>

<style>
  .staging-view {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: #0f0f1a;
  }

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
    margin-left: auto;
    font-size: 12px;
    padding: 2px 8px;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 10px;
    color: #888;
  }

  .empty-state {
    padding: 24px;
    text-align: center;
    color: #666;
    font-size: 14px;
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
</style>
