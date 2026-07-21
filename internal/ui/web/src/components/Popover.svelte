<script lang="ts">
  import type { Snippet } from 'svelte';
  import IconButton from '$components/IconButton.svelte';
  import { portal } from '$lib/portal';

  interface Props {
    label: string;
    // 'left' opens beside the trigger (the icon rail), 'right' below and
    // right-aligned (the mobile header).
    align?: 'left' | 'right';
    size?: 'sm' | 'md';
    width?: number;
    onopen?: () => void;
    trigger: Snippet;
    children: Snippet<[() => void]>;
  }
  let {
    label,
    align = 'left',
    size = 'sm',
    width = 320,
    onopen,
    trigger,
    children
  }: Props = $props();

  let open = $state(false);
  let triggerEl: HTMLElement | null = $state(null);
  // The panel is a fixed layer measured off the trigger, so an ancestor with
  // overflow hidden (the app shell's main column) cannot clip it.
  let pos = $state({ top: 0, left: 0, width: 0 });

  function place() {
    if (!triggerEl) return;
    const r = triggerEl.getBoundingClientRect();
    const margin = 8;
    const w = Math.min(width, window.innerWidth - margin * 2);
    const left =
      align === 'right'
        ? Math.max(margin, r.right - w)
        : Math.min(r.right + margin, window.innerWidth - w - margin);
    const top =
      align === 'right'
        ? r.bottom + margin
        : Math.max(margin, Math.min(r.bottom, window.innerHeight - margin));
    pos = { top, left, width: w };
  }

  function toggle() {
    open = !open;
    if (!open) return;
    place();
    onopen?.();
  }

  const close = () => (open = false);
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && close()} onresize={() => open && place()} />

<div class="relative" bind:this={triggerEl}>
  <IconButton title={label} active={open} onclick={toggle} {size}>
    {@render trigger()}
  </IconButton>

  {#if open}
    <!-- Click-away backdrop; the panel sits above it. -->
    <button use:portal type="button" tabindex="-1" aria-hidden="true" class="fixed inset-0 z-70 cursor-default" onclick={close}
    ></button>
    <div
      use:portal
      class="z-80 rounded-xl border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-2xl"
      style="position: fixed; left: {pos.left}px; width: {pos.width}px; {align === 'right'
        ? `top: ${pos.top}px`
        : `bottom: ${Math.max(8, window.innerHeight - pos.top)}px`}"
    >
      {@render children(close)}
    </div>
  {/if}
</div>
