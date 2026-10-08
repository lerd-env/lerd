<script lang="ts">
  import Dropdown from '$components/Dropdown.svelte';
  import SettingsCard from '$components/SettingsCard.svelte';
  import { status as dumpsStatusValue, setDumpsBuffer } from '$stores/dumps';
  import { m } from '../../paraglide/messages.js';

  // The Debug window's own settings: how many events lerd-ui keeps for the
  // lenses.

  // Buffer sizes offered, from the floor to the ceiling lerd-ui allows, each
  // with what it costs at about 4.5 KB an event.
  const BUFFER_SIZES = [3000, 5000, 10000, 15000, 20000];
  // A size set from the CLI or MCP is listed too, so the menu shows it.
  const bufferOptions = $derived(
    [...new Set([...BUFFER_SIZES, $dumpsStatusValue?.capacity ?? 0])]
      .filter((n) => n > 0)
      .sort((a, b) => a - b)
      .map((n) => ({ value: String(n), label: m.dumps_bridge_bufferOption({ count: n.toLocaleString(), mb: Math.floor((n * 4.5) / 1000) }) }))
  );
  let bufferError = $state('');
  async function resizeBuffer(v: string) {
    bufferError = '';
    try {
      await setDumpsBuffer(Number(v));
    } catch (e) {
      bufferError = e instanceof Error ? e.message : String(e);
    }
  }
</script>

<!-- Laid out like the System > Lerd settings: one card per setting, the title
     and its control on one row and what it does underneath. -->
<div class="p-3 @container">
  <div class="grid grid-cols-1 @3xl:grid-cols-2 gap-3">
    {#if $dumpsStatusValue?.capacity}
      <SettingsCard>
        <div class="flex items-center justify-between gap-4 mb-2">
          <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.dumps_bridge_buffer()}</span>
          <Dropdown value={String($dumpsStatusValue.capacity)} options={bufferOptions} onchange={resizeBuffer} minMenuWidth={240} align="right" />
        </div>
        <p class="text-xs {bufferError ? 'text-amber-600 dark:text-amber-400' : 'text-gray-500 dark:text-gray-400'}">{bufferError || m.dumps_bridge_bufferHint()}</p>
      </SettingsCard>
    {/if}
  </div>
</div>
