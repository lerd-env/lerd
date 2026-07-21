<script lang="ts">
  // Shared checkbox for the Debug lens toolbars (worker capture, test runs),
  // so the three lenses don't each carry their own copy of the markup.
  interface Props {
    label: string;
    checked: boolean;
    disabled?: boolean;
    // hint renders muted after the label, e.g. a count of what's being hidden.
    hint?: string;
    onchange: (checked: boolean) => void;
  }
  let { label, checked, disabled = false, hint = '', onchange }: Props = $props();
</script>

<label
  class="inline-flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400 cursor-pointer select-none whitespace-nowrap"
>
  <input
    type="checkbox"
    class="rounded-sm border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card text-lerd-red focus:ring-lerd-red"
    {checked}
    {disabled}
    onchange={(e) => onchange((e.currentTarget as HTMLInputElement).checked)}
  />
  {label}
  {#if hint}<span class="text-gray-400 dark:text-gray-500">{hint}</span>{/if}
</label>
