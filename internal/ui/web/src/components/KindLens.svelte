<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import { debugSearch } from '$stores/debugLens';
  import { startDumpsStream, stopDumpsStream, clearDumps } from '$stores/dumps';
  import {
    queryFilterSite,
    queryFilterWorker,
    knownWorkerCommands,
    devtoolsStatus,
    debugCaptureEnabled,
    refreshDevtoolsStatus,
    setDebugCapture,
    toggleDevtoolsWorkers
  } from '$stores/queries';
  import { buildKindGroups, knownDebugSites, debugEvents } from '$stores/debugEvents';
  import EmptyState from '$components/EmptyState.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import LensToggle from '$components/LensToggle.svelte';
  import TestEventsToggle from '$components/TestEventsToggle.svelte';
  import TraceBlock from '$components/TraceBlock.svelte';
  import SourcePath from '$components/SourcePath.svelte';
  import LensLoadMore from '$components/LensLoadMore.svelte';
  import LensGroupLabel from '$components/LensGroupLabel.svelte';
  import { windowGroups, LENS_PAGE } from '$lib/lensWindow';
  import { m } from '../paraglide/messages.js';

  interface Props {
    kind: 'jobs' | 'views' | 'mail' | 'cache' | 'events' | 'http';
    siteScope?: string;
  }
  let { kind, siteScope = '' }: Props = $props();
  const scoped = $derived(siteScope !== '');
  // Event `kind` on the wire is singular.
  const wireKind = $derived(
    ({ jobs: 'job', views: 'view', mail: 'mail', cache: 'cache', events: 'event', http: 'http' })[kind]
  );

  let localText = $state('');
  let textInput = $state('');
  // Jobs report a whole lifecycle (queued, processing, then the outcome), so
  // that lens gets a status filter to cut three rows per job down to one.
  let statusFilter = $state('');

  onMount(() => {
    startDumpsStream();
    void refreshDevtoolsStatus();
    if (scoped) textInput = get(debugSearch);
  });
  onDestroy(() => stopDumpsStream());

  let textTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const v = textInput;
    if (textTimer) clearTimeout(textTimer);
    textTimer = setTimeout(() => (scoped ? debugSearch.set(v) : (localText = v)), 100);
  });

  // Scoped lenses share one search (debugSearch) so it carries across the site's
  // Debug tabs; unscoped keeps a local search.
  const effectiveText = $derived(scoped ? $debugSearch : localText);
  const groups = $derived(
    buildKindGroups($debugEvents, wireKind, scoped ? siteScope : $queryFilterSite, effectiveText, scoped, $queryFilterWorker, Boolean($devtoolsStatus?.workers), statusFilter)
  );

  // Only the newest LENS_PAGE rows render; the rest arrive as the user
  // reaches the end. Changing a filter or tab starts the window over.
  let limit = $state(LENS_PAGE);
  const win = $derived(windowGroups(groups, (g) => g.events, limit));
  const filterKey = $derived(`${wireKind}|${scoped ? siteScope : $queryFilterSite}|${effectiveText}|${$queryFilterWorker}|${statusFilter}`);
  $effect(() => {
    filterKey;
    limit = LENS_PAGE;
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

  const jobStatuses = $derived(
    wireKind !== 'job'
      ? []
      : Array.from(
          new Set(
            $debugEvents
              .filter((ev) => ev.kind === 'job')
              .map((ev) => (ev.data as { status?: string } | undefined)?.status)
              .filter((v): v is string => Boolean(v))
          )
        ).sort()
  );

  // A Laravel job's payload is only readable where it was dispatched, so the
  // worker's rows borrow it from the queued row they share a uuid with. The
  // other frameworks put it on every row and never reach this.
  const payloadByUuid = $derived(
    wireKind !== 'job'
      ? new Map<string, Record<string, string>>()
      : new Map(
          $debugEvents
            .filter((ev) => ev.kind === 'job')
            .map((ev) => (ev.data ?? {}) as { uuid?: string; payload?: Record<string, string> })
            .filter((d) => Boolean(d.uuid && d.payload))
            .map((d) => [d.uuid as string, d.payload as Record<string, string>])
        )
  );

  let expanded = $state<Record<string, boolean>>({});
  const toggleRow = (id: string) => (expanded[id] = !expanded[id]);
  function localTime(ts: string): string {
    const d = new Date(ts);
    return isNaN(d.getTime()) ? ts : d.toLocaleTimeString();
  }

  const fmtMs = (n: number) => (n < 10 ? n.toFixed(2) : n.toFixed(1));

  const EMERALD = 'bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300';
  const ROSE = 'bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300';
  const AMBER = 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300';
  const SKY = 'bg-sky-100 dark:bg-sky-900/40 text-sky-700 dark:text-sky-300';

  // Status badge tone per status/op value.
  function tone(v: string): string {
    if (v === 'processed' || v === 'hit') return EMERALD;
    if (v === 'failed') return ROSE;
    if (v === 'miss' || v === 'forget') return AMBER;
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
  <div class="flex items-center gap-2 px-3 py-3 border-b border-gray-200 dark:border-lerd-border flex-wrap">
    <input
      class="text-xs px-2 py-1 rounded-sm border border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card flex-1 min-w-[140px]"
      placeholder={m.debug_searchPlaceholder()}
      bind:value={textInput}
    />
    {#if !scoped}
      <Dropdown
        value={$queryFilterSite}
        options={[
          { value: '', label: m.dumps_filter_allSites() },
          ...$knownDebugSites.map((s) => ({ value: s, label: s || m.dumps_unknownSite() }))
        ]}
        onchange={(v) => queryFilterSite.set(v)}
      />
    {/if}
    {#if jobStatuses.length > 1}
      <Dropdown
        value={statusFilter}
        options={[
          { value: '', label: m.jobs_filter_allStatuses() },
          ...jobStatuses.map((s) => ({ value: s, label: s }))
        ]}
        onchange={(v) => (statusFilter = v)}
      />
    {/if}
    {#if $knownWorkerCommands.length > 0}
      <Dropdown
        value={$queryFilterWorker}
        options={[
          { value: '', label: m.queries_filter_allWorkers() },
          ...$knownWorkerCommands.map((c) => ({ value: c, label: c }))
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
    <button type="button" class="text-xs rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5" onclick={() => clearDumps()}>{m.common_clear()}</button>
  </div>

  <div class="flex-1 overflow-y-auto px-3 pb-3">
    {#if groups.length === 0}
      {#if !$debugCaptureEnabled}
        <div class="px-3 py-10 text-center space-y-3">
          <p class="text-sm text-gray-500 dark:text-gray-400">{m.debug_disabled_title()}</p>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">{m.debug_disabled_body()}</p>
          <button type="button" disabled={enabling} onclick={onEnable} class="inline-flex items-center gap-1.5 text-xs rounded-sm border border-emerald-500/40 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 px-3 py-1.5 hover:border-emerald-500 hover:bg-emerald-100 dark:hover:bg-emerald-900/40 disabled:opacity-50">
            {enabling ? m.queries_enabling() : m.debug_enable()}
          </button>
        </div>
      {:else}
        <EmptyState title={m.debug_waiting_title()}>
          {#snippet hint()}{m.debug_waiting_body()}{/snippet}
        </EmptyState>
      {/if}
    {:else}
      {#each win.pages as page (page.group.key)}
        {@const group = page.group}
        <section class="mb-4">
          <header class="flex items-center gap-2 mb-1 sticky top-0 bg-gray-50 dark:bg-lerd-bg py-1 -mx-3 px-3 z-1">
            {#if group.worker}<span class="text-[10px] font-semibold uppercase tracking-wide rounded-sm px-1.5 py-0.5 bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300 shrink-0">{m.queries_worker_badge()}</span>{/if}
            <LensGroupLabel label={group.label} />
            <span class="text-xs text-gray-400 ml-auto whitespace-nowrap font-mono">{localTime(group.ts)}</span>
            <span class="text-xs text-gray-400 whitespace-nowrap">{page.total}</span>
          </header>
          {#each page.rows as ev (ev.id)}
            {@const d = (ev.data ?? {}) as Record<string, any>}
            <div class="rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card mb-1.5 overflow-hidden">
              <button type="button" class="w-full text-left px-2.5 py-1.5 flex items-start gap-2 hover:bg-gray-50 dark:hover:bg-white/5" onclick={() => toggleRow(ev.id)}>
                <span class="flex-1 break-all text-xs text-gray-800 dark:text-gray-200">
                  {#if wireKind === 'job'}{d.class}
                  {:else if wireKind === 'view'}{d.name}
                  {:else if wireKind === 'mail'}{d.subject || '(no subject)'}
                  {:else if wireKind === 'cache'}<code>{d.key}</code>
                  {:else if wireKind === 'http'}<span class="font-mono">{d.method} {d.url}</span>
                  {:else}{d.name}{/if}
                </span>
                <span class="flex items-center gap-1 shrink-0">
                  {#if wireKind === 'job'}{#if d.time_ms}<span class="text-[11px] tabular-nums text-gray-400 dark:text-gray-500">{fmtMs(d.time_ms)} ms</span>{/if}<span class="text-[10px] rounded-sm px-1 py-0.5 {tone(d.status)}">{d.status}</span>
                  {:else if wireKind === 'cache'}<span class="text-[10px] rounded-sm px-1 py-0.5 {tone(d.op)}">{d.op}</span>
                  {:else if wireKind === 'http' && d.status}<span class="text-[10px] tabular-nums rounded-sm px-1 py-0.5 {httpTone(d.status)}">{d.status}</span>
                  {:else if wireKind === 'http'}<span class="text-[10px] rounded-sm px-1 py-0.5 {d.failed ? ROSE : SKY}">{d.failed ? 'failed' : m.http_sent()}</span>
                  {:else if wireKind === 'mail' && d.to?.length}<span class="text-[11px] text-gray-400 break-all">→ {d.to[0]}</span>{/if}
                </span>
              </button>
              {#if expanded[ev.id]}
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
                    {@const payload = d.payload ?? (d.uuid ? payloadByUuid.get(d.uuid) : undefined)}
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
                  {#if wireKind === 'mail'}
                    <div class="text-gray-400 break-all">
                      {#if d.from?.length}from {d.from.join(', ')} · {/if}to {(d.to ?? []).join(', ')}{#if d.cc?.length} · cc {d.cc.join(', ')}{/if}
                    </div>
                    {#if d.html}<iframe sandbox="" class="w-full h-64 bg-white rounded-sm border border-gray-200 dark:border-lerd-border" srcdoc={d.html} title={d.subject ?? 'mail'}></iframe>{/if}
                  {/if}
                  {#if wireKind !== 'view'}<TraceBlock src={ev.src} trace={d.trace} />{/if}
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
