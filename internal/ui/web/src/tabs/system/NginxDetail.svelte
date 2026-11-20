<script lang="ts">
  import DetailPanel from '$components/DetailPanel.svelte';
  import DetailHeader from '$components/DetailHeader.svelte';
  import DetailTabs, { type TabItem } from '$components/DetailTabs.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import CheckUpdatesButton from '$components/CheckUpdatesButton.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import LogViewer from '$components/LogViewer.svelte';
  import { logHighlight } from '$lib/logHighlight';
  import { notifyLocalFailure, notifyLocalInfo } from '$lib/notify';
  import NginxConfigTab from './NginxConfigTab.svelte';
  import { status, updateNginx, checkNginxUpdate, loadStatus, goWebServer, switchWebServer } from '$stores/status';
  import ConfirmModal from '$components/ConfirmModal.svelte';
  import { accessMode } from '$stores/accessMode';
  import { m } from '../../paraglide/messages.js';

  type TabId = 'logs' | 'config';
  let active = $state<TabId>('logs');
  const tabs: TabItem<TabId>[] = [
    { id: 'logs', label: m.services_tabs_logs() },
    { id: 'config', label: m.services_tabs_tuning() }
  ];

  let updating = $state(false);
  let confirmSwitch = $state(false);
  let switching = $state(false);

  async function applySwitch() {
    switching = true;
    try {
      const res = await switchWebServer($goWebServer ? 'nginx' : 'go');
      if (!res.ok) notifyLocalFailure('update_check', m.system_webServer_switchFailed(), res.error || m.common_failed());
      await loadStatus();
    } finally {
      switching = false;
      confirmSwitch = false;
    }
  }
  let checking = $state(false);

  async function applyUpdate() {
    updating = true;
    try {
      const res = await updateNginx();
      if (!res.ok) notifyLocalFailure('update_check', m.system_nginx(), res.error || m.common_failed());
      else await loadStatus();
    } finally {
      updating = false;
    }
  }

  async function runCheck() {
    checking = true;
    try {
      const res = await checkNginxUpdate();
      if (res.ok) await loadStatus();
      notifyLocalInfo(
        'update_check',
        m.system_nginx(),
        !res.ok
          ? m.system_tools_checkFailed()
          : res.update_available
            ? m.system_tools_checkFound()
            : m.system_nginx_checkUpToDate()
      );
    } finally {
      checking = false;
    }
  }
</script>

{#snippet trailing()}
  <div class="flex items-center gap-2">
    {#if $status.nginx.update_available && $accessMode.localControl && !$goWebServer}
      <DetailButton tone="success" onclick={applyUpdate} loading={updating} disabled={updating} title={m.system_nginx_updateHint()}>
        {m.services_update()}
      </DetailButton>
    {/if}
    {#if $accessMode.localControl}
      <DetailButton onclick={() => (confirmSwitch = true)}>{m.system_webServer_change()}</DetailButton>
    {/if}
    {#if $accessMode.localControl && !$goWebServer}
      <CheckUpdatesButton onclick={runCheck} {checking} title={m.system_nginx_checkTitle()} />
    {/if}
    <StatusPill
      tone={$status.nginx.running ? 'ok' : 'error'}
      label={$status.nginx.running ? m.common_running() : m.common_stopped()}
    />
  </div>
{/snippet}

<DetailPanel>
  <DetailHeader title={$goWebServer ? m.system_webServer() : m.system_nginx()} {trailing} />
  <DetailTabs {tabs} {active} onchange={(id) => (active = id)} />
  {#if active === 'config'}
    <!-- Keyed so switching servers reloads the other file. -->
    {#key $goWebServer}
      <NginxConfigTab target={$goWebServer ? 'web' : 'nginx'} />
    {/key}
  {:else}
    <LogViewer path={$goWebServer ? '/api/logs/lerd-web' : '/api/logs/lerd-nginx'} highlight={logHighlight} />
  {/if}
</DetailPanel>

<ConfirmModal
  open={confirmSwitch}
  title={m.system_webServer_change()}
  body={$goWebServer ? m.system_webServer_toNginx() : m.system_webServer_toGo()}
  confirmLabel={m.system_webServer_switch()}
  loading={switching}
  size="md"
  onconfirm={applySwitch}
  onclose={() => (confirmSwitch = false)}
/>
