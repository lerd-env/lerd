<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Lens, LensGroup } from '$lib/lens';
  import LensLoadMore from '$components/LensLoadMore.svelte';
  import { m } from '../paraglide/messages.js';

  // The scrolling list every Debug lens shares: one page of request groups
  // from lerd-ui, more as the end comes into view, and a pill for what arrived
  // while the list was scrolled away from the top.
  interface Props {
    lens: Lens;
    group: Snippet<[LensGroup]>;
    empty: Snippet;
  }
  let { lens, group, empty }: Props = $props();
  const { groups, hasMore, total, fresh, loading } = lens;

  let scroller = $state<HTMLDivElement | null>(null);
  function onscroll() {
    lens.setAtTop((scroller?.scrollTop ?? 0) < 40);
  }
  function toTop() {
    scroller?.scrollTo({ top: 0 });
    lens.setAtTop(true);
    void lens.refresh();
  }

  const shown = $derived($groups.reduce((n, g) => n + g.rows.length, 0));
  // The footer reads the next page of requests, and once there are none, the
  // rest of any request whose rows were cut.
  function more() {
    if ($hasMore) return void lens.loadMore();
    const partial = $groups.find((g) => g.rows.length < g.count);
    if (partial) void lens.loadRows(partial.key);
  }
</script>

<div bind:this={scroller} {onscroll} class="relative flex-1 overflow-y-auto px-3 pb-3">
  {#if $fresh > 0}
    <div class="sticky top-2 z-10 flex justify-center pointer-events-none">
      <button
        type="button"
        onclick={toTop}
        class="pointer-events-auto text-xs rounded-full px-3 py-1 bg-lerd-red text-white shadow"
      >{m.debug_newEvents({ count: $fresh })}</button>
    </div>
  {/if}
  {#if $groups.length === 0}
    {#if !$loading}{@render empty()}{/if}
  {:else}
    {#each $groups as g (g.key)}
      {@render group(g)}
      {#if g.rows.length < g.count}
        <div class="-mt-3 mb-4 flex justify-center">
          <button
            type="button"
            class="text-[11px] text-gray-500 dark:text-gray-400 hover:text-lerd-red"
            onclick={() => void lens.loadRows(g.key)}
          >{m.debug_loadMore({ shown: g.rows.length, total: g.count })}</button>
        </div>
      {/if}
    {/each}
    <LensLoadMore {shown} total={$total} onmore={more} />
  {/if}
</div>
