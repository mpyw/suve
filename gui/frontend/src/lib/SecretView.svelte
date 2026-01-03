<script lang="ts">
  import { onMount } from 'svelte';
  import { SecretList, SecretShow, SecretLog } from '../../wailsjs/go/main/App';
  import type { main } from '../../wailsjs/go/models';
  import CloseIcon from './icons/CloseIcon.svelte';
  import EyeIcon from './icons/EyeIcon.svelte';
  import EyeOffIcon from './icons/EyeOffIcon.svelte';
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
          <button class="btn-close" on:click={closeDetail}>
            <CloseIcon />
          </button>
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
</style>
