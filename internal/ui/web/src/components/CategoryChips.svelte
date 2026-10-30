<script lang="ts">
  // Category chips for a catalog, the empty key meaning all. They only show
  // when there is more than one category to pick between.
  interface Props {
    category: string;
    categories: { key: string; label: string }[];
    groupLabel: string;
  }
  let { category = $bindable(), categories, groupLabel }: Props = $props();
</script>

{#if categories.length > 2}
  <div class="flex flex-wrap gap-1.5" role="group" aria-label={groupLabel}>
    {#each categories as c (c.key)}
      <button
        type="button"
        aria-pressed={category === c.key}
        onclick={() => (category = c.key)}
        class="h-6 px-2.5 rounded-md border text-[11px] font-medium transition-colors {category === c.key
          ? 'border-lerd-red/50 text-lerd-red bg-lerd-red/5'
          : 'border-gray-200 dark:border-lerd-border text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'}"
      >{c.label}</button>
    {/each}
  </div>
{/if}
