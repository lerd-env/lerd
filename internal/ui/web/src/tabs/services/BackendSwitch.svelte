<script lang="ts">
  import ToggleButton from '$components/ToggleButton.svelte';
  import DivergenceConfirmModal from './DivergenceConfirmModal.svelte';
  import { sites, setSiteDBBackend, loadSites } from '$stores/sites';
  import { hostDB } from '$stores/dbBackend';
  import { dbEngineDisplay, dbServiceUnit } from '$stores/services';
  import { accessMode } from '$stores/accessMode';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    domain: string;
  }
  let { domain }: Props = $props();

  const site = $derived($sites.find((s) => s.domain === domain));
  const current = $derived<'host' | 'container'>(site?.db_external ? 'host' : 'container');
  // Reachable unless a probe explicitly says the host server isn't live. null
  // (not probed) is treated as reachable — the server still validates.
  const hostReachable = $derived($hostDB ? $hostDB.live : true);
  // Human-readable engine name + systemd unit for the host-backend tooltips, from
  // the probe (so a Postgres site says "PostgreSQL"/"postgresql", not "MySQL").
  const hostEngine = $derived(dbEngineDisplay($hostDB?.service_name ?? 'mysql'));
  const hostUnit = $derived(dbServiceUnit($hostDB?.service_name ?? 'mysql'));
  // Host backend is loopback-only; on a confirmed LAN dashboard the server rejects
  // it, so don't offer it there (matches the global toggle in ServiceHeader).
  const hostLocked = $derived($accessMode.checked && !$accessMode.loopback);
  // Block switching TO host when it's locked or unreachable; never block leaving it.
  const hostBlocked = $derived((hostLocked || !hostReachable) && current !== 'host');

  let busy = $state(false);
  let confirmTarget = $state<'host' | 'container' | null>(null);

  function request(target: 'host' | 'container') {
    if (busy || target === current) return;
    confirmTarget = target;
  }

  async function doSwitch(): Promise<{ ok: boolean; error?: string }> {
    if (!confirmTarget) return { ok: true };
    busy = true;
    try {
      const res = await setSiteDBBackend(domain, confirmTarget);
      if (res.ok) await loadSites();
      return res;
    } finally {
      busy = false;
    }
  }
</script>

<span class="inline-flex items-center">
  <ToggleButton
    label={m.services_backend_lerd()}
    on={current === 'container'}
    loading={busy && confirmTarget === 'container'}
    disabled={busy}
    rounding="rounded-l-md border-r-0"
    title={m.services_backend_lerdTitle({ domain, engine: hostEngine })}
    onclick={() => request('container')}
  />
  <ToggleButton
    label={m.services_backend_host()}
    on={current === 'host'}
    loading={busy && confirmTarget === 'host'}
    disabled={busy || hostBlocked}
    rounding="rounded-r-md"
    title={hostLocked && current !== 'host'
      ? m.services_hostDB_loopbackOnly({ engine: hostEngine })
      : hostReachable || current === 'host'
        ? m.services_backend_hostTitle({ domain, engine: hostEngine })
        : m.services_hostSetup_subtitle({ engine: hostEngine, unit: hostUnit })}
    onclick={() => request('host')}
  />
</span>

<DivergenceConfirmModal
  open={confirmTarget !== null}
  {domain}
  target={confirmTarget ?? 'host'}
  engine={hostEngine}
  onclose={() => (confirmTarget = null)}
  onconfirm={doSwitch}
/>
