<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import DashboardCard from './DashboardCard.svelte';
  import { stats, statsLoaded, startStatsPolling, formatBytes } from '$stores/stats';
  import { serviceLabel } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  let stop: (() => void) | null = null;
  onMount(() => {
    stop = startStatsPolling();
  });
  onDestroy(() => {
    if (stop) stop();
  });

  const top = $derived($stats.containers.slice(0, 5));
  const memPercentOfHost = $derived(
    $stats.host_mem_bytes > 0 ? ($stats.total_mem_bytes / $stats.host_mem_bytes) * 100 : 0
  );
  const cpuBarWidth = $derived(Math.min(100, $stats.total_cpu_percent));
  const memBarWidth = $derived(Math.min(100, memPercentOfHost));

  function shortName(n: string): string {
    return n.startsWith('lerd-') ? n.slice(5) : n;
  }
</script>

<DashboardCard title={m.dashboard_resources_title()}>
  {#if $statsLoaded && !$stats.available}
    <p class="text-sm text-gray-500 dark:text-gray-400">{m.dashboard_resources_unavailable()}</p>
  {:else if !$statsLoaded}
    <p class="text-sm text-gray-400 dark:text-gray-500">{m.dashboard_resources_loading()}</p>
  {:else}
    <div class="grid grid-cols-2 gap-3">
      <div>
        <div class="flex items-baseline justify-between mb-1">
          <span class="text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">{m.dashboard_resources_cpu()}</span>
          <span class="text-sm font-semibold text-gray-900 dark:text-white tabular-nums">{$stats.total_cpu_percent.toFixed(1)}%</span>
        </div>
        <div class="h-1.5 rounded-full bg-gray-100 dark:bg-white/5 overflow-hidden">
          <div class="h-full bg-emerald-500 transition-[width] duration-500 ease-out" style="width: {cpuBarWidth}%"></div>
        </div>
      </div>

      <div>
        <div class="flex items-baseline justify-between mb-1">
          <span class="text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">{m.dashboard_resources_memory()}</span>
          <span class="text-sm font-semibold text-gray-900 dark:text-white tabular-nums">{formatBytes($stats.total_mem_bytes)}</span>
        </div>
        <div class="h-1.5 rounded-full bg-gray-100 dark:bg-white/5 overflow-hidden">
          <div class="h-full bg-sky-500 transition-[width] duration-500 ease-out" style="width: {memBarWidth}%"></div>
        </div>
        {#if $stats.host_mem_bytes > 0}
          <div class="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">
            {memPercentOfHost.toFixed(1)}% {m.dashboard_resources_ofHost()} {formatBytes($stats.host_mem_bytes)}
          </div>
        {/if}
      </div>
    </div>

    {#if top.length > 0}
      <div class="pt-2 border-t border-gray-100 dark:border-lerd-border space-y-1">
        <div class="text-[10px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wide">{m.dashboard_resources_top()}</div>
        {#each top as c (c.name)}
          <div class="flex items-center gap-2 text-xs">
            <span class="flex-1 truncate text-gray-600 dark:text-gray-300">{shortName(c.name)}</span>
            <span class="shrink-0 font-mono tabular-nums text-gray-500 dark:text-gray-400 w-16 text-right">{formatBytes(c.mem_bytes)}</span>
            <span class="shrink-0 font-mono tabular-nums text-gray-400 dark:text-gray-500 w-12 text-right">{c.cpu_percent.toFixed(1)}%</span>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</DashboardCard>
