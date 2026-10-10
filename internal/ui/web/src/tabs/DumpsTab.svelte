<script lang="ts">
  import LensSearch from '$components/LensSearch.svelte';
  import { onMount, onDestroy, untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { debugSearch, showTests } from '$stores/debugLens';
  import { status, filterSite, filterCtx, filterText, refreshStatus, clearDumps, toggleDumps, flashDump } from '$stores/dumps';
  import { lensScope, providePickRequest } from '$stores/debugEvents';
  import DumpEntry from '$components/DumpEntry.svelte';
  import TestEventsToggle from '$components/TestEventsToggle.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import LensList from '$components/LensList.svelte';
  import LensGroupLabel from '$components/LensGroupLabel.svelte';
  import { groupLabel } from '$lib/eventGroup';
  import { createLens, fetchFacets } from '$lib/lens';
  import { m } from '../paraglide/messages.js';

  interface Props {
    // siteScope pins the site filter for this view. When set, the site
    // picker is hidden and only events whose ctx.site matches the scope
    // are rendered. Other filters (ctx, text) remain user-controlled and
    // the global filterSite store stays untouched.
    siteScope?: string;
    // pinned is one request's view: no toolbar, and no filter left from the Debug tab.
    pinned?: boolean;
  }
  let { siteScope = '', pinned = false }: Props = $props();
  // The Debug tab narrows this to one request, or to the route a search names.
  const scope = lensScope();
  const scoped = $derived(siteScope !== '');

  // When scoped (embedded in SiteDetail), search and context filters are
  // local-only — the global filterCtx / filterText writables stay
  // untouched so the System > Debug bridge view doesn't inherit a stale
  // search and vice versa. The unscoped instance keeps using the global
  // stores so user choices persist between visits.
  let localCtx = $state<'' | 'fpm' | 'cli'>('');
  const effectiveCtx = $derived(pinned ? '' : scoped ? localCtx : $filterCtx);
  // Scoped lenses share one search (debugSearch) so it carries between the site's
  // Debug tabs; the unscoped System view keeps its own global filterText.
  const effectiveText = $derived(pinned || $scope.rid || $scope.route ? '' : scoped ? $debugSearch : $filterText);
  const site = $derived(scoped ? siteScope : $filterSite);

  // lerd-ui groups the dumps by request and pages them; the tab holds one page.
  const lens = createLens();
  onDestroy(() => lens.destroy());
  $effect(() => {
    lens.set({ kind: 'dump', site, ctx: effectiveCtx, rid: $scope.rid, route: $scope.route, q: effectiveText, tests: $showTests });
  });
  const lensGroups = lens.groups;
  // A dump that arrives while the tab is open flashes; the first page does not.
  let newestId: string | null = null;
  $effect(() => {
    const top = $lensGroups[0]?.rows[0]?.event.id ?? '';
    if (newestId !== null && top && top !== newestId) flashDump(top);
    if (top) newestId = top;
  });
  let sites = $state<string[]>([]);
  $effect(() => {
    void $lensGroups;
    if (!scoped) fetchFacets({ tests: $showTests }).then((f) => (sites = f.sites), () => {});
  });

  let textInput = $state('');
  // Across every site there is no request timeline, so a clicked id becomes this
  // lens's search; inside a site the Debug tab takes the click instead.
  if (!untrack(() => siteScope)) providePickRequest((id) => (textInput = id));

  onMount(() => {
    void refreshStatus();
    if (scoped) textInput = get(debugSearch);
  });

  let textTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const v = textInput;
    if (textTimer) clearTimeout(textTimer);
    textTimer = setTimeout(() => {
      if (scoped) {
        debugSearch.set(v);
      } else {
        filterText.set(v);
      }
    }, 100);
  });

  async function onClear() {
    await clearDumps();
    await lens.refresh();
  }

  let enabling = $state(false);
  async function onEnable() {
    if (enabling) return;
    enabling = true;
    try {
      await toggleDumps(true);
      await refreshStatus();
    } finally {
      enabling = false;
    }
  }
</script>

<div class="flex flex-col h-full overflow-hidden">
  {#if !pinned}
  <div class="flex items-center gap-2 px-3 py-3 border-b border-gray-200 dark:border-lerd-border flex-wrap">
    <LensSearch bind:value={textInput} placeholder={m.dumps_searchPlaceholder()} />
    {#if !scoped}
      <Dropdown
        value={$filterSite}
        options={[
          { value: '', label: m.dumps_filter_allSites() },
          ...sites.map((s) => ({ value: s, label: s || m.dumps_unknownSite() }))
        ]}
        onchange={(v) => filterSite.set(v)}
      />
    {/if}
    {#if scoped}
      <Dropdown
        value={localCtx}
        options={[
          { value: '', label: m.dumps_filter_allContexts() },
          { value: 'fpm', label: m.dumps_filter_web() },
          { value: 'cli', label: m.dumps_filter_cli() }
        ]}
        onchange={(v) => (localCtx = v as '' | 'fpm' | 'cli')}
      />
    {:else}
      <Dropdown
        value={$filterCtx}
        options={[
          { value: '', label: m.dumps_filter_allContexts() },
          { value: 'fpm', label: m.dumps_filter_web() },
          { value: 'cli', label: m.dumps_filter_cli() }
        ]}
        onchange={(v) => filterCtx.set(v as '' | 'fpm' | 'cli')}
      />
    {/if}
    <TestEventsToggle />
    <button
      type="button"
      class="text-xs rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5"
      onclick={onClear}
    >
      {m.common_clear()}
    </button>
  </div>
  {/if}

  <LensList {lens}>
    {#snippet empty()}
      {#if !$status?.enabled}
        <div class="px-3 py-10 text-center space-y-3">
          <p class="text-sm text-gray-500 dark:text-gray-400">{m.dumps_disabled_title()}</p>
          <p class="text-[11px] text-gray-500 dark:text-gray-400">
            {m.dumps_disabled_body()}
          </p>
          <button
            type="button"
            disabled={enabling}
            onclick={onEnable}
            class="inline-flex items-center gap-1.5 text-xs rounded-sm border border-emerald-500/40 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 px-3 py-1.5 hover:border-emerald-500 hover:bg-emerald-100 dark:hover:bg-emerald-900/40 disabled:opacity-50"
          >
            {enabling ? m.dumps_enabling() : m.dumps_enable()}
          </button>
        </div>
      {:else}
        <EmptyState title={m.dumps_waiting_title()}>
          {#snippet hint()}
            {m.dumps_waiting_body()}
          {/snippet}
        </EmptyState>
      {/if}
    {/snippet}
    {#snippet group(g)}
      <section class="mb-4">
        <header class="flex items-center gap-2 mb-1 sticky top-0 bg-gray-50 dark:bg-lerd-bg py-1 -mx-3 px-3 z-1">
          {#if !pinned}<LensGroupLabel label={groupLabel(g.rows[0].event, scoped)} />{/if}
          <span class="text-xs text-gray-400 ml-auto">{m.dumps_groupCount({ count: g.count })}</span>
        </header>
        {#each g.rows as row (row.event.id)}
          <DumpEntry event={row.event} />
        {/each}
      </section>
    {/snippet}
  </LensList>
</div>
