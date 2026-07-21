<script lang="ts" module>
  export interface TabItem<T extends string = string> {
    id: T;
    label: string;
    hidden?: boolean;
    count?: number;
  }
</script>

<script lang="ts" generics="T extends string">
  import type { Snippet } from 'svelte';

  interface Props {
    tabs: TabItem<T>[];
    active: T;
    onchange: (id: T) => void;
    actions?: Snippet;
  }
  let { tabs, active, onchange, actions }: Props = $props();

  // A lone tab can't be switched to anything, so the bar is just noise. Hide it
  // (and the empty 0-tab case) and let the content fill the space instead.
  const visible = $derived(tabs.filter((t) => !t.hidden));
</script>

{#if visible.length > 1 || actions}
  <div class="flex items-end justify-between gap-3 border-b border-gray-100 dark:border-lerd-border pt-3 px-3 shrink-0">
    <div class="flex items-end gap-4 min-w-0 overflow-x-auto">
      {#if visible.length > 1}
        {#each visible as t (t.id)}
          <button
            onclick={() => onchange(t.id)}
            class="shrink-0 pb-1 text-xs font-medium transition-colors border-b-2 flex items-center gap-1 {active === t.id
              ? 'border-lerd-red text-lerd-red'
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
