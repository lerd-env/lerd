<script lang="ts">
  import { version } from '$stores/version';
  import { parseBuildVersion } from '$lib/buildVersion';
  import ChannelBadge from './ChannelBadge.svelte';

  // A pre-release shows its tag and lets the badge carry the rest, as the rail does.
  const base = $derived(parseBuildVersion($version.current).base);
</script>

<span class="inline-flex items-center gap-2">
  <ChannelBadge />
  {#if base}
    <span class="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full bg-gray-100 dark:bg-white/5 text-gray-600 dark:text-gray-400 font-mono">v{base}</span>
  {/if}
</span>
