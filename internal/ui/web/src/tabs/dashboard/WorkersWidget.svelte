<script lang="ts">
  import DashboardCard from './DashboardCard.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import StatusDot from '$components/StatusDot.svelte';
  import WorkerIcon from '$components/WorkerIcon.svelte';
  import FrameworkMark from '$components/FrameworkMark.svelte';
  import { workerGroups, workerSiteName, parentSiteDomain, type Service } from '$stores/services';
  import { sites } from '$stores/sites';
  import { get } from 'svelte/store';
  import {
    unhealthyWorkers,
    healAll,
    healLoading,
    healDoneCount,
    healTotalCount,
    loadWorkerHealth
  } from '$stores/workerHealth';
  import { goToTab } from '$stores/route';
  import { m } from '../../paraglide/messages.js';

  // A suspended worker's unit is stopped, so it drops out of /api/services
  // entirely. To keep it visible (asleep) instead of silently vanishing, we
  // re-synthesize one entry per site/worktree suspended worker from the sites
  // store and merge it into its group alongside the running ones.
  interface AsleepItem {
    id: string;
    label: string;
    site: string;
  }
  interface MergedGroup {
    key: string;
    label: string;
    running: Service[];
    asleep: AsleepItem[];
  }

  // A suspended worker reaches the widget as a bare name, so its group label
  // comes from whichever running group already carries it, and only falls back
  // to the name itself when nothing of that worker is up anywhere.
  const groupLabelFor = (key: string) =>
    $workerGroups.find((g) => g.key === key)?.label ||
    key.charAt(0).toUpperCase() + key.slice(1);

  // The Stripe listener is lerd's own worker rather than a framework's, so it
  // has no definition to declare a mark; it borrows the one the stripe-mock
  // preset already ships.
  const PRESET_MARKS: Record<string, string> = { stripe: 'stripe-mock' };

  function siteFor(name: string) {
    return $sites.find((s) => s.name === name.split('/')[0]);
  }

  // Which groups the user has opened or closed, remembered so the card comes
  // back the way it was left. A group nobody has touched is absent here, which
  // is what lets a failing one open itself without overruling a deliberate
  // close.
  const OPEN_KEY = 'lerd:workers:open';
  let openState = $state<Record<string, boolean>>(loadOpenState());

  function loadOpenState(): Record<string, boolean> {
    try {
      return JSON.parse(localStorage.getItem(OPEN_KEY) || '{}') as Record<string, boolean>;
    } catch {
      return {};
    }
  }

  function toggleGroup(key: string, open: boolean) {
    openState = { ...openState, [key]: !open };
    try {
      localStorage.setItem(OPEN_KEY, JSON.stringify(openState));
    } catch {
      /* storage may be unavailable (private mode, quota) */
    }
  }

  interface GroupCounts {
    up: number;
    idle: number;
    down: number;
  }

  function countsFor(g: MergedGroup): GroupCounts {
    const down = g.running.filter((i) => isItemFailing(i)).length;
    return {
      up: g.running.filter((i) => i.status === 'active' && !isItemFailing(i)).length,
      idle: g.asleep.length,
      down
    };
  }

  // Naming a framework on every row says nothing while a group holds one: the
  // heading's mark already carries it. A group spanning two is the case where
  // the rows genuinely differ, so there the heading goes neutral and each row
  // takes its own framework mark instead.
  function frameworksIn(g: MergedGroup): string[] {
    const names = [
      ...g.running.map((i) => workerSiteName(i)),
      ...g.asleep.map((i) => i.label)
    ];
    return [...new Set(names.map((n) => siteFor(n)?.framework).filter(Boolean) as string[])];
  }

  const activeIn = (g: MergedGroup) => g.running.filter((i) => i.status === 'active').length;
  const unitsIn = (g: MergedGroup) => g.running.length + g.asleep.length;

  const groups = $derived.by((): MergedGroup[] => {
    const map = new Map<string, MergedGroup>();
    for (const g of $workerGroups) {
      map.set(g.key, { key: g.key, label: g.label, running: g.items, asleep: [] });
    }
    const addAsleep = (worker: string, label: string, site: string) => {
      let g = map.get(worker);
      if (!g) {
        g = { key: worker, label: groupLabelFor(worker), running: [], asleep: [] };
        map.set(worker, g);
      }
      g.asleep.push({ id: worker + ':' + label, label, site });
    };
    for (const s of $sites) {
      const name = s.name;
      if (!name) continue;
      for (const w of s.idle_suspended_workers || []) addAsleep(w, name, name);
      for (const wt of s.worktrees || []) {
        for (const w of wt.idle_suspended_workers || [])
          addAsleep(w, name + '/' + (wt.branch || ''), name);
      }
    }
    // Busiest group first: the count of workers actually up, then the size of
    // the group, so a big idle group never outranks a smaller one that is
    // working. Equal on both, the label decides and the order stays stable.
    return [...map.values()].sort(
      (a, b) => activeIn(b) - activeIn(a) || unitsIn(b) - unitsIn(a) || a.label.localeCompare(b.label)
    );
  });

  const totalUnits = $derived(groups.reduce((n, g) => n + g.running.length + g.asleep.length, 0));
  const totalActive = $derived(
    groups.reduce((n, g) => n + g.running.filter((i) => i.status === 'active').length, 0)
  );
  const asleepCount = $derived(groups.reduce((n, g) => n + g.asleep.length, 0));
  const failingCount = $derived($unhealthyWorkers.length);

  function isItemFailing(item: Service): boolean {
    return unhealthyFor(item) !== undefined;
  }

  function unhealthyFor(item: Service) {
    return $unhealthyWorkers.find((u) => u.unit === item.name || u.unit === 'lerd-' + item.name);
  }

  // The last line the unit printed rides on the failing tile itself, which is
  // what let the red list above the groups go: the failure and the worker it
  // belongs to are one object now instead of two.
  function errorFor(item: Service): string | undefined {
    return unhealthyFor(item)?.last_error;
  }

  function jumpToSite(item: Service) {
    const domain = parentSiteDomain(item);
    if (domain) {
      goToTab('sites', domain);
      return;
    }
    // Last-chance lookup: scan the sites store directly. Hard-coding
    // a TLD here breaks for custom-domain sites and for users running
    // on a non-default .test TLD.
    const name = workerSiteName(item).split('/')[0];
    const site = get(sites).find((x) => x.name === name);
    if (site && site.domain) {
      goToTab('sites', site.domain);
    }
  }

  function jumpToSiteName(name: string) {
    const site = get(sites).find((x) => x.name === name);
    if (site && site.domain) goToTab('sites', site.domain);
  }

  async function onHeal() {
    const r = await healAll();
    await loadWorkerHealth();
    if (!r.ok && r.error) console.error('[lerd] heal failed:', r.error);
  }
