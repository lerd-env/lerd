<script lang="ts">
  import DetailHeader from '$components/DetailHeader.svelte';
  import { status, checkToolUpdates, type ToolStatus } from '$stores/status';
  import { accessMode } from '$stores/accessMode';
  import ToolCard from '$components/ToolCard.svelte';
  import ToolsUpdateModal from '$components/ToolsUpdateModal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import CheckUpdatesButton from '$components/CheckUpdatesButton.svelte';
  import { notifyLocalInfo } from '$lib/notify';
  import { m } from '../../paraglide/messages.js';

  const tools = $derived($status.tools ?? []);
  const outdated = $derived(tools.filter((t) => t.update_available));
  // What was outdated when the dialog opened, null while it is closed.
  let updateBatch = $state<ToolStatus[] | null>(null);

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
  <DetailHeader title={m.system_tools_title()}>
    {#snippet trailing()}
      <div class="flex items-center gap-2">
        {#if outdated.length > 0 && $accessMode.localControl}
          <DetailButton tone="success" onclick={() => (updateBatch = [...outdated])}>{m.services_updateAll()}</DetailButton>
        {/if}
        <CheckUpdatesButton onclick={runCheck} {checking} title={m.system_tools_checkTitle()} />
      </div>
    {/snippet}
  </DetailHeader>

  <div class="p-3 space-y-3">
    <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_tools_description()}</p>

    <div class="flex flex-wrap gap-3">
      {#each tools as tool (tool.name)}
        <ToolCard {tool} />
      {/each}
    </div>
  </div>
</div>

{#if updateBatch}
  <ToolsUpdateModal tools={updateBatch} onclose={() => (updateBatch = null)} />
{/if}
