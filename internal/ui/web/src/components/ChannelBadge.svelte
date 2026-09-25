<script lang="ts">
  import { version } from '$stores/version';
  import { parseBuildVersion } from '$lib/buildVersion';
  import { copyText } from '$lib/clipboard';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  // Marks a dev or beta build; a release gets nothing. Only a dev build's commit
  // is worth copying, so only that badge is a button.
  let { placement = 'bottom', class: extra = '' }: { placement?: 'right' | 'bottom'; class?: string } = $props();

  const build = $derived(parseBuildVersion($version.current));
  const tone = $derived(
    build.channel === 'beta'
      ? 'bg-amber-100 text-amber-800 dark:bg-amber-500/15 dark:text-amber-300'
      : 'bg-sky-100 text-sky-800 hover:bg-sky-200 dark:bg-sky-500/15 dark:text-sky-300 dark:hover:bg-sky-500/25'
  );
  const cls = $derived(`${extra} ${tone} px-1.5 py-px rounded-full text-[10px] font-semibold uppercase tracking-wide`);

  let copied = $state(false);
  async function copy(commit: string) {
    copied = await copyText(commit);
    if (copied) setTimeout(() => (copied = false), 1500);
  }
</script>

{#if build.channel !== 'release'}
  {#if build.commit}
    {@const commit = build.commit}
    {@const hint = m.version_copyHint({ value: commit })}
    <button
      type="button"
      onclick={() => copy(commit)}
      use:tooltip={{ label: copied ? m.common_copied() : hint, placement }}
      aria-label={hint}
      class="{cls} cursor-copy transition-colors"
    >{build.channel}</button>
  {:else}
    <span class={cls}>{build.channel}</span>
  {/if}
{/if}
