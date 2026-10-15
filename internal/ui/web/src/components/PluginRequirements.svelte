<script lang="ts">
  import { m } from '../paraglide/messages.js';

  interface Props {
    requires: string[];
    // The ones this machine lacks, drawn in red.
    missing?: string[];
  }

  let { requires, missing = [] }: Props = $props();
</script>

<div class="flex flex-wrap items-center gap-1 mt-1">
  <span class="text-[11px] text-gray-400">{m.system_plugins_requires()}</span>
  {#each requires as req (req)}
    {#if missing.includes(req)}
      <span class="text-[10px] font-mono px-1.5 py-0.5 rounded-sm bg-red-50 dark:bg-red-500/10 text-red-600 dark:text-red-400" title={m.system_plugins_notInstalled()}>{req}</span>
    {:else}
      <span class="text-[10px] font-mono px-1.5 py-0.5 rounded-sm bg-gray-100 dark:bg-white/5 text-gray-500 dark:text-gray-400">{req}</span>
    {/if}
  {/each}
</div>
