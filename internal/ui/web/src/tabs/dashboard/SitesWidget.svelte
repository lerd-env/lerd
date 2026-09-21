<script lang="ts">
  import DashboardCard from './DashboardCard.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import Icon from '$components/Icon.svelte';
  import SiteTile from '$tabs/sites/SiteTile.svelte';
  import { sites, sitesLoaded, siteWorkerFailing } from '$stores/sites';
  import { openLinkModal } from '$stores/modals';
  import { goToTab } from '$stores/route';
  import { accessMode } from '$stores/accessMode';
  import { m } from '../../paraglide/messages.js';

  const total = $derived($sites.length);
  const running = $derived($sites.filter((s) => s.fpm_running && !s.paused).length);
  const failing = $derived($sites.filter((s) => siteWorkerFailing(s)).length);

  // The backend serialises sites.yaml in registry order — AddSite appends
  // and RemoveSite preserves the rest's positions, so the position of a
  // site in the array reflects when it was registered (oldest first).
  // Reverse and drop paused sites so the dashboard shows the most recently
  // added active projects at the top — paused sites are still visible on
  // the Sites tab.
  const sorted = $derived($sites.filter((s) => !s.paused).reverse());
</script>

<DashboardCard title={m.dashboard_sites_title()} tone={failing > 0 ? 'critical' : 'default'}>
  {#snippet badge()}
    {#if $sitesLoaded}
      <StatusPill
        tone={failing > 0 ? 'error' : running > 0 ? 'ok' : 'muted'}
        label={m.dashboard_sites_summary({ running, total })}
      />
    {/if}
  {/snippet}

  {#if $sitesLoaded && total === 0}
    <p class="text-sm text-gray-500 dark:text-gray-400">
      {@html m.sites_emptyHint({ cmd: '<code class="bg-gray-100 dark:bg-white/5 px-1 rounded-sm font-mono">lerd park</code>' })}
    </p>
  {:else}
    <div class="space-y-1.5">
      {#each sorted as site (site.domain)}
        <SiteTile {site} compact />
      {/each}
    </div>
  {/if}

  {#snippet footer()}
    <div class="flex flex-wrap items-center gap-2">
      {#if $accessMode.localControl}
        <button
          onclick={openLinkModal}
          class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-lerd-red hover:bg-lerd-redhov text-lerd-onred transition-colors"
        >
          <Icon name="plus" class="w-3.5 h-3.5" />
          {m.dashboard_sites_link()}
        </button>
      {/if}
      <button
        onclick={() => goToTab('sites')}
        class="ml-auto text-xs font-medium text-lerd-red hover:text-lerd-redhov"
      >{m.dashboard_sites_open()}</button>
    </div>
  {/snippet}
</DashboardCard>
