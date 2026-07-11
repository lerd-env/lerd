<script lang="ts">
  import StatusDot from '$components/StatusDot.svelte';
  import Icon from '$components/Icon.svelte';
  import ServiceCardShell from '$components/ServiceCardShell.svelte';
  import ServiceDashboardButton from '$components/ServiceDashboardButton.svelte';
  import ServiceIcon from '$components/ServiceIcon.svelte';
  import { goToTab } from '$stores/route';
  import { serviceLabel, type Service } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();
</script>

<ServiceCardShell>
  <button
    type="button"
    onclick={() => goToTab('services', svc.name)}
    class="flex min-w-0 flex-1 items-center gap-3 text-left"
  >
    <ServiceIcon name={svc.name} category={svc.category} icon={svc.icon} />
    <span class="min-w-0 flex-1">
      <span class="flex items-center gap-1.5">
        <span class="truncate text-sm font-semibold text-gray-900 dark:text-white">{serviceLabel(svc.name)}</span>
        {#if svc.update_available}
          <span
            class="shrink-0 text-[10px] font-medium text-emerald-600 dark:text-emerald-400"
            title={svc.latest_version ? m.services_updateAvailableTo({ tag: svc.latest_version }) : m.services_updateAvailable()}
          >↑</span>
        {/if}
      </span>
      <span class="flex items-center gap-2 text-[10px] text-gray-500 dark:text-gray-400">
        <span class="flex items-center gap-1">
          <StatusDot color={svc.status === 'active' ? 'green' : 'gray'} />
          {svc.status === 'active' ? m.common_running() : m.common_stopped()}
        </span>
        {#if svc.version}
          <span class="truncate font-mono tabular-nums text-gray-400 dark:text-gray-500">{svc.version}</span>
        {/if}
        {#if svc.site_count > 0}
          <span class="inline-flex shrink-0 items-center gap-1 tabular-nums" title={m.common_sites()}>
            <Icon name="sites" class="w-3 h-3" />
            {svc.site_count}
          </span>
        {/if}
      </span>
    </span>
  </button>
  <ServiceDashboardButton name={svc.name} />
</ServiceCardShell>
