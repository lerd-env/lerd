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
  const updates = $derived($coreServices.filter((s) => s.update_available).length);

  // The card scrolls once the list outgrows it, so a service with an update
  // can sit below the fold. Float those to the top and the tile's own arrow
  // is on screen without a banner repeating it. Sort is stable, so the rest
  // keeps the order the store hands over.
  const rank = (s: Service) => (s.update_available ? 0 : 1);
  const sorted = $derived([...$coreServices].sort((a, b) => rank(a) - rank(b)));
</script>

<DashboardCard title={m.dashboard_services_title()} tone={updates > 0 ? 'warn' : 'default'}>
  {#snippet badge()}
    {#if $servicesLoaded}
      <div class="flex items-center gap-1.5">
        <StatusPill
          tone={total === 0 ? 'muted' : running === total ? 'ok' : running > 0 ? 'warn' : 'error'}
          label={m.dashboard_services_summary({ running, total })}
        />
        {#if updates > 0}
          <StatusPill tone="warn" label={m.dashboard_services_updates({ count: updates })} />
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
