<script lang="ts">
  import type { Snippet } from 'svelte';

  // Compact icon-button toggle: a glyph that turns emerald with a slow
  // pulsing dot while the feature is on. Shared by the dump bridge and
  // profiler toggles so both read the same way wherever they appear.

  interface Props {
    enabled: boolean;
    busy?: boolean;
    title?: string;
    onclick?: (e: MouseEvent) => void;
    children: Snippet;
  }

  let { enabled, busy = false, title, onclick, children }: Props = $props();
</script>

<button
  type="button"
  {title}
  {onclick}
  disabled={busy}
  aria-pressed={enabled}
  class="relative w-6 h-6 flex items-center justify-center rounded-sm transition-colors disabled:opacity-40
    {enabled
      ? 'text-emerald-600 dark:text-emerald-400 hover:bg-emerald-50 dark:hover:bg-emerald-900/20'
      : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-white/5'}"
>
  {@render children()}

  {#if enabled}
    <span class="absolute top-1 right-1 flex items-center justify-center w-2 h-2">
      <span class="absolute inline-flex w-full h-full rounded-full bg-emerald-400 opacity-75 lerd-pulse-ping"></span>
      <span class="relative inline-flex w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
    </span>
  {/if}
</button>

<style>
  /* Slower-than-Tailwind ping so the dot reads as ambient activity rather
     than an attention-grabbing alert. */
  .lerd-pulse-ping {
    animation: lerd-pulse-ping 2s cubic-bezier(0, 0, 0.2, 1) infinite;
  }
  @keyframes lerd-pulse-ping {
    0% {
      transform: scale(1);
      opacity: 0.75;
    }
    75%,
    100% {
      transform: scale(2);
      opacity: 0;
    }
  }
</style>
