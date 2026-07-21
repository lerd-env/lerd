<script lang="ts">
  import { m } from '../paraglide/messages.js';

  interface Props {
    shown: number;
    total: number;
    onmore: () => void;
  }
  let { shown, total, onmore }: Props = $props();

  let sentinel = $state<HTMLDivElement | null>(null);

  // Auto-advance when the footer scrolls into view; the button stays as the
  // fallback for environments without IntersectionObserver.
  $effect(() => {
    if (!sentinel || shown >= total || typeof IntersectionObserver === 'undefined') return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) onmore();
      },
      { rootMargin: '300px' }
    );
    io.observe(sentinel);
    return () => io.disconnect();
  });
</script>

{#if shown < total}
  <div bind:this={sentinel} class="py-3 flex justify-center">
    <button
      type="button"
      class="text-xs rounded-sm border border-gray-300 dark:border-lerd-border px-3 py-1 text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-white/5"
      onclick={onmore}
    >
      {m.debug_loadMore({ shown, total })}
    </button>
  </div>
{/if}
