<script lang="ts">
  interface Props {
    activeView?: 'param' | 'secret' | 'staging';
    stagingCount?: number;
    onnavigate?: (view: 'param' | 'secret' | 'staging') => void;
  }

  let { activeView = 'param', stagingCount = 0, onnavigate }: Props = $props();

  function navigate(view: 'param' | 'secret' | 'staging') {
    onnavigate?.(view);
  }
</script>

<aside class="sidebar">
  <div class="logo">
    <span class="logo-text">suve</span>
    <span class="logo-sub">AWS Secrets Viewer</span>
  </div>

  <nav class="nav">
    <button
      class="nav-item"
      class:active={activeView === 'param'}
      onclick={() => navigate('param')}
    >
      <span class="nav-icon">P</span>
      <span class="nav-label">Parameters</span>
    </button>

    <button
      class="nav-item"
      class:active={activeView === 'secret'}
      onclick={() => navigate('secret')}
    >
      <span class="nav-icon">S</span>
      <span class="nav-label">Secrets</span>
    </button>

    <button
      class="nav-item"
      class:active={activeView === 'staging'}
      onclick={() => navigate('staging')}
    >
      <span class="nav-icon">*</span>
      <span class="nav-label">Staging</span>
      {#if stagingCount > 0}
        <span class="staging-count">{stagingCount}</span>
      {/if}
    </button>
  </nav>
</aside>

<style>
  .sidebar {
    width: 200px;
    height: 100%;
    background: #1a1a2e;
    display: flex;
    flex-direction: column;
    border-right: 1px solid #2d2d44;
  }

  .logo {
    padding: 20px 16px;
    border-bottom: 1px solid #2d2d44;
  }

  .logo-text {
    font-size: 24px;
    font-weight: bold;
    color: #e94560;
    display: block;
  }

  .logo-sub {
    font-size: 10px;
    color: #666;
    display: block;
    margin-top: 2px;
  }

  .nav {
    padding: 12px 8px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 12px;
    border: none;
    background: transparent;
    color: #a0a0a0;
    cursor: pointer;
    border-radius: 6px;
    transition: all 0.2s;
    text-align: left;
    font-size: 14px;
  }

  .nav-item:hover {
    background: #252542;
    color: #fff;
  }

  .nav-item.active {
    background: #e94560;
    color: #fff;
  }

  .nav-icon {
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

  .nav-label {
    flex: 1;
  }

  .staging-count {
    min-width: 18px;
    height: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 10px;
    font-weight: bold;
    background: #e94560;
    color: #fff;
    border-radius: 50%;
    padding: 0 4px;
  }

  .nav-item.active .staging-count {
    background: #fff;
    color: #e94560;
  }
</style>
