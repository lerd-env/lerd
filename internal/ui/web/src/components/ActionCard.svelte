<script lang="ts">
  import type { Snippet } from 'svelte';

  // The shared shell of an item card with a bottom icon-action row, as the
  // Databases tab established it: header content up top, actions pinned to the
  // bottom divider, anything else (status lines) below.
  interface Props {
    header: Snippet;
    actions?: Snippet;
    children?: Snippet;
  }
  let { header, actions, children }: Props = $props();
</script>

<div class="flex h-full flex-col rounded-xl border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-3 transition duration-150 hover:border-gray-300 dark:hover:border-white/15 hover:shadow-sm">
  <!-- mb-2 keeps the meta line off the divider on the tallest card in a row,
       where mt-auto has no slack left to space them apart. -->
  <div class="flex items-start gap-2 mb-2">{@render header()}</div>
  {#if actions}
    <div class="flex items-center gap-0.5 mt-auto pt-2 border-t border-gray-100 dark:border-lerd-border/60">{@render actions()}</div>
  {/if}
  {#if children}{@render children()}{/if}
</div>
