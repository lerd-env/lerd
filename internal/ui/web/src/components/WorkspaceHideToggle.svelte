<script lang="ts">
  import Icon from './Icon.svelte';
  import { status } from '$stores/status';
  import { setWorkspacePrivate } from '$stores/workspaces';
  import { m } from '../paraglide/messages.js';

  // Needs a `group` ancestor: shown while hovering it, and always once the
  // workspace is hidden. Keeps its box when invisible so the heading never
  // shifts, and stops the press so a draggable header doesn't drag.
  interface Props {
    workspace: string;
  }
  let { workspace }: Props = $props();

  const saved = $derived(($status.private_workspaces ?? []).includes(workspace));
  // The flag comes back with the next status snapshot, which lags the click, so
  // the icon shows the pending choice until then and drops it on a refusal.
  let pending = $state<boolean | null>(null);
  $effect(() => {
    saved;
    pending = null;
  });
  const hidden = $derived(pending ?? saved);
  const label = $derived(hidden ? m.sites_showWhileStreaming() : m.sites_hideWhileStreaming());

  async function toggle(e: MouseEvent) {
    e.stopPropagation();
    pending = !hidden;
    const res = await setWorkspacePrivate(workspace, pending);
    if (res.ok) return;
    pending = null;
    console.error('workspace privacy failed:', res.error);
  }
</script>

{#if $status.streaming_enabled}
  <button
    type="button"
    title={label}
    aria-label={label}
    aria-pressed={hidden}
    onclick={toggle}
    onmousedown={(e) => e.stopPropagation()}
    class="flex w-5 h-5 -my-1 shrink-0 items-center justify-center rounded transition-colors {hidden
      ? 'text-lerd-red'
      : 'invisible group-hover:visible focus-visible:visible text-gray-400 hover:text-lerd-red'}"
  >
    <Icon name={hidden ? 'eyeOff' : 'eye'} class="w-3.5 h-3.5" />
  </button>
{/if}
