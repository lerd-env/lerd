<script lang="ts">
  import { hostMysql, setMysqlPublishedPort } from '$stores/dbBackend';
  import { m } from '../../paraglide/messages.js';

  // The conventional alternative port lerd-mysql moves to so the host system
  // MySQL can keep 127.0.0.1:3306. Users can pick another via the advanced field.
  const ALT_PORT = 3307;

  const probe = $derived($hostMysql);
  const present = $derived(probe?.socket_present ?? false);
  const lerdPort = $derived(probe?.lerd_port ?? 3306);
  const moved = $derived(lerdPort !== 3306);
  // A host server exists and lerd-mysql still sits on 3306 → they'll fight for it.
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
    const res = await setMysqlPublishedPort(port);
    busy = false;
    if (!res.ok) error = res.error || 'Failed to change the port';
  }

  function move() {
    apply(ALT_PORT, m.services_mysqlPort_moveConfirm({ port: ALT_PORT }));
  }

  function saveAdvanced() {
    if (!Number.isInteger(portInput) || portInput < 1 || portInput > 65535) {
      error = m.services_mysqlPort_invalid();
      return;
    }
    if (portInput === lerdPort) return;
    apply(portInput, m.services_mysqlPort_saveConfirm({ port: portInput }));
  }

  function reset() {
    apply(0, m.services_mysqlPort_resetConfirm());
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
        {m.services_mysqlPort_conflictTitle()}
      </p>
      <p class="text-[11px] text-amber-700 dark:text-amber-300/80 mt-0.5">
        {m.services_mysqlPort_conflictBody()}
      </p>
      <button
        onclick={move}
        disabled={busy}
        class="mt-2 inline-flex items-center rounded-md bg-amber-600 px-2.5 py-1 text-[11px] font-medium text-white hover:bg-amber-700 disabled:opacity-50"
      >
        {m.services_mysqlPort_moveButton({ port: ALT_PORT })}
      </button>
    {:else}
      <p class="text-xs font-semibold text-emerald-900 dark:text-emerald-200">
        {m.services_mysqlPort_coexistTitle()}
      </p>
      <p class="text-[11px] text-emerald-700 dark:text-emerald-300/80 mt-0.5 font-mono">
        {m.services_mysqlPort_lerdLine({ port: lerdPort })}
      </p>
      {#if present}
        <p class="text-[11px] text-emerald-700 dark:text-emerald-300/80 font-mono">
          {m.services_mysqlPort_hostLine()}
        </p>
      {/if}
    {/if}

    <!-- Advanced: set a specific published port, or reset to the default. -->
    <div class="mt-2 flex items-center gap-2 flex-wrap">
      <label class="text-[11px] text-stone-500 dark:text-stone-400" for="mysql-pub-port">
        {m.services_mysqlPort_advancedLabel()}
      </label>
      <input
        id="mysql-pub-port"
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
        {m.services_mysqlPort_save()}
      </button>
      {#if moved}
        <button
          onclick={reset}
          disabled={busy}
          class="text-[11px] text-stone-500 hover:text-stone-700 dark:text-stone-400 dark:hover:text-stone-200 underline disabled:opacity-50"
        >
          {m.services_mysqlPort_reset()}
        </button>
      {/if}
    </div>

    {#if error}
      <p class="mt-1.5 text-[11px] text-red-600 dark:text-red-400">{error}</p>
    {/if}
  </div>
{/if}
