<script lang="ts">
  import Toggle from '$components/Toggle.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { accessMode } from '$stores/accessMode';
  import { m } from '../paraglide/messages.js';

  // A setting's switch where this browser may change it, and a read-only pill
  // where it may not (a remote dashboard without full access).
  let {
    on,
    loading = false,
    onclick,
    titleOn,
    titleOff
  }: { on: boolean; loading?: boolean; onclick: () => void; titleOn: string; titleOff: string } = $props();
</script>

{#if $accessMode.localControl}
  <Toggle {on} {loading} {onclick} title={on ? titleOff : titleOn} />
{:else}
  <StatusPill size="sm" tone={on ? 'ok' : 'muted'} label={on ? m.common_enabled() : m.common_disabled()} />
{/if}
