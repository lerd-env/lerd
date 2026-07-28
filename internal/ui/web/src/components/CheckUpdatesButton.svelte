<script lang="ts">
  import { m } from '../paraglide/messages.js';

  // size follows the surface: the dashboard widget sits in a tighter row than
  // the settings cards do.
  let {
    onclick,
    checking = false,
    title = undefined,
    size = 'md'
  }: {
    onclick: () => void;
    checking?: boolean;
    title?: string;
    size?: 'sm' | 'md';
  } = $props();

  const sizing = $derived(
    size === 'sm' ? 'px-2.5 py-1 rounded-md text-xs' : 'px-3 py-1.5 rounded-lg text-sm'
  );
</script>

<button
  type="button"
  {onclick}
  {title}
  disabled={checking}
  class="shrink-0 inline-flex items-center gap-1.5 font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-300 disabled:opacity-40 transition-colors {sizing}"
>
  {#if checking}
    <svg class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
    </svg>
    {m.system_lerd_checking()}
  {:else}
    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h5M20 20v-5h-5M20.49 9A9 9 0 005.64 5.64L4 4m16 16l-1.64-1.64A9 9 0 014.51 15"/>
    </svg>
    {m.system_lerd_checkForUpdates()}
  {/if}
</button>
