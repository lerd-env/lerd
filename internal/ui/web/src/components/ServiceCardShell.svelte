<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    compact?: boolean;
    children: Snippet;
  }
  let { compact = false, children }: Props = $props();

  // Full static class strings for Tailwind. The compact variant is the site
  // overview's tighter card; the default is the services dashboard grid.
  const BASE =
    'group flex items-center border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card transition duration-150 hover:border-gray-300 dark:hover:border-white/15';
  const VARIANT = {
    full: 'gap-3 rounded-xl p-3 hover:-translate-y-0.5 hover:shadow-lg hover:shadow-black/5',
    compact: 'gap-2.5 rounded-lg p-2.5 hover:shadow-sm'
  };

  const shell = $derived(`${BASE} ${compact ? VARIANT.compact : VARIANT.full}`);
</script>

<div class={shell}>
  {@render children()}
</div>
