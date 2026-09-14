<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import DashboardCard from './DashboardCard.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import CleanupModal from './CleanupModal.svelte';
  import UsageModal from './UsageModal.svelte';
  import { stats, statsLoaded, startStatsPolling, formatBytes } from '$stores/stats';
  import { disk, startDiskPolling, runCleanup } from '$stores/disk';
  import { m } from '../../paraglide/messages.js';

  let stop: (() => void) | null = null;
  let stopDisk: (() => void) | null = null;
  onMount(() => {
    stop = startStatsPolling();
    stopDisk = startDiskPolling();
  });
  onDestroy(() => {
    if (stop) stop();
    if (stopDisk) stopDisk();
  });

  let modalOpen = $state(false);
  let usageOpen = $state(false);
  let cleaning = $state(false);
  let cleanupError = $state<string | undefined>(undefined);

  function openModal() {
    cleanupError = undefined;
    modalOpen = true;
  }

  async function doCleanup() {
    cleaning = true;
    cleanupError = undefined;
    const res = await runCleanup();
    cleaning = false;
    if (res.ok) {
      modalOpen = false;
    } else {
      cleanupError = res.error ?? 'Cleanup failed';
    }
  }

  const rows = $derived($stats.containers);
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
          <span class="text-sm font-semibold text-gray-900 dark:text-white tabular-nums">{$stats.total_cpu_percent.toFixed(2)}%</span>
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

    {#if $disk.available && ($disk.used_by_lerd_bytes > 0 || $disk.reclaimable_bytes > 0)}
      <div class="pt-2 mt-2 border-t border-gray-100 dark:border-lerd-border">
        <div class="flex items-center justify-between gap-2">
          <div class="flex gap-5">
            <button
              type="button"
              class="text-left rounded focus:outline-none focus-visible:ring-2 focus-visible:ring-lerd-red/40"
              onclick={() => (usageOpen = true)}
            >
              <div class="text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">{m.dashboard_disk_usedByLerd()}</div>
              <div class="text-sm font-semibold text-gray-900 dark:text-white tabular-nums underline decoration-dotted underline-offset-2">
                {formatBytes($disk.used_by_lerd_bytes)}
              </div>
            </button>
            {#if $disk.reclaimable_bytes > 0}
              <div>
                <div class="text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">{m.dashboard_disk_reclaimable()}</div>
                <div class="text-sm font-semibold text-gray-900 dark:text-white tabular-nums">{formatBytes($disk.reclaimable_bytes)}</div>
              </div>
            {/if}
          </div>
          {#if $disk.reclaimable_bytes > 0}
            <DetailButton tone="warn" onclick={openModal}>{m.dashboard_disk_cleanup()}</DetailButton>
          {/if}
        </div>
        {#if $disk.lerd_bytes > 0 && $disk.other_bytes > 0}
          <div class="text-[10px] text-gray-400 dark:text-gray-500 mt-1">
            {m.dashboard_disk_split({
              lerd: formatBytes($disk.lerd_bytes),
              other: formatBytes($disk.other_bytes)
            })}
          </div>
        {/if}
        {#if $disk.held_bytes > 0}
          <div class="text-[10px] text-gray-400 dark:text-gray-500 mt-1">
            {m.dashboard_disk_held({ size: formatBytes($disk.held_bytes) })}
          </div>
        {/if}
      </div>
    {/if}

    {#if rows.length > 0}
      <div class="pt-2 border-t border-gray-100 dark:border-lerd-border space-y-1">
        <div class="text-[10px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wide">{m.dashboard_resources_top()}</div>
        <div class="space-y-1 max-h-44 xl:max-h-none overflow-y-auto pr-1">
          {#each rows as c (c.name)}
            <div class="flex items-center gap-2 text-xs">
              <span class="flex-1 truncate text-gray-600 dark:text-gray-300">{shortName(c.name)}</span>
              <span class="shrink-0 font-mono tabular-nums text-gray-500 dark:text-gray-400 w-16 text-right">{formatBytes(c.mem_bytes)}</span>
              <span class="shrink-0 font-mono tabular-nums text-gray-400 dark:text-gray-500 w-14 text-right">{c.cpu_percent.toFixed(2)}%</span>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  {/if}
</DashboardCard>

<UsageModal
  open={usageOpen}
  images={$disk.used_images}
  usedBytes={$disk.used_by_lerd_bytes}
  onclose={() => (usageOpen = false)}
/>

<CleanupModal
  open={modalOpen}
  images={$disk.images}
  reclaimableBytes={$disk.reclaimable_bytes}
  loading={cleaning}
  error={cleanupError}
  onconfirm={doCleanup}
  onclose={() => {
    if (!cleaning) modalOpen = false;
  }}
/>
