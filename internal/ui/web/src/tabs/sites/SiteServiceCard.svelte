<script lang="ts">
  import ServiceCardShell from '$components/ServiceCardShell.svelte';
  import ServiceDashboardButton from '$components/ServiceDashboardButton.svelte';
  import ServiceIcon from '$components/ServiceIcon.svelte';
  import { services, serviceLabel } from '$stores/services';
  import { goToTab } from '$stores/route';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    name: string;
  }
  let { name }: Props = $props();

  const svc = $derived($services.find((s) => s.name === name));
  const active = $derived(svc?.status === 'active');
</script>

<ServiceCardShell compact>
  <button
    type="button"
    onclick={() => goToTab('services', name)}
    title={'Open ' + serviceLabel(name)}
    class="flex min-w-0 flex-1 items-center gap-2.5 text-left"
  >
    <ServiceIcon {name} compact />
    <span class="min-w-0 flex-1">
      <span class="block text-xs font-semibold text-gray-800 dark:text-gray-100 truncate">{serviceLabel(name)}</span>
      <span class="flex items-center gap-1 text-[10px] text-gray-500 dark:text-gray-400">
        <span class="w-1.5 h-1.5 rounded-full {active ? 'bg-emerald-500' : 'bg-gray-400 dark:bg-gray-600'}"></span>
        {active ? m.common_running() : m.common_stopped()}
      </span>
    </span>
  </button>
  <ServiceDashboardButton {name} />
</ServiceCardShell>
