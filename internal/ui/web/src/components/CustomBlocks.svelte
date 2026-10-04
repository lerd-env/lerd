<script lang="ts">
  import KeyValueTable from './KeyValueTable.svelte';
  import StructuredValue from './StructuredValue.svelte';
  import CustomChart from './CustomChart.svelte';
  import RequestStats from './RequestStats.svelte';

  // The blocks of a tab an app built through lerd/debug, in the order it added
  // them: tables, names and values, code, text and charts.

  interface Props {
    blocks: Array<Record<string, any>>;
    // columns lays the blocks out in a grid on a wide screen, a block spanning
    // as many as it asked for; a narrow screen shows one column.
    columns?: number;
  }
  let { blocks, columns = 1 }: Props = $props();
  const span = (b: Record<string, any>) => Math.min(Math.max(Number(b.span ?? 1) || 1, 1), columns);
  const BOX = 'rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card overflow-hidden text-xs';
  const HEAD = 'px-3 py-2 font-semibold text-gray-800 dark:text-gray-100 border-b border-gray-100 dark:border-lerd-border/60';
  const cell = (row: unknown, col: string, i: number) => (Array.isArray(row) ? row[i] : (row as Record<string, unknown>)?.[col]);
</script>

<div class="grid grid-cols-1 gap-4 md:[grid-template-columns:repeat(var(--cols),minmax(0,1fr))]" style="--cols: {columns}">
{#each blocks as b, bi (bi)}
  <div class="min-w-0 md:[grid-column:span_var(--span)/span_var(--span)]" style="--span: {span(b)}">
  {#if b.type === 'counters'}
    {#if b.title}<h4 class="mb-1.5 text-xs font-semibold text-gray-800 dark:text-gray-100">{b.title}</h4>{/if}
    <RequestStats stats={Object.entries(b.counters ?? {}).map(([label, value]) => ({ label, value: String(value) }))} />
  {:else if b.type === 'kv'}
    <KeyValueTable title={b.title ?? ''} values={b.values ?? {}} />
  {:else if b.type === 'table'}
    <div class={BOX}>
      {#if b.title}<h4 class={HEAD}>{b.title}</h4>{/if}
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead><tr>{#each b.columns ?? [] as c (c)}<th class="px-3 py-1.5 text-left text-[10px] uppercase tracking-wide text-gray-500 dark:text-gray-400 font-medium">{c}</th>{/each}</tr></thead>
          <tbody>
            {#each b.rows ?? [] as row, ri (ri)}
              <tr class="border-t border-gray-100 dark:border-lerd-border/60 odd:bg-gray-50/60 dark:odd:bg-white/[0.02] align-top">
                {#each b.columns ?? [] as c, ci (c)}<td class="px-3 py-1.5"><StructuredValue value={cell(row, c, ci)} /></td>{/each}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {:else if b.type === 'code'}
    <div class={BOX}>
      {#if b.title}<h4 class={HEAD}>{b.title}</h4>{/if}
      <pre class="px-3 py-2 font-mono text-[11px] whitespace-pre-wrap break-words max-h-96 overflow-y-auto">{b.code}</pre>
    </div>
  {:else if b.type === 'text'}
    <div class={BOX}>
      {#if b.title}<h4 class={HEAD}>{b.title}</h4>{/if}
      <p class="px-3 py-2 whitespace-pre-wrap">{b.text}</p>
    </div>
  {:else if b.type === 'chart'}
    <div class={BOX}>
      {#if b.title}<h4 class={HEAD}>{b.title}</h4>{/if}
      <div class="p-3"><CustomChart chart={b.chart} labels={b.labels ?? []} series={b.series ?? []} /></div>
    </div>
  {/if}
  </div>
{/each}
</div>
