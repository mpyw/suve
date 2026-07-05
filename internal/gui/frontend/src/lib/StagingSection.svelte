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
    onapply: () => void;
    onreset: () => void;
    onedit: (entry: gui.StagingDiffEntry) => void;
    onunstage: (name: string) => void;
    onaddtag: (entryName: string) => void;
    onedittag: (entryName: string, key: string, value: string) => void;
    onremovetag: (entryName: string, key: string) => void;
    oncanceluntag: (entryName: string, key: string) => void;
  }

  let {
    icon,
    title,
    emptyText,
    entries,
    tagEntries,
    viewMode,
    onapply,
    onreset,
    onedit,
    onunstage,
    onaddtag,
    onedittag,
    onremovetag,
    oncanceluntag,
  }: Props = $props();

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
</script>

<div class="section">
  <div class="section-header">
    <h3 class="section-title">
      <span class="section-icon">{icon}</span>
      {title}
    </h3>
    <span class="count-badge">{entries.length}</span>
    <div class="section-actions">
      {#if entries.length > 0}
        <button class="btn-section btn-apply-sm" onclick={onapply}>Apply</button>
        <button class="btn-section btn-reset-sm" onclick={onreset}>Reset</button>
      {/if}
    </div>
  </div>

  {#if entries.length === 0}
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
            <span class="entry-name">{entry.name}</span>
            <div class="entry-actions">
              {#if showEditButton(entry)}
                <button class="btn-entry" onclick={() => onedit(entry)}>Edit</button>
              {/if}
              <button class="btn-entry btn-unstage" onclick={() => onunstage(entry.name)}>Unstage</button>
            </div>
          </div>
          <div class="entry-tags">
            {#if tagEntry?.addTags && Object.keys(tagEntry.addTags).length > 0}
              <div class="tag-changes tag-add">
                <span class="tag-label">+ Tags:</span>
                {#each Object.entries(tagEntry.addTags) as [key, value]}
                  <button class="tag-item tag-item-editable" type="button" onclick={() => onedittag(entry.name, key, value)}>
                    {key}={value}
                    <span class="tag-delete-btn" role="button" tabindex="0" onclick={(e: MouseEvent) => { e.stopPropagation(); onremovetag(entry.name, key); }} onkeydown={(e: KeyboardEvent) => { e.stopPropagation(); if (e.key === 'Enter') onremovetag(entry.name, key); }}>×</span>
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
                    <button class="tag-cancel-btn" onclick={() => oncanceluntag(entry.name, key)} title="Cancel untag">↩</button>
                  </span>
                {/each}
              </div>
            {/if}
            {#if entry.operation !== 'delete'}
              <button class="btn-add-tag" onclick={() => onaddtag(entry.name)}>+ Add Tag</button>
            {/if}
          </div>
          {#if entry.operation === 'delete'}
            {#if viewMode === 'diff'}
              <div class="entry-diff">
                <DiffDisplay
                  oldValue={entry.awsValue || ''}
                  newValue="(deleted)"
                  oldLabel="AWS"
                  newLabel="Staged"
                  oldSubLabel={entry.awsIdentifier || ''}
                />
              </div>
            {:else}
              <pre class="entry-value entry-value-delete">(will be deleted)</pre>
            {/if}
          {:else if entry.stagedValue !== undefined && entry.stagedValue !== ''}
            {#if viewMode === 'diff' && entry.operation !== 'create'}
              <div class="entry-diff">
                <DiffDisplay
                  oldValue={entry.awsValue || ''}
                  newValue={entry.stagedValue}
                  oldLabel="AWS"
                  newLabel="Staged"
                  oldSubLabel={entry.awsIdentifier || ''}
                />
              </div>
            {:else}
              <pre class="entry-value">{entry.stagedValue}</pre>
            {/if}
          {/if}
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

  .tag-cancel-btn:hover {
    color: #4caf50;
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
</style>
