<script lang="ts">
  import { toDumpNodes } from '$lib/structured';
  import DumpView from './DumpView.svelte';

  // A captured value as a tree you can fold open when it is structured, and as
  // plain text when it is not.

  interface Props {
    value: unknown;
    // open unfolds the first level, for a value that is the point of its row.
    open?: boolean;
    class?: string;
  }
  let { value, open = false, class: cls = '' }: Props = $props();
  const nodes = $derived(toDumpNodes(value));
</script>

{#if nodes}
  <div class="font-mono text-[11px] {cls}">
    {#each nodes as node, i (i)}<DumpView {node} initiallyOpen={open} />{/each}
  </div>
{:else}
  <span class="font-mono text-[11px] break-all {cls}">{typeof value === 'string' ? value : String(value ?? '')}</span>
{/if}
