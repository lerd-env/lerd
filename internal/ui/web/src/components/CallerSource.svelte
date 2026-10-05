<script lang="ts">
  import type { Frame } from '$lib/sourceLabel';
  import { callerClass, callerIndex } from '$lib/sourceLabel';
  import Popover from './Popover.svelte';
  import SourcePath from './SourcePath.svelte';
  import TraceView from './TraceView.svelte';
  import { traceCodePref } from '$lib/traceFrames';
  import { sourceAvailable } from '$lib/sourceCode';
  import Icon from './Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  // The line that triggered an event, named by its class where it has one,
  // with the whole stack it was called through one click away.
  interface Props {
    file: string;
    line?: number;
    trace?: Frame[];
    muted?: boolean;
    nested?: boolean;
  }
  let { file, line, trace = [], muted = true, nested = false }: Props = $props();
  let showCode = $state(traceCodePref());
</script>

{#snippet button(toggle: () => void, open: boolean)}
  <button type="button" aria-label={m.trace_show()} aria-expanded={open} onclick={toggle} use:tooltip={m.trace_show()} class="shrink-0 flex items-center {open ? 'text-gray-600 dark:text-gray-200' : 'text-gray-300 dark:text-gray-600 hover:text-gray-600 dark:hover:text-gray-200'}">
    <Icon name="stack" class="w-3.5 h-3.5" />
  </button>
{/snippet}

<span class="inline-flex items-center gap-1 min-w-0 max-w-full">
  <span class="min-w-0"><SourcePath {file} {line} label={callerClass(file, line, trace)} {muted} short /></span>
  {#if trace.length > 1}
    <Popover label={m.trace_show()} width={showCode && $sourceAvailable ? 860 : 520} align="auto" {nested}>
      {#snippet triggerButton(toggle, open)}{@render button(toggle, open)}{/snippet}
      {#snippet children()}
        <div class="flex flex-col h-[min(480px,var(--popover-room,70vh))]"><TraceView {trace} start={callerIndex(trace, file, line)} bind:showCode /></div>
      {/snippet}
    </Popover>
  {/if}
</span>