</script>

<DashboardCard title={m.dashboard_workers_title()} tone={failingCount > 0 ? 'critical' : 'default'}>
  {#snippet badge()}
    <div class="flex items-center gap-1.5">
      {#if failingCount > 0}
        <StatusPill tone="error" label={m.dashboard_workers_failing({ count: failingCount })} />
      {:else if totalUnits > 0}
        <StatusPill tone="ok" label={m.dashboard_workers_summary({ active: totalActive, total: totalUnits })} />
        {#if asleepCount > 0}
          <StatusPill tone="muted" label={m.dashboard_workers_asleep({ count: asleepCount, total: totalUnits })} />
        {/if}
      {:else}
        <StatusPill tone="muted" label={m.dashboard_workers_none()} />
      {/if}
    </div>
  {/snippet}

  {#if totalUnits === 0}
    <p class="text-sm text-gray-500 dark:text-gray-400">{m.dashboard_workers_empty()}</p>
  {:else}
    <div>
      {#each groups as g, idx (g.key)}
        {@const counts = countsFor(g)}
        {@const fws = frameworksIn(g)}
        {@const mixed = fws.length > 1}
        {@const open = openState[g.key] ?? counts.down > 0}
        <div class={idx > 0 ? 'mt-2 pt-2 border-t border-gray-100 dark:border-lerd-border' : ''}>
          <button
            type="button"
            onclick={() => toggleGroup(g.key, open)}
            aria-expanded={open}
            class="group/head w-full flex items-center gap-1.5 px-1 py-0.5 -mx-1 rounded-sm text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-white/3 transition-colors"
          >
            <svg
              class="w-3 h-3 shrink-0 text-gray-400 dark:text-gray-500 transition-transform {open ? 'rotate-90' : ''}"
              fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
            </svg>
            <WorkerIcon
              worker={g.key}
              framework={mixed ? undefined : fws[0]}
              preset={PRESET_MARKS[g.key]}
              heading
              tint={!mixed}
            />
            <span class="flex-1 truncate text-left">{g.label}</span>
            <span class="flex items-center gap-1.5 font-normal normal-case tracking-normal tabular-nums">
              {#if counts.down > 0}
                <span class="text-red-600 dark:text-red-400">{m.dashboard_workers_groupDown({ count: counts.down })}</span>
              {/if}
              {#if counts.up > 0}
                <span class="text-emerald-600 dark:text-emerald-500">{m.dashboard_workers_groupUp({ count: counts.up })}</span>
              {/if}
              {#if counts.idle > 0}
                <span class="text-sky-600 dark:text-sky-400">{m.dashboard_workers_groupIdle({ count: counts.idle })}</span>
              {/if}
            </span>
          </button>
          <div class="mt-1 {open ? '' : 'hidden'}">
            {#each g.running as item (item.name)}
              {@const name = workerSiteName(item)}
              {@const failing = isItemFailing(item)}
              {@const error = errorFor(item)}
              <button
                type="button"
                onclick={() => jumpToSite(item)}
                class="group w-full flex items-center gap-2 px-1 py-1 -mx-1 rounded-sm text-left text-xs transition-colors {failing
                  ? 'bg-red-50/60 dark:bg-red-500/5 hover:bg-red-50 dark:hover:bg-red-500/10'
                  : 'hover:bg-gray-50 dark:hover:bg-white/3'}"
              >
                <StatusDot
                  color={failing ? 'red' : item.status === 'active' ? 'green' : 'gray'}
                  size="xs"
                  pulse={failing}
                />
                {#if mixed}
                  <FrameworkMark name={siteFor(name)?.framework} size="sm" />
                {/if}
                <span class="flex-1 truncate {failing
                  ? 'text-red-800 dark:text-red-300'
                  : 'text-gray-600 dark:text-gray-300 group-hover:text-lerd-red'} transition-colors">{name}</span>
                {#if failing && error}
                  <span title={error} class="max-w-[45%] truncate font-mono text-[10px] text-red-700/80 dark:text-red-300/70">{error}</span>
                {/if}
              </button>
            {/each}
            {#each g.asleep as item (item.id)}
              <button
                type="button"
                onclick={() => jumpToSiteName(item.site)}
                class="group w-full flex items-center gap-2 px-1 py-1 -mx-1 rounded-sm text-left text-xs hover:bg-gray-50 dark:hover:bg-white/3 transition-colors"
              >
                <svg class="w-3 h-3 shrink-0 text-sky-500 dark:text-sky-400" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                  <path d="M21.752 15.002A9.72 9.72 0 0 1 18 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 0 0 3 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 0 0 9.002-5.998z" />
                </svg>
                {#if mixed}
                  <FrameworkMark name={siteFor(item.label)?.framework} size="sm" />
                {/if}
                <span class="flex-1 truncate text-gray-600 dark:text-gray-300 group-hover:text-lerd-red transition-colors">{item.label}</span>
                <span class="text-[10px] text-sky-600 dark:text-sky-400">{m.sites_idle()}</span>
              </button>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  {/if}

  {#snippet footer()}
    {#if failingCount > 0}
      {@const pct = $healTotalCount > 0 ? Math.round(($healDoneCount / $healTotalCount) * 100) : 0}
      <button
        onclick={onHeal}
        disabled={$healLoading}
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-amber-600 hover:bg-amber-700 text-white disabled:opacity-50 transition-colors"
      >
        {#if $healLoading}
          {m.dashboard_workers_healing({ done: $healDoneCount, total: $healTotalCount, pct })}
        {:else}
          {m.dashboard_workers_healAll()}
        {/if}
      </button>
    {:else}
      <span class="text-xs text-gray-400 dark:text-gray-500">{m.dashboard_workers_allGood()}</span>
    {/if}
  {/snippet}
</DashboardCard>
