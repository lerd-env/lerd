<script lang="ts">
  import DashboardCard from './DashboardCard.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import Icon from '$components/Icon.svelte';
  import InstalledServiceTile from '$tabs/services/InstalledServiceTile.svelte';
  import { coreServices, servicesLoaded, type Service } from '$stores/services';
  import { openPresetModal } from '$stores/modals';
  import { goToTab } from '$stores/route';
  import { accessMode } from '$stores/accessMode';
  import { m } from '../../paraglide/messages.js';

  const total = $derived($coreServices.length);
  const running = $derived($coreServices.filter((s) => s.status === 'active').length);
  // A sleeping service is healthy, just stopped until the next request, so it
  // gets its own badge and does not turn the running count amber.
  const asleep = $derived($coreServices.filter((s) => s.status !== 'active' && s.idle_suspended).length);
  const awake = $derived(running + asleep);
  const updates = $derived($coreServices.filter((s) => s.update_available).length);

  // The card scrolls once the list outgrows it, so a service with an update
  // can sit below the fold. Float those to the top and the tile's own arrow
  // is on screen without a banner repeating it. Below them the busiest
  // services lead, on the count of sites wired to each; the sort is stable,
  // so an equal count keeps the order the store hands over.
  const rank = (s: Service) => (s.update_available ? 0 : 1);
  const sorted = $derived(
    [...$coreServices].sort(
      (a, b) => rank(a) - rank(b) || (b.site_count ?? 0) - (a.site_count ?? 0)
    )
  );
</script>

<DashboardCard title={m.dashboard_services_title()} tone={updates > 0 ? 'warn' : 'default'}>
  {#snippet badge()}
    {#if $servicesLoaded}
      <!-- Three pills outgrow a narrow card, so they stay compact and wrap onto
           a second row rather than squeezing their labels onto two lines. -->
      <div class="flex flex-wrap items-center justify-end gap-1.5 min-w-0">
        <StatusPill
          size="sm"
          tone={total === 0 ? 'muted' : awake === total ? 'ok' : awake > 0 ? 'warn' : 'error'}
          label={m.dashboard_services_summary({ running, total })}
        />
        {#if asleep > 0}
          <StatusPill size="sm" tone="asleep" label={m.dashboard_services_asleep({ count: asleep, total })} />
        {/if}
        {#if updates > 0}
          <StatusPill size="sm" tone="warn" label={m.dashboard_services_updates({ count: updates })} />
        {/if}
      </div>
    {/if}
  {/snippet}

  {#if $servicesLoaded && total === 0}
    <p class="text-sm text-gray-500 dark:text-gray-400">{m.dashboard_services_empty()}</p>
  {:else}
    <div class="space-y-1.5">
      {#each sorted as svc (svc.name)}
        <InstalledServiceTile {svc} compact />
      {/each}
    </div>
  {/if}

  {#snippet footer()}
    <div class="flex flex-wrap items-center gap-2">
      {#if $accessMode.localControl}
        <button
          onclick={openPresetModal}
          class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-lerd-red hover:bg-lerd-redhov text-lerd-onred transition-colors"
        >
          <Icon name="plus" class="w-3.5 h-3.5" />
          {m.dashboard_services_add()}
        </button>
      {/if}
      <button
        onclick={() => goToTab('services')}
        class="ml-auto text-xs font-medium text-lerd-red hover:text-lerd-redhov"
      >{m.dashboard_services_open()}</button>
    </div>
  {/snippet}
</DashboardCard>
