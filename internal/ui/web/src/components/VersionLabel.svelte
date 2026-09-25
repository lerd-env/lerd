<script lang="ts">
  import { version } from '$stores/version';
  import { parseBuildVersion } from '$lib/buildVersion';
  import ChannelBadge from './ChannelBadge.svelte';

  // rail stacks the version over its badge in the narrow column; header sits them in a row.
  let { size = 'rail' }: { size?: 'rail' | 'header' } = $props();
  const versionText = $derived(size === 'rail' ? 'text-[10px]' : 'text-xs');

  // A dev build's full describe string wraps into several lines in the narrow
  // rail, so pre-releases show the tag plus a badge.
  const build = $derived(parseBuildVersion($version.current));
</script>

{#if build.channel === 'release'}
  <span class="{versionText} text-gray-400 dark:text-gray-600 font-mono {size === 'rail' ? 'pb-1' : ''}">v{$version.current}</span>
{:else}
  {#if build.base}
    <span class="{versionText} text-gray-400 dark:text-gray-600 font-mono">v{build.base}</span>
  {/if}
  <ChannelBadge placement={size === 'rail' ? 'right' : 'bottom'} class={size === 'rail' ? 'mt-0.5 mb-1' : ''} />
{/if}
