<script lang="ts">
  import type { QueryFrame } from '$lib/dumpsStream';
  import SourcePath from './SourcePath.svelte';
  import { m } from '../paraglide/messages.js';

  interface Props {
    src?: { file: string; line: number };
    trace?: QueryFrame[];
  }
  let { src, trace = [] }: Props = $props();
  let open = $state(false);

  // The most useful single frame: first application frame, then innermost, then src.
  const primary = $derived(
    trace.find((f) => !f.file.includes('/vendor/')) ??
      trace[0] ??
      (src?.file ? { func: '', file: src.file, line: src.line } : undefined)
  );
</script>

{#if primary}
  <div class="text-gray-700 dark:text-gray-200">
    {#if primary.func}<span class="font-semibold">{primary.func}</span> · {/if}
    <SourcePath file={primary.file} line={primary.line} />
  </div>
{/if}
{#if trace.length > 1}
  <div>
    <button
      type="button"
      class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 underline"
      onclick={() => (open = !open)}
    >{open ? m.queries_hideTrace() : m.queries_details()}</button>
    {#if open}
      <ol class="font-mono space-y-0.5 mt-1">
        {#each trace as frame}
          {@const app = !frame.file.includes('/vendor/')}
          <li class={app ? 'text-gray-700 dark:text-gray-200' : 'text-gray-500 dark:text-gray-400'}>
            <span class={app ? 'font-semibold' : ''}>{frame.func}</span> ·
            <SourcePath file={frame.file} line={frame.line} muted={!app} />
          </li>
        {/each}
      </ol>
    {/if}
  </div>
{/if}
