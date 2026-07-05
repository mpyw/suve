<script lang="ts">
  import './common.css';

  interface Props {
    tags: Array<{ key: string; value: string }> | undefined;
    serviceClass: 'param' | 'secret';
    onadd: () => void;
    onremove: (key: string) => void;
  }

  let { tags, serviceClass, onadd, onremove }: Props = $props();
</script>

<div class="detail-section">
  <div class="section-header">
    <h4>Tags</h4>
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
