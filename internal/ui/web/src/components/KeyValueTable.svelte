<script lang="ts">
  import StructuredValue from './StructuredValue.svelte';
  // Names and values as a two-column table: headers, cookies, input, session.

  interface Props {
    title: string;
    values?: Record<string, unknown>;
  }
  let { title, values = {} }: Props = $props();
  const entries = $derived(Object.entries(values));
</script>

{#if entries.length}
  <section class="rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card overflow-hidden text-xs">
    <h4 class="px-3 py-2 font-semibold text-gray-800 dark:text-gray-100 border-b border-gray-100 dark:border-lerd-border/60">{title} <span class="font-normal text-gray-400">{entries.length}</span></h4>
    {#each entries as [k, v] (k)}
      <div class="grid grid-cols-[minmax(0,14rem)_minmax(0,1fr)] gap-3 px-3 py-1.5 border-t first:border-t-0 border-gray-100 dark:border-lerd-border/60 odd:bg-gray-50/60 dark:odd:bg-white/[0.02]">
        <span class="font-mono text-[11px] text-gray-500 dark:text-gray-400 truncate" title={k}>{k}</span>
        <StructuredValue value={v} class="text-gray-800 dark:text-gray-200 min-w-0" />
      </div>
    {/each}
  </section>
{/if}
