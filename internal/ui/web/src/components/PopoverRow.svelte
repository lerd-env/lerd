<script lang="ts">
  import type { Snippet } from 'svelte';

  // One clickable line in a popover list, with an optional mark either side.
  interface Props {
    label: string;
    onclick: () => void;
    lead?: Snippet;
    trail?: Snippet;
  }
  let { label, onclick, lead, trail }: Props = $props();
</script>

<li>
  <button
    type="button"
    {onclick}
    class="w-full flex items-center gap-2 px-3 py-1.5 text-xs text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5 transition-colors"
  >
    {@render lead?.()}
    <span class="flex-1 truncate text-left">{label}</span>
    {@render trail?.()}
  </button>
</li>
