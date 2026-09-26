<script lang="ts">
  import { version } from '$stores/version';
  import { parseBuildVersion } from '$lib/buildVersion';
  import ChannelBadge from './ChannelBadge.svelte';

  // 'sm' matches the compact pills the dashboard's card headers use.
  let { size = 'md' }: { size?: 'sm' | 'md' } = $props();

  // A pre-release shows its tag and lets the badge carry the rest, as the rail does.
  const base = $derived(parseBuildVersion($version.current).base);
  const sizeClass = $derived(size === 'sm' ? 'text-[10px] px-2 py-0.5' : 'text-xs px-2.5 py-1');
</script>

<span class="inline-flex items-center gap-2">
  <ChannelBadge />
  {#if base}
    <span class="inline-flex items-center gap-1.5 whitespace-nowrap {sizeClass} font-medium rounded-full bg-gray-100 dark:bg-white/5 text-gray-600 dark:text-gray-400 font-mono">v{base}</span>
  {/if}
</span>
