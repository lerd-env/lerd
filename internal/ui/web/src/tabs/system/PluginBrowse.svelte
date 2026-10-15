<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import ConfirmModal from '$components/ConfirmModal.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Icon from '$components/Icon.svelte';
  import PluginRequirements from '$components/PluginRequirements.svelte';
  import { loadCatalog, installListedPlugin, type CatalogEntry } from '$stores/plugins';
  import { notifyLocalFailure, notifyLocalInfo } from '$lib/notify';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    // A plugin to bring into view, from a plugins.lerd.sh "Install in Lerd" link.
    focus?: string;
  }
  let { focus = '' }: Props = $props();

  let entries = $state<CatalogEntry[]>([]);
  let loadError = $state('');
  let loaded = $state(false);
  let query = $state('');
  let sort = $state('updated');
  let confirm = $state<CatalogEntry | null>(null);
  let installing = $state('');

  // The same orders plugins.lerd.sh offers, newest or most first, by name on a tie.
  const byName = (a: CatalogEntry, b: CatalogEntry) => a.name.localeCompare(b.name);
  const SORTS: Record<string, (a: CatalogEntry, b: CatalogEntry) => number> = {
    updated: (a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? '') || byName(a, b),
    stars: (a, b) => (b.stars ?? 0) - (a.stars ?? 0) || byName(a, b),
    added: (a, b) => (b.added_at ?? '').localeCompare(a.added_at ?? '') || byName(a, b),
    name: byName
  };
  const sortOptions = $derived([
    { value: 'updated', label: m.system_plugins_sortUpdated() },
    { value: 'stars', label: m.system_plugins_sortStars() },
    { value: 'added', label: m.system_plugins_sortNewest() },
    { value: 'name', label: m.system_plugins_sortName() }
  ]);

  const shown = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return entries
      .filter((e) => !q || [e.name, e.description, e.author, ...(e.tags ?? [])].join(' ').toLowerCase().includes(q))
      .sort(SORTS[sort] ?? SORTS.updated);
  });

  async function load() {
    try {
      entries = await loadCatalog();
      loadError = '';
    } catch (err) {
      loadError = err instanceof Error ? err.message : String(err);
    } finally {
      loaded = true;
    }
  }

  onMount(async () => {
    await load();
    if (!focus) return;
    await tick();
    document.getElementById('plugin-card-' + focus)?.scrollIntoView({ block: 'center' });
  });

  async function install() {
    const entry = confirm;
    if (!entry) return;
    installing = entry.name;
    try {
      const err = await installListedPlugin(entry.name);
      if (err) {
        notifyLocalFailure('plugin_install', m.system_plugins_installFailed(), err);
        return;
      }
      notifyLocalInfo('plugin_install', m.system_plugins_title(), m.system_plugins_installed());
      await load();
    } finally {
      installing = '';
      confirm = null;
    }
  }
</script>

<div class="space-y-3">
  <div class="flex flex-wrap gap-2">
    <input
      type="search"
      bind:value={query}
      placeholder={m.system_plugins_search()}
      aria-label={m.system_plugins_search()}
      class="flex-1 min-w-48 text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-2.5 py-1.5 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red/50"
    />
    <Dropdown value={sort} options={sortOptions} onchange={(v) => (sort = v)} />
  </div>

  {#if loadError}
    <EmptyState title={m.system_plugins_catalogFailed()} size="sm">
      {#snippet hint()}<span class="break-words">{loadError}</span>{/snippet}
    </EmptyState>
  {:else if loaded && shown.length === 0}
    <EmptyState title={m.system_plugins_noMatch()} size="sm" />
  {:else}
    <div class="grid gap-3 grid-cols-[repeat(auto-fill,minmax(240px,1fr))]">
      {#each shown as e (e.name)}
        <article
          id="plugin-card-{e.name}"
          class="flex flex-col rounded-lg border overflow-hidden bg-white dark:bg-lerd-card {e.name === focus
            ? 'border-lerd-red ring-1 ring-lerd-red'
            : 'border-gray-200 dark:border-lerd-border'}"
        >
          {#if e.images?.length}
            <img src={e.images[0]} alt="" loading="lazy" class="aspect-video w-full object-cover object-left-top bg-gray-100 dark:bg-white/5 border-b border-gray-200 dark:border-lerd-border" />
          {:else}
            <div class="aspect-video flex items-center justify-center bg-gray-100 dark:bg-white/5 border-b border-gray-200 dark:border-lerd-border text-gray-400">
              <Icon name="cube" />
            </div>
          {/if}
          <div class="flex-1 flex flex-col gap-1 p-3">
            <div class="flex items-baseline justify-between gap-2">
              <span class="text-sm font-medium text-gray-900 dark:text-white truncate">{e.name}</span>
              {#if e.author}<span class="text-xs text-gray-400 truncate">{e.author}</span>{/if}
            </div>
            {#if e.description}<p class="text-xs text-gray-500 dark:text-gray-400 line-clamp-2">{e.description}</p>{/if}
            {#if e.requires?.length}<PluginRequirements requires={e.requires} missing={e.missing ?? []} />{/if}
            <div class="mt-auto pt-2 flex items-center justify-between gap-2">
              <span class="text-xs text-gray-400 tabular-nums">{e.stars ? '★ ' + e.stars.toLocaleString() : ''}</span>
              {#if e.installed}
                <span class="text-xs font-medium text-emerald-600 dark:text-emerald-400">{m.system_plugins_installedBadge()}</span>
              {:else if !e.compatible}
                <span class="text-xs text-gray-500 dark:text-gray-400">{m.system_plugins_needsLerd({ constraint: e.lerd ?? '' })}</span>
              {:else if e.missing?.length}
                <span class="text-xs text-red-600 dark:text-red-400">{m.system_plugins_missing({ list: e.missing.join(', ') })}</span>
              {:else}
                <DetailButton tone="primary" onclick={() => (confirm = e)} loading={installing === e.name} disabled={installing !== ''}>
                  {m.common_install()}
                </DetailButton>
              {/if}
            </div>
          </div>
        </article>
      {/each}
    </div>
  {/if}
</div>

<ConfirmModal
  open={confirm !== null}
  title={m.system_plugins_installConfirmTitle({ name: confirm?.name ?? '' })}
  body={m.system_plugins_installConfirmBody({ commit: confirm?.commit.slice(0, 7) ?? '' })}
  confirmLabel={m.common_install()}
  loading={installing !== ''}
  onconfirm={install}
  onclose={() => (confirm = null)}
/>
