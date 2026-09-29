<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    compact?: boolean;
    // A service the site does not have yet: a dashed border sets it apart from
    // the ones it does.
    suggested?: boolean;
    children: Snippet;
  }
  let { compact = false, suggested = false, children }: Props = $props();

  // Full static class strings for Tailwind. The compact variant is the site
  // overview's tighter card; the default is the services dashboard grid.
  const BASE =
    'group flex items-center border transition duration-150 hover:border-gray-300 dark:hover:border-white/15';
  // A suggestion is an empty slot, not a card: no fill, and a dashed edge a
  // shade stronger so it still reads as a slot without the fill behind it.
  const FILL = {
    card: 'border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card',
    suggested: 'border-dashed border-gray-300 dark:border-white/15 bg-transparent'
  };
  const VARIANT = {
    full: 'gap-3 rounded-xl p-3 hover:-translate-y-0.5 hover:shadow-lg hover:shadow-black/5',
    compact: 'gap-2.5 rounded-lg p-2.5 hover:shadow-sm'
  };

  const shell = $derived(
    `${BASE} ${compact ? VARIANT.compact : VARIANT.full} ${suggested ? FILL.suggested : FILL.card}`
  );
</script>

<div class={shell}>
  {@render children()}
</div>
