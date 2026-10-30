<script lang="ts">
  import { onMount } from 'svelte';
  import StatusDot from '$components/StatusDot.svelte';
  import Icon from '$components/Icon.svelte';
  import DashboardHeader from '$components/DashboardHeader.svelte';
  import DashboardSection from '$components/DashboardSection.svelte';
  import ServiceCardShell from '$components/ServiceCardShell.svelte';
  import ViewSwitch from '$components/ViewSwitch.svelte';
  import CatalogFilter from '$components/CatalogFilter.svelte';
  import CategoryChips from '$components/CategoryChips.svelte';
  import InstalledServiceTile from './InstalledServiceTile.svelte';
  import PresetCard from './PresetCard.svelte';
  import UpdateAllModal from './UpdateAllModal.svelte';
  import {
    coreServices,
    servicesLoaded,
    serviceLabel,
    updatesLabel,
    type Service,
    type ServiceUpdateResult
  } from '$stores/services';
  import { installablePresets, presetsLoaded, loadPresets } from '$stores/presets';
  import { accessMode } from '$stores/accessMode';
  import { routeRest, goToTab } from '$stores/route';
  import { groupByCategory, categoryOf, categoryChips, CATEGORY_LABELS } from '$lib/presetCategories';
  import { m } from '../../paraglide/messages.js';

  const DISCOVER = '_discover';

  onMount(() => {
    loadPresets();
  });

  const running = $derived($coreServices.filter((s) => s.status === 'active').length);
  const total = $derived($coreServices.length);
  const outdated = $derived($coreServices.filter((s) => s.update_available));
  const updates = $derived(outdated.length);
  // Ordered by type like the rail, and within a type the services waiting on an
  // update lead so they are not lost among the rest.
  let installedCategory = $state('');
  const installedCategories = $derived(categoryChips($coreServices));
  const shownInstalled = $derived.by(() => {
    const matches = $coreServices.filter((s) => !installedCategory || categoryOf(s) === installedCategory);
    return groupByCategory(matches).flatMap((g) => [...g.items.filter((s) => s.update_available), ...g.items.filter((s) => !s.update_available)]);
  });

  // #services is the installed list, #services/_discover the presets to add;
  // a service name never holds an underscore, so no service can take the route.
  const discovering = $derived($routeRest === DISCOVER);
  let query = $state('');
  let category = $state('');
  const views = $derived([
    { title: m.services_dash_installed(), icon: 'grid' as const, active: !discovering, onclick: () => goToTab('services') },
    { title: m.services_dash_discover(), icon: 'download' as const, active: discovering, onclick: () => goToTab('services', DISCOVER) }
  ]);

  // A snapshot of what was outdated when the dialog opened, null while it is closed.
  let updateBatch = $state<Service[] | null>(null);
  let results = $state<ServiceUpdateResult[]>([]);
  const updated = $derived(results.filter((r) => r.ok).length);
  const failed = $derived(results.filter((r) => !r.ok));

  function openUpdateAll() {
    results = [];
    updateBatch = [...outdated];
  }
  const sitesServed = $derived(
    new Set($coreServices.flatMap((s) => s.site_domains ?? [])).size
  );

  const shownPresets = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return $installablePresets.filter(
      (p) =>
        (!category || categoryOf(p) === category) &&
        (!q || [p.name, serviceLabel(p.name), p.description ?? ''].some((t) => t.toLowerCase().includes(q)))
    );
  });
  const groups = $derived(groupByCategory(shownPresets));
  const categories = $derived(categoryChips($installablePresets));
  // A category whose last item went away drops its chip, so drop the filter too.
  $effect(() => {
    if (category && !categories.some((c) => c.key === category)) category = '';
    if (installedCategory && !installedCategories.some((c) => c.key === installedCategory)) installedCategory = '';
  });
</script>

