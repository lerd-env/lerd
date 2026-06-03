<script lang="ts">
  import { onMount } from 'svelte';
  import {
    horizonReload,
    horizonReloadLoading,
    loadHorizonReload,
    setHorizonReload
  } from '$stores/horizonReload';
  import Toggle from '$components/Toggle.svelte';
  import { m } from '../../paraglide/messages.js';

  onMount(loadHorizonReload);

  let applyError = $state('');

  async function toggle() {
    if ($horizonReloadLoading) return;
    applyError = '';
    const r = await setHorizonReload(!$horizonReload);
    if (!r.ok) {
      applyError = r.error
        ? m.system_horizonReload_apply_failed() + ' (' + r.error + ')'
        : m.system_horizonReload_apply_failed();
    }
  }
</script>

<div class="flex-1 overflow-y-auto">
  <div class="flex flex-wrap items-center justify-between gap-y-2 p-3 border-b border-gray-100 dark:border-lerd-border">
    <span class="font-semibold text-gray-900 dark:text-white text-base">{m.system_horizonReload_title()}</span>
    <span
      class="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full {$horizonReload
        ? 'bg-emerald-100 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-500'
        : 'bg-gray-100 dark:bg-white/5 text-gray-600 dark:text-gray-400'}"
    >
      <span class="w-1.5 h-1.5 rounded-full {$horizonReload ? 'bg-emerald-500' : 'bg-gray-400'}"></span>
      {$horizonReload ? m.system_horizonReload_badge_on() : m.system_horizonReload_badge_off()}
    </span>
  </div>

  <div class="p-3 space-y-4">
    <p class="text-sm text-gray-600 dark:text-gray-400">{m.system_horizonReload_description()}</p>

    <div class="flex items-center justify-between gap-3 p-3 rounded-sm border border-gray-200 dark:border-lerd-border">
      <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{m.system_horizonReload_toggle_label()}</span>
      <Toggle
        on={$horizonReload}
        tone="emerald"
        loading={$horizonReloadLoading}
        title={$horizonReload ? m.system_horizonReload_badge_on() : m.system_horizonReload_badge_off()}
        onclick={toggle}
      />
    </div>

    <div class="text-xs text-gray-500 dark:text-gray-400 bg-gray-50 dark:bg-lerd-card/50 rounded-sm px-3 py-2 border border-gray-200 dark:border-lerd-border">
      <span class="font-medium text-gray-700 dark:text-gray-300">{m.system_horizonReload_note_label()}</span>
      {m.system_horizonReload_note_body()}
    </div>

    {#if applyError}
      <p class="text-xs text-red-500">{applyError}</p>
    {/if}
  </div>
</div>
