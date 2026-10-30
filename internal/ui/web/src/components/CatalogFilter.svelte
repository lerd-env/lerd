<script lang="ts">
  import type { Snippet } from 'svelte';
  import CategoryChips from './CategoryChips.svelte';

  // A catalog's search box and its category chips.
  interface Props {
    query: string;
    category: string;
    categories: { key: string; label: string }[];
    placeholder: string;
    groupLabel: string;
    trailing?: Snippet;
  }
  let { query = $bindable(), category = $bindable(), categories, placeholder, groupLabel, trailing }: Props = $props();
</script>

<div class="flex flex-wrap items-center gap-2">
  <input
    type="search"
    bind:value={query}
    {placeholder}
    aria-label={placeholder}
    class="flex-1 min-w-48 h-7 text-xs bg-white dark:bg-lerd-card border border-gray-200 dark:border-lerd-border rounded-md px-2.5 text-gray-700 dark:text-gray-200 placeholder:text-gray-400 focus:outline-hidden focus:border-lerd-red/50"
  />
  {#if trailing}{@render trailing()}{/if}
</div>
<CategoryChips bind:category {categories} {groupLabel} />
