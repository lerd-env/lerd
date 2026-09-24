<script lang="ts">
  import EmptyState from './EmptyState.svelte';
  import StreamingToggle from './StreamingToggle.svelte';
  import { status } from '$stores/status';
  import { m } from '../paraglide/messages.js';

  let { size = 'md' }: { size?: 'sm' | 'md' } = $props();
</script>

{#if $status.streaming_mode}
  <EmptyState title={m.nav_streamingOn()} {size}>
    {#snippet hint()}<StreamingToggle />{/snippet}
  </EmptyState>
{:else}
  <EmptyState title={m.sites_empty()} {size}>
    {#snippet hint()}
      {@html m.sites_emptyHint({ cmd: '<code class="bg-gray-100 dark:bg-white/5 px-1 rounded-sm">lerd park</code>' })}
    {/snippet}
  </EmptyState>
{/if}
