<script lang="ts">
  import type { Snippet } from 'svelte';
  import CloseIcon from './icons/CloseIcon.svelte';
  import './common.css';

  interface Props {
    title: string;
    show?: boolean;
    onclose?: () => void;
    children?: Snippet;
  }

  let { title, show = false, onclose, children }: Props = $props();

  function handleClose() {
    onclose?.();
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      handleClose();
    }
  }

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      handleClose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if show}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="modal-backdrop" onclick={handleBackdropClick}>
    <div class="modal">
      <div class="modal-header">
        <h3 class="modal-title">{title}</h3>
        <button class="btn-close" onclick={handleClose}>
          <CloseIcon />
        </button>
      </div>
      <div class="modal-content">
        {@render children?.()}
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal {
    background: #1a1a2e;
    border-radius: 8px;
    min-width: 400px;
    max-width: 600px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.5);
  }

  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px;
    border-bottom: 1px solid #2d2d44;
  }

  .modal-title {
    margin: 0;
    font-size: 16px;
    color: #fff;
  }

  .modal-content {
    padding: 16px;
    overflow-y: auto;
  }
</style>
