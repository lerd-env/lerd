<script lang="ts">
  import { tab, goToTab } from '$stores/route';
  import { m } from '../paraglide/messages.js';

  // Back from an item to its section's list, as the mobile layout shows it.
</script>

<button
  type="button"
  onclick={() => goToTab($tab)}
  class="flex items-center justify-center w-7 h-7 shrink-0 rounded-sm text-gray-400 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
  title={m.common_back()}
  aria-label={m.common_back()}
>
  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
  </svg>
</button>
