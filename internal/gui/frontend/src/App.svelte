<script lang="ts">
  import { onMount } from 'svelte';
  import Sidebar from './lib/Sidebar.svelte';
  import ParamView from './lib/ParamView.svelte';
  import SecretView from './lib/SecretView.svelte';
  import StagingView from './lib/StagingView.svelte';
  import { StagingStatus } from '../wailsjs/go/gui/App';

  let activeView: 'param' | 'secret' | 'staging' = 'param';
  let stagingCount = 0;

  function handleNavigate(event: CustomEvent<'param' | 'secret' | 'staging'>) {
    activeView = event.detail;
  }

  function handleStagingCountChange(event: CustomEvent<number>) {
    stagingCount = event.detail;
  }

  async function loadStagingCount() {
    try {
      const status = await StagingStatus();
      stagingCount = (status?.param?.length || 0) + (status?.secret?.length || 0);
    } catch (e) {
      stagingCount = 0;
    }
  }

  onMount(() => {
    loadStagingCount();
  });
</script>

<div class="app">
  <Sidebar {activeView} {stagingCount} on:navigate={handleNavigate} />

  <main class="main-content">
    {#if activeView === 'param'}
      <ParamView />
    {:else if activeView === 'secret'}
      <SecretView />
    {:else if activeView === 'staging'}
      <StagingView on:countChange={handleStagingCountChange} />
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
