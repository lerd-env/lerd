<script lang="ts">
  import type { Frame } from '$lib/sourceLabel';
  import { callerClass, fromCaller } from '$lib/sourceLabel';
  import Popover from './Popover.svelte';
  import SourcePath from './SourcePath.svelte';
  import TraceFrames from './TraceFrames.svelte';
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
  const frames = $derived(fromCaller(trace, file, line));
</script>

{#snippet button(toggle: () => void, open: boolean)}
  <button type="button" aria-label={m.trace_show()} aria-expanded={open} onclick={toggle} use:tooltip={m.trace_show()} class="shrink-0 flex items-center {open ? 'text-gray-600 dark:text-gray-200' : 'text-gray-300 dark:text-gray-600 hover:text-gray-600 dark:hover:text-gray-200'}">
    <Icon name="stack" class="w-3.5 h-3.5" />
  </button>
{/snippet}

<span class="inline-flex items-center gap-1 min-w-0 max-w-full">
  <span class="min-w-0"><SourcePath {file} {line} label={callerClass(file, line, trace)} {muted} short /></span>
  {#if frames.length > 1}
    <Popover label={m.trace_show()} width={760} align="auto" {nested}>
      {#snippet triggerButton(toggle, open)}{@render button(toggle, open)}{/snippet}
      {#snippet children()}
        <div class="px-3 pb-2 text-[11px]">
          <div class="sticky top-0 -mx-3 px-3 py-2 bg-white dark:bg-lerd-card text-[10px] uppercase tracking-wide text-gray-400">{m.trace_title({ count: frames.length })}</div>
          <TraceFrames trace={frames} />
        </div>
      {/snippet}
    </Popover>
  {/if}
</span>
