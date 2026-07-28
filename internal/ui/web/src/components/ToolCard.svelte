<script lang="ts">
  import StatusPill from '$components/StatusPill.svelte';
  import { updateTool, type ToolStatus } from '$stores/status';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  let { tool }: { tool: ToolStatus } = $props();

  // The big number is what's on disk; a missing tool shows the pin dimmed as
  // the version an install would bring, an unknown one an explicit question.
  const display = $derived(!tool.present ? tool.pinned : tool.installed || '?');

  let busy = $state(false);
  let error = $state('');

  // The pending line is the affordance, so it becomes the action rather than the
  // card growing a control of its own. The result stays on this card: a refused
  // checksum names the tool it refused.
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
    {#if !tool.present}
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
    <button
      type="button"
      onclick={apply}
      disabled={busy}
      use:tooltip={m.system_tools_updateTooltip({ name: tool.name, version: tool.pinned })}
      class="inline-flex items-center gap-1 text-xs font-medium text-yellow-700 dark:text-yellow-400 hover:underline disabled:no-underline disabled:opacity-70"
    >
      {#if busy}
        <svg class="w-3.5 h-3.5 shrink-0 animate-spin" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"/>
        </svg>
        {m.system_tools_updating()}
      {:else}
        <svg class="w-3.5 h-3.5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"/>
        </svg>
        {m.system_tools_updateAvailable({ version: tool.pinned })}
      {/if}
    </button>
  {/if}
  {#if error}
    <p class="text-xs text-red-500 break-words" use:tooltip={error}>{error}</p>
  {/if}
</div>
