<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import { debugSearch } from '$stores/debugLens';
  import { startDumpsStream, stopDumpsStream, clearDumps } from '$stores/dumps';
  import { queryFilterSite } from '$stores/queries';
  import { browserCaptureEnabled, loadBrowserCaptureStatus, setBrowserCapture } from '$stores/browserCapture';
  import { buildKindGroups, knownDebugSites, debugEvents, facetOf } from '$stores/debugEvents';
  import EmptyState from '$components/EmptyState.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import LensLoadMore from '$components/LensLoadMore.svelte';
  import LensGroupLabel from '$components/LensGroupLabel.svelte';
  import BrowserCaptureSettings from '$components/BrowserCaptureSettings.svelte';
  import Modal from '$components/Modal.svelte';
  import { windowGroups, LENS_PAGE } from '$lib/lensWindow';
  import { m } from '../paraglide/messages.js';

  // Page views, JavaScript errors, console messages and failed requests the
  // injected capture script reported, grouped per page view.

  interface Props {
    siteScope?: string;
  }
  let { siteScope = '' }: Props = $props();
  const scoped = $derived(siteScope !== '');

  let localText = $state('');
  let textInput = $state('');
  let showSettings = $state(false);
  let typeFilter = $state('');

  onMount(() => {
    startDumpsStream();
    void loadBrowserCaptureStatus();
    if (scoped) textInput = get(debugSearch);
  });
  onDestroy(() => stopDumpsStream());

  let textTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const v = textInput;
    if (textTimer) clearTimeout(textTimer);
    textTimer = setTimeout(() => (scoped ? debugSearch.set(v) : (localText = v)), 100);
  });

  const effectiveText = $derived(scoped ? $debugSearch : localText);
  const groups = $derived(buildKindGroups($debugEvents, 'browser', scoped ? siteScope : $queryFilterSite, effectiveText, scoped, '', true, typeFilter));
  // Only the types that were actually reported, grouped and ordered the way a
  // reader scans for trouble: errors first, page views last.
  const TYPES: Array<{ value: string; label: () => string; group: () => string }> = [
    { value: 'error', label: m.browser_type_error, group: m.browser_group_errors },
    { value: 'rejection', label: m.browser_type_rejection, group: m.browser_group_errors },
    { value: 'console.error', label: () => 'console.error', group: m.browser_settings_console },
    { value: 'console.warn', label: () => 'console.warn', group: m.browser_settings_console },
    { value: 'network', label: m.browser_settings_network, group: m.browser_group_network },
    { value: 'resource', label: m.browser_settings_resources, group: m.browser_group_network },
    { value: 'event', label: m.browser_settings_events, group: m.browser_group_page },
    { value: 'navigation', label: m.browser_settings_navigation, group: m.browser_group_page }
  ];
  const typeOptions = $derived.by(() => {
    const seen = new Set($debugEvents.filter((ev) => ev.kind === 'browser').map(facetOf));
    return TYPES.filter((t) => seen.has(t.value)).map((t) => ({ value: t.value, label: t.label(), group: t.group() }));
  });

  let limit = $state(LENS_PAGE);
  const win = $derived(windowGroups(groups, (g) => g.events, limit));
  $effect(() => {
    void `${scoped ? siteScope : $queryFilterSite}|${effectiveText}|${typeFilter}`;
    limit = LENS_PAGE;
  });

  let enabling = $state(false);
  async function onEnable() {
    if (enabling) return;
    enabling = true;
    try {
      await setBrowserCapture(true);
    } finally {
      enabling = false;
    }
  }

  let expanded = $state<Record<string, boolean>>({});
  const toggleRow = (id: string) => (expanded[id] = !expanded[id]);
  function localTime(ts: string): string {
    const d = new Date(ts);
    return isNaN(d.getTime()) ? ts : d.toLocaleTimeString();
  }

  const ROSE = 'bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300';
  const AMBER = 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300';
  const GREY = 'bg-gray-100 dark:bg-white/10 text-gray-500 dark:text-gray-400';
  const SKY = 'bg-sky-100 dark:bg-sky-900/40 text-sky-700 dark:text-sky-300';

  function badge(d: Record<string, any>): { text: string; tone: string } {
    if (d.type === 'console') return { text: `console.${d.level}`, tone: d.level === 'warn' ? AMBER : ROSE };
    if (d.type === 'network') return { text: d.status ? String(d.status) : 'failed', tone: !d.status || d.status >= 500 ? ROSE : AMBER };
    if (d.type === 'resource') return { text: `<${d.tag}>`, tone: AMBER };
    if (d.type === 'event') return { text: d.name, tone: SKY };
    if (d.type === 'navigation') return { text: d.nav, tone: GREY };
    return { text: d.type, tone: ROSE };
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
    {#if typeOptions.length > 1}
      <Dropdown
        value={typeFilter}
        options={[{ value: '', label: m.browser_filter_allTypes() }, ...typeOptions]}
        minMenuWidth={200}
        onchange={(v) => (typeFilter = v)}
      />
    {/if}
    {#if scoped}
      <button type="button" aria-haspopup="dialog" class="text-xs rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5" onclick={() => (showSettings = !showSettings)}>{m.common_settings()}</button>
    {/if}
    <button type="button" class="text-xs rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5" onclick={() => clearDumps('browser')}>{m.common_clear()}</button>
  </div>

  {#if scoped}
    <Modal open={showSettings} title={m.browser_settings_title({ site: siteScope })} size="xl" onclose={() => (showSettings = false)}>
      <BrowserCaptureSettings site={siteScope} />
    </Modal>
  {/if}

  <div class="flex-1 overflow-y-auto px-3 pb-3">
    {#if groups.length === 0}
      {#if !$browserCaptureEnabled}
        <div class="px-3 py-10 text-center space-y-3">
          <p class="text-sm text-gray-500 dark:text-gray-400">{m.browser_disabled_title()}</p>
          <p class="text-[11px] text-gray-500 dark:text-gray-400">{m.browser_disabled_body()}</p>
          <button type="button" disabled={enabling} onclick={onEnable} class="inline-flex items-center gap-1.5 text-xs rounded-sm border border-emerald-500/40 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 px-3 py-1.5 hover:border-emerald-500 hover:bg-emerald-100 dark:hover:bg-emerald-900/40 disabled:opacity-50">
            {enabling ? m.queries_enabling() : m.browser_enable()}
          </button>
        </div>
      {:else}
        <EmptyState title={m.debug_waiting_title()}>
          {#snippet hint()}{m.browser_waiting_body()}{/snippet}
        </EmptyState>
      {/if}
    {:else}
      {#each win.pages as page (page.group.key)}
        {@const group = page.group}
        <section class="mb-4">
          <header class="flex items-center gap-2 mb-1 sticky top-0 bg-gray-50 dark:bg-lerd-bg py-1 -mx-3 px-3 z-1">
            <LensGroupLabel label={group.label} />
            <span class="text-xs text-gray-400 ml-auto whitespace-nowrap font-mono">{localTime(group.ts)}</span>
            <span class="text-xs text-gray-400 whitespace-nowrap">{page.total}</span>
          </header>
          {#each page.rows as ev (ev.id)}
            {@const d = (ev.data ?? {}) as Record<string, any>}
            {@const b = badge(d)}
            <div class="rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card mb-1.5 overflow-hidden">
              <button type="button" class="w-full text-left px-2.5 py-1.5 flex items-start gap-2 hover:bg-gray-50 dark:hover:bg-white/5" onclick={() => toggleRow(ev.id)}>
                <span class="flex-1 break-all text-xs text-gray-800 dark:text-gray-200 font-mono">{d.message}</span>
                <span class="text-[10px] rounded-sm px-1 py-0.5 shrink-0 {b.tone}">{b.text}</span>
              </button>
              {#if expanded[ev.id]}
                <div class="px-2.5 pb-2 pt-1 border-t border-gray-100 dark:border-lerd-border/50 text-[11px] space-y-1.5">
                  {#if d.file}<div class="font-mono text-gray-700 dark:text-gray-200 break-all">{d.file}{d.line ? `:${d.line}` : ''}{d.col ? `:${d.col}` : ''}</div>{/if}
                  {#if d.type === 'network'}<div class="font-mono text-gray-700 dark:text-gray-200 break-all">{d.method} {d.request}</div>{/if}
                  {#if d.type === 'resource'}<div class="font-mono text-gray-700 dark:text-gray-200 break-all">{d.request}</div>{/if}
                  {#if d.type === 'network' && !d.status}<div class="text-gray-500 dark:text-gray-400">{d.cross ? m.browser_noResponseCross() : m.browser_noResponse()}</div>{/if}
                  {#if d.stack}<pre class="whitespace-pre-wrap break-all text-gray-600 dark:text-gray-300">{d.stack}</pre>{/if}
                  <div class="text-gray-400 break-all">{d.url}</div>
                  {#if d.ua}<div class="text-gray-400 break-all">{d.ua}</div>{/if}
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
