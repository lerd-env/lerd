<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { dumps, startDumpsStream, stopDumpsStream, clearDumps } from '$stores/dumps';
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
  import { buildKindGroups, knownDebugSites } from '$stores/debugEvents';
  import EmptyState from '$components/EmptyState.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import TraceBlock from '$components/TraceBlock.svelte';
  import { openInEditor } from '$lib/editor';
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

  onMount(() => {
    startDumpsStream();
    void refreshDevtoolsStatus();
  });
  onDestroy(() => stopDumpsStream());

  let localText = $state('');
  let textInput = $state('');
  let textTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const v = textInput;
    if (textTimer) clearTimeout(textTimer);
    textTimer = setTimeout(() => (localText = v), 100);
  });

  const groups = $derived(
    buildKindGroups($dumps, wireKind, scoped ? siteScope : $queryFilterSite, localText, scoped, $queryFilterWorker)
  );

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

  let expanded = $state<Record<string, boolean>>({});
  const toggleRow = (id: string) => (expanded[id] = !expanded[id]);
  function localTime(ts: string): string {
    const d = new Date(ts);
    return isNaN(d.getTime()) ? ts : d.toLocaleTimeString();
  }

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
      <input type="checkbox" class="rounded-sm border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card text-lerd-red focus:ring-lerd-red" checked={Boolean($devtoolsStatus?.workers)} disabled={togglingWorkers} onchange={onToggleWorkers} />
      {m.queries_show_workers()}
    </label>
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
      {#each groups as group (group.key)}
        <section class="mb-4">
          <header class="flex items-center gap-2 mb-1 sticky top-0 bg-gray-50 dark:bg-lerd-bg py-1 -mx-3 px-3 z-1">
            {#if group.worker}<span class="text-[10px] font-semibold uppercase tracking-wide rounded-sm px-1.5 py-0.5 bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300 shrink-0">{m.queries_worker_badge()}</span>{/if}
            <span class="text-sm truncate">{group.label}</span>
            <span class="text-xs text-gray-400 ml-auto whitespace-nowrap font-mono">{localTime(group.ts)}</span>
            <span class="text-xs text-gray-400 whitespace-nowrap">{group.events.length}</span>
          </header>
          {#each group.events as ev (ev.id)}
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
                  {#if wireKind === 'job'}<span class="text-[10px] rounded-sm px-1 py-0.5 {tone(d.status)}">{d.status}</span>
                  {:else if wireKind === 'cache'}<span class="text-[10px] rounded-sm px-1 py-0.5 {tone(d.op)}">{d.op}</span>
                  {:else if wireKind === 'http' && d.status}<span class="text-[10px] tabular-nums rounded-sm px-1 py-0.5 {httpTone(d.status)}">{d.status}</span>
                  {:else if wireKind === 'http'}<span class="text-[10px] rounded-sm px-1 py-0.5 {d.failed ? ROSE : SKY}">{d.failed ? 'failed' : m.http_sent()}</span>
                  {:else if wireKind === 'mail' && d.to?.length}<span class="text-[11px] text-gray-400 break-all">→ {d.to[0]}</span>{/if}
                </span>
              </button>
              {#if expanded[ev.id]}
                <div class="px-2.5 pb-2 pt-1 border-t border-gray-100 dark:border-lerd-border/50 text-[11px] space-y-1.5">
                  {#if wireKind === 'job' && d.exception}<div class="text-rose-600 dark:text-rose-400 break-all">{d.exception}</div>{/if}
                  {#if wireKind === 'job' && d.connection}<div class="text-gray-400">{d.connection}</div>{/if}
                  {#if wireKind === 'cache' && d.store}<div class="text-gray-400">store: {d.store}</div>{/if}
                  {#if wireKind === 'view' && d.path}
                    <div>
                      <span class="text-gray-400 mr-1">{m.views_template()}:</span>
                      <button type="button" class="font-mono text-lerd-red hover:underline break-all" onclick={() => openInEditor(d.path, 1)} title={m.queries_openInEditor()}>{d.path}</button>
                    </div>
                  {/if}
                  {#if wireKind === 'view' && d.data_keys?.length}<div class="text-gray-500 dark:text-gray-400">{m.views_data()}: {d.data_keys.join(', ')}</div>{/if}
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
    {/if}
  </div>
</div>
