<script lang="ts">
  import Sidebar from './lib/Sidebar.svelte';
  import ParamView from './lib/ParamView.svelte';
  import SecretView from './lib/SecretView.svelte';
  import StagingView from './lib/StagingView.svelte';

  let activeView: 'param' | 'secret' | 'staging' = 'param';

  function handleNavigate(event: CustomEvent<'param' | 'secret' | 'staging'>) {
    activeView = event.detail;
  }
</script>

<div class="app">
  <Sidebar {activeView} on:navigate={handleNavigate} />

  <main class="main-content">
    {#if activeView === 'param'}
      <ParamView />
    {:else if activeView === 'secret'}
      <SecretView />
    {:else if activeView === 'staging'}
      <StagingView />
    {/if}
  </main>
</div>

<style>
  .app {
    display: flex;
    height: 100vh;
    width: 100vw;
    overflow: hidden;
  }

  .main-content {
    flex: 1;
    overflow: hidden;
  }
</style>
