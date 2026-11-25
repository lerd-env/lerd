<script lang="ts" module>
  import type { IconName } from '$components/Icon.svelte';

  export interface CellAction {
    icon: IconName;
    label: string;
    onclick: () => void;
    open?: boolean; // its popover is showing
    menu?: boolean; // the popover is a menu
  }
</script>

<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';

  // Round actions that rest as one cell with three dots and divide into a cell
  // each on hover, keyboard focus or a tap.
  let { actions }: { actions: CellAction[] } = $props();

  const uid = $props.id();
  const CELL = 28;
  const GAP = 8;

  let root: HTMLDivElement | undefined = $state();
  let slots: HTMLElement[] = $state([]);
  let hovered = $state(false);
  let focused = $state(false);
  let tapped = $state(false);

  const mergeable = $derived(actions.length > 1);
  const anyOpen = $derived(actions.some((a) => a.open));
  const expanded = $derived(!mergeable || hovered || focused || tapped || anyOpen);
  const width = $derived(actions.length * CELL + (actions.length - 1) * GAP);

  // Merged, every cell sits on the last one, so the group rests in its corner.
  function shift(i: number): string {
    const px = expanded ? 0 : (actions.length - 1 - i) * (CELL + GAP);
    return `left: ${i * (CELL + GAP)}px; transform: translateX(${px}px)`;
  }

  function enter(e: PointerEvent) {
    // A finger's pointerenter precedes its click: splitting here would put a
    // real button under the tap meant for the dots.
    if (e.pointerType !== 'touch') hovered = true;
  }
  function outside(e: PointerEvent) {
    if (!root?.contains(e.target as Node)) tapped = false;
  }
  function focusin(e: FocusEvent) {
    // A mouse click focuses its button too, and must not hold the cells apart.
    if ((e.target as Element).matches(':focus-visible')) focused = true;
  }
</script>

<svelte:document onpointerdown={outside} />

<div
  bind:this={root}
  role="group"
  data-expanded={expanded}
  class="cells"
  class:expanded
  style="width: {width}px"
  onpointerenter={enter}
  onpointerleave={() => (hovered = false)}
  onfocusin={focusin}
  onfocusout={() => (focused = false)}
>
  <!-- Blurring the discs and cutting the result at two levels makes near ones
       flow together: the outer cut is the rim, the inner one the body. -->
  <svg class="absolute w-0 h-0" aria-hidden="true">
    <filter id="{uid}-goo" x="-25%" y="-50%" width="150%" height="200%" color-interpolation-filters="sRGB">
      <feGaussianBlur in="SourceAlpha" stdDeviation="3.5" result="blur" />
      <feColorMatrix in="blur" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 9 -3.6" result="outer" />
      <feColorMatrix in="blur" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 9 -4.6" result="inner" />
      <feFlood class="rim" />
      <feComposite in2="outer" operator="in" result="rim" />
      <feFlood class="body" />
      <feComposite in2="inner" operator="in" result="body" />
      <feMerge>
        <feMergeNode in="rim" />
        <feMergeNode in="body" />
      </feMerge>
    </filter>
  </svg>

  <div class="absolute inset-0" style="filter: url(#{uid}-goo)" aria-hidden="true">
    {#each actions as a, i (a.icon)}
      <span class="cell moving bg-black" style={shift(i)}></span>
    {/each}
  </div>

  {#each actions as a, i (a.icon)}
    <!-- Where the button settles, so its tooltip doesn't aim at one mid-flight. -->
    <span bind:this={slots[i]} class="cell slot" style="left: {i * (CELL + GAP)}px"></span>
    <button
      type="button"
      onclick={a.onclick}
      use:tooltip={{ label: anyOpen ? '' : a.label, placement: 'top', anchor: slots[i] }}
      aria-label={a.label}
      aria-haspopup={a.menu ? 'menu' : undefined}
      aria-expanded={a.open}
      class="cell moving action flex items-center justify-center border border-transparent text-gray-800 dark:text-gray-200 hover:border-lerd-red hover:text-lerd-red"
      style={shift(i)}
    >
      <Icon name={a.icon} class="w-3.5 h-3.5" />
    </button>
  {/each}

  {#if mergeable}
    <!-- For the pointer only: the buttons above stay in the tab order and in the
         accessibility tree, and focusing one splits the cells. -->
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <span
      data-cell-dots
      aria-hidden="true"
      onclick={() => (tapped = true)}
      class="cell dots right-0 flex items-center justify-center text-gray-800 dark:text-gray-200"
    >
      <Icon name="more" class="w-4 h-4 rotate-90 [&_path]:[stroke-width:3]" />
    </span>
  {/if}
</div>

<style>
  .cells {
    --cell-body: var(--color-gray-100);
    --cell-rim: var(--color-gray-200);
    position: relative;
    height: 28px;
    pointer-events: none;
  }
  :global(.dark) .cells {
    --cell-body: color-mix(in srgb, white 10%, var(--color-lerd-card));
    --cell-rim: color-mix(in srgb, white 23%, var(--color-lerd-card));
  }
  .cells.expanded {
    pointer-events: auto;
  }
  .body {
    flood-color: var(--cell-body);
  }
  .rim {
    flood-color: var(--cell-rim);
  }
  .cell {
    position: absolute;
    top: 0;
    width: 28px;
    height: 28px;
    border-radius: 9999px;
  }
  .slot {
    pointer-events: none;
  }
  .moving {
    transition: transform 420ms cubic-bezier(0.3, 1.35, 0.5, 1);
  }
  .action {
    opacity: 0;
    transition:
      transform 420ms cubic-bezier(0.3, 1.35, 0.5, 1),
      opacity 120ms,
      color 150ms,
      border-color 150ms;
  }
  .expanded .action {
    opacity: 1;
    transition-delay: 0ms, 130ms, 0ms, 0ms;
  }
  .dots {
    cursor: pointer;
    pointer-events: auto;
    transition: opacity 150ms 180ms;
  }
  .expanded .dots {
    opacity: 0;
    pointer-events: none;
    transition-delay: 0ms;
  }
</style>
