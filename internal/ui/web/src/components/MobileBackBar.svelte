<script lang="ts">
  import { tab, routeRest, goToTab } from '$stores/route';
  import { m } from '../paraglide/messages.js';

  function back() {
    goToTab($tab);
  }

  const title = $derived($routeRest || $tab);
</script>

<div class="flex items-center gap-2 px-3 py-1.5 border-b border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shrink-0 sticky top-0 z-10">
  <button
    onclick={back}
    class="flex items-center justify-center w-7 h-7 -ml-1 rounded-sm text-gray-400 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
    title={m.common_back()}
    aria-label={m.common_back()}
  >
    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
    </svg>
  </button>
  <span class="text-sm font-semibold text-gray-900 dark:text-white truncate">{title}</span>
</div>
