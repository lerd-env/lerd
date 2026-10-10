<script lang="ts">
  import LensSearch from '$components/LensSearch.svelte';
  import { onMount, onDestroy, untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { debugSearch, showTests } from '$stores/debugLens';
  import { clearDumps } from '$stores/dumps';
  import {
    queryFilterSite,
    queryFilterWorker,
    devtoolsStatus,
    debugCaptureEnabled,
    refreshDevtoolsStatus,
    setDebugCapture,
    toggleDevtoolsWorkers
  } from '$stores/queries';
  import { lensScope, providePickRequest } from '$stores/debugEvents';
  import EmptyState from '$components/EmptyState.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import LensToggle from '$components/LensToggle.svelte';
  import TestEventsToggle from '$components/TestEventsToggle.svelte';
  import TraceBlock from '$components/TraceBlock.svelte';
  import SourcePath from '$components/SourcePath.svelte';
  import LensList from '$components/LensList.svelte';
  import LensGroupLabel from '$components/LensGroupLabel.svelte';
  import { groupLabel } from '$lib/eventGroup';
  import { createLens, fetchEvent, fetchFacets, type FacetLists } from '$lib/lens';
  import type { DumpEvent } from '$lib/dumpEvent';
  import { m } from '../paraglide/messages.js';

  interface Props {
    kind: 'jobs' | 'views' | 'mail' | 'cache' | 'events' | 'http' | 'logs' | 'exceptions' | 'messages';
    siteScope?: string;
    // pinned is one request's view: no toolbar, and no filter left from the Debug tab.
    pinned?: boolean;
  }
  let { kind, siteScope = '', pinned = false }: Props = $props();
  // The Debug tab narrows this to one request, or to the route a search names.
  const scope = lensScope();
  const scoped = $derived(siteScope !== '');
  // Event `kind` on the wire is singular.
  const wireKind = $derived(
    ({ jobs: 'job', views: 'view', mail: 'mail', cache: 'cache', events: 'event', http: 'http', logs: 'log', exceptions: 'exception', messages: 'message' })[
      kind
    ]
  );

  let localText = $state('');
  let textInput = $state('');
  // Across every site there is no request timeline, so a clicked id becomes this
  // lens's search; inside a site the Debug tab takes the click instead.
  if (!untrack(() => siteScope)) providePickRequest((id) => (textInput = id));
  // Jobs report a whole lifecycle (queued, processing, then the outcome) and a
  // request logs at a handful of levels, so both lenses get a filter that cuts
  // the list down to the rows being looked for.
  let facetFilter = $state('');

  onMount(() => {
    void refreshDevtoolsStatus();
    if (scoped) textInput = get(debugSearch);
  });

  let textTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const v = textInput;
    if (textTimer) clearTimeout(textTimer);
    textTimer = setTimeout(() => (scoped ? debugSearch.set(v) : (localText = v)), 100);
  });

  // Scoped lenses share one search (debugSearch) so it carries across the site's
  // Debug tabs; unscoped keeps a local search.
  const worker = $derived(pinned ? '' : $queryFilterWorker);
  const effectiveText = $derived(pinned || $scope.rid || $scope.route ? '' : scoped ? $debugSearch : localText);
  const site = $derived(scoped ? siteScope : $queryFilterSite);

  // lerd-ui groups, searches and pages this lens; the tab holds one page.
  const lens = createLens();
  onDestroy(() => lens.destroy());
  $effect(() => {
    lens.set({
      kind: wireKind,
      site,
      rid: $scope.rid,
      route: $scope.route,
      q: effectiveText,
      worker,
      workers: wireKind === 'job' || Boolean($devtoolsStatus?.workers),
      facet: facetFilter,
      tests: $showTests
    });
  });

  // What the toolbar filters by, read alongside the lens and again as it moves.
  let facetLists = $state<FacetLists>({ sites: [], workers: [], values: [] });
  const lensGroups = lens.groups;
  $effect(() => {
    void $lensGroups;
    const want = { site, kind: wireKind, tests: $showTests };
    fetchFacets(want).then((f) => (facetLists = f), () => {});
  });

  let enabling = $state(false);
  async function onEnable() {
    if (enabling) return;
    enabling = true;
    try {
      await setDebugCapture(true);
    } finally {
      enabling = false;
    }
  }

  let togglingWorkers = $state(false);
  async function onToggleWorkers(checked: boolean) {
    if (togglingWorkers) return;
    togglingWorkers = true;
    try {
      await toggleDevtoolsWorkers(checked);
      await refreshDevtoolsStatus();
    } finally {
      togglingWorkers = false;
    }
  }

  // Levels read in severity order rather than alphabetically, which is the
  // order someone scanning for the bad ones expects them in.
  const LEVELS = ['emergency', 'alert', 'critical', 'error', 'warning', 'notice', 'info', 'debug'];
  const facetField = $derived(
    wireKind === 'job'
      ? 'status'
      : wireKind === 'log' || wireKind === 'exception'
        ? 'level'
        : wireKind === 'message'
          ? 'channel'
          : ''
  );
  const facets = $derived.by(() => {
    if (!facetField) return [] as string[];
    const seen = facetLists.values;
    const known = LEVELS.filter((l) => seen.includes(l));
    const rest = seen.filter((v) => !LEVELS.includes(v)).sort();
    return facetField === 'level' ? [...known, ...rest] : rest;
  });

  async function clear() {
    await clearDumps();
    await lens.refresh();
  }

  let expanded = $state<Record<string, boolean>>({});
  // An opened row reads its whole event: the list leaves out the call stack
  // and a mail's HTML.
  let details = $state<Record<string, DumpEvent>>({});
  function toggleRow(id: string) {
    expanded[id] = !expanded[id];
    if (!expanded[id]) delete details[id];
    else if (!details[id]) fetchEvent(id).then((e) => (expanded[id] ? (details[id] = e) : undefined), () => {});
  }
  // A row that leaves the page lets go of its whole event, mail HTML included.
  $effect(() => {
    const held = new Set($lensGroups.flatMap((g) => g.rows.map((r) => r.event.id)));
    for (const id of Object.keys(details)) if (!held.has(id)) delete details[id];
  });
  function localTime(ts: string): string {
    const d = new Date(ts);
    return isNaN(d.getTime()) ? ts : d.toLocaleTimeString();
  }

  const fmtMs = (n: number) => (n < 10 ? n.toFixed(2) : n.toFixed(1));

  const EMERALD = 'bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300';
  const ROSE = 'bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300';
  const AMBER = 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300';
  const SKY = 'bg-sky-100 dark:bg-sky-900/40 text-sky-700 dark:text-sky-300';
  const GREY = 'bg-gray-100 dark:bg-white/10 text-gray-500 dark:text-gray-400';

  // Status badge tone per status/op value.
  function tone(v: string): string {
    if (v === 'processed' || v === 'hit') return EMERALD;
    if (v === 'failed') return ROSE;
    if (v === 'miss' || v === 'forget') return AMBER;
    return SKY;
  }
  // Log levels take the same tones as everything else in the window: what wants
  // attention is rose, what might is amber, the rest stays quiet.
  function levelTone(level: string): string {
    if (['emergency', 'alert', 'critical', 'error'].includes(level)) return ROSE;
    if (['warning', 'notice'].includes(level)) return AMBER;
    // Debug is the level a request writes most of and reads least of, so it
    // stays grey rather than taking the tone info is in.
    if (level === 'debug') return GREY;
    return SKY;
  }
  function httpTone(status: number): string {
    if (!status || status >= 500) return ROSE;
    if (status >= 400) return AMBER;
    if (status >= 200 && status < 300) return EMERALD;
    return SKY;
  }
