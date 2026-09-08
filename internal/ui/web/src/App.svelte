<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import NavRail from '$components/NavRail.svelte';
  import SidePanel from '$components/SidePanel.svelte';
  import MobileHeader from '$components/MobileHeader.svelte';
  import MobileNav from '$components/MobileNav.svelte';
  import MobileBackBar from '$components/MobileBackBar.svelte';
  import { tab, routeRest } from '$stores/route';
  import { loadVersion } from '$stores/version';
  import { loadAccessMode, accessMode } from '$stores/accessMode';
  import { loadStatus, statusLoaded, allCoreRunning } from '$stores/status';
  import { loadPhpVersions } from '$stores/phpVersions';
  import { loadNodeVersions } from '$stores/nodeVersions';
  import { loadAutostart, startOnDashboardOpen } from '$stores/autostart';
  import { loadPalettes } from '$stores/palettes';
  import { loadIdle } from '$stores/idle';
  import { loadSites } from '$stores/sites';
  import { loadServices } from '$stores/services';
  import { loadServiceIcons } from '$stores/serviceIcons';
  import { loadFrameworkMarks } from '$stores/frameworkMarks';
  import { loadWorkerMarks } from '$stores/workerMarks';
  import { loadWorkerHealth } from '$stores/workerHealth';
  import { watchActiveRun } from '$stores/wizard';
  import { connectWs, disconnectWs } from '$lib/ws';
  import { initDashboardRoute } from '$stores/dashboard';
  import { autoStartOnce } from '$stores/lerdLifecycle';
  import { handOverToVhost } from '$lib/vhost';
  import '$stores/activity';
  import { mobileView } from '$stores/mobileView';
  import ModalHost from './modals/ModalHost.svelte';
  import DashboardOverlay from '$components/DashboardOverlay.svelte';
  import WorkerHealthBanner from '$components/WorkerHealthBanner.svelte';
  import NotifyBanner from '$components/NotifyBanner.svelte';
  import NotificationToasts from '$components/NotificationToasts.svelte';
  import WizardBubble from '$components/WizardBubble.svelte';
  import CommandPalette from '$components/CommandPalette.svelte';
  import CommandRunModal from '$components/CommandRunModal.svelte';
  import { initNotify } from '$lib/notify';

  import SitesTab from '$tabs/SitesTab.svelte';
  import ServicesTab from '$tabs/ServicesTab.svelte';
  import SystemTab from '$tabs/SystemTab.svelte';
  import SitesDetail from '$tabs/SitesDetail.svelte';
  import ServicesDetail from '$tabs/ServicesDetail.svelte';
  import SystemDetail from '$tabs/SystemDetail.svelte';
  import AppsPage from '$tabs/AppsPage.svelte';
  import DashboardTab from '$tabs/DashboardTab.svelte';

  function handlePageHide() {
    disconnectWs();
  }

  onMount(() => {
    loadVersion();
    loadAccessMode();
    loadStatus();
    loadPhpVersions();
    loadNodeVersions();
    loadAutostart();
    loadPalettes();
    loadIdle();
    loadSites();
    loadServices();
    loadServiceIcons();
    loadFrameworkMarks();
    loadWorkerMarks();
    loadWorkerHealth();
    // A scaffold or setup the wizard sent to the background outlives the page,
    // so the dashboard asks on load whether anything is still going.
    watchActiveRun();
    connectWs();
    initDashboardRoute();
    initNotify();
    window.addEventListener('pagehide', handlePageHide);
    // 127.0.0.1:7073 is the fallback for a stopped stack. Once the vhost is
    // back, move onto it rather than settle on a second origin that would ask
    // for its own notification permission and push subscription.
    void handOverToVhost();
  });

  onDestroy(() => {
    window.removeEventListener('pagehide', handlePageHide);
    disconnectWs();
  });

  // With "start when the dashboard opens" on, finding the stack down is what
  // brings it up, so opening the app is enough and no terminal is needed.
  // autoStartOnce keeps this to a single attempt per page load.
  $effect(() => {
    if ($startOnDashboardOpen && $accessMode.localControl && $statusLoaded && !$allCoreRunning) {
      void autoStartOnce();
    }
  });

  // On mobile, show the detail pane once an item is selected (routeRest non-empty).
  // System tab always has a default selection (lerd) so we only show detail there
  // if the user explicitly picked something, to avoid jumping past the list.
  const showMobileDetail = $derived(Boolean($routeRest));
  const onApps = $derived($mobileView === 'apps');
  const onDashboard = $derived($tab === 'dashboard');
</script>

<div class="h-screen flex">
  <NavRail />

  {#if !onDashboard}
    <SidePanel>
      {#if $tab === 'sites'}
        <SitesTab />
      {:else if $tab === 'services'}
        <ServicesTab />
      {:else if $tab === 'system'}
        <SystemTab />
      {/if}
    </SidePanel>
  {/if}

  <main class="flex-1 flex flex-col overflow-hidden">
    {#if !showMobileDetail}
      <MobileHeader />
    {/if}

    <div class="hidden md:flex flex-col flex-1 overflow-hidden">
      {#if $tab === 'dashboard'}
        <DashboardTab />
      {:else if $tab === 'sites'}
        <SitesDetail />
      {:else if $tab === 'services'}
        <ServicesDetail />
      {:else if $tab === 'system'}
        <SystemDetail />
      {/if}
    </div>

    {#if onApps}
      <div class="md:hidden flex-1 flex flex-col overflow-hidden pb-16">
        <AppsPage />
      </div>
    {:else if onDashboard}
      <div class="md:hidden flex-1 overflow-y-auto pb-16">
        <DashboardTab />
      </div>
    {:else if !showMobileDetail}
      <div class="md:hidden flex-1 overflow-y-auto pb-16">
        {#if $tab === 'sites'}
          <SitesTab />
        {:else if $tab === 'services'}
          <ServicesTab />
        {:else if $tab === 'system'}
          <SystemTab />
        {/if}
      </div>
    {:else}
      <div class="md:hidden flex-1 flex flex-col overflow-hidden pb-16">
        <MobileBackBar />
        {#if $tab === 'sites'}
          <SitesDetail />
        {:else if $tab === 'services'}
          <ServicesDetail />
        {:else if $tab === 'system'}
          <SystemDetail />
        {/if}
      </div>
    {/if}
  </main>

  <MobileNav />
  <ModalHost />
  <DashboardOverlay />
  <WorkerHealthBanner />
  <NotifyBanner />
  <NotificationToasts />
  <WizardBubble />
  <CommandPalette />
  <CommandRunModal />
</div>
