<script lang="ts">
  import { tooltip } from '$lib/tooltip';

  interface Item {
    value: string;
    label: string;
    note?: string;
    // The longer explanation behind the note, shown on hover.
    hint?: string;
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
      class="flex {columns && item.note ? 'items-start' : 'items-center'} gap-2 py-1 text-sm text-gray-700 dark:text-gray-300 {disabled
        ? 'opacity-60'
        : 'cursor-pointer'}"
      use:tooltip={item.hint ?? ''}
    >
      <input
        type="checkbox"
        {disabled}
        checked={selected.includes(item.value)}
        onchange={() => toggle(item.value)}
        class="rounded-sm border-gray-300 dark:border-lerd-border {columns && item.note ? 'mt-0.5' : ''}"
      />
      <!-- In columns a note goes under its label, since beside it a package name
           would push the label out of a narrow cell. -->
      <span class="{columns ? 'min-w-0' : 'contents'}">
        <span class="{columns ? 'block' : ''} truncate">{item.label}</span>
        {#if item.note}
          <span class="{columns ? 'block truncate' : ''} text-xs text-gray-500 dark:text-gray-400">{item.note}</span>
        {/if}
      </span>
    </label>
  {/each}
</div>
