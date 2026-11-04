<script lang="ts">
  import StatusPill from '$components/StatusPill.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { updateTool, type ToolStatus } from '$stores/status';
  import { accessMode } from '$stores/accessMode';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  let { tool }: { tool: ToolStatus } = $props();

  // The big number is what's on disk; a missing tool shows the pin dimmed as
  // the version an install would bring, an unknown one an explicit question.
  const display = $derived(!tool.present ? tool.pinned : tool.installed || '?');

  let busy = $state(false);
  let error = $state('');

  // The result stays on this card: a refused checksum names the tool it refused.
  async function apply() {
    if (busy) return;
    busy = true;
    error = '';
    const res = await updateTool(tool.name);
    busy = false;
    if (!res.ok) error = res.error || m.common_failed();
  }
</script>

<div class="shrink-0 w-[11rem] flex flex-col gap-2.5 rounded-2xl border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card p-3">
  <div class="flex items-center justify-between gap-2">
    <span class="font-mono text-xs font-semibold text-gray-700 dark:text-gray-300">{tool.name}</span>
    {#if tool.external}
      <StatusPill tone="muted" label={m.system_tools_external()} />
    {:else if !tool.present}
      <StatusPill tone="muted" label={m.system_tools_notInstalled()} />
    {:else if tool.update_available}
      <StatusPill tone="warn" label={m.system_lerd_updateTag()} />
    {:else if !tool.installed}
      <StatusPill tone="muted" label={m.system_tools_versionUnknown()} />
    {:else}
      <StatusPill tone="ok" label={m.system_tools_current()} />
    {/if}
  </div>
  <span class="font-mono text-2xl font-semibold tabular-nums tracking-tight leading-none {tool.present && tool.installed ? 'text-gray-900 dark:text-gray-100' : 'text-gray-400 dark:text-gray-600'}">
    {display}
  </span>
  {#if tool.update_available}
    <span class="text-xs font-medium text-yellow-700 dark:text-yellow-400 truncate">{m.system_tools_updateAvailable({ version: tool.pinned })}</span>
    {#if $accessMode.localControl}
      <DetailButton
        tone="success"
        block
        onclick={apply}
        loading={busy}
        disabled={busy}
        title={m.system_tools_updateTooltip({ name: tool.name, version: tool.pinned })}
      >{m.services_update()}</DetailButton>
    {/if}
  {/if}
  {#if tool.external}
    <span class="text-xs text-gray-500 dark:text-gray-400">{m.system_tools_externalHint()}</span>
  {/if}
  {#if error}
    <p class="text-xs text-red-500 break-words" use:tooltip={error}>{error}</p>
  {/if}
</div>
