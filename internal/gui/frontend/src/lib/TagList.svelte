<script lang="ts">
  import './common.css';

  interface Props {
    tags: Array<{ key: string; value: string }> | undefined;
    serviceClass: 'param' | 'secret';
    provider?: string;
    onadd: () => void;
    onremove: (key: string) => void;
  }

  let { tags, serviceClass, provider = '', onadd, onremove }: Props = $props();

  // suve uses one metadata vocabulary ("Tags") across every provider. Google
  // Cloud natively calls this same key=value metadata "labels", so surface a
  // secondary, non-primary hint for GCloud users without renaming the field.
  const nativeHint = $derived(provider === 'googlecloud' ? 'Google Cloud: labels' : '');
</script>

<div class="detail-section">
  <div class="section-header">
    <div class="section-heading">
      <h4>Tags</h4>
      {#if nativeHint}
        <span class="tag-native-hint" title={nativeHint}>{nativeHint}</span>
      {/if}
    </div>
    <button class="btn-action-sm" onclick={onadd}>+ Add</button>
  </div>
  {#if tags && tags.length > 0}
    <div class="tags-list">
      {#each tags as tag}
        <div class="tag-item">
          <span class="tag-key {serviceClass}">{tag.key}</span>
          <span class="tag-separator">=</span>
          <span class="tag-value">{tag.value}</span>
          <button class="btn-tag-remove" onclick={() => onremove(tag.key)} title="Remove tag">×</button>
        </div>
      {/each}
    </div>
  {:else}
    <p class="no-tags">No tags</p>
  {/if}
</div>

<style>
  .section-heading {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }

  .tag-native-hint {
    font-size: 10px;
    color: #888;
    font-weight: normal;
    white-space: nowrap;
  }
</style>
