<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import { m } from '../paraglide/messages.js';

  interface Props {
    label: string;
    count: number;
    collapsed: boolean;
    ontoggle: () => void;
    // Workspace headers are dragged by the header itself, so the whole row
    // shows a grab cursor rather than carrying a separate handle.
    draggable?: boolean;
    trailing?: Snippet; // rename/delete menu, on workspace sections
  }
  let { label, count, collapsed, ontoggle, draggable = false, trailing }: Props = $props();
</script>

<div class="flex items-center gap-1 pl-2 pr-2 border-t border-gray-100 dark:border-lerd-border bg-gray-50/60 dark:bg-white/2">
  <button
    type="button"
    onclick={ontoggle}
    aria-expanded={!collapsed}
    aria-label={collapsed ? m.workspaces_expand() : m.workspaces_collapse()}
    class="flex-1 min-w-0 flex items-center gap-1.5 py-1.5 text-left text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 transition-colors {draggable
      ? 'cursor-grab active:cursor-grabbing'
      : ''}"
  >
    <Icon
      name="chevron"
      class="w-3 h-3 shrink-0 transition-transform {collapsed ? '-rotate-90' : ''}"
    />
    <span class="truncate">{label}</span>
    <span class="shrink-0 font-normal normal-case tracking-normal text-gray-400 dark:text-gray-600">{count}</span>
  </button>
  {#if trailing}{@render trailing()}{/if}
</div>
