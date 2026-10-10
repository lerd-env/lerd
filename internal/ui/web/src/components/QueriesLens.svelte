<script lang="ts">
  import LensSearch from '$components/LensSearch.svelte';
  import { onMount, onDestroy, untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { debugSearch, showTests } from '$stores/debugLens';
  import { clearDumps } from '$stores/dumps';
  import {
    SLOW_MS,
    queryFilterText,
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
  import CopyButton from '$components/CopyButton.svelte';
  import LensList from '$components/LensList.svelte';
  import LensGroupLabel from '$components/LensGroupLabel.svelte';
  import { groupLabel } from '$lib/eventGroup';
  import { createLens, fetchEvent, fetchFacets, type FacetLists } from '$lib/lens';
  import type { DumpEvent, QueryData } from '$lib/dumpEvent';
  import { inlineBindings } from '$lib/sqlInline';
  import { m } from '../paraglide/messages.js';

  interface Props {
    // siteScope pins the query view to one site (embedded in SiteDetail).
    // Empty = the global System view. When scoped, the search box stays
    // local so it doesn't bleed into the global lens, mirroring DumpsTab.
    siteScope?: string;
    // pinned is one request's view: no toolbar, and no filter left from the Debug tab.
    pinned?: boolean;
  }
  let { siteScope = '', pinned = false }: Props = $props();
  // The Debug tab narrows this to one request, or to the route a search names.
  const scope = lensScope();
  const scoped = $derived(siteScope !== '');
  const text = $derived(pinned || $scope.rid || $scope.route ? '' : scoped ? $debugSearch : $queryFilterText);
  const worker = $derived(pinned ? '' : $queryFilterWorker);
  const site = $derived(scoped ? siteScope : $queryFilterSite);

  let textInput = $state('');
  // Across every site there is no request timeline, so a clicked id becomes this
  // lens's search; inside a site the Debug tab takes the click instead.
  if (!untrack(() => siteScope)) providePickRequest((id) => (textInput = id));

  onMount(() => {
    void refreshDevtoolsStatus();
    // Scoped lenses share one search (debugSearch), which a deep link like the
    // timing view's Inspect queries seeds; mirror it into the input on open.
    if (scoped) textInput = get(debugSearch);
  });

  // lerd-ui groups the queries by request and flags duplicates and N+1s; the
  // tab holds one page.
  const lens = createLens();
  onDestroy(() => lens.destroy());
  $effect(() => {
    lens.set({
      kind: 'query',
      site,
      rid: $scope.rid,
      route: $scope.route,
      q: text,
      worker,
      workers: Boolean($devtoolsStatus?.workers),
      tests: $showTests
    });
  });

  let facetLists = $state<FacetLists>({ sites: [], workers: [], values: [] });
  const lensGroups = lens.groups;
  $effect(() => {
    void $lensGroups;
    fetchFacets({ site, tests: $showTests }).then((f) => (facetLists = f), () => {});
  });

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

  let textTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const v = textInput;
    if (textTimer) clearTimeout(textTimer);
    textTimer = setTimeout(() => {
      if (scoped) debugSearch.set(v);
      else queryFilterText.set(v);
    }, 100);
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

  async function onClear() {
    await clearDumps();
    await lens.refresh();
  }

  const fmtMs = (n: number) => (n < 10 ? n.toFixed(2) : n.toFixed(1));
  function localTime(ts: string): string {
    const d = new Date(ts);
    return isNaN(d.getTime()) ? ts : d.toLocaleTimeString();
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
  // A row that leaves the page lets go of its whole event, mail HTML included,
  // and closes, so coming back it opens and reads it again.
  $effect(() => {
    const held = new Set($lensGroups.flatMap((g) => g.rows.map((r) => r.event.id)));
    for (const id of Object.keys(details)) {
      if (held.has(id)) continue;
      delete details[id];
      delete expanded[id];
    }
  });
</script>

<div class="flex flex-col h-full overflow-hidden">
  {#if !pinned}
  <div class="flex items-center gap-2 px-3 py-3 border-b border-gray-200 dark:border-lerd-border flex-wrap">
    <LensSearch bind:value={textInput} placeholder={m.queries_searchPlaceholder()} />
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
    {#if $devtoolsStatus?.workers && facetLists.workers.length > 0}
      <Dropdown
        value={$queryFilterWorker}
        options={[
          { value: '', label: m.queries_filter_allWorkers() },
          ...facetLists.workers.map((c) => ({ value: c, label: c }))
        ]}
        onchange={(v) => queryFilterWorker.set(v)}
      />
    {/if}
    <LensToggle
      label={m.queries_show_workers()}
      checked={Boolean($devtoolsStatus?.workers)}
      disabled={togglingWorkers}
      onchange={onToggleWorkers}
    />
    <TestEventsToggle />
    <button
      type="button"
      class="text-xs rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5"
      onclick={onClear}
      title={m.queries_clearCaptured()}
    >
      {m.common_clear()}
    </button>
  </div>
  {/if}

  <LensList {lens}>
    {#snippet empty()}
      {#if !$debugCaptureEnabled}
        <div class="px-3 py-10 text-center space-y-3">
          <p class="text-sm text-gray-500 dark:text-gray-400">{m.queries_disabled_title()}</p>
          <p class="text-[11px] text-gray-500 dark:text-gray-400">{m.queries_disabled_body()}</p>
          <button
            type="button"
            disabled={enabling}
            onclick={onEnable}
            class="inline-flex items-center gap-1.5 text-xs rounded-sm border border-emerald-500/40 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 px-3 py-1.5 hover:border-emerald-500 hover:bg-emerald-100 dark:hover:bg-emerald-900/40 disabled:opacity-50"
          >
            {enabling ? m.queries_enabling() : m.queries_enable()}
          </button>
        </div>
      {:else}
        <EmptyState title={m.queries_waiting_title()}>
          {#snippet hint()}
            {m.queries_waiting_body()}
          {/snippet}
        </EmptyState>
      {/if}
    {/snippet}
    {#snippet group(g)}
      {@const ev0 = g.rows[0].event}
        <section class="mb-4">
          <header class="flex items-center gap-2 mb-1 sticky top-0 bg-gray-50 dark:bg-lerd-bg py-1 -mx-3 px-3 z-1">
            {#if ev0.ctx.worker}
              <span class="text-[10px] font-semibold uppercase tracking-wide rounded-sm px-1.5 py-0.5 bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300 shrink-0">{m.queries_worker_badge()}</span>
            {/if}
            {#if !pinned}<LensGroupLabel label={groupLabel(ev0, scoped)} />{/if}
            {#if g.n_plus_one}
              <span class="text-[10px] font-semibold uppercase tracking-wide rounded-sm px-1.5 py-0.5 bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300">{m.queries_nplusone_badge()}</span>
            {/if}
            <span class="text-xs text-gray-400 ml-auto whitespace-nowrap font-mono">{localTime(ev0.ts)}</span>
            <span class="text-xs text-gray-400 whitespace-nowrap">
              {m.queries_rollup({ count: g.count, ms: fmtMs(g.total_ms) })}
            </span>
          </header>
          {#each g.rows as row (row.event.id)}
            {@const data = row.event.data as QueryData}
            {@const duplicate = row.dup >= 2}
            {@const slow = data.time_ms >= SLOW_MS}
            <div
              class="rounded-sm border mb-1.5 overflow-hidden {duplicate
                ? 'border-amber-300 dark:border-amber-700/50 bg-amber-50 dark:bg-amber-900/10'
                : 'border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card'}"
            >
              <div class="flex items-stretch">
                <button
                  type="button"
                  class="flex-1 min-w-0 text-left px-2.5 py-1.5 flex items-start gap-2 hover:bg-gray-50 dark:hover:bg-white/5"
                  onclick={() => toggleRow(row.event.id)}
                >
                  <code class="text-xs flex-1 break-all text-gray-800 dark:text-gray-200">{data.sql}</code>
                  <span class="flex items-center gap-1 shrink-0">
                    {#if duplicate}
                      <span class="text-[10px] rounded-sm px-1 py-0.5 bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300" title={m.queries_dup_title()}>{m.queries_dup_badge({ count: row.dup })}</span>
                    {/if}
                    <span
                      class="text-[11px] tabular-nums rounded-sm px-1 py-0.5 {slow ? 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300' : 'text-gray-400'}"
                    >{fmtMs(data.time_ms)} ms{#if slow}&nbsp;{m.queries_slow_badge()}{/if}</span>
                  </span>
                </button>
                <CopyButton
                  text={() => inlineBindings(data.sql, data.bindings)}
                  label={m.queries_copySql()}
                  class="px-2 hover:bg-gray-50 dark:hover:bg-white/5 border-l border-gray-100 dark:border-lerd-border/50"
                />
              </div>
              {#if expanded[row.event.id]}
                <div class="px-2.5 pb-2 pt-1 border-t border-gray-100 dark:border-lerd-border/50 text-[11px] space-y-1.5">
                  {#if data.connection}
                    <div class="text-gray-400">{data.connection}{#if data.rw_type}&nbsp;({data.rw_type}){/if}</div>
                  {/if}
                  {#if data.bindings && data.bindings.length > 0}
                    <div>
                      <span class="text-gray-400 mr-1">{m.queries_bindings()}:</span>
                      <code class="text-gray-700 dark:text-gray-300 break-all">{JSON.stringify(data.bindings)}</code>
                    </div>
                  {/if}
                  <TraceBlock src={row.event.src} trace={(details[row.event.id]?.data as QueryData | undefined)?.trace} />
                </div>
              {/if}
            </div>
          {/each}
        </section>
    {/snippet}
  </LensList>
</div>
