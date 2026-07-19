<script lang="ts">
  // A section label inside a list panel. `divider` draws the top border that
  // separates one group from the one above; the first group in a panel omits
  // it so it doesn't double up with the panel header's bottom border.
  let { label, divider = true }: { label: string; divider?: boolean } = $props();
</script>

<div
  class="px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500 {divider
    ? 'border-t border-gray-100 dark:border-lerd-border'
    : ''}"
>
  {label}
</div>
