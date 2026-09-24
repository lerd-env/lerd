<script lang="ts">
  import DashboardCard from './DashboardCard.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import StreamingToggle from '$components/StreamingToggle.svelte';
  import SitesEmptyState from '$components/SitesEmptyState.svelte';
  import Icon from '$components/Icon.svelte';
  import SiteTile from '$tabs/sites/SiteTile.svelte';
  import { sites, sitesLoaded, siteWorkerFailing } from '$stores/sites';
  import { status } from '$stores/status';
  import { openLinkModal } from '$stores/modals';
  import { goToTab } from '$stores/route';
  import { accessMode } from '$stores/accessMode';
  import { sitesSort } from '$stores/sitesSort';
  import { sortSites } from '$lib/sitesOrder';
  import { m } from '../../paraglide/messages.js';

  const total = $derived($sites.length);
  const running = $derived($sites.filter((s) => s.fpm_running && !s.paused).length);
  const failing = $derived($sites.filter((s) => siteWorkerFailing(s)).length);

  // Same order as the Sites tab, so the dashboard never contradicts the list
  // the user arranged. Paused sites stay out; they are only on the Sites tab.
  const sorted = $derived(sortSites($sites.filter((s) => !s.paused), $sitesSort));
</script>

<DashboardCard title={m.dashboard_sites_title()} tone={failing > 0 ? 'critical' : 'default'}>
  {#snippet badge()}
    {#if $sitesLoaded}
      <span class="inline-flex items-center gap-1.5">
        <!-- The empty state carries the toggle once streaming hides every site. -->
        {#if total > 0 || !$status.streaming_mode}<StreamingToggle />{/if}
        <StatusPill
          tone={failing > 0 ? 'error' : running > 0 ? 'ok' : 'muted'}
          label={m.dashboard_sites_summary({ running, total })}
        />
      </span>
    {/if}
  {/snippet}

  {#if $sitesLoaded && total === 0}
    <SitesEmptyState size="sm" />
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
