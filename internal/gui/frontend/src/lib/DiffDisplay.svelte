<script lang="ts">
  import { computeInlineDiff } from './diff-utils';

  interface Props {
    oldValue: string;
    newValue: string;
    oldLabel?: string;
    newLabel?: string;
    oldSubLabel?: string;
    newSubLabel?: string;
  }

  let { oldValue, newValue, oldLabel = 'Old', newLabel = 'New', oldSubLabel = '', newSubLabel = '' }: Props = $props();

  let diff = $derived(computeInlineDiff(oldValue, newValue));
</script>

<div class="diff-container">
  <div class="diff-side">
    <div class="diff-side-header">
      <span class="diff-label">{oldLabel}</span>
      {#if oldSubLabel}
        <span class="diff-sublabel" title={oldSubLabel}>{oldSubLabel.length > 16 ? oldSubLabel.substring(0, 16) + '...' : oldSubLabel}</span>
      {/if}
    </div>
    <pre class="diff-value diff-old">{#each diff.oldSegments as seg}<span class={seg.type}>{seg.text}</span>{/each}</pre>
  </div>
  <div class="diff-side">
    <div class="diff-side-header">
      <span class="diff-label">{newLabel}</span>
      {#if newSubLabel}
        <span class="diff-sublabel" title={newSubLabel}>{newSubLabel.length > 16 ? newSubLabel.substring(0, 16) + '...' : newSubLabel}</span>
      {/if}
    </div>
    <pre class="diff-value diff-new">{#each diff.newSegments as seg}<span class={seg.type}>{seg.text}</span>{/each}</pre>
  </div>
</div>

<style>
  .diff-container {
    display: flex;
    gap: 16px;
    margin-bottom: 16px;
  }

  .diff-side {
    flex: 1;
    min-width: 0;
  }

  .diff-side-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }

  .diff-label {
    font-size: 12px;
    font-weight: bold;
    color: #888;
  }

  .diff-sublabel {
    font-family: monospace;
    font-size: 10px;
    color: #666;
  }

  .diff-value {
    margin: 0;
    padding: 12px;
    border-radius: 4px;
    font-family: monospace;
    font-size: 12px;
    white-space: pre-wrap;
    word-break: break-all;
    min-height: 100px;
    max-height: 300px;
    overflow-y: auto;
  }

  .diff-old {
    background: rgba(244, 67, 54, 0.1);
    border: 1px solid rgba(244, 67, 54, 0.3);
    color: #ef9a9a;
  }

  .diff-new {
    background: rgba(76, 175, 80, 0.1);
    border: 1px solid rgba(76, 175, 80, 0.3);
    color: #a5d6a7;
  }

  /* Inline highlighting */
  .diff-old .removed {
    background: rgba(244, 67, 54, 0.4);
    color: #fff;
    border-radius: 2px;
  }

  .diff-new .added {
    background: rgba(76, 175, 80, 0.4);
    color: #fff;
    border-radius: 2px;
  }

  .unchanged {
    opacity: 1; /* Default styling, no special highlight */
  }
</style>
