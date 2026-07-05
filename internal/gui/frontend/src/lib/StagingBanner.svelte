<script lang="ts">
  interface Props {
    stagingStatus: { hasEntry: boolean; hasTags: boolean } | null;
    onnavigate?: () => void;
  }

  let { stagingStatus, onnavigate }: Props = $props();

  function getStagingMessage(): string | null {
    if (!stagingStatus || (!stagingStatus.hasEntry && !stagingStatus.hasTags)) {
      return null;
    }
    if (stagingStatus.hasEntry && stagingStatus.hasTags) {
      return 'This item has staged value and tag changes';
    }
    if (stagingStatus.hasEntry) {
      return 'This item has staged value changes';
    }
    return 'This item has staged tag changes';
  }
</script>

{#if getStagingMessage()}
  <!-- Using div instead of button to avoid conflicts with Playwright button selectors -->
  <div class="staging-banner" role="link" tabindex="0" onclick={onnavigate} onkeydown={(e) => e.key === 'Enter' && onnavigate?.()}>
    <span class="staging-icon">⚠</span>
    <span class="staging-text">{getStagingMessage()}</span>
    <span class="staging-link">View in Staging →</span>
  </div>
{/if}

<style>
  .staging-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 10px 12px;
    margin: 8px 0 0 0;
    background: rgba(255, 193, 7, 0.15);
    border: 1px solid rgba(255, 193, 7, 0.4);
    border-radius: 6px;
    cursor: pointer;
    transition: background-color 0.2s, border-color 0.2s;
    text-align: left;
  }

  .staging-banner:hover {
    background: rgba(255, 193, 7, 0.25);
    border-color: rgba(255, 193, 7, 0.6);
  }

  .staging-icon {
    color: #ffc107;
    font-size: 14px;
    flex-shrink: 0;
  }

  .staging-text {
    color: #e0e0e0;
    font-size: 13px;
    flex: 1;
  }

  .staging-link {
    color: #ffc107;
    font-size: 12px;
    font-weight: 500;
    flex-shrink: 0;
  }
</style>
