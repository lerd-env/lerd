<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    label: string;
    hint?: string;
    children: Snippet;
  }
  let { label, hint = '', children }: Props = $props();
</script>

<div class="space-y-1.5">
  <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{label}</div>
  {@render children()}
  {#if hint}
    <div class="text-xs text-gray-500 dark:text-gray-400">{hint}</div>
  {/if}
</div>
