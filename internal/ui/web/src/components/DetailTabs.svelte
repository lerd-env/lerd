<script lang="ts" module>
  export interface TabItem<T extends string = string> {
    id: T;
    label: string;
    hidden?: boolean;
    count?: number;
    // Drawn dimmer than its siblings: the tab is worth keeping but what it
    // shows is no longer live, like a stopped worker's journal.
    muted?: boolean;
  }
</script>

<script lang="ts" generics="T extends string">
  import type { Snippet } from 'svelte';
  import { tablist } from '$lib/tablist';

  interface Props {
    tabs: TabItem<T>[];
    active: T;
    onchange: (id: T) => void;
    actions?: Snippet;
    // keepSingle draws the bar for a lone tab, where it says what the view is.
    keepSingle?: boolean;
    // snap makes a row that overflows scroll a whole tab at a time, keeping
    // the active one in view.
    snap?: boolean;
  }
  let { tabs, active, onchange, actions, keepSingle = false, snap = false }: Props = $props();

  let row = $state<HTMLElement | null>(null);
  $effect(() => {
    void active;
    if (snap) row?.querySelector('[aria-selected="true"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  });

  // A lone tab can't be switched to anything, so the bar is just noise. Hide it
  // (and the empty 0-tab case) and let the content fill the space instead.
  const visible = $derived(tabs.filter((t) => !t.hidden));
  const showTabs = $derived(visible.length > 1 || (keepSingle && visible.length === 1));
</script>

{#if showTabs || actions}
  <div class="flex items-end justify-between gap-3 border-b border-gray-100 dark:border-lerd-border pt-3 px-3 shrink-0">
    <div bind:this={row} use:tablist role={showTabs ? 'tablist' : undefined} class="flex items-end gap-4 min-w-0 overflow-x-auto {snap ? 'snap-x snap-mandatory overscroll-x-contain' : ''}">
      {#if showTabs}
        {#each visible as t (t.id)}
          <button
            role="tab"
            aria-selected={active === t.id}
            tabindex={active === t.id ? 0 : -1}
            onclick={() => onchange(t.id)}
            class="shrink-0 pb-1 text-xs font-medium transition-colors border-b-2 flex items-center gap-1 {snap ? 'snap-start' : ''} {active === t.id
              ? 'border-lerd-red text-lerd-red'
              : t.muted
                ? 'border-transparent text-gray-400 dark:text-gray-600 hover:text-gray-600 dark:hover:text-gray-400'
                : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'}"
          >{t.label}{#if t.count}<span class="text-[10px] tabular-nums rounded-full px-1.5 py-px bg-gray-200/70 dark:bg-white/10 text-gray-600 dark:text-gray-300">{t.count}</span>{/if}</button>
        {/each}
      {/if}
    </div>
    {#if actions}
      <div class="flex items-center gap-2 pb-1.5 shrink-0">{@render actions()}</div>
    {/if}
  </div>
{/if}
