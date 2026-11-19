<script lang="ts">
  import type { Snippet } from 'svelte';

  // One row of a settings list: the title with its description under it, the
  // control on the right, and any extra detail full width below.
  let {
    title,
    description = '',
    control,
    children
  }: { title: string; description?: string; control?: Snippet; children?: Snippet } = $props();
</script>

<div class="px-4 py-3">
  <div class="flex items-center justify-between gap-6">
    <div class="min-w-0">
      <p class="text-sm font-semibold text-gray-700 dark:text-gray-300">{title}</p>
      {#if description}
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{description}</p>
      {/if}
    </div>
    {#if control}
      <div class="shrink-0 flex items-center gap-2">{@render control()}</div>
    {/if}
  </div>
  {#if children}{@render children()}{/if}
</div>
