<script lang="ts">
  import { onMount } from 'svelte';
  import ListGroupHeader from './ListGroupHeader.svelte';
  import EmptyState from './EmptyState.svelte';
  import Icon from './Icon.svelte';
  import {
    docsIndex,
    docsIndexFailed,
    docsLocation,
    loadDocsIndex,
    loadDocsPage,
    searchDocs,
    goToDocsPage,
    type DocsPage,
    type DocsPageMeta,
    type DocsSearchResult
  } from '$stores/docs';
  import { m } from '../paraglide/messages.js';

  let query = $state('');
  let results = $state<DocsSearchResult[]>([]);
  let page = $state<DocsPage | null>(null);
  let failed = $state(false);
  let scrollEl = $state<HTMLElement | null>(null);

  const route = $derived($docsLocation?.route ?? '');
  const anchor = $derived($docsLocation?.anchor ?? '');
  const searching = $derived(query.trim().length > 0);

  const groups = $derived.by(() => {
    const out: { label: string; pages: DocsPageMeta[] }[] = [];
    for (const p of $docsIndex) {
      const last = out[out.length - 1];
      if (last && last.label === p.section_label) last.pages.push(p);
      else out.push({ label: p.section_label, pages: [p] });
    }
    return out;
  });

  onMount(() => {
    void loadDocsIndex();
  });

  $effect(() => {
    const q = query.trim();
    if (!q) {
      results = [];
      return;
    }
    const id = setTimeout(async () => {
      results = await searchDocs(q);
    }, 180);
    return () => clearTimeout(id);
  });

  $effect(() => {
    const wanted = route;
    if (!wanted) {
      page = null;
      failed = false;
      return;
    }
    let stale = false;
    void loadDocsPage(wanted).then((p) => {
      if (stale) return;
      page = p;
      failed = p === null;
    });
    return () => {
      stale = true;
    };
  });

  // Heading links inside a page carry the anchor in the route hash, so the jump
  // is ours to make: the browser has no element matching the whole hash.
  $effect(() => {
    const target = anchor;
    if (!page || !scrollEl) return;
    requestAnimationFrame(() => {
      const el = target ? scrollEl?.querySelector(`[id="${CSS.escape(target)}"]`) : null;
      if (el) el.scrollIntoView();
      else scrollEl?.scrollTo({ top: 0 });
    });
  });

  const rowClass =
    'w-full text-left px-3 py-1.5 text-[13px] transition-colors truncate ' +
    'text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5';
</script>

<div class="flex-1 flex overflow-hidden">
  <div
    class="{route
      ? 'hidden md:flex'
      : 'flex'} flex-col w-full md:w-56 lg:w-64 shrink-0 border-r border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card overflow-hidden"
  >
    <div class="p-2 border-b border-gray-200 dark:border-lerd-border shrink-0">
      <input
        bind:value={query}
        type="search"
        placeholder={m.docs_search_placeholder()}
        aria-label={m.docs_search_placeholder()}
        class="w-full rounded-sm border border-gray-200 dark:border-lerd-border bg-transparent px-2 py-1.5 text-[13px] text-gray-700 dark:text-gray-200 placeholder:text-gray-400 focus:border-lerd-red focus:outline-hidden"
      />
    </div>

    <div class="flex-1 overflow-y-auto">
      {#if $docsIndexFailed}
        <EmptyState title={m.docs_index_failed()} size="sm" />
      {:else if searching}
        {#if results.length === 0}
          <EmptyState title={m.docs_no_results()} size="sm" />
        {:else}
          {#each results as r (r.route)}
            <button
              onclick={() => goToDocsPage(r.route)}
              class="w-full text-left px-3 py-2 border-b border-gray-100 dark:border-lerd-border/60 hover:bg-gray-50 dark:hover:bg-white/5 transition-colors {r.route ===
              route
                ? 'bg-gray-50 dark:bg-white/5'
                : ''}"
            >
              <div class="text-[13px] text-gray-700 dark:text-gray-200 truncate">{r.title}</div>
              {#if r.snippet}
                <div class="text-[11px] text-gray-500 dark:text-gray-400 line-clamp-2">
                  {r.snippet}
                </div>
              {/if}
            </button>
          {/each}
        {/if}
      {:else}
        {#each groups as g, i (g.label)}
          <ListGroupHeader label={g.label} divider={i > 0} />
          {#each g.pages as p (p.route)}
            <button
              onclick={() => goToDocsPage(p.route)}
              class="{rowClass} {p.route === route
                ? 'bg-gray-50 dark:bg-white/5 text-lerd-red'
                : ''}"
            >
              {p.title}
            </button>
          {/each}
        {/each}
      {/if}
    </div>
  </div>

  <div class="{route ? 'flex' : 'hidden md:flex'} flex-col flex-1 overflow-hidden">
    {#if route}
      <button
        onclick={() => goToDocsPage('')}
        class="md:hidden flex items-center gap-2 px-3 py-2 text-[13px] text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-lerd-border shrink-0"
      >
        <Icon name="back" class="w-4 h-4" />
        {m.nav_documentation()}
      </button>
    {/if}

    <div bind:this={scrollEl} class="flex-1 overflow-y-auto">
      {#if failed}
        <EmptyState title={m.docs_load_failed()} />
      {:else if page}
        <article class="docs-prose mx-auto max-w-3xl px-5 py-6">
          {@html page.html}
        </article>
      {:else if !route}
        <EmptyState title={m.docs_pick_page()} />
      {/if}
    </div>
  </div>
</div>
