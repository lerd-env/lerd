<script lang="ts">
  import { hostDB, setPublishedPort } from '$stores/dbBackend';
  import { dbEngineDisplay, type Service } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();

  // The lerd container managing this DB service and its engine display name,
  // used in the coexistence copy (e.g. "lerd-postgres", "PostgreSQL").
  const container = $derived(`lerd-${svc.name}`);
  const engine = $derived(dbEngineDisplay(svc.name));

  const probe = $derived($hostDB);
  // The engine's canonical port (3306 MySQL, 5432 Postgres), from the probe.
  const defaultPort = $derived(probe?.port ?? 3306);
  // The conventional alternative port lerd moves to so the host server can keep
  // the default (3307 / 5433). Users can pick another via the advanced field.
  const altPort = $derived(defaultPort + 1);

  const present = $derived(probe?.socket_present ?? false);
  const lerdPort = $derived(probe?.lerd_port ?? defaultPort);
  const moved = $derived(lerdPort !== defaultPort);
  // A host server exists and lerd still sits on the default port → they'll fight for it.
  const conflict = $derived(present && !moved);
  // Show whenever there's something to manage: a conflict, or a moved port.
  const visible = $derived(moved || present);

  let busy = $state(false);
  let error = $state('');
  // The advanced numeric field; seeded from the effective port.
  let portInput = $state(0);
  $effect(() => {
    portInput = lerdPort;
  });

  async function apply(port: number, confirmMsg: string) {
    if (busy) return;
    if (!confirm(confirmMsg)) return;
    busy = true;
    error = '';
    const res = await setPublishedPort(svc.name, port);
    busy = false;
    if (!res.ok) error = res.error || 'Failed to change the port';
  }

  function move() {
    apply(altPort, m.services_dbPort_moveConfirm({ container, port: altPort }));
  }

  function saveAdvanced() {
    if (!Number.isInteger(portInput) || portInput < 1 || portInput > 65535) {
      error = m.services_dbPort_invalid();
      return;
    }
    if (portInput === lerdPort) return;
    apply(portInput, m.services_dbPort_saveConfirm({ container, port: portInput }));
  }

  function reset() {
    apply(0, m.services_dbPort_resetConfirm({ container, port: defaultPort }));
  }
</script>

{#if visible}
  <div
    class="rounded-lg border border-l-4 px-3 py-2.5 {conflict
      ? 'border-amber-300 dark:border-amber-500/40 border-l-amber-500 bg-amber-50/70 dark:bg-amber-500/10'
      : 'border-emerald-300 dark:border-emerald-500/40 border-l-emerald-500 bg-emerald-50/60 dark:bg-emerald-500/10'}"
  >
    {#if conflict}
      <p class="text-xs font-semibold text-amber-900 dark:text-amber-200">
        {m.services_dbPort_conflictTitle()}
      </p>
      <p class="text-[11px] text-amber-700 dark:text-amber-300/80 mt-0.5">
        {m.services_dbPort_conflictBody({ container, engine, port: defaultPort })}
      </p>
      <button
        onclick={move}
        disabled={busy}
        class="mt-2 inline-flex items-center rounded-md bg-amber-600 px-2.5 py-1 text-[11px] font-medium text-white hover:bg-amber-700 disabled:opacity-50"
      >
        {m.services_dbPort_moveButton({ container, port: altPort })}
      </button>
    {:else}
      <p class="text-xs font-semibold text-emerald-900 dark:text-emerald-200">
        {m.services_dbPort_coexistTitle({ engine })}
      </p>
      <p class="text-[11px] text-emerald-700 dark:text-emerald-300/80 mt-0.5 font-mono">
        {m.services_dbPort_lerdLine({ container, port: lerdPort })}
      </p>
      {#if present}
        <p class="text-[11px] text-emerald-700 dark:text-emerald-300/80 font-mono">
          {m.services_dbPort_hostLine({ engine, port: defaultPort })}
        </p>
      {/if}
    {/if}

    <!-- Advanced: set a specific published port, or reset to the default. -->
    <div class="mt-2 flex items-center gap-2 flex-wrap">
      <label class="text-[11px] text-stone-500 dark:text-stone-400" for="db-pub-port">
        {m.services_dbPort_advancedLabel()}
      </label>
      <input
        id="db-pub-port"
        type="number"
        min="1"
        max="65535"
        bind:value={portInput}
        disabled={busy}
        class="w-20 rounded border border-stone-300 dark:border-stone-600 bg-white dark:bg-stone-800 px-1.5 py-0.5 text-[11px] font-mono disabled:opacity-50"
      />
      <button
        onclick={saveAdvanced}
        disabled={busy || portInput === lerdPort}
        class="rounded-md border border-stone-300 dark:border-stone-600 px-2 py-0.5 text-[11px] font-medium hover:bg-stone-100 dark:hover:bg-stone-700 disabled:opacity-50"
      >
        {m.services_dbPort_save()}
      </button>
      {#if moved}
        <button
          onclick={reset}
          disabled={busy}
          class="text-[11px] text-stone-500 hover:text-stone-700 dark:text-stone-400 dark:hover:text-stone-200 underline disabled:opacity-50"
        >
          {m.services_dbPort_reset({ port: defaultPort })}
        </button>
      {/if}
    </div>

    {#if error}
      <p class="mt-1.5 text-[11px] text-red-600 dark:text-red-400">{error}</p>
    {/if}
  </div>
{/if}
