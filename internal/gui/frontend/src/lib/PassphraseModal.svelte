<script lang="ts">
  import EyeIcon from './icons/EyeIcon.svelte';
  import EyeOffIcon from './icons/EyeOffIcon.svelte';
  import Modal from './Modal.svelte';
  import './common.css';

  interface Props {
    show?: boolean;
    mode: 'encrypt' | 'decrypt';
    title?: string;
    onsubmit?: (passphrase: string) => void;
    oncancel?: () => void;
    loading?: boolean;
    error?: string;
  }

  let { show = false, mode, title, onsubmit, oncancel, loading = false, error = '' }: Props = $props();

  let passphrase = $state('');
  let confirmPassphrase = $state('');
  let showPassword = $state(false);
  let showConfirmPassword = $state(false);
  let localError = $state('');

  // Plaintext warning confirmation
  let showPlaintextWarning = $state(false);

  // Reset state when modal opens/closes
  $effect(() => {
    if (show) {
      passphrase = '';
      confirmPassphrase = '';
      showPassword = false;
      showConfirmPassword = false;
      localError = '';
      showPlaintextWarning = false;
    }
  });

  const modalTitle = $derived(title || (mode === 'encrypt' ? 'Enter Passphrase for Encryption' : 'Enter Passphrase for Decryption'));

  function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    localError = '';

    if (mode === 'encrypt') {
      // For encrypt mode
      if (passphrase === '') {
        // Empty passphrase - show warning
        showPlaintextWarning = true;
        return;
      }
      // Validate confirmation
      if (passphrase !== confirmPassphrase) {
        localError = 'Passphrases do not match';
        return;
      }
    }

    onsubmit?.(passphrase);
  }

  function handleConfirmPlaintext() {
    showPlaintextWarning = false;
    onsubmit?.('');
  }

  function handleCancelPlaintext() {
    showPlaintextWarning = false;
  }

  function handleClose() {
    showPlaintextWarning = false;
    oncancel?.();
  }
</script>

<Modal title={modalTitle} show={show} onclose={handleClose}>
  {#if showPlaintextWarning}
    <div class="plaintext-warning">
      <div class="warning-icon">!</div>
      <h4>Store without encryption?</h4>
      <p class="warning-text">Secrets will be stored as plain text on disk. This is not recommended for sensitive data.</p>
      <div class="form-actions">
        <button type="button" class="btn-secondary" onclick={handleCancelPlaintext}>Cancel</button>
        <button type="button" class="btn-warning" onclick={handleConfirmPlaintext}>
          Continue without encryption
        </button>
      </div>
    </div>
  {:else}
    <form class="passphrase-form" onsubmit={handleSubmit}>
      {#if error || localError}
        <div class="modal-error">{error || localError}</div>
      {/if}

      <div class="form-group">
        <label for="passphrase">
          {mode === 'encrypt' ? 'Passphrase (empty for plain text)' : 'Passphrase'}
        </label>
        <div class="password-input-wrapper">
          <input
            id="passphrase"
            type={showPassword ? 'text' : 'password'}
            class="form-input"
            bind:value={passphrase}
            placeholder={mode === 'encrypt' ? 'Leave empty for plain text' : 'Enter passphrase'}
            disabled={loading}
          />
          <button
            type="button"
            class="password-toggle"
            onclick={() => showPassword = !showPassword}
            tabindex={-1}
          >
            {#if showPassword}
              <EyeOffIcon />
            {:else}
              <EyeIcon />
            {/if}
          </button>
        </div>
      </div>

      {#if mode === 'encrypt' && passphrase !== ''}
        <div class="form-group">
          <label for="confirm-passphrase">Confirm Passphrase</label>
          <div class="password-input-wrapper">
            <input
              id="confirm-passphrase"
              type={showConfirmPassword ? 'text' : 'password'}
              class="form-input"
              bind:value={confirmPassphrase}
              placeholder="Re-enter passphrase"
              disabled={loading}
            />
            <button
              type="button"
              class="password-toggle"
              onclick={() => showConfirmPassword = !showConfirmPassword}
              tabindex={-1}
            >
              {#if showConfirmPassword}
                <EyeOffIcon />
              {:else}
                <EyeIcon />
              {/if}
            </button>
          </div>
        </div>
      {/if}

      <div class="form-actions">
        <button type="button" class="btn-secondary" onclick={handleClose} disabled={loading}>
          Cancel
        </button>
        <button type="submit" class="btn-primary" disabled={loading}>
          {loading ? 'Processing...' : 'Continue'}
        </button>
      </div>
    </form>
  {/if}
</Modal>

<style>
  .passphrase-form {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .form-group label {
    font-size: 12px;
    color: #888;
    text-transform: uppercase;
  }

  .password-input-wrapper {
    position: relative;
    display: flex;
    align-items: center;
  }

  .form-input {
    flex: 1;
    padding: 10px 40px 10px 12px;
    background: #0f0f1a;
    border: 1px solid #2d2d44;
    border-radius: 4px;
    color: #fff;
    font-size: 14px;
  }

  .form-input:focus {
    outline: none;
    border-color: #e94560;
  }

  .form-input:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .password-toggle {
    position: absolute;
    right: 8px;
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    padding: 4px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .password-toggle:hover {
    color: #fff;
  }

  .form-actions {
    display: flex;
    gap: 12px;
    justify-content: flex-end;
    margin-top: 8px;
  }

  .btn-secondary {
    padding: 8px 16px;
    background: #2d2d44;
    color: #fff;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
  }

  .btn-secondary:hover:not(:disabled) {
    background: #3d3d54;
  }

  .btn-secondary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn-primary {
    padding: 8px 16px;
    background: #e94560;
    color: #fff;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
  }

  .btn-primary:hover:not(:disabled) {
    background: #d63050;
  }

  .btn-primary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .modal-error {
    padding: 10px 12px;
    background: rgba(244, 67, 54, 0.2);
    border: 1px solid #f44336;
    border-radius: 4px;
    color: #f44336;
    font-size: 13px;
  }

  /* Plaintext warning styles */
  .plaintext-warning {
    text-align: center;
    padding: 16px 0;
  }

  .warning-icon {
    width: 48px;
    height: 48px;
    border-radius: 50%;
    background: rgba(255, 152, 0, 0.2);
    border: 2px solid #ff9800;
    color: #ff9800;
    font-size: 24px;
    font-weight: bold;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0 auto 16px;
  }

  .plaintext-warning h4 {
    margin: 0 0 12px;
    color: #fff;
    font-size: 16px;
  }

  .warning-text {
    color: #ff9800;
    font-size: 13px;
    margin: 0 0 20px;
    line-height: 1.5;
  }

  .btn-warning {
    padding: 8px 16px;
    background: #ff9800;
    color: #000;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
    font-weight: 500;
  }

  .btn-warning:hover {
    background: #f57c00;
  }
</style>
