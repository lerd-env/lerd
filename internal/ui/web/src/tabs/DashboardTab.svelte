<script lang="ts">
  import { sites } from '$stores/sites';
  import { coreServices } from '$stores/services';
  import { unhealthyWorkers } from '$stores/workerHealth';
  import { statusLoaded, coreDown } from '$stores/status';
  import HeroStatus from './dashboard/HeroStatus.svelte';
  import SetupCard from './dashboard/SetupCard.svelte';
  import SystemHealthWidget from './dashboard/SystemHealthWidget.svelte';
  import LerdInfoWidget from './dashboard/LerdInfoWidget.svelte';
  import SitesWidget from './dashboard/SitesWidget.svelte';
  import ServicesWidget from './dashboard/ServicesWidget.svelte';
  import WorkersWidget from './dashboard/WorkersWidget.svelte';
  import ResourcesWidget from './dashboard/ResourcesWidget.svelte';
  import { openCommandPalette } from '$stores/commandPalette';
  import { setupVisible } from '$stores/setup';
  import { fade } from 'svelte/transition';
  import { m } from '../paraglide/messages.js';

  // Stack status sits beside the title in both states, so neither a healthy
  // nor a broken stack burns a row of vertical space above the cards.
  const sitesRunning = $derived($sites.filter((s) => s.fpm_running && !s.paused).length);
  const sitesTotal = $derived($sites.length);
  const servicesActive = $derived($coreServices.filter((s) => s.status === 'active').length);
  const everythingHealthy = $derived($statusLoaded && $unhealthyWorkers.length === 0 && $coreDown.length === 0);
</script>

<!-- On desktop the header line moves onto the content's top border so the frame corner can curve. -->
<div class="flex-1 min-h-0 flex flex-col overflow-y-auto md:bg-lerd-chrome-light md:dark:bg-lerd-card">
  <div class="shrink-0 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-3 py-1.5 page-header md:border-b-0!">
    <div class="min-w-0 flex flex-wrap items-center gap-x-3 gap-y-1">
      <h1 class="sr-only">{m.dashboard_title()}</h1>
      {#if everythingHealthy}
        <p class="inline-flex items-center gap-2 px-2.5 py-1 rounded-full border border-emerald-200/70 dark:border-emerald-500/20 bg-emerald-50/60 dark:bg-emerald-500/5 text-xs font-medium text-emerald-800 dark:text-emerald-300">
          <span class="relative inline-flex w-2 h-2">
            <span class="absolute inline-flex w-full h-full rounded-full bg-emerald-400 opacity-70 animate-ping"></span>
            <span class="relative inline-flex w-2 h-2 rounded-full bg-emerald-500"></span>
          </span>
          {m.dashboard_hero_allGood({ sitesRunning, sitesTotal, servicesActive })}
        </p>
      {:else if $coreDown.length > 0}
        <HeroStatus />
      {:else}
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.dashboard_subtitle()}</p>
      {/if}
    </div>
    <button
      type="button"
      onclick={openCommandPalette}
      title={m.dashboard_searchHint()}
      class="hidden sm:inline-flex items-center gap-2 w-64 px-3 py-1.5 rounded-full bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-500 dark:text-gray-400 transition-colors"
    >
      <svg class="w-3.5 h-3.5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
      </svg>
      <span class="text-xs">{m.dashboard_search()}</span>
      <kbd class="ml-auto text-[10px] font-mono bg-white dark:bg-white/10 border border-gray-200 dark:border-lerd-border rounded-sm px-1 py-px">⌘K</kbd>
    </button>
  </div>

  <div class="p-3 space-y-3 bg-gray-50 dark:bg-lerd-bg md:border-l md:border-t md:rounded-tl-xl border-lerd-chromeborder dark:border-lerd-border flex-1 xl:min-h-0 xl:flex xl:flex-col">
    <div class="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-3 xl:flex-1 xl:min-h-0 xl:auto-rows-fr">
      <!-- Setup leads the grid while it runs; the Lerd card it stands in for returns to the end. -->
      {#if $setupVisible}
        <SetupCard />
      {/if}
      <SitesWidget />
      <ServicesWidget />
      <WorkersWidget />
      <SystemHealthWidget />
      <ResourcesWidget />
      {#if !$setupVisible}
        <!-- Fades in only when setup hands its slot over; intros skip the first render. -->
        <div class="grid" in:fade={{ duration: 500 }}><LerdInfoWidget /></div>
      {/if}
    </div>
  </div>
</div>
