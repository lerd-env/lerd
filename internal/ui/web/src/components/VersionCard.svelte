<script lang="ts">
  import type { Snippet } from 'svelte';

  // The card shared by tools and Node versions: a name, a status pill and the
  // version as the big number, with whatever acts on it underneath.
  let {
    heading,
    pill,
    children,
    value,
    dimmed = false,
    wide = false,
    highlight = false
  }: {
    heading: Snippet;
    pill?: Snippet;
    children?: Snippet;
    value: string;
    dimmed?: boolean;
    wide?: boolean;
    highlight?: boolean;
  } = $props();
</script>

<div class="shrink-0 {wide ? 'w-[15rem]' : 'w-[11rem]'} flex flex-col gap-2.5 rounded-2xl border {highlight ? 'border-lerd-red/60' : 'border-gray-200 dark:border-lerd-border'} bg-white dark:bg-lerd-card p-3 transition-colors">
  <div class="flex items-center justify-between gap-2">
    {@render heading()}
    {@render pill?.()}
  </div>
  <span class="font-mono text-2xl font-semibold tabular-nums tracking-tight leading-none {dimmed ? 'text-gray-400 dark:text-gray-600' : 'text-gray-900 dark:text-gray-100'}">
    {value}
  </span>
  {@render children?.()}
</div>
