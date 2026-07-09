<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { services, serviceLabel } from '$stores/services';
  import { openServiceDashboard } from '$stores/dashboard';
  import { adminServiceFor } from '$stores/presetSuggestions';
  import { m } from '../paraglide/messages.js';

  interface Props {
    name: string;
  }
  let { name }: Props = $props();

  const svc = $derived($services.find((s) => s.name === name));
  const active = $derived(svc?.status === 'active');
  // A service with no dashboard of its own (mysql, redis) is reached through
  // its suggested admin tool, the same resolution the service page uses.
  const target = $derived(
    !svc ? undefined : svc.dashboard ? svc : (adminServiceFor(svc, $services) ?? undefined)
  );
  const label = $derived(
    target && target.name !== name
      ? m.services_openAdmin({ name: serviceLabel(target.name) })
      : m.services_dashboard()
  );
</script>

{#if active && target?.dashboard}
  <button
    type="button"
    onclick={() => openServiceDashboard(target)}
    use:tooltip={label}
    aria-label={label}
    class="shrink-0 flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
  >
    <Icon name="external" class="w-3.5 h-3.5" />
  </button>
{/if}
