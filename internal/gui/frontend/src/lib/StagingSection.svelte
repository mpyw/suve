<script lang="ts">
  import type { gui } from '../../wailsjs/go/models';
  import DiffDisplay from './DiffDisplay.svelte';
  import './common.css';

  interface Props {
    icon: string;
    title: string;
    emptyText: string;
    entries: gui.StagingDiffEntry[];
    tagEntries: gui.StagingDiffTagEntry[];
    viewMode: 'diff' | 'value';
    // Whether this service supports tags; when false the section renders no tag
    // chips or "Add Tag" control. Driven by the service capability (HasTags).
    hasTags?: boolean;
    // Whether to show each entry's namespace as a badge (Azure App Configuration
    // only). "(NULL)" is the null/default namespace.
    showNamespace?: boolean;
    onapply: () => void;
    onreset: () => void;
    onedit: (entry: gui.StagingDiffEntry) => void;
    // Inline row actions may return a promise so the per-row busy guard can await
    // the backend call and keep the button disabled until it settles (#568).
    onunstage: (name: string, namespace: string) => void | Promise<void>;
    onaddtag: (entryName: string, namespace: string) => void;
    onedittag: (entryName: string, namespace: string, key: string, value: string) => void;
    onremovetag: (entryName: string, namespace: string, key: string) => void | Promise<void>;
    oncanceluntag: (entryName: string, namespace: string, key: string) => void | Promise<void>;
  }

  let {
    icon,
    title,
    emptyText,
    entries,
    tagEntries,
    viewMode,
    hasTags = true,
    showNamespace = false,
    onapply,
    onreset,
    onedit,
    onunstage,
    onaddtag,
    onedittag,
    onremovetag,
    oncanceluntag,
  }: Props = $props();

  // Per-row in-flight guard: an inline action (Unstage / tag × / ↩) fires an
  // async backend call; without a guard a double-click double-fires and the
  // second call targets an already-removed key and flashes a false error. Track
  // the in-flight action keys and disable / ignore re-entry until the call
  // settles (#568).
  let busyActions = $state(new Set<string>());

  async function runRowAction(key: string, action: () => void | Promise<void>) {
    if (busyActions.has(key)) return; // ignore re-entry (double-click)
    busyActions = new Set(busyActions).add(key);
    try {
      await action();
    } finally {
      const next = new Set(busyActions);
      next.delete(key);
      busyActions = next;
    }
  }

  function getOperationColor(op: string): string {
    switch (op?.toLowerCase()) {
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

  function hasValueChange(entry: gui.StagingDiffEntry): boolean {
    return entry.stagedValue !== undefined && entry.stagedValue !== '';
  }

  function showEditButton(entry: gui.StagingDiffEntry): boolean {
    return entry.operation !== 'delete' && hasValueChange(entry);
  }

  function findTagEntry(name: string): gui.StagingDiffTagEntry | undefined {
    return tagEntries.find(t => t.name === name);
  }

  // Tag entries with no matching value entry represent a tag-only staged change.
  // The value-entry loop only draws tags nested inside a value entry, so these
  // get their own rows and are counted toward the section total and gating.
  const tagOnlyEntries = $derived(tagEntries.filter(t => !entries.some(e => e.name === t.name)));
</script>

<div class="section">
  <div class="section-header">
    <h3 class="section-title">
      <span class="section-icon">{icon}</span>
      {title}
    </h3>
    <span class="count-badge">{entries.length + tagOnlyEntries.length}</span>
    <div class="section-actions">
      {#if entries.length + tagOnlyEntries.length > 0}
        <button class="btn-section btn-apply-sm" onclick={onapply}>Apply</button>
        <button class="btn-section btn-reset-sm" onclick={onreset}>Reset</button>
      {/if}
    </div>
  </div>

  {#if entries.length === 0 && tagOnlyEntries.length === 0}
    <div class="empty-state">{emptyText}</div>
  {:else}
    <ul class="entry-list">
      {#each entries as entry}
        {@const tagEntry = findTagEntry(entry.name)}
        <li class="entry-item">
          <div class="entry-header">
            <span class="operation-badge" style="background: {getOperationColor(entry.operation || '')}">
              {entry.operation}
            </span>
            {#if showNamespace}
              <span class="namespace-badge">{entry.namespace || '(NULL)'}</span>
            {/if}
            <span class="entry-name">{entry.name}</span>
            <div class="entry-actions">
              {#if showEditButton(entry)}
                <button class="btn-entry" onclick={() => onedit(entry)}>Edit</button>
              {/if}
              <button class="btn-entry btn-unstage" disabled={busyActions.has(`u:${entry.namespace}:${entry.name}`)} onclick={() => runRowAction(`u:${entry.namespace}:${entry.name}`, () => onunstage(entry.name, entry.namespace))}>Unstage</button>
            </div>
          </div>
          <div class="entry-tags">
            {#if hasTags}
            {#if tagEntry?.addTags && Object.keys(tagEntry.addTags).length > 0}
              <div class="tag-changes tag-add">
                <span class="tag-label">+ Tags:</span>
                {#each Object.entries(tagEntry.addTags) as [key, value]}
                  <button class="tag-item tag-item-editable" type="button" onclick={() => onedittag(entry.name, entry.namespace, key, value)}>
                    {key}={value}
                    <span class="tag-delete-btn" role="button" tabindex="0" onclick={(e: MouseEvent) => { e.stopPropagation(); runRowAction(`rt:${entry.namespace}:${entry.name}:${key}`, () => onremovetag(entry.name, entry.namespace, key)); }} onkeydown={(e: KeyboardEvent) => { e.stopPropagation(); if (e.key === 'Enter') runRowAction(`rt:${entry.namespace}:${entry.name}:${key}`, () => onremovetag(entry.name, entry.namespace, key)); }}>×</span>
                  </button>
                {/each}
              </div>
            {/if}
            {#if tagEntry?.removeTags && Object.keys(tagEntry.removeTags).length > 0}
              <div class="tag-changes tag-remove">
                <span class="tag-label">- Tags:</span>
                {#each Object.entries(tagEntry.removeTags) as [key, value]}
                  <span class="tag-item">
                    {value ? `${key}=${value}` : key}
                    <button class="tag-cancel-btn" disabled={busyActions.has(`cu:${entry.namespace}:${entry.name}:${key}`)} onclick={() => runRowAction(`cu:${entry.namespace}:${entry.name}:${key}`, () => oncanceluntag(entry.name, entry.namespace, key))} title="Cancel untag">↩</button>
                  </span>
                {/each}
              </div>
            {/if}
            {#if entry.operation !== 'delete'}
              <button class="btn-add-tag" onclick={() => onaddtag(entry.name, entry.namespace)}>+ Add Tag</button>
            {/if}
            {/if}
          </div>
          {#if entry.operation === 'delete'}
            {#if viewMode === 'diff'}
              <div class="entry-diff">
                <DiffDisplay
                  oldValue={entry.remoteValue || ''}
                  newValue="(deleted)"
                  oldLabel="Remote"
                  newLabel="Staged"
                  oldSubLabel={entry.remoteIdentifier || ''}
                />
              </div>
            {:else}
              <pre class="entry-value entry-value-delete">(will be deleted)</pre>
            {/if}
          {:else if entry.stagedValue !== undefined && entry.stagedValue !== ''}
            {#if viewMode === 'diff' && entry.operation !== 'create'}
              <div class="entry-diff">
                <DiffDisplay
                  oldValue={entry.remoteValue || ''}
                  newValue={entry.stagedValue}
                  oldLabel="Remote"
                  newLabel="Staged"
                  oldSubLabel={entry.remoteIdentifier || ''}
                />
              </div>
            {:else}
              <pre class="entry-value">{entry.stagedValue}</pre>
            {/if}
          {/if}
        </li>
      {/each}
      {#each tagOnlyEntries as tagEntry}
        <li class="entry-item">
          <div class="entry-header">
            <span class="entry-name">{tagEntry.name}</span>
            {#if showNamespace}
              <span class="namespace-badge">{tagEntry.namespace || '(NULL)'}</span>
            {/if}
            <div class="entry-actions">
              <button class="btn-entry btn-unstage" disabled={busyActions.has(`u:${tagEntry.namespace}:${tagEntry.name}`)} onclick={() => runRowAction(`u:${tagEntry.namespace}:${tagEntry.name}`, () => onunstage(tagEntry.name, tagEntry.namespace))}>Unstage</button>
            </div>
          </div>
          <div class="entry-tags">
            {#if hasTags}
            {#if tagEntry.addTags && Object.keys(tagEntry.addTags).length > 0}
              <div class="tag-changes tag-add">
                <span class="tag-label">+ Tags:</span>
                {#each Object.entries(tagEntry.addTags) as [key, value]}
                  <button class="tag-item tag-item-editable" type="button" onclick={() => onedittag(tagEntry.name, tagEntry.namespace, key, value)}>
                    {key}={value}
                    <span class="tag-delete-btn" role="button" tabindex="0" onclick={(e: MouseEvent) => { e.stopPropagation(); runRowAction(`rt:${tagEntry.namespace}:${tagEntry.name}:${key}`, () => onremovetag(tagEntry.name, tagEntry.namespace, key)); }} onkeydown={(e: KeyboardEvent) => { e.stopPropagation(); if (e.key === 'Enter') runRowAction(`rt:${tagEntry.namespace}:${tagEntry.name}:${key}`, () => onremovetag(tagEntry.name, tagEntry.namespace, key)); }}>×</span>
                  </button>
                {/each}
              </div>
            {/if}
            {#if tagEntry.removeTags && Object.keys(tagEntry.removeTags).length > 0}
              <div class="tag-changes tag-remove">
                <span class="tag-label">- Tags:</span>
                {#each Object.entries(tagEntry.removeTags) as [key, value]}
                  <span class="tag-item">
                    {value ? `${key}=${value}` : key}
                    <button class="tag-cancel-btn" disabled={busyActions.has(`cu:${tagEntry.namespace}:${tagEntry.name}:${key}`)} onclick={() => runRowAction(`cu:${tagEntry.namespace}:${tagEntry.name}:${key}`, () => oncanceluntag(tagEntry.name, tagEntry.namespace, key))} title="Cancel untag">↩</button>
                  </span>
                {/each}
              </div>
            {/if}
            <button class="btn-add-tag" onclick={() => onaddtag(tagEntry.name, tagEntry.namespace)}>+ Add Tag</button>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .section {
    background: #1a1a2e;
    border-radius: 8px;
    overflow: hidden;
    /* Keep the section at its natural height inside the flex-column
       .staging-content; without this it shrinks to the viewport and its
       overflow:hidden clips the entries (rows past the fold vanish and nothing
       scrolls). The scroll belongs to .staging-content. */
    flex-shrink: 0;
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

  .btn-entry:hover:not(:disabled) {
    background: #3d3d54;
  }

  .btn-entry:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn-unstage {
    background: #666;
  }

  .btn-unstage:hover:not(:disabled) {
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

  .entry-diff {
    margin-top: 12px;
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

  .entry-value-delete {
    color: #ef9a9a;
  }

  /* Tag display styles */
  .entry-tags {
    margin-top: 8px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .tag-changes {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    font-size: 12px;
    padding: 4px 8px;
    border-radius: 4px;
  }

  .tag-add {
    background: rgba(76, 175, 80, 0.1);
    border: 1px solid rgba(76, 175, 80, 0.3);
  }

  .tag-remove {
    background: rgba(244, 67, 54, 0.1);
    border: 1px solid rgba(244, 67, 54, 0.3);
  }

  .tag-label {
    font-weight: bold;
  }

  .tag-add .tag-label {
    color: #4caf50;
  }

  .tag-remove .tag-label {
    color: #f44336;
  }

  .tag-item {
    font-family: monospace;
    padding: 2px 6px;
    border-radius: 3px;
    background: rgba(255, 255, 255, 0.1);
    color: #ddd;
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .tag-item-editable {
    cursor: pointer;
    transition: background 0.2s;
  }

  .tag-item-editable:hover {
    background: rgba(255, 255, 255, 0.2);
  }

  .tag-delete-btn,
  .tag-cancel-btn {
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
    padding: 0 2px;
    margin-left: 2px;
  }

  .tag-delete-btn:hover {
    color: #f44336;
  }

  .tag-cancel-btn:hover:not(:disabled) {
    color: #4caf50;
  }

  .tag-cancel-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn-add-tag {
    padding: 2px 8px;
    font-size: 11px;
    border: 1px dashed #4caf50;
    border-radius: 3px;
    background: transparent;
    color: #4caf50;
    cursor: pointer;
    margin-top: 4px;
    transition: all 0.2s;
  }

  .btn-add-tag:hover {
    background: rgba(76, 175, 80, 0.1);
    border-style: solid;
  }

  /* Per-entry namespace badge (Azure App Configuration). */
  .namespace-badge {
    display: inline-block;
    margin-right: 8px;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 11px;
    background: rgba(120, 120, 120, 0.18);
    color: #888;
    white-space: nowrap;
  }
</style>
