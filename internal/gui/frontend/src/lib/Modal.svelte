<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import CloseIcon from './icons/CloseIcon.svelte';
  import './common.css';

  export let title: string;
  export let show = false;

  const dispatch = createEventDispatcher<{ close: void }>();

  function handleClose() {
    dispatch('close');
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

<svelte:window on:keydown={handleKeydown} />

{#if show}
  <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
  <div class="modal-backdrop" on:click={handleBackdropClick}>
    <div class="modal">
      <div class="modal-header">
        <h3 class="modal-title">{title}</h3>
        <button class="btn-close" on:click={handleClose}>
          <CloseIcon />
        </button>
      </div>
      <div class="modal-content">
        <slot />
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
