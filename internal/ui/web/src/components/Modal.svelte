<script lang="ts">
  import { onMount, onDestroy, type Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import { dialog } from '$lib/dialog';
  import { m } from '../paraglide/messages.js';

  interface Props {
    open: boolean;
    title: string;
    onclose: () => void;
    size?: 'sm' | 'md' | 'lg' | 'xl' | 'full';
    children: Snippet;
    footer?: Snippet;
    // origin is the element's box the dialog grows out of and shrinks back
    // into, a debug bar chip; without one it fades in as it always has.
    origin?: DOMRect | null;
  }
  let { open, title, onclose, size = 'md', children, footer, origin = null }: Props = $props();
  let panel: HTMLElement | null = $state(null);
  let backdrop: HTMLElement | null = $state(null);
  let closing = false;

  const calm = () => !origin || typeof window.matchMedia !== 'function' || window.matchMedia('(prefers-reduced-motion: reduce)').matches || !('animate' in HTMLElement.prototype);
  function from(): string {
    const r = panel!.getBoundingClientRect();
    if (!origin || !r.width) return 'translateY(16px) scale(.96)';
    return `translate(${origin.left + origin.width / 2 - (r.left + r.width / 2)}px, ${origin.top + origin.height / 2 - (r.top + r.height / 2)}px) scale(.12)`;
  }
  $effect(() => {
    if (!open || !panel || calm()) return;
    closing = false;
    backdrop?.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 200, easing: 'ease-out' });
    panel.animate([{ transform: from(), opacity: 0 }, { transform: 'none', opacity: 1 }], { duration: 320, easing: 'cubic-bezier(.2,.9,.25,1)' });
  });
  function requestClose() {
    if (calm() || !panel) return onclose();
    if (closing) return;
    closing = true;
    backdrop?.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 220, easing: 'ease-in', fill: 'forwards' });
    panel.animate([{ transform: 'none', opacity: 1 }, { transform: from(), opacity: 0 }], { duration: 220, easing: 'cubic-bezier(.4,0,.8,.6)', fill: 'forwards' }).onfinish = () => onclose();
  }
  const uid = $props.id();
  const titleId = `${uid}-title`;

  const widthClass = $derived(
    size === 'sm'
      ? 'max-w-sm'
      : size === 'lg'
        ? 'max-w-2xl'
        : size === 'xl'
          ? 'max-w-5xl'
          : size === 'full'
            ? 'max-w-none h-[calc(100vh-2rem)] flex flex-col'
            : 'max-w-lg'
  );

  // An open modal owns Escape. Marking the event handled lets layers
  // underneath (full-screen editors, hover menus) leave this keypress alone.
  function onKey(e: KeyboardEvent) {
    if (e.key !== 'Escape' || !open) return;
    e.preventDefault();
    requestClose();
  }

  onMount(() => window.addEventListener('keydown', onKey));
  onDestroy(() => window.removeEventListener('keydown', onKey));
</script>

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center">
    <button
      bind:this={backdrop}
      class="absolute inset-0 bg-black/50 {origin ? '' : 'lerd-fade-in'}"
      aria-label={m.common_close()}
      onclick={requestClose}
    ></button>
    <div
      bind:this={panel}
      use:dialog
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      tabindex="-1"
      class="relative outline-none bg-white dark:bg-lerd-card border border-gray-200 dark:border-lerd-border rounded-xl shadow-2xl w-full {widthClass} mx-4 {origin ? '' : 'lerd-panel-in'} transition-[max-width] duration-200 ease-out"
    >
      <div class="flex items-center justify-between gap-3 px-5 py-4 border-b border-gray-100 dark:border-lerd-border">
        <h3 id={titleId} class="min-w-0 break-words font-semibold text-gray-900 dark:text-white">{title}</h3>
        <button
          onclick={requestClose}
          class="shrink-0 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
          title={m.common_close()}
          aria-label={m.common_close()}
        >
          <Icon name="close" class="w-5 h-5" />
        </button>
      </div>
      {@render children()}
      {#if footer}
        <div class="px-5 py-3 border-t border-gray-100 dark:border-lerd-border flex items-center justify-end gap-2">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
{/if}
