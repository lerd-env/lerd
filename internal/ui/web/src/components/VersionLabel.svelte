<script lang="ts">
  import { version } from '$stores/version';
  import { parseBuildVersion } from '$lib/buildVersion';
  import { copyText } from '$lib/clipboard';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  // rail stacks the version over its badge in the narrow column; header sits them in a row.
  let { size = 'rail' }: { size?: 'rail' | 'header' } = $props();
  const versionText = $derived(size === 'rail' ? 'text-[10px]' : 'text-xs');

  // A dev build's full describe string wraps into several lines in the narrow
  // rail, so pre-releases show the tag plus a badge; only a commit is worth copying.
  const build = $derived(parseBuildVersion($version.current));
  const badgeTone = $derived(
    build.channel === 'beta'
      ? 'bg-amber-100 text-amber-800 dark:bg-amber-500/15 dark:text-amber-300'
      : 'bg-sky-100 text-sky-800 hover:bg-sky-200 dark:bg-sky-500/15 dark:text-sky-300 dark:hover:bg-sky-500/25'
  );
  const badgeClass = $derived(
    `${size === 'rail' ? 'mt-0.5 mb-1' : ''} px-1.5 py-px rounded-full text-[10px] font-semibold uppercase tracking-wide`
  );

  let copied = $state(false);
  async function copy(commit: string) {
    copied = await copyText(commit);
    if (copied) setTimeout(() => (copied = false), 1500);
  }
</script>

{#if build.channel === 'release'}
  <span class="{versionText} text-gray-400 dark:text-gray-600 font-mono {size === 'rail' ? 'pb-1' : ''}">v{$version.current}</span>
{:else}
  {#if build.base}
    <span class="{versionText} text-gray-400 dark:text-gray-600 font-mono">v{build.base}</span>
  {/if}
  {#if build.commit}
    {@const commit = build.commit}
    {@const hint = m.version_copyHint({ value: commit })}
    <button
      type="button"
      onclick={() => copy(commit)}
      use:tooltip={{ label: copied ? m.common_copied() : hint, placement: size === 'rail' ? 'right' : 'bottom' }}
      aria-label={hint}
      class="{badgeClass} {badgeTone} cursor-copy transition-colors"
    >{build.channel}</button>
  {:else}
    <span class="{badgeClass} {badgeTone}">{build.channel}</span>
  {/if}
{/if}