<div class="flex-1 flex flex-col min-h-0">
  <DashboardHeader title={m.nav_services()} stats={$servicesLoaded && total > 0 ? summary : undefined} trailing={$accessMode.localControl ? viewSwitch : undefined} />

  <!-- Keyed so each view opens at its top, not where the other was scrolled to. -->
  {#key discovering}
  <div class="flex-1 min-h-0 overflow-y-auto p-4 space-y-6">
    {#if !discovering}
      <section class="space-y-4">
        {#if $accessMode.localControl && updates > 0}
          <div class="flex justify-end">
            <button
              onclick={openUpdateAll}
              disabled={updateBatch !== null}
              class="inline-flex items-center gap-1 text-xs font-medium text-emerald-600 hover:text-emerald-700 disabled:opacity-50 dark:text-emerald-400 dark:hover:text-emerald-300"
            >
              <span>↑</span>
              {m.services_updateAll()}
            </button>
          </div>
        {/if}
        {#if results.length > 0}
          <div class="text-xs space-y-0.5">
            {#if updated > 0}
              <p class="text-emerald-600 dark:text-emerald-400">{m.services_updateAll_done({ count: updated })}</p>
            {/if}
            {#each failed as r (r.name)}
              <p class="text-red-600 dark:text-red-400">{serviceLabel(r.name)}: {r.error}</p>
            {/each}
          </div>
        {/if}
        {#if $servicesLoaded && total === 0}
          <p class="text-sm text-gray-500 dark:text-gray-400">{m.services_dash_noInstalled()}</p>
        {/if}
        <CategoryChips bind:category={installedCategory} categories={installedCategories} groupLabel={m.services_dash_installed()} />
        <div data-testid="installed-grid" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-2">
          {#each shownInstalled as svc (svc.name)}
            <InstalledServiceTile {svc} compact />
          {/each}
          {#if $accessMode.localControl}
            <ServiceCardShell compact suggested>
              <button type="button" onclick={() => goToTab('services', DISCOVER)} class="flex min-w-0 flex-1 items-center gap-2.5 text-left text-sm font-medium text-gray-500 dark:text-gray-400 hover:text-lerd-red">
                <span class="shrink-0 flex items-center justify-center w-8 h-8 rounded-lg bg-lerd-red/10 text-lerd-red"><Icon name="plus" class="w-4 h-4" /></span>
                {m.services_dash_addService()}
              </button>
            </ServiceCardShell>
          {/if}
        </div>
      </section>
    {:else if $accessMode.localControl}
      <section class="space-y-3">
        <CatalogFilter bind:query bind:category {categories} placeholder={m.services_preset_searchPlaceholder()} groupLabel={m.services_dash_discover()} />
        {#if !$presetsLoaded}
          <p class="text-xs text-gray-400">{m.common_loading()}</p>
        {:else if $installablePresets.length === 0}
          <p class="text-sm text-gray-500 dark:text-gray-400">{m.services_preset_allInstalled()}</p>
        {:else if groups.length === 0}
          <p class="text-sm text-gray-500 dark:text-gray-400">{m.services_preset_noMatches()}</p>
        {:else}
          <div class="space-y-4">
            {#each groups as group (group.key)}
              <DashboardSection label={CATEGORY_LABELS[group.key]()} sub>
                {#each group.items as preset (preset.name)}
                  <PresetCard {preset} category={group.key} />
                {/each}
              </DashboardSection>
            {/each}
          </div>
        {/if}
      </section>
    {/if}
  </div>
  {/key}
</div>

{#if updateBatch}
  <UpdateAllModal
    services={updateBatch}
    onclose={() => (updateBatch = null)}
    ondone={(r) => (results = r)}
  />
{/if}

{#snippet viewSwitch()}<ViewSwitch {views} />{/snippet}

{#snippet summary()}
  <span class="inline-flex items-center gap-1.5">
    <StatusDot color={running > 0 ? 'green' : 'gray'} />
    {m.dashboard_services_summary({ running, total })}
  </span>
  {#if updates > 0 && $accessMode.localControl}
    <button type="button" onclick={openUpdateAll} disabled={updateBatch !== null} class="inline-flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400 hover:underline disabled:opacity-50">
      <span aria-hidden="true">↑</span>
      {updatesLabel(updates)}
    </button>
  {:else if updates > 0}
    <span class="inline-flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
      <span aria-hidden="true">↑</span>
      {updatesLabel(updates)}
    </span>
  {/if}
  {#if sitesServed > 0}
    <span class="inline-flex items-center gap-1.5">
      <Icon name="sites" class="w-3.5 h-3.5" />
      {m.services_dash_sitesServed({ count: sitesServed })}
    </span>
  {/if}
{/snippet}
