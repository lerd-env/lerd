<script lang="ts">
  import DashboardCard from './DashboardCard.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import StatusDot from '$components/StatusDot.svelte';
  import Icon from '$components/Icon.svelte';
  import { coreServices, servicesLoaded, serviceLabel } from '$stores/services';
  import { openPresetModal } from '$stores/modals';
  import { goToTab } from '$stores/route';
  import { accessMode } from '$stores/accessMode';
  import { m } from '../../paraglide/messages.js';

  const total = $derived($coreServices.length);
  const running = $derived($coreServices.filter((s) => s.status === 'active').length);
  const updates = $derived($coreServices.filter((s) => s.update_available));
</script>

<DashboardCard title={m.dashboard_services_title()} tone={updates.length > 0 ? 'warn' : 'default'}>
  {#snippet badge()}
    {#if $servicesLoaded}
      <StatusPill
        tone={total === 0 ? 'muted' : running === total ? 'ok' : running > 0 ? 'warn' : 'error'}
        label={m.dashboard_services_summary({ running, total })}
      />
    {/if}
  {/snippet}

  {#if $servicesLoaded && total === 0}
    <p class="text-sm text-gray-500 dark:text-gray-400">{m.dashboard_services_empty()}</p>
  {:else}
    {#if updates.length > 0}
      <button
        onclick={() => goToTab('services', updates[0].name)}
        class="w-full flex items-center justify-between gap-2 text-left text-sm text-yellow-700 dark:text-yellow-400 bg-yellow-50 dark:bg-yellow-500/10 hover:bg-yellow-100 dark:hover:bg-yellow-500/20 border border-yellow-200 dark:border-yellow-500/30 rounded-lg px-3 py-2 transition-colors"
      >
        <span class="inline-flex items-center gap-2">
          <svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"/>
          </svg>
          {m.dashboard_services_updates({ count: updates.length })}
        </span>
        <span class="text-xs">→</span>
      </button>
    {/if}

    <div class="grid grid-cols-2 gap-x-4 gap-y-1.5">
      {#each $coreServices as svc (svc.name)}
        <button
          onclick={() => goToTab('services', svc.name)}
          class="flex items-center gap-2 text-sm text-left text-gray-700 dark:text-gray-300 hover:text-lerd-red transition-colors py-0.5"
        >
          <StatusDot color={svc.status === 'active' ? 'green' : 'gray'} />
          <span class="flex-1 truncate">{serviceLabel(svc.name)}</span>
          {#if svc.version}
            <span class="text-[10px] font-mono text-gray-400 dark:text-gray-500 truncate">{svc.version}</span>
          {/if}
        </button>
      {/each}
    </div>
  {/if}

  {#snippet footer()}
    <div class="flex flex-wrap items-center gap-2">
      {#if $accessMode.loopback}
        <button
          onclick={openPresetModal}
          class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-lerd-red hover:bg-lerd-redhov text-white transition-colors"
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
