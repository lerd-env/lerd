<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import { debugSearch } from '$stores/debugLens';
  import { dumps, startDumpsStream, stopDumpsStream, clearDumps } from '$stores/dumps';
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
  import EmptyState from '$components/EmptyState.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import TraceBlock from '$components/TraceBlock.svelte';
  import { inlineBindings } from '$lib/sqlInline';
  import type { QueryRow } from '$stores/queries';
  import { m } from '../paraglide/messages.js';

  interface Props {
    // siteScope pins the query view to one site (embedded in SiteDetail).
    // Empty = the global System view. When scoped, the search box stays
    // local so it doesn't bleed into the global lens, mirroring DumpsTab.
    siteScope?: string;
  }
  let { siteScope = '' }: Props = $props();
  const scoped = $derived(siteScope !== '');

  // Queries ride the dumps SSE stream (shared receiver), so mounting this lens
  // opens the same reference-counted connection a DumpsTab would.
  let textInput = $state('');

  onMount(() => {
    startDumpsStream();
    void refreshDevtoolsStatus();
    // Scoped lenses share one search (debugSearch), which a deep link like the
    // timing view's Inspect queries seeds; mirror it into the input on open.
    if (scoped) textInput = get(debugSearch);
  });
  onDestroy(() => {
    stopDumpsStream();
    for (const t of Object.values(copyTimers)) clearTimeout(t);
  });

  const groups = $derived(
    buildQueryGroups($dumps, scoped ? siteScope : $queryFilterSite, scoped ? $debugSearch : $queryFilterText, scoped, $queryFilterWorker, Boolean($devtoolsStatus?.workers))
  );

  let togglingWorkers = $state(false);
  async function onToggleWorkers(e: Event) {
    if (togglingWorkers) return;
    togglingWorkers = true;
    try {
      await toggleDevtoolsWorkers((e.currentTarget as HTMLInputElement).checked);
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

  // Copy the query with its bindings inlined so it pastes straight into a SQL
  // editor. Feedback is per-row (keyed by event id) and clears after 1.5s.
  let copied = $state<Record<string, boolean>>({});
  const copyTimers: Record<string, ReturnType<typeof setTimeout>> = {};
  async function copyRow(row: QueryRow) {
    try {
      await navigator.clipboard.writeText(inlineBindings(row.data.sql, row.data.bindings));
      copied[row.event.id] = true;
      if (copyTimers[row.event.id]) clearTimeout(copyTimers[row.event.id]);
      copyTimers[row.event.id] = setTimeout(() => (copied[row.event.id] = false), 1500);
    } catch {
      /* clipboard unavailable; leave the row untouched */
    }
  }
</script>

<div class="flex flex-col h-full overflow-hidden">
  <div class="flex items-center gap-2 px-3 py-3 border-b border-gray-200 dark:border-lerd-border flex-wrap">
    <div class="relative flex-1 min-w-[140px]">
      <input
        class="w-full text-xs pl-2 pr-6 py-1 rounded-sm border border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card"
        placeholder={m.queries_searchPlaceholder()}
        bind:value={textInput}
      />
      {#if textInput}
        <button
          type="button"
          onclick={() => (textInput = '')}
          title={m.queries_clearFilter()}
          aria-label={m.queries_clearFilter()}
          class="absolute right-1 top-1/2 -translate-y-1/2 w-4 h-4 flex items-center justify-center rounded-sm text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-white/10"
        >
          <svg class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" viewBox="0 0 24 24"><path d="M6 6l12 12M18 6L6 18" /></svg>
        </button>
      {/if}
    </div>
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
    <label class="inline-flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400 cursor-pointer select-none whitespace-nowrap">
      <input
        type="checkbox"
        class="rounded-sm border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card text-lerd-red focus:ring-lerd-red"
        checked={Boolean($devtoolsStatus?.workers)}
        disabled={togglingWorkers}
        onchange={onToggleWorkers}
      />
      {m.queries_show_workers()}
    </label>
    <button
      type="button"
      class="text-xs rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5"
      onclick={onClear}
      title={m.queries_clearCaptured()}
    >
      {m.common_clear()}
    </button>
  </div>

  <div class="flex-1 overflow-y-auto px-3 pb-3">
    {#if groups.length === 0}
      {#if !$debugCaptureEnabled}
        <div class="px-3 py-10 text-center space-y-3">
          <p class="text-sm text-gray-500 dark:text-gray-400">{m.queries_disabled_title()}</p>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">{m.queries_disabled_body()}</p>
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
      {#each groups as group (group.key)}
        <section class="mb-4">
          <header class="flex items-center gap-2 mb-1 sticky top-0 bg-gray-50 dark:bg-lerd-bg py-1 -mx-3 px-3 z-1">
            {#if group.worker}
              <span class="text-[10px] font-semibold uppercase tracking-wide rounded-sm px-1.5 py-0.5 bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300 shrink-0">{m.queries_worker_badge()}</span>
            {/if}
            <span class="text-sm truncate">{group.label}</span>
            {#if group.nPlusOne}
              <span class="text-[10px] font-semibold uppercase tracking-wide rounded-sm px-1.5 py-0.5 bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300">{m.queries_nplusone_badge()}</span>
            {/if}
            <span class="text-xs text-gray-400 ml-auto whitespace-nowrap font-mono">{localTime(group.ts)}</span>
            <span class="text-xs text-gray-400 whitespace-nowrap">
              {m.queries_rollup({ count: group.count, ms: fmtMs(group.totalMs) })}
            </span>
          </header>
          {#each group.rows as row (row.event.id)}
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
                      <span class="text-[10px] rounded-sm px-1 py-0.5 bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300" title="duplicate query in this request">{m.queries_dup_badge({ count: row.dupCount })}</span>
                    {/if}
                    <span
                      class="text-[11px] tabular-nums rounded-sm px-1 py-0.5 {row.slow ? 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300' : 'text-gray-400'}"
                    >{fmtMs(row.data.time_ms)} ms{#if row.slow}&nbsp;{m.queries_slow_badge()}{/if}</span>
                  </span>
                </button>
                <button
                  type="button"
                  class="shrink-0 px-2 flex items-center text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 hover:bg-gray-50 dark:hover:bg-white/5 border-l border-gray-100 dark:border-lerd-border/50 {copied[row.event.id] ? 'text-emerald-600 dark:text-emerald-500' : ''}"
                  onclick={() => copyRow(row)}
                  title={m.queries_copySql()}
                  aria-label={m.queries_copySql()}
                >
                  {#if copied[row.event.id]}
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24"><path d="M20 6L9 17l-5-5" /></svg>
                  {:else}
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></svg>
                  {/if}
                </button>
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
    {/if}
  </div>
</div>
