<script lang="ts">
  import {
    unhealthyWorkers,
    healAll,
    healLoading,
    healDoneCount,
    healTotalCount,
    loadWorkerHealth
  } from '$stores/workerHealth';
  import { coreServices } from '$stores/services';
  import { sites } from '$stores/sites';
  import { status, statusLoaded, dnsState } from '$stores/status';
  import { accessMode } from '$stores/accessMode';
  import { goToTab } from '$stores/route';
  import {
    lerdStart,
    lerdStarting,
    lerdStopping,
    lerdStartStep,
    lerdStartUnit,
    lerdStartDone,
    lerdStartTotal
  } from '$stores/lerdLifecycle';
  import { m } from '../../paraglide/messages.js';

  type HeroPriority = 'error' | 'ok';

  const failingWorkers = $derived($unhealthyWorkers.length);

  const coreDown = $derived.by(() => {
    if (!$statusLoaded) return [] as string[];
    const issues: string[] = [];
    // Only a genuine lerd-dns outage is a core failure. "degraded" means
    // lerd-dns is healthy but the system resolver is bypassed (typically a
    // VPN), which lerd recovers from on its own, so it doesn't belong here.
    if ($status.dns?.enabled !== false && dnsState($status) === 'down') issues.push('DNS');
    if (!$status.nginx.running) issues.push('Nginx');
    if (!$status.watcher_running) issues.push('Watcher');
    return issues;
  });

  const priority = $derived.by((): HeroPriority => {
    if (failingWorkers > 0 || coreDown.length > 0) return 'error';
    return 'ok';
  });

  const sitesRunning = $derived($sites.filter((s) => s.fpm_running && !s.paused).length);
  const sitesTotal = $derived($sites.length);
  const servicesActive = $derived($coreServices.filter((s) => s.status === 'active').length);

  async function onHeal() {
    await healAll();
    await loadWorkerHealth();
  }

  // The stage ids the start stream emits, mapped to their own message. A unit
  // name is shown as-is: it is the same identifier the CLI prints.
  const startStepLabel = $derived.by(() => {
    if ($lerdStartUnit) return $lerdStartUnit;
    switch ($lerdStartStep) {
      case 'preparing':
        return m.dashboard_hero_startStep_preparing();
      case 'images':
        return m.dashboard_hero_startStep_images();
      case 'units':
        return m.dashboard_hero_startStep_units();
      case 'dns':
        return m.dashboard_hero_startStep_dns();
      default:
        return '';
    }
  });

  const startButtonLabel = $derived.by(() => {
    if (!$lerdStarting) return m.dashboard_hero_startLerd();
    if ($lerdStartTotal > 0)
      return m.dashboard_hero_startingLerdCount({ done: $lerdStartDone, total: $lerdStartTotal });
    return m.dashboard_hero_startingLerd();
  });

  const failingWorkerSites = $derived.by(() => {
    const set = new Set<string>();
    for (const u of $unhealthyWorkers) set.add(u.site);
    return [...set];
  });
</script>

{#if priority === 'error'}
  <div class="rounded-xl border-l-4 border-l-red-500 border border-red-200 dark:border-red-500/30 bg-red-50 dark:bg-red-500/10 px-3 py-3">
    <div class="flex flex-wrap items-center gap-3">
      <span class="relative flex shrink-0">
        <span class="animate-ping absolute inline-flex h-2.5 w-2.5 rounded-full bg-red-400 opacity-75"></span>
        <span class="relative inline-flex h-2.5 w-2.5 rounded-full bg-red-500"></span>
      </span>
      <div class="flex-1 min-w-0">
        {#if coreDown.length > 0}
          <p class="text-sm font-semibold text-red-900 dark:text-red-200">
            {m.dashboard_hero_coreDown({ components: coreDown.join(', ') })}
          </p>
          <p class="text-xs text-red-700 dark:text-red-300/80 mt-0.5 truncate">
            {$lerdStarting && startStepLabel ? startStepLabel : m.dashboard_hero_coreDownHint()}
          </p>
        {:else}
          <p class="text-sm font-semibold text-red-900 dark:text-red-200">
            {m.dashboard_hero_workersFailing({ count: failingWorkers })}
          </p>
          <p class="text-xs text-red-700 dark:text-red-300/80 mt-0.5 truncate">
            {failingWorkerSites.join(', ')}
          </p>
        {/if}
      </div>
      {#if coreDown.length > 0}
        {#if $accessMode.localControl}
          <button
            onclick={lerdStart}
            disabled={$lerdStarting || $lerdStopping}
            class="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium bg-red-600 hover:bg-red-700 text-white disabled:opacity-50 transition-colors"
          >{startButtonLabel}</button>
        {/if}
        <button
          onclick={() => goToTab('system', 'lerd')}
          class="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium border border-red-300 dark:border-red-500/40 text-red-800 dark:text-red-200 hover:bg-red-100 dark:hover:bg-red-500/15 transition-colors"
        >{m.dashboard_hero_openSystem()}</button>
      {:else}
        <button
          onclick={onHeal}
          disabled={$healLoading}
          class="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium bg-amber-600 hover:bg-amber-700 text-white disabled:opacity-50 transition-colors"
        >
          {#if $healLoading}
            {m.dashboard_workers_healing({ done: $healDoneCount, total: $healTotalCount, pct: $healTotalCount > 0 ? Math.round(($healDoneCount / $healTotalCount) * 100) : 0 })}
          {:else}
            {m.dashboard_workers_healAll()}
          {/if}
        </button>
      {/if}
    </div>
  </div>
{:else}
  <div class="rounded-xl border border-emerald-200/70 dark:border-emerald-500/20 bg-emerald-50/60 dark:bg-emerald-500/5 px-3 py-2 flex items-center gap-3">
    <span class="relative flex shrink-0">
      <span class="absolute inline-flex h-2 w-2 rounded-full bg-emerald-400 opacity-75 animate-ping"></span>
      <span class="relative inline-flex h-2 w-2 rounded-full bg-emerald-500"></span>
    </span>
    <p class="text-xs font-medium text-emerald-800 dark:text-emerald-300">
      {m.dashboard_hero_allGood({
        sitesRunning,
        sitesTotal,
        servicesActive
      })}
    </p>
  </div>
{/if}
