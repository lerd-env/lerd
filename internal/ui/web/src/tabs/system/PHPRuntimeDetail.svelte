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
  import ConfirmModal from '$components/ConfirmModal.svelte';
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
  // Only offered when moving to native: on the container runtime those images
  // are what serves every site.
  let removeImages = $state(false);
  let imagesNote = $state('');
  // A switch can spend minutes rebuilding images that were removed, so what it
  // is doing is shown rather than left behind a spinner.
  let progress = $state<string[]>([]);

  function pick(mode: PHPRuntime) {
    if ($phpRuntimeLoading) return;
    draft = mode;
    onselect?.(mode);
  }

  async function apply() {
    applyError = '';
    imagesNote = '';
    progress = [];
    const res = await setPHPRuntime(draft, removeImages && draft === 'native', (line) => {
      // Bounded: an image build is chatty and only the tail is worth showing.
      progress = [...progress, line].slice(-8);
    });
    if (!res.ok) {
      applyError = res.error || '';
      return;
    }
    // The runtime has moved either way, so a reclaim that could not finish is
    // reported without making the switch look failed.
    if (res.imagesError) {
      imagesNote = res.imagesError;
    } else if (res.imagesRemoved) {
      imagesNote = m.system_phpRuntime_imagesRemoved({ count: String(res.imagesRemoved) });
    }
    confirmOpen = false;
    removeImages = false;
    progress = [];
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

  <ConfirmModal
    open={confirmOpen}
    title={m.system_phpRuntime_title()}
    body={m.system_phpRuntime_confirm()}
    confirmLabel={$phpRuntimeLoading ? m.system_phpRuntime_applying() : m.system_phpRuntime_apply()}
    loading={$phpRuntimeLoading}
    onconfirm={apply}
    onclose={() => (confirmOpen = false)}
  >
    {#snippet extra()}
      {#if draft === 'native'}
        <label class="flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            bind:checked={removeImages}
            disabled={$phpRuntimeLoading}
            class="mt-0.5 rounded border-gray-300 dark:border-lerd-border text-lerd-red focus:ring-lerd-red"
          />
          <span class="text-sm">
            <span class="text-gray-800 dark:text-gray-200">{m.system_phpRuntime_removeImages()}</span>
            <span class="block text-xs text-gray-500 dark:text-gray-400">
              {m.system_phpRuntime_removeImagesHint()}
            </span>
          </span>
        </label>
      {/if}
      {#if progress.length > 0}
        <pre class="mt-3 max-h-40 overflow-y-auto rounded-lg bg-gray-50 dark:bg-white/5 p-2 text-[11px] leading-relaxed text-gray-600 dark:text-gray-400 whitespace-pre-wrap">{progress.join('\n')}</pre>
      {/if}
      {#if applyError}
        <p class="mt-2 text-sm text-red-600 dark:text-red-400">{applyError}</p>
      {/if}
    {/snippet}
  </ConfirmModal>
{/if}
