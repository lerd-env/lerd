<script lang="ts">
  interface Item {
    value: string;
    label: string;
    note?: string;
  }

  interface Props {
    items: Item[];
    selected: string[];
    onchange: (selected: string[]) => void;
    disabled?: boolean;
    columns?: boolean;
  }
  let { items, selected, onchange, disabled = false, columns = true }: Props = $props();

  function toggle(value: string) {
    onchange(
      selected.includes(value) ? selected.filter((s) => s !== value) : [...selected, value]
    );
  }
</script>

<div class="{columns ? 'grid grid-cols-2 sm:grid-cols-3 gap-x-4' : 'space-y-0.5'}">
  {#each items as item (item.value)}
    <label
      class="flex items-center gap-2 py-1 text-sm text-gray-700 dark:text-gray-300 {disabled
        ? 'opacity-60'
        : 'cursor-pointer'}"
    >
      <input
        type="checkbox"
        {disabled}
        checked={selected.includes(item.value)}
        onchange={() => toggle(item.value)}
        class="rounded-sm border-gray-300 dark:border-lerd-border"
      />
      <span class="truncate">{item.label}</span>
      {#if item.note}
        <span class="text-xs text-gray-500 dark:text-gray-400">{item.note}</span>
      {/if}
    </label>
  {/each}
</div>
