<script lang="ts">
  import { status, checkToolUpdates } from '$stores/status';
  import ToolCard from '$components/ToolCard.svelte';
  import CheckUpdatesButton from '$components/CheckUpdatesButton.svelte';
  import { notifyLocalInfo } from '$lib/notify';
  import { m } from '../../paraglide/messages.js';

  const tools = $derived($status.tools ?? []);

  let checking = $state(false);

  // The pins come from one manifest cached for a day, so this is a single
  // check for every card and the only way out of that window. The result goes
  // to the toast surface; the cards themselves follow from the next status.
  async function runCheck() {
    checking = true;
    try {
      const res = await checkToolUpdates();
      const found = res.tools?.some((t) => t.update_available);
      notifyLocalInfo(
        'update_check',
        m.system_tools_title(),
        !res.ok
          ? m.system_tools_checkFailed()
          : found
            ? m.system_tools_checkFound()
            : m.system_tools_checkUpToDate()
      );
    } finally {
      checking = false;
    }
  }
</script>

<div class="flex-1 overflow-y-auto">
  <div class="flex flex-wrap items-center justify-between gap-y-2 p-3 border-b border-gray-100 dark:border-lerd-border">
    <span class="font-semibold text-gray-900 dark:text-white text-base">{m.system_tools_title()}</span>
    <CheckUpdatesButton onclick={runCheck} {checking} title={m.system_tools_checkTitle()} />
  </div>

  <div class="p-3 space-y-3">
    <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_tools_description()}</p>

    <div class="flex flex-wrap gap-3">
      {#each tools as tool (tool.name)}
        <ToolCard {tool} />
      {/each}
    </div>
  </div>
</div>
