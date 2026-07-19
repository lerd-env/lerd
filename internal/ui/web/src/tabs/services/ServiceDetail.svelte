<script lang="ts">
  import DetailPanel from '$components/DetailPanel.svelte';
  import LogViewer from '$components/LogViewer.svelte';
  import DetailTabs, { type TabItem } from '$components/DetailTabs.svelte';
  import ServiceHeader from './ServiceHeader.svelte';
  import ServiceSiteBadges from './ServiceSiteBadges.svelte';
  import ServiceEnvTab from './ServiceEnvTab.svelte';
  import ServiceTuningTab from './ServiceTuningTab.svelte';
  import ServiceToolsTab from './ServiceToolsTab.svelte';
  import ServicePortsTab from './ServicePortsTab.svelte';
  import ServiceDatabasesTab from './ServiceDatabasesTab.svelte';
  import PresetSuggestionBanner from './PresetSuggestionBanner.svelte';
  import { isServiceWorker, type Service } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();

  type TabId = 'databases' | 'logs' | 'env' | 'config' | 'tools' | 'ports';
  // A database engine opens on its Databases tab, since the databases it holds
  // are the primary thing to look at; every other service opens on logs. The
  // default is applied by the effect below on first run (shownService starts
  // empty), so switching services always lands on the right tab.
  let active = $state<TabId>('logs');
  let shownService = $state('');

  const hasEnv = $derived(Boolean(svc.env_vars && Object.keys(svc.env_vars).length > 0));
  const hasTools = $derived(Boolean(svc.client_shims && svc.client_shims.length > 0));
  // Workers publish nothing, so the ports tab tracks the header gear's old guard.
  const hasPorts = $derived(!isServiceWorker(svc));
  const tabs = $derived<TabItem<TabId>[]>([
    { id: 'databases', label: m.databases_title(), hidden: !svc.is_database },
    { id: 'logs', label: m.services_tabs_logs() },
    { id: 'env', label: m.services_env_title(), hidden: !hasEnv },
    { id: 'config', label: m.services_tabs_tuning(), hidden: !svc.tunable },
    { id: 'tools', label: m.services_tabs_tools(), hidden: !hasTools },
    { id: 'ports', label: m.services_tabs_ports(), hidden: !hasPorts }
  ]);

  // Selecting a different service resets to its default tab; within one service
  // the user's tab choice sticks, falling back off any tab hidden for it.
  $effect(() => {
    const fallback: TabId = svc.is_database ? 'databases' : 'logs';
    if (svc.name !== shownService) {
      shownService = svc.name;
      active = fallback;
      return;
    }
    if (active === 'databases' && !svc.is_database) active = fallback;
    if (active === 'env' && !hasEnv) active = fallback;
    if (active === 'config' && !svc.tunable) active = fallback;
    if (active === 'tools' && !hasTools) active = fallback;
    if (active === 'ports' && !hasPorts) active = fallback;
  });

  const logPath = $derived.by(() => {
    if (svc.queue_site) return `/api/queue/${svc.queue_site}/logs`;
    if (svc.horizon_site) return `/api/horizon/${svc.horizon_site}/logs`;
    if (svc.stripe_listener_site) return `/api/stripe/${svc.stripe_listener_site}/logs`;
    if (svc.schedule_worker_site) return `/api/schedule/${svc.schedule_worker_site}/logs`;
    if (svc.reverb_site) return `/api/reverb/${svc.reverb_site}/logs`;
    if (svc.worker_site && svc.worker_name) {
      const site = svc.worker_worktree ? `${svc.worker_site}-${svc.worker_worktree}` : svc.worker_site;
      return `/api/worker/${site}/${svc.worker_name}/logs`;
    }
    return `/api/logs/lerd-${svc.name}`;
  });

  function highlight(line: string): string | null {
    if (/ERROR|Error/.test(line)) return 'text-red-500';
    if (/WARNING|Warning/.test(line)) return 'text-yellow-600 dark:text-yellow-400';
    return null;
  }
</script>

<DetailPanel>
  {#key svc.name}
    <ServiceHeader {svc} />
    <ServiceSiteBadges {svc} />
  {/key}
  <PresetSuggestionBanner {svc} />
  <DetailTabs {tabs} {active} onchange={(id) => (active = id)} />
  {#if active === 'databases'}
    <ServiceDatabasesTab {svc} />
  {:else if active === 'logs'}
    {#key svc.name + ':' + logPath}
      <LogViewer path={logPath} {highlight} />
    {/key}
  {:else if active === 'env'}
    <ServiceEnvTab {svc} />
  {:else if active === 'config'}
    <ServiceTuningTab {svc} />
  {:else if active === 'tools'}
    <ServiceToolsTab {svc} />
  {:else if active === 'ports'}
    <ServicePortsTab {svc} />
  {/if}
</DetailPanel>
