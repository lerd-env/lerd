<script lang="ts">
  import { onMount } from 'svelte';
  import {
    loadSiteAnalytics,
    TIME_RANGES,
    type Analytics,
    type RouteStat,
    type TimeRange
  } from '$stores/analytics';
  import { profilerEnabled, setProfiler } from '$stores/profiler';
  import { openProfiler } from '$stores/dashboard';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: { domain: string };
    activeWorktreeBranch?: string;
  }
  let { site, activeWorktreeBranch = '' }: Props = $props();

  let range = $state<TimeRange>('1h');
  let data = $state<Analytics | null>(null);
  let tab = $state<'routes' | 'recent'>('routes');

  async function load() {
    const d = site.domain;
    const b = activeWorktreeBranch;
    const rg = range;
    try {
      const a = await loadSiteAnalytics(d, rg, b);
      if (d !== site.domain || b !== activeWorktreeBranch || rg !== range) return;
      data = a;
    } catch {
      if (d !== site.domain || b !== activeWorktreeBranch || rg !== range) return;
      data = null;
    }
  }

  $effect(() => {
    site.domain;
    activeWorktreeBranch;
    range;
    load();
  });

  onMount(() => {
    const poll = setInterval(load, 10000);
    return () => clearInterval(poll);
  });

  const hasData = $derived(!!data && data.samples > 0);
  const errorCount = $derived((data?.status.c4xx ?? 0) + (data?.status.c5xx ?? 0));
  const errorRate = $derived(data && data.samples ? (errorCount / data.samples) * 100 : 0);
  // The slowest list ranks and reads by each route's recent p95, so a route that
  // was slow but has since been fixed drops down or off as newer, faster requests
  // arrive, even while its old slow samples still sit in the window's overall p95.
  const recentP95 = (r: RouteStat) => r.recent_p95_millis ?? r.p95_millis;
  const slowest = $derived(
    [...(data?.routes ?? [])].sort((a, b) => recentP95(b) - recentP95(a)).slice(0, 5)
  );
  const histMax = $derived(Math.max(1, ...(data?.distribution ?? []).map((b) => b.count)));
  const routeMax = $derived(Math.max(1, ...(data?.routes ?? []).map((r) => r.p95_millis)));
  const slowMax = $derived(Math.max(1, ...slowest.map((r) => recentP95(r))));

  // Throughput area+line in a 0..100 box (scaled to any width via preserveAspect),
  // with points placed by their real time so the x axis reads as a clock, and the
  // peak carried out so the y axis can be labelled. Null when there's no traffic.
  const tput = $derived.by(() => {
    const pts = data?.throughput ?? [];
    if (pts.length === 0) return null;
    const max = Math.max(1, ...pts.map((p) => p.count));
    const first = pts[0].at_millis;
    const last = pts[pts.length - 1].at_millis;
    const span = Math.max(1, last - first);
    const x = (at: number) => (pts.length === 1 ? 50 : ((at - first) / span) * 100);
    const y = (v: number) => 96 - (v / max) * 92;
    const line = pts.map((p, i) => `${i ? 'L' : 'M'}${x(p.at_millis).toFixed(1)},${y(p.count).toFixed(1)}`).join(' ');
    const area = `M${x(first).toFixed(1)},100 ${line.replace('M', 'L')} L${x(last).toFixed(1)},100 Z`;
    return { max, first, last, line, area };
  });

  function fmtMs(ms: number): string {
    if (ms >= 1000) return (ms / 1000).toFixed(ms >= 10000 ? 0 : 1).replace(/\.0$/, '') + ' s';
    return Math.round(ms) + ' ms';
  }
  function sev(ms: number): 'good' | 'warn' | 'bad' {
    return ms < 100 ? 'good' : ms < 500 ? 'warn' : 'bad';
  }
  const SEV_TEXT = { good: 'text-emerald-600 dark:text-emerald-400', warn: 'text-amber-600 dark:text-amber-400', bad: 'text-red-600 dark:text-red-400' };
  const SEV_BG = { good: 'bg-emerald-500', warn: 'bg-amber-500', bad: 'bg-red-500' };
  function bucketSev(upperMillis: number): 'good' | 'warn' | 'bad' {
    if (upperMillis === 0) return 'bad';
    return upperMillis <= 100 ? 'good' : upperMillis <= 500 ? 'warn' : 'bad';
  }
  function bucketLabel(b: { upper_millis: number }, i: number, arr: { upper_millis: number }[]): string {
    if (b.upper_millis === 0) return '>' + fmtMs(arr[i - 1]?.upper_millis ?? 1000).replace(' ', '');
    return '<' + fmtMs(b.upper_millis).replace(' ', '');
  }
  const METH = {
    GET: 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10',
    POST: 'text-sky-600 dark:text-sky-400 bg-sky-500/10',
    PUT: 'text-amber-600 dark:text-amber-400 bg-amber-500/10',
    PATCH: 'text-amber-600 dark:text-amber-400 bg-amber-500/10',
    DELETE: 'text-red-600 dark:text-red-400 bg-red-500/10'
  } as Record<string, string>;
  function methClass(m: string): string {
    return METH[m] ?? 'text-gray-500 dark:text-gray-400 bg-gray-500/10';
  }
  function statusClass(s: number): string {
    if (s < 300) return 'text-emerald-600 dark:text-emerald-400';
    if (s < 400) return 'text-sky-600 dark:text-sky-400';
    if (s < 500) return 'text-amber-600 dark:text-amber-400';
    return 'text-red-600 dark:text-red-400';
  }
  function fmtTime(atMillis: number): string {
    return new Date(atMillis).toLocaleTimeString([], { hour12: false });
  }

  let arming = $state(false);
  function routeUrl(r: RouteStat): string {
    if (r.method !== 'GET' || !r.example) return '';
    return `https://${site.domain}${r.example}`;
  }
  async function profileRoute(r: RouteStat) {
    if (arming) return;
    const url = routeUrl(r);
    const t = url ? window.open('', '_blank') : null;
    arming = true;
    try {
      if (!$profilerEnabled) await setProfiler(true);
      if (t && url) t.location.href = url;
      openProfiler();
    } catch {
      t?.close();
    } finally {
      arming = false;
    }
  }
