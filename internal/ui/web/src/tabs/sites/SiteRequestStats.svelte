<script lang="ts">
  import { onMount } from 'svelte';
  import { apiJson } from '$lib/api';
  import { profilerEnabled, loadProfilerStatus, setProfiler } from '$stores/profiler';
  import { openProfiler } from '$stores/dashboard';
  import { m } from '../../paraglide/messages.js';

  interface RouteStat {
    route: string;
    method: string;
    example: string;
    p95_millis: number;
    multiplier: number;
    samples: number;
  }
  interface SiteStats {
    site: string;
    median_millis: number;
    samples: number;
    slow: RouteStat[];
  }

  interface Props {
    domain: string;
    branch?: string;
  }
  let { domain, branch = '' }: Props = $props();

  let stats = $state<SiteStats | null>(null);

  // Poll on the watcher's snapshot cadence so the panel tracks live traffic
  // without hammering the endpoint. Re-fetches when the domain or branch changes.
  async function load() {
    const reqDomain = domain;
    const reqBranch = branch;
    const q = reqBranch ? `?branch=${encodeURIComponent(reqBranch)}` : '';
    try {
      const s = await apiJson<SiteStats>(`/api/sites/${encodeURIComponent(reqDomain)}/stats${q}`);
      // Drop a response that resolved after the user switched site/branch, so a
      // slow request for the old site never paints its numbers over the new one.
      if (reqDomain !== domain || reqBranch !== branch) return;
      stats = s;
    } catch {
      if (reqDomain !== domain || reqBranch !== branch) return;
      stats = null;
    }
  }

  onMount(() => {
    // The $effect below fires load() on mount too, so don't also call it here or
    // the panel double-fetches every time it opens.
    void loadProfilerStatus();
    const id = setInterval(load, 10000);
    return () => clearInterval(id);
  });

  $effect(() => {
    domain;
    branch;
    load();
  });

  const hasData = $derived(!!stats && stats.samples > 0);
  // The API sends slow as null (not []) when there are no flagged routes, so
  // normalise it before any .length / iteration.
  const slowRoutes = $derived<RouteStat[]>(stats?.slow ?? []);

  // A GET route with a concrete example path can be opened straight in a browser.
  // POST and other methods aren't navigable, so they render as plain text.
  function routeUrl(r: RouteStat): string {
    if (r.method !== 'GET' || !r.example) return '';
    return `https://${domain}${r.example}`;
  }

  let arming = $state(false);

  // Profile one route in a click: arm the profiler, wait for it to actually be
  // armed (the toggle only resolves once the server has reloaded nginx), then
  // load the route so that request is captured and switch to the Profiler where
  // the fresh capture lands on top. The tab is opened synchronously here, before
  // the await, and navigated afterwards, because a window.open past an await is
  // outside the click gesture and gets popup-blocked. Non-navigable routes (POST,
  // etc.) just arm and open the Profiler for the user to reproduce.
  async function profileRoute(r: RouteStat) {
    if (arming) return;
    const url = routeUrl(r);
    const tab = url ? window.open('', '_blank') : null;
    arming = true;
    try {
      if (!$profilerEnabled) await setProfiler(true);
      if (tab && url) tab.location.href = url;
      openProfiler();
    } catch {
      tab?.close();
    } finally {
      arming = false;
    }
  }
</script>

<div class="px-3 pt-3 shrink-0">
  <div class="rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-3 py-2.5">
    <div class="flex items-center justify-between gap-3">
      <div class="flex items-center gap-2 text-xs font-medium text-gray-700 dark:text-gray-200">
        <svg class="w-4 h-4 text-gray-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="9" />
          <path d="M12 7v5l3 2" />
        </svg>
        {m.sites_reqstats_title()}
      </div>
      {#if hasData}
        <span class="text-xs tabular-nums text-gray-500 dark:text-gray-400">
          {m.sites_reqstats_typical({ ms: stats!.median_millis })}
        </span>
      {/if}
    </div>

    {#if !hasData}
      <p class="mt-1 text-[11px] text-gray-400 dark:text-gray-500 italic">{m.sites_reqstats_watching()}</p>
    {:else if slowRoutes.length > 0}
      <div class="mt-2">
        <p class="text-[11px] uppercase tracking-wide text-gray-400 dark:text-gray-500 mb-1">{m.sites_reqstats_slowHeading()}</p>
        <ul class="flex flex-col gap-1">
          {#each slowRoutes as r (r.route)}
            <li class="flex items-center justify-between gap-3 text-xs">
              {#if routeUrl(r)}
                <a
                  href={routeUrl(r)}
                  target="_blank"
                  rel="noopener noreferrer"
                  title={r.example}
                  class="font-mono text-gray-700 dark:text-gray-200 truncate min-w-0 inline-flex items-center gap-1 hover:text-lerd-red hover:underline"
                >
                  <span class="truncate">{r.route}</span>
                  <svg class="w-3 h-3 shrink-0 opacity-60" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                    <path d="M15 3h6v6" />
                    <path d="M10 14 21 3" />
                  </svg>
                </a>
              {:else}
                <span class="font-mono text-gray-700 dark:text-gray-200 truncate min-w-0" title={r.route}>{r.route}</span>
              {/if}
              <span class="flex items-center gap-2 shrink-0 tabular-nums">
                <span class="text-amber-600 dark:text-amber-400">{m.sites_reqstats_multiplier({ mult: r.multiplier })}</span>
                <span class="text-gray-500 dark:text-gray-400">{r.p95_millis} ms</span>
                <span class="text-gray-400 dark:text-gray-500">{m.sites_reqstats_samples({ n: r.samples })}</span>
                <button
                  type="button"
                  onclick={() => profileRoute(r)}
                  disabled={arming}
                  title={m.sites_reqstats_profile()}
                  class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded border border-gray-200 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:border-lerd-red hover:text-lerd-red disabled:opacity-50 transition-colors"
                >
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z" />
                  </svg>
                  {m.sites_reqstats_profile()}
                </button>
              </span>
            </li>
          {/each}
        </ul>
        {#if $profilerEnabled}
          <p class="mt-1.5 text-[11px] text-gray-400 dark:text-gray-500">{m.sites_reqstats_profileHint()}</p>
        {/if}
      </div>
    {:else}
      <p class="mt-1 flex items-center gap-1.5 text-[11px] text-emerald-600 dark:text-emerald-400">
        <svg class="w-3.5 h-3.5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M20 6 9 17l-5-5" />
        </svg>
        {m.sites_reqstats_allGood()}
      </p>
    {/if}
  </div>
</div>
