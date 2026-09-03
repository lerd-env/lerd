<script lang="ts">
  import type { Snippet } from 'svelte';

  // A section label inside a list panel. `divider` draws the top border that
  // separates one group from the one above; the first group in a panel omits
  // it so it doesn't double up with the panel header's bottom border.
  // `trailing` is an optional right-aligned slot for a single group-wide mark.
  let {
    label,
    divider = true,
    trailing
  }: { label: string; divider?: boolean; trailing?: Snippet } = $props();
</script>

<div
  class="flex items-center justify-between gap-2 px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500 {divider
    ? 'border-t border-gray-100 dark:border-lerd-border'
    : ''}"
>
  <span class="truncate">{label}</span>
  {#if trailing}{@render trailing()}{/if}
</div>
