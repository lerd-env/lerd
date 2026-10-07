<script lang="ts">
  import type { RequestStats } from '$lib/requestStats';
  import { formatBytes } from '$lib/bytes';
  import { m } from '../paraglide/messages.js';

  // One request's cost at a glance, the dots matching its timeline's colours.

  interface Props {
    stats: RequestStats;
  }
  let { stats }: Props = $props();

  const ms = (n: number) => `${n < 10 ? n.toFixed(1) : Math.round(n)} ms`;
  const tiles = $derived(
    [
      { label: m.sites_timing_responseTime(), value: ms(stats.response), dot: '' },
      stats.memory !== undefined && { label: m.request_stat_memory(), value: formatBytes(stats.memory), dot: '' },
      stats.app !== undefined && { label: m.request_stat_app(), value: ms(stats.app), dot: 'bg-blue-500' },
      stats.database !== undefined && { label: m.request_stat_database(), value: ms(stats.database), dot: 'bg-amber-400' },
      stats.queue !== undefined && { label: m.request_stat_queue(), value: ms(stats.queue), dot: 'bg-slate-400' }
    ].filter((t) => t !== false)
  );
</script>

<dl class="flex flex-wrap rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card divide-x divide-gray-100 dark:divide-lerd-border/60">
  {#each tiles as t (t.label)}
    <div class="flex flex-col-reverse px-4 py-2 min-w-[7rem]">
      <dt class="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
        {#if t.dot}<span class="w-1.5 h-1.5 rounded-full {t.dot}" aria-hidden="true"></span>{/if}{t.label}
      </dt>
      <dd class="text-lg font-semibold tabular-nums text-sky-700 dark:text-sky-300">{t.value}</dd>
    </div>
  {/each}
</dl>
