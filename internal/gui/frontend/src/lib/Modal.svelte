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
    // Only handle Escape when modal is shown
    if (show && e.key === 'Escape') {
      handleClose();
    }
  }

</script>

<svelte:window onkeydown={handleKeydown} />

{#if show}
  <div class="modal-backdrop">
    <button type="button" class="modal-backdrop-dismiss" aria-label="Dismiss" onclick={handleClose}></button>
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

  /* Full-cover dismiss target behind the modal: clicking outside the modal
     closes it. A <button> (not a <div onclick>) keeps it keyboard-focusable and
     satisfies a11y; Escape also closes via the window handler. */
  .modal-backdrop-dismiss {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    background: transparent;
    border: none;
    padding: 0;
    margin: 0;
    cursor: default;
  }

  .modal {
    position: relative;
    z-index: 1;
    background: #1a1a2e;
    border-radius: 8px;
    /* min(400px, 90vw) so the modal never overflows a 375px viewport. */
    min-width: min(400px, 90vw);
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
