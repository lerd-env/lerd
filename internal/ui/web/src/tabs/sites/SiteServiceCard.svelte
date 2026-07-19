<script lang="ts">
  import ServiceCardShell from '$components/ServiceCardShell.svelte';
  import ServiceDashboardButton from '$components/ServiceDashboardButton.svelte';
  import ServiceIcon from '$components/ServiceIcon.svelte';
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { services, serviceLabel } from '$stores/services';
  import { databaseAdminFor, openDatabaseAdmin } from '$stores/dashboard';
  import { goToTab } from '$stores/route';
  import { openServiceInstallModal } from '$stores/modals';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    name: string;
    database?: string;
  }
  let { name, database = '' }: Props = $props();

  const svc = $derived($services.find((s) => s.name === name));
  const installed = $derived(Boolean(svc));
  const active = $derived(svc?.status === 'active');

  // For a database engine whose admin tool is installed, the card opens that
  // admin straight to this site's database instead of the tool's root.
  const dbAdmin = $derived.by(() => {
    void $services;
    if (!svc?.is_database || !database) return null;
    return databaseAdminFor(name);
  });

  // A service listed in .lerd.yaml but absent from the installed set was never
  // installed. Offer to install it rather than routing to a Services tab entry
  // that isn't there.
  function open() {
    if (installed) goToTab('services', name);
    else openServiceInstallModal(name);
  }

  const dot = $derived(
    !installed ? 'bg-amber-500' : active ? 'bg-emerald-500' : 'bg-gray-400 dark:bg-gray-600'
  );
  const status = $derived(
    !installed ? m.services_notInstalled() : active ? m.common_running() : m.common_stopped()
  );
</script>

<ServiceCardShell compact>
  <button
    type="button"
    onclick={open}
    title={installed ? 'Open ' + serviceLabel(name) : m.services_install_tooltip({ name: serviceLabel(name) })}
    class="flex min-w-0 flex-1 items-center gap-2.5 text-left"
  >
    <ServiceIcon {name} compact />
    <span class="min-w-0 flex-1">
      <span class="block text-xs font-semibold text-gray-800 dark:text-gray-100 truncate">{serviceLabel(name)}</span>
      <span class="flex items-center gap-1 text-[10px] text-gray-500 dark:text-gray-400">
        <span class="w-1.5 h-1.5 rounded-full {dot}"></span>
        {status}
      </span>
    </span>
  </button>
  {#if dbAdmin}
    <button
      type="button"
      use:tooltip={m.databases_openIn({ name: serviceLabel(dbAdmin.name) })}
      aria-label={m.databases_openIn({ name: serviceLabel(dbAdmin.name) })}
      onclick={() => openDatabaseAdmin(name, database)}
      class="shrink-0 flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
    >
      <Icon name="database" class="w-3.5 h-3.5" />
    </button>
  {:else if installed}
    <ServiceDashboardButton {name} />
  {/if}
</ServiceCardShell>
