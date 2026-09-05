<script lang="ts">
  import { onMount } from 'svelte';
  import { phpRuntime, phpRuntimeApplies, loadPHPRuntime, type PHPRuntime } from '$stores/phpRuntime';
  import { workerModeApplies, loadWorkerMode } from '$stores/workerMode';
  import PHPRuntimeDetail from './PHPRuntimeDetail.svelte';
  import WorkerModeDetail from './WorkerModeDetail.svelte';

  onMount(() => {
    loadPHPRuntime();
    loadWorkerMode();
  });

  // Follows the pending selection rather than the saved runtime, so choosing
  // container reveals the worker choice immediately and both can be set in one
  // pass. How workers are launched is a container-only decision: under native
  // every PHP process is a host process, so there is no exec versus
  // per-worker-container choice left to make.
  let selected = $state<PHPRuntime>('container');
  const showWorkerMode = $derived($workerModeApplies && selected !== 'native');
</script>

<div class="flex-1 overflow-y-auto">
  {#if $phpRuntimeApplies}
    <PHPRuntimeDetail onselect={(m) => (selected = m)} />
  {/if}
  {#if showWorkerMode}
    <div class="border-t-8 border-gray-100 dark:border-lerd-bg">
      <WorkerModeDetail />
    </div>
  {/if}
</div>