</script>

<div class="flex flex-col h-full overflow-hidden">
  {#if !pinned}
  <div class="flex items-center gap-2 px-3 py-3 border-b border-gray-200 dark:border-lerd-border flex-wrap">
    <LensSearch bind:value={textInput} placeholder={m.debug_searchPlaceholder()} />
    {#if !scoped}
      <Dropdown
        value={$queryFilterSite}
        options={[
          { value: '', label: m.dumps_filter_allSites() },
          ...facetLists.sites.map((s) => ({ value: s, label: s || m.dumps_unknownSite() }))
        ]}
        onchange={(v) => queryFilterSite.set(v)}
      />
    {/if}
    {#if facets.length > 1}
      <Dropdown
        value={facetFilter}
        options={[
          {
            value: '',
            label:
              facetField === 'level'
                ? m.logs_filter_allLevels()
                : facetField === 'channel'
                  ? m.messages_filter_allChannels()
                  : m.jobs_filter_allStatuses()
          },
          ...facets.map((s) => ({ value: s, label: s }))
        ]}
        onchange={(v) => (facetFilter = v)}
      />
    {/if}
    {#if facetLists.workers.length > 0}
      <Dropdown
        value={$queryFilterWorker}
        options={[
          { value: '', label: m.queries_filter_allWorkers() },
          ...facetLists.workers.map((c) => ({ value: c, label: c }))
        ]}
        onchange={(v) => queryFilterWorker.set(v)}
      />
    {/if}
    {#if wireKind !== 'job'}
      <LensToggle
        label={m.queries_show_workers()}
        checked={Boolean($devtoolsStatus?.workers)}
        disabled={togglingWorkers}
        onchange={onToggleWorkers}
      />
    {/if}
    <TestEventsToggle />
    <button type="button" class="text-xs rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5" onclick={() => void clear()}>{m.common_clear()}</button>
  </div>
  {/if}

  <LensList {lens}>
    {#snippet empty()}
      {#if !$debugCaptureEnabled}
        <div class="px-3 py-10 text-center space-y-3">
          <p class="text-sm text-gray-500 dark:text-gray-400">{m.debug_disabled_title()}</p>
          <p class="text-[11px] text-gray-500 dark:text-gray-400">{m.debug_disabled_body()}</p>
          <button type="button" disabled={enabling} onclick={onEnable} class="inline-flex items-center gap-1.5 text-xs rounded-sm border border-emerald-500/40 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 px-3 py-1.5 hover:border-emerald-500 hover:bg-emerald-100 dark:hover:bg-emerald-900/40 disabled:opacity-50">
            {enabling ? m.queries_enabling() : m.debug_enable()}
          </button>
        </div>
      {:else}
        <EmptyState title={m.debug_waiting_title()}>
          {#snippet hint()}{m.debug_waiting_body()}{/snippet}
        </EmptyState>
      {/if}
    {/snippet}
    {#snippet group(g)}
      {@const ev0 = g.rows[0].event}
        <section class="mb-4">
          <header class="flex items-center gap-2 mb-1 sticky top-0 bg-gray-50 dark:bg-lerd-bg py-1 -mx-3 px-3 z-1">
            {#if ev0.ctx.worker}<span class="text-[10px] font-semibold uppercase tracking-wide rounded-sm px-1.5 py-0.5 bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300 shrink-0">{m.queries_worker_badge()}</span>{/if}
            {#if !pinned}<LensGroupLabel label={groupLabel(ev0, scoped)} />{/if}
            <span class="text-xs text-gray-400 ml-auto whitespace-nowrap font-mono">{localTime(ev0.ts)}</span>
            <span class="text-xs text-gray-400 whitespace-nowrap">{g.count}</span>
          </header>
          {#each g.rows as row (row.event.id)}
            {@const ev = row.event}
            {@const d = (ev.data ?? {}) as Record<string, any>}
            <div class="rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card mb-1.5 overflow-hidden">
              <button type="button" class="w-full text-left px-2.5 py-1.5 flex items-start gap-2 hover:bg-gray-50 dark:hover:bg-white/5" onclick={() => toggleRow(ev.id)}>
                <span class="flex-1 break-all text-xs text-gray-800 dark:text-gray-200">
                  {#if wireKind === 'job'}{d.class}
                  {:else if wireKind === 'view'}{d.name}
                  {:else if wireKind === 'mail'}{d.subject || '(no subject)'}
                  {:else if wireKind === 'cache'}<code>{d.key}</code>
                  {:else if wireKind === 'http'}<span class="font-mono">{d.method} {d.url}</span>
                  {:else if wireKind === 'log'}{d.message}
                  {:else if wireKind === 'exception'}{#if d.type && d.type !== 'message'}<span class="font-mono">{d.type}</span>{' '}{/if}{d.message}
                  {:else if wireKind === 'message'}{d.body || d.notification || '(no body)'}
                  {:else}{d.name}{/if}
                </span>
                <span class="flex items-center gap-1 shrink-0">
                  {#if wireKind === 'job'}{#if d.time_ms}<span class="text-[11px] tabular-nums text-gray-500 dark:text-gray-400">{fmtMs(d.time_ms)} ms</span>{/if}<span class="text-[10px] rounded-sm px-1 py-0.5 {tone(d.status)}">{d.status}</span>
                  {:else if wireKind === 'cache'}<span class="text-[10px] rounded-sm px-1 py-0.5 {tone(d.op)}">{d.op}</span>
                  {:else if wireKind === 'http' && d.status}<span class="text-[10px] tabular-nums rounded-sm px-1 py-0.5 {httpTone(d.status)}">{d.status}</span>
                  {:else if wireKind === 'http'}<span class="text-[10px] rounded-sm px-1 py-0.5 {d.failed ? ROSE : SKY}">{d.failed ? 'failed' : m.http_sent()}</span>
                  {:else if wireKind === 'mail' && d.to?.length}<span class="text-[11px] text-gray-400 break-all">→ {d.to[0]}</span>
                  {:else if wireKind === 'log'}{#if d.channel}<span class="text-[11px] text-gray-400">{d.channel}</span>{/if}<span class="text-[10px] rounded-sm px-1 py-0.5 {levelTone(d.level)}">{d.level}</span>
                  {:else if wireKind === 'exception'}{#if d.source}<span class="text-[11px] text-gray-400">{d.source}</span>{/if}<span class="text-[10px] rounded-sm px-1 py-0.5 {levelTone(d.level)}">{d.level}</span>
                  {:else if wireKind === 'message'}{#if d.to}<span class="text-[11px] text-gray-400 break-all">→ {d.to}</span>{/if}{#if d.transport}<span class="text-[10px] rounded-sm px-1 py-0.5 {SKY}">{d.transport}</span>{/if}<span class="text-[10px] rounded-sm px-1 py-0.5 {GREY}">{d.channel}</span>{/if}
                </span>
              </button>
              {#if expanded[ev.id]}
                {@const full = (details[ev.id]?.data ?? {}) as Record<string, any>}
                <div class="px-2.5 pb-2 pt-1 border-t border-gray-100 dark:border-lerd-border/50 text-[11px] space-y-1.5">
                  {#if wireKind === 'job' && d.exception}<div class="text-rose-600 dark:text-rose-400 break-all">{d.exception}</div>{/if}
                  {#if wireKind === 'job'}
                    {@const bits = [
                      d.connection ?? '',
                      d.queue ? `${m.jobs_queue()}: ${d.queue}` : '',
                      d.attempts ? `${m.jobs_attempts()}: ${d.attempts}` : ''
                    ].filter(Boolean)}
                    {#if bits.length}<div class="text-gray-400">{bits.join(' · ')}</div>{/if}
                  {/if}
                  {#if wireKind === 'cache' && d.store}<div class="text-gray-400">store: {d.store}</div>{/if}
                  {#if wireKind === 'view' && d.path}
                    <div>
                      <span class="text-gray-400 mr-1">{m.views_template()}:</span>
                      <SourcePath file={d.path} />
                    </div>
                  {/if}
                  {#if wireKind === 'view' && d.data_keys?.length}
                    <div>
                      <div class="text-gray-400 mb-0.5">{m.views_data()}</div>
                      <table class="w-full border-collapse font-mono">
                        <tbody>
                          {#each d.data_keys as k (k)}
                            <tr class="border-t border-gray-100 dark:border-lerd-border/40 align-top">
                              <td class="py-0.5 pr-3 text-gray-500 dark:text-gray-400 whitespace-nowrap w-px">{k}</td>
                              <td class="py-0.5 break-all">{d.data_preview?.[k] ?? ''}</td>
                            </tr>
                          {/each}
                        </tbody>
                      </table>
                    </div>
                  {/if}
                  {#if wireKind === 'job'}
                    {@const payload = d.payload}
                    {#if payload && Object.keys(payload).length}
                      <div>
                        <div class="text-gray-400 mb-0.5">{m.jobs_payload()}</div>
                        <table class="w-full border-collapse font-mono">
                          <tbody>
                            {#each Object.entries(payload) as [k, v] (k)}
                              <tr class="border-t border-gray-100 dark:border-lerd-border/40 align-top">
                                <!-- A dotted key is one level inside the value above it, so it reads as its child. -->
                                <td class="py-0.5 pr-3 text-gray-500 dark:text-gray-400 whitespace-nowrap w-px {k.includes('.') ? 'pl-3' : ''}">{k}</td>
                                <td class="py-0.5 break-all">{v}</td>
                              </tr>
                            {/each}
                          </tbody>
                        </table>
                      </div>
                    {/if}
                  {/if}
                  {#if wireKind === 'message' && (d.from || d.notification)}
                    <div class="text-gray-400">
                      {[d.from ? `from ${d.from}` : '', d.notification ?? ''].filter(Boolean).join(' · ')}
                    </div>
                  {/if}
                  {#if wireKind === 'exception' && d.previous}
                    <div class="text-gray-400">caused by {d.previous}</div>
                  {/if}
                  {#if wireKind === 'log' && d.context}
                    <pre class="whitespace-pre-wrap break-all text-gray-700 dark:text-gray-300">{d.context}</pre>
                  {/if}
                  {#if wireKind === 'mail'}
                    <div class="text-gray-400 break-all">
                      {#if d.from?.length}from {d.from.join(', ')} · {/if}to {(d.to ?? []).join(', ')}{#if d.cc?.length} · cc {d.cc.join(', ')}{/if}
                    </div>
                    {#if d.views?.length}
                      <div class="text-gray-400 break-all">{m.debug_mail_renderedFrom()} {d.views.join(', ')}</div>
                    {/if}
                    {#if full.html}<iframe sandbox="" class="w-full h-64 bg-white rounded-sm border border-gray-200 dark:border-lerd-border" srcdoc={full.html} title={d.subject ?? 'mail'}></iframe>{/if}
                  {/if}
                  {#if wireKind !== 'view'}<TraceBlock src={ev.src} trace={full.trace} />{/if}
                </div>
              {/if}
            </div>
          {/each}
        </section>
    {/snippet}
  </LensList>
</div>
