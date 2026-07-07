<script lang="ts">
  import { untrack } from 'svelte';
  import PortsEditor from '$components/PortsEditor.svelte';
  import { status } from '$stores/status';
  import { setFpmPorts } from '$stores/phpVersions';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    version: string;
  }
  let { version }: Props = $props();

  const current = $derived($status.php_fpms.find((f) => f.version === version)?.ports ?? []);

  // Env-wide FPM ports apply immediately: adding or removing a card persists and
  // restarts the version's FPM right away, so there is no Save step to forget.
  let ports = $state<string[]>([]);
  let saving = $state(false);
  let error = $state('');

  // Reseed on version change or a broadcast. `saving` is read untracked so the
  // save's own saving->false transition never re-runs this (the old flicker),
  // and a mid-save broadcast is skipped so it can't clobber the optimistic list.
  $effect(() => {
    version;
    const c = current;
    if (untrack(() => saving)) return;
    ports = [...c];
    error = '';
  });

  async function persist(next: string[]) {
    ports = next; // optimistic; reconciled with the resolved set below
    saving = true;
    error = '';
    try {
      const res = await setFpmPorts(version, next);
      if (!res.ok) {
        error = res.error || m.common_failed();
        ports = [...current]; // roll back to the last persisted set
        return;
      }
      if (res.ports) ports = [...res.ports];
    } finally {
      saving = false;
    }
  }
</script>

<div class="flex flex-col h-full">
  <div class="flex-1 overflow-y-auto p-3 sm:p-5 space-y-4">
    <div class="flex items-center gap-2">
      <span class="text-sm font-medium text-gray-800 dark:text-gray-200">
        {m.system_php_ports_title()}
      </span>
      {#if saving}
        <span class="text-[11px] text-gray-400 dark:text-gray-500">{m.services_ports_applying()}</span>
      {/if}
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400 -mt-2">
      {m.system_php_ports_help({ version })}
    </p>

    <PortsEditor
      {ports}
      disabled={saving}
      empty={m.system_php_ports_empty()}
      onadd={(spec) => persist([...ports, spec])}
      onremove={(spec) => persist(ports.filter((p) => p !== spec))}
    />

    {#if error}
      <p class="text-xs text-red-500">{error}</p>
    {/if}
  </div>
</div>
