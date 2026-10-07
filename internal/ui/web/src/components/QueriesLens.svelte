<script lang="ts">
  import LensSearch from '$components/LensSearch.svelte';
  import { onMount, onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import { debugSearch } from '$stores/debugLens';
  import { startDumpsStream, stopDumpsStream, clearDumps } from '$stores/dumps';
  import {
    buildQueryGroups,
    queryFilterText,
    queryFilterSite,
    queryFilterWorker,
    knownQuerySites,
    knownWorkerCommands,
    devtoolsStatus,
    debugCaptureEnabled,
    refreshDevtoolsStatus,
    setDebugCapture,
    toggleDevtoolsWorkers
  } from '$stores/queries';
  import { lensEvents, providePickRequest } from '$stores/debugEvents';
  import EmptyState from '$components/EmptyState.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import LensToggle from '$components/LensToggle.svelte';
  import TestEventsToggle from '$components/TestEventsToggle.svelte';
  import TraceBlock from '$components/TraceBlock.svelte';
  import CopyButton from '$components/CopyButton.svelte';
  import LensLoadMore from '$components/LensLoadMore.svelte';
  import LensGroupLabel from '$components/LensGroupLabel.svelte';
  import { windowGroups, LENS_PAGE } from '$lib/lensWindow';
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
  // The request filter over the lenses narrows this to one request.
  const debugEvents = lensEvents();
  const scoped = $derived(siteScope !== '');
  const text = $derived(pinned ? '' : scoped ? $debugSearch : $queryFilterText);
  const worker = $derived(pinned ? '' : $queryFilterWorker);

  // Queries ride the dumps SSE stream (shared receiver), so mounting this lens
  // opens the same reference-counted connection a DumpsTab would.
  let textInput = $state('');
  // Across every site there is no request timeline, so a clicked id becomes this
  // lens's search; inside a site the Debug tab takes the click instead.
  if (!siteScope) providePickRequest((id) => (textInput = id));

  onMount(() => {
    startDumpsStream();
    void refreshDevtoolsStatus();
    // Scoped lenses share one search (debugSearch), which a deep link like the
    // timing view's Inspect queries seeds; mirror it into the input on open.
    if (scoped) textInput = get(debugSearch);
  });
  onDestroy(() => {
    stopDumpsStream();
  });

  const groups = $derived(
    buildQueryGroups($debugEvents, scoped ? siteScope : $queryFilterSite, text, scoped, worker, Boolean($devtoolsStatus?.workers))
  );

  // Only the newest LENS_PAGE rows render; the rest arrive as the user
  // reaches the end. Changing a filter starts the window over.
  let limit = $state(LENS_PAGE);
  const win = $derived(windowGroups(groups, (g) => g.rows, limit));
  const filterKey = $derived(
    `${scoped ? siteScope : $queryFilterSite}|${text}|${worker}`
  );
  $effect(() => {
    filterKey;
    limit = LENS_PAGE;
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
  }

  const fmtMs = (n: number) => (n < 10 ? n.toFixed(2) : n.toFixed(1));
  function localTime(ts: string): string {
    const d = new Date(ts);
    return isNaN(d.getTime()) ? ts : d.toLocaleTimeString();
  }
  let expanded = $state<Record<string, boolean>>({});
  const toggleRow = (id: string) => (expanded[id] = !expanded[id]);
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
          ...$knownQuerySites.map((s) => ({ value: s, label: s || m.dumps_unknownSite() }))
        ]}
        onchange={(v) => queryFilterSite.set(v)}
      />
    {/if}
    {#if $devtoolsStatus?.workers && $knownWorkerCommands.length > 0}
      <Dropdown
        value={$queryFilterWorker}
        options={[
          { value: '', label: m.queries_filter_allWorkers() },
          ...$knownWorkerCommands.map((c) => ({ value: c, label: c }))
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

  <div class="flex-1 overflow-y-auto px-3 pb-3">
    {#if groups.length === 0}
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
    {:else}
      {#each win.pages as page (page.group.key)}
        {@const group = page.group}
        <section class="mb-4">
          <header class="flex items-center gap-2 mb-1 sticky top-0 bg-gray-50 dark:bg-lerd-bg py-1 -mx-3 px-3 z-1">
            {#if group.worker}
              <span class="text-[10px] font-semibold uppercase tracking-wide rounded-sm px-1.5 py-0.5 bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300 shrink-0">{m.queries_worker_badge()}</span>
            {/if}
            {#if !pinned}<LensGroupLabel label={group.label} />{/if}
            {#if group.nPlusOne}
              <span class="text-[10px] font-semibold uppercase tracking-wide rounded-sm px-1.5 py-0.5 bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300">{m.queries_nplusone_badge()}</span>
            {/if}
            <span class="text-xs text-gray-400 ml-auto whitespace-nowrap font-mono">{localTime(group.ts)}</span>
            <span class="text-xs text-gray-400 whitespace-nowrap">
              {m.queries_rollup({ count: group.count, ms: fmtMs(group.totalMs) })}
            </span>
          </header>
          {#each page.rows as row (row.event.id)}
            <div
              class="rounded-sm border mb-1.5 overflow-hidden {row.duplicate
                ? 'border-amber-300 dark:border-amber-700/50 bg-amber-50 dark:bg-amber-900/10'
                : 'border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card'}"
            >
              <div class="flex items-stretch">
                <button
                  type="button"
                  class="flex-1 min-w-0 text-left px-2.5 py-1.5 flex items-start gap-2 hover:bg-gray-50 dark:hover:bg-white/5"
                  onclick={() => toggleRow(row.event.id)}
                >
                  <code class="text-xs flex-1 break-all text-gray-800 dark:text-gray-200">{row.data.sql}</code>
                  <span class="flex items-center gap-1 shrink-0">
                    {#if row.duplicate}
                      <span class="text-[10px] rounded-sm px-1 py-0.5 bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300" title={m.queries_dup_title()}>{m.queries_dup_badge({ count: row.dupCount })}</span>
                    {/if}
                    <span
                      class="text-[11px] tabular-nums rounded-sm px-1 py-0.5 {row.slow ? 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300' : 'text-gray-400'}"
                    >{fmtMs(row.data.time_ms)} ms{#if row.slow}&nbsp;{m.queries_slow_badge()}{/if}</span>
                  </span>
                </button>
                <CopyButton
                  text={() => inlineBindings(row.data.sql, row.data.bindings)}
                  label={m.queries_copySql()}
                  class="px-2 hover:bg-gray-50 dark:hover:bg-white/5 border-l border-gray-100 dark:border-lerd-border/50"
                />
              </div>
              {#if expanded[row.event.id]}
                <div class="px-2.5 pb-2 pt-1 border-t border-gray-100 dark:border-lerd-border/50 text-[11px] space-y-1.5">
                  {#if row.data.connection}
                    <div class="text-gray-400">{row.data.connection}{#if row.data.rw_type}&nbsp;({row.data.rw_type}){/if}</div>
                  {/if}
                  {#if row.data.bindings && row.data.bindings.length > 0}
                    <div>
                      <span class="text-gray-400 mr-1">{m.queries_bindings()}:</span>
                      <code class="text-gray-700 dark:text-gray-300 break-all">{JSON.stringify(row.data.bindings)}</code>
                    </div>
                  {/if}
                  <TraceBlock src={row.event.src} trace={row.data.trace} />
                </div>
              {/if}
            </div>
          {/each}
        </section>
      {/each}
      <LensLoadMore shown={win.shown} total={win.total} onmore={() => (limit += LENS_PAGE)} />
    {/if}
  </div>
</div>