</script>

{#snippet kpi(label: string, value: string, unit: string, meta: string, tone: string)}
  <div class="rounded-lg border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-3">
    <div class="text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500">{label}</div>
    <div class="mt-2 text-2xl font-semibold tracking-tight tabular-nums {tone}">
      {value}<span class="text-sm font-medium text-gray-400 dark:text-gray-500 ml-0.5">{unit}</span>
    </div>
    <div class="mt-1.5 text-[11px] text-gray-500 dark:text-gray-400 truncate">{meta}</div>
  </div>
{/snippet}

<section>
  <div class="mb-2.5 flex items-center justify-between gap-3">
    <h3 class="text-[11px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500">
      {m.sites_reqstats_title()}
    </h3>
    <div class="inline-flex rounded-md border border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-white/5 p-0.5">
      {#each TIME_RANGES as rg (rg)}
        <button
          type="button"
          onclick={() => (range = rg)}
          class="px-2 py-0.5 text-[11px] rounded-sm transition-colors {range === rg
            ? 'bg-white dark:bg-lerd-card text-gray-800 dark:text-gray-100 font-medium shadow-sm'
            : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'}"
        >{rg}</button>
      {/each}
    </div>
  </div>

  {#if !hasData}
    <div class="rounded-lg border border-dashed border-gray-200 dark:border-lerd-border p-6 text-center text-xs text-gray-400 dark:text-gray-500">
      {m.sites_reqstats_watching()}
    </div>
  {:else if data}
    <!-- KPIs -->
    <div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
      {@render kpi(m.sites_timing_typical(), String(data.median_millis), 'ms', 'median', 'text-gray-800 dark:text-gray-100')}
      {@render kpi('p95', String(data.p95_millis), 'ms', m.sites_timing_tail(), sev(data.p95_millis) === 'bad' ? SEV_TEXT.bad : 'text-gray-800 dark:text-gray-100')}
      {@render kpi(m.sites_timing_requests(), data.samples.toLocaleString(), '', m.sites_timing_inWindow({ range }), 'text-gray-800 dark:text-gray-100')}
      {@render kpi(m.sites_timing_errorRate(), errorRate.toFixed(errorRate < 10 ? 1 : 0), '%', m.sites_timing_errorsOf({ errors: errorCount, total: data.samples }), errorCount > 0 ? SEV_TEXT.warn : 'text-gray-800 dark:text-gray-100')}
    </div>

    {#if data.cold_starts > 0}
      <div class="mt-2 flex items-center gap-1.5 text-[11px] text-gray-400 dark:text-gray-500">
        <span class="text-sky-500 dark:text-sky-400">❄</span>
        {m.sites_timing_coldExcluded({ n: data.cold_starts })}
      </div>
    {/if}

    <!-- charts -->
    <div class="mt-3 grid md:grid-cols-2 gap-3">
      <div class="rounded-lg border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-3">
        <div class="text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500 mb-3">{m.sites_timing_responseTime()}</div>
        <div class="flex items-end gap-1.5 h-24">
          {#each data.distribution as b, i (i)}
            <div class="flex-1 flex flex-col items-center justify-end h-full gap-1" use:tooltip={`${b.count} · ${bucketLabel(b, i, data.distribution)}`}>
              <div class="w-full rounded-t-sm {SEV_BG[bucketSev(b.upper_millis)]}" style="height:{Math.max(2, (b.count / histMax) * 100)}%"></div>
              <div class="text-[8px] text-gray-400 dark:text-gray-500 leading-none">{bucketLabel(b, i, data.distribution)}</div>
            </div>
          {/each}
        </div>
      </div>

      <div class="rounded-lg border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-3">
        <div class="flex items-baseline justify-between mb-3">
          <div class="text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500">{m.sites_timing_throughput()}</div>
          <div class="text-[10px] text-gray-400 dark:text-gray-500">{m.sites_timing_perMin()}</div>
        </div>
        {#if tput}
          <div class="flex gap-1.5">
            <div class="flex flex-col justify-between items-end w-6 py-0.5 text-[9px] tabular-nums text-gray-400 dark:text-gray-500 leading-none">
              <span>{tput.max}</span>
              <span>0</span>
            </div>
            <div class="flex-1 min-w-0">
              <svg viewBox="0 0 100 100" preserveAspectRatio="none" class="w-full h-24">
                <line x1="0" y1="4" x2="100" y2="4" class="stroke-gray-200/70 dark:stroke-white/5" stroke-width="1" vector-effect="non-scaling-stroke" />
                <path d={tput.area} class="fill-lerd-red/10" />
                <path d={tput.line} fill="none" class="stroke-lerd-red" stroke-width="1.5" vector-effect="non-scaling-stroke" stroke-linejoin="round" />
              </svg>
              <div class="flex justify-between text-[9px] tabular-nums text-gray-400 dark:text-gray-500 mt-1">
                <span>{fmtTime(tput.first)}</span>
                <span>{fmtTime(tput.last)}</span>
              </div>
            </div>
          </div>
        {:else}
          <div class="h-24 flex items-center justify-center text-[11px] text-gray-400 dark:text-gray-500">{m.sites_reqstats_watching()}</div>
        {/if}
      </div>
    </div>

    <!-- slowest routes -->
    <div class="mt-3 rounded-lg border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-3">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500 mb-2.5">{m.sites_timing_slowest()}</div>
      <div class="flex flex-col gap-2">
        {#each slowest as r (r.method + r.route)}
          <button type="button" onclick={() => profileRoute(r)} disabled={arming} use:tooltip={m.sites_reqstats_profile()}
            class="grid grid-cols-[minmax(7rem,14rem)_1fr_auto] items-center gap-3 text-left group disabled:opacity-60">
            <span class="flex items-center gap-1.5 min-w-0">
              <span class="shrink-0 font-mono text-[9px] font-semibold px-1 py-0.5 rounded {methClass(r.method)}">{r.method}</span>
              <span class="font-mono text-xs text-gray-700 dark:text-gray-200 truncate group-hover:text-lerd-red">{r.route.replace(r.method + ' ', '')}</span>
            </span>
            <span class="h-2 rounded-full bg-gray-100 dark:bg-white/5 overflow-hidden">
              <span class="block h-full rounded-full {SEV_BG[sev(recentP95(r))]}" style="width:{(recentP95(r) / slowMax) * 100}%"></span>
            </span>
            <span class="text-xs font-semibold tabular-nums text-right {SEV_TEXT[sev(recentP95(r))]}">{fmtMs(recentP95(r))}</span>
          </button>
        {/each}
      </div>
    </div>

    <!-- routes / recent -->
    <div class="mt-3 rounded-lg border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card">
      <div class="flex items-center gap-4 px-3 pt-2.5 border-b border-gray-100 dark:border-lerd-border">
        <button type="button" onclick={() => (tab = 'routes')}
          class="pb-2 text-xs font-medium border-b-2 -mb-px transition-colors {tab === 'routes' ? 'border-lerd-red text-lerd-red' : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'}">{m.sites_timing_routes()}</button>
        <button type="button" onclick={() => (tab = 'recent')}
          class="pb-2 text-xs font-medium border-b-2 -mb-px transition-colors {tab === 'recent' ? 'border-lerd-red text-lerd-red' : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'}">{m.sites_timing_recent()}</button>
      </div>

      {#if tab === 'routes'}
        <div class="overflow-x-auto">
          <table class="w-full text-xs">
            <thead>
              <tr class="text-[10px] uppercase tracking-wider text-gray-400 dark:text-gray-500">
                <th class="text-left font-semibold px-3 py-2">{m.sites_timing_route()}</th>
                <th class="text-right font-semibold px-3 py-2">p50</th>
                <th class="text-right font-semibold px-3 py-2">p95</th>
                <th class="text-left font-semibold px-3 py-2 w-24">{m.sites_timing_latency()}</th>
                <th class="text-right font-semibold px-3 py-2">{m.sites_timing_requests()}</th>
              </tr>
            </thead>
            <tbody>
              {#each data.routes as r (r.method + r.route)}
                <tr class="border-t border-gray-100 dark:border-lerd-border hover:bg-gray-50 dark:hover:bg-white/[0.02]">
                  <td class="px-3 py-2">
                    <span class="flex items-center gap-1.5 min-w-0">
                      <span class="shrink-0 font-mono text-[9px] font-semibold px-1 py-0.5 rounded {methClass(r.method)}">{r.method}</span>
                      <span class="font-mono text-gray-700 dark:text-gray-200 truncate">{r.route.replace(r.method + ' ', '')}</span>
                    </span>
                  </td>
                  <td class="px-3 py-2 text-right tabular-nums text-gray-600 dark:text-gray-300">{fmtMs(r.p50_millis)}</td>
                  <td class="px-3 py-2 text-right tabular-nums font-medium {SEV_TEXT[sev(r.p95_millis)]}">{fmtMs(r.p95_millis)}</td>
                  <td class="px-3 py-2">
                    <span class="block h-1.5 rounded-full bg-gray-100 dark:bg-white/5 overflow-hidden">
                      <span class="block h-full rounded-full {SEV_BG[sev(r.p95_millis)]}" style="width:{(r.p95_millis / routeMax) * 100}%"></span>
                    </span>
                  </td>
                  <td class="px-3 py-2 text-right tabular-nums text-gray-600 dark:text-gray-300">{r.samples.toLocaleString()}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <div class="divide-y divide-gray-100 dark:divide-lerd-border">
          {#each data.recent as r (r.at_millis + r.uri)}
            <div class="flex items-center gap-3 px-3 py-2 text-xs">
              <span class="shrink-0 font-mono text-[11px] tabular-nums text-gray-400 dark:text-gray-500">{fmtTime(r.at_millis)}</span>
              <span class="shrink-0 font-mono text-[9px] font-semibold px-1 py-0.5 rounded {methClass(r.method)}">{r.method}</span>
              <span class="font-mono text-gray-700 dark:text-gray-200 truncate flex-1 min-w-0">{r.uri}</span>
              {#if r.cold}
                <span class="shrink-0 text-[11px] text-sky-500 dark:text-sky-400" use:tooltip={m.sites_timing_coldHint()} aria-label={m.sites_timing_cold()}>❄</span>
              {/if}
              <span class="shrink-0 font-mono text-[11px] font-semibold {statusClass(r.status)}">{r.status}</span>
              <span class="shrink-0 tabular-nums font-medium text-right w-16 {r.cold ? 'text-gray-400 dark:text-gray-500' : SEV_TEXT[sev(r.millis)]}">{fmtMs(r.millis)}</span>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</section>
