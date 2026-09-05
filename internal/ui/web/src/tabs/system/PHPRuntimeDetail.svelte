<script lang="ts">
  import { onMount } from 'svelte';
  import {
    phpRuntime,
    phpRuntimeApplies,
    phpRuntimeLoading,
    loadPHPRuntime,
    setPHPRuntime,
    type PHPRuntime
  } from '$stores/phpRuntime';
  import ModeOptionCard from '$components/ModeOptionCard.svelte';
  import Modal from '$components/Modal.svelte';
  import { m } from '../../paraglide/messages.js';

  // The page around this pane shows the worker-runtime choice only when the
  // pending selection is container, so the selection has to leave this
  // component before it is applied, not after.
  interface Props {
    onselect?: (mode: PHPRuntime) => void;
  }
  let { onselect }: Props = $props();

  onMount(loadPHPRuntime);

  let draft = $state<PHPRuntime>($phpRuntime);
  $effect(() => {
    draft = $phpRuntime;
    onselect?.($phpRuntime);
  });

  const dirty = $derived(draft !== $phpRuntime);
  let confirmOpen = $state(false);
  let applyError = $state('');

  function pick(mode: PHPRuntime) {
    if ($phpRuntimeLoading) return;
    draft = mode;
    onselect?.(mode);
  }

  async function apply() {
    applyError = '';
    const res = await setPHPRuntime(draft);
    if (!res.ok) {
      applyError = res.error || '';
      return;
    }
    confirmOpen = false;
  }
</script>

{#if $phpRuntimeApplies}
  <div>
    <div class="flex flex-wrap items-center justify-between gap-y-2 p-3 border-b border-gray-100 dark:border-lerd-border">
      <span class="font-semibold text-gray-900 dark:text-white text-base">{m.system_phpRuntime_title()}</span>
      <span
        class="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full {$phpRuntime === 'native'
          ? 'bg-emerald-100 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-500'
          : 'bg-sky-100 dark:bg-sky-500/10 text-sky-700 dark:text-sky-400'}"
      >
        <span class="w-1.5 h-1.5 rounded-full {$phpRuntime === 'native' ? 'bg-emerald-500' : 'bg-sky-500'}"></span>
        {$phpRuntime === 'native' ? m.system_phpRuntime_nativeBadge() : m.system_phpRuntime_containerBadge()}
      </span>
    </div>

    <div class="p-3 space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">{m.system_phpRuntime_description()}</p>

      <ModeOptionCard
        selected={draft === 'container'}
        disabled={$phpRuntimeLoading}
        accent="sky"
        title={m.system_phpRuntime_container_title()}
        description={m.system_phpRuntime_container_description()}
        onclick={() => pick('container')}
      />

      <ModeOptionCard
        selected={draft === 'native'}
        disabled={$phpRuntimeLoading}
        accent="emerald"
        title={m.system_phpRuntime_native_title()}
        description={m.system_phpRuntime_native_description()}
        onclick={() => pick('native')}
      />

      <div class="flex items-center gap-2">
        <button
          type="button"
          class="px-3 py-1.5 rounded-lg text-sm font-medium bg-lerd-red hover:bg-lerd-redhov text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          disabled={!dirty || $phpRuntimeLoading}
          onclick={() => {
            applyError = '';
            confirmOpen = true;
          }}
        >
          {$phpRuntimeLoading ? m.system_phpRuntime_applying() : m.system_phpRuntime_apply()}
        </button>
      </div>
    </div>
  </div>

  <Modal open={confirmOpen} title={m.system_phpRuntime_title()} onclose={() => !$phpRuntimeLoading && (confirmOpen = false)}>
    <div class="p-4 space-y-3">
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.system_phpRuntime_confirm()}</p>
      {#if applyError}
        <p class="text-sm text-red-600 dark:text-red-400">{applyError}</p>
      {/if}
      <div class="flex justify-end gap-2">
        <button
          type="button"
          class="px-3 py-1.5 rounded-lg text-sm font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-300 disabled:opacity-50 transition-colors"
          disabled={$phpRuntimeLoading}
          onclick={() => (confirmOpen = false)}
        >
          {m.common_cancel()}
        </button>
        <button
          type="button"
          class="px-3 py-1.5 rounded-lg text-sm font-medium bg-lerd-red hover:bg-lerd-redhov text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          disabled={$phpRuntimeLoading}
          onclick={apply}
        >
          {$phpRuntimeLoading ? m.system_phpRuntime_applying() : m.system_phpRuntime_apply()}
        </button>
      </div>
    </div>
  </Modal>
{/if}
