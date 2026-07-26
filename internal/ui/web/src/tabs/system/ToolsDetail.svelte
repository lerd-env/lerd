<script lang="ts">
  import { status } from '$stores/status';
  import ToolCard from '$components/ToolCard.svelte';
  import { m } from '../../paraglide/messages.js';

  const tools = $derived($status.tools ?? []);
  const anyUpdate = $derived(tools.some((t) => t.update_available));
</script>

<div class="flex-1 overflow-y-auto">
  <div class="flex flex-wrap items-center justify-between gap-y-2 p-3 border-b border-gray-100 dark:border-lerd-border">
    <span class="font-semibold text-gray-900 dark:text-white text-base">{m.system_tools_title()}</span>
  </div>

  <div class="p-3 space-y-3">
    <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_tools_description()}</p>

    <div class="flex flex-wrap gap-3">
      {#each tools as tool (tool.name)}
        <ToolCard {tool} />
      {/each}
    </div>

    {#if anyUpdate}
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {@html m.system_tools_updateHint({ cmd: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">lerd tools:update</code>' })}
      </p>
    {/if}
  </div>
</div>
