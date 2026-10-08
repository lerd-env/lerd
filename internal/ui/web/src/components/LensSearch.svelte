<script lang="ts">
  import { m } from '../paraglide/messages.js';

  // The search box every Debug lens carries, with the × that clears it.
  let { value = $bindable(''), placeholder }: { value?: string; placeholder: string } = $props();
</script>

<div class="relative flex-1 min-w-[140px]">
  <input
    class="w-full text-xs pl-2 pr-6 py-1 rounded-sm border border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card"
    {placeholder}
    bind:value
  />
  {#if value}
    <button
      type="button"
      onclick={() => (value = '')}
      title={m.queries_clearFilter()}
      aria-label={m.queries_clearFilter()}
      class="absolute right-1 top-1/2 -translate-y-1/2 w-4 h-4 flex items-center justify-center rounded-sm text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-white/10"
    >
      <svg class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" viewBox="0 0 24 24"><path d="M6 6l12 12M18 6L6 18" /></svg>
    </button>
  {/if}
</div>
