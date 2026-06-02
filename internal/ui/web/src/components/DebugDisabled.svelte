<script lang="ts">
  import { setDebugCapture } from '$stores/queries';
  import { m } from '../paraglide/messages.js';

  let enabling = $state(false);
  async function enable() {
    if (enabling) return;
    enabling = true;
    try {
      await setDebugCapture(true);
    } finally {
      enabling = false;
    }
  }
</script>

<div class="px-3 py-10 text-center space-y-3">
  <p class="text-sm text-gray-500 dark:text-gray-400">{m.debug_disabled_title()}</p>
  <p class="text-[11px] text-gray-400 dark:text-gray-500">{m.debug_disabled_body()}</p>
  <button
    type="button"
    disabled={enabling}
    onclick={enable}
    class="inline-flex items-center gap-1.5 text-xs rounded-sm border border-emerald-500/40 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 px-3 py-1.5 hover:border-emerald-500 hover:bg-emerald-100 dark:hover:bg-emerald-900/40 disabled:opacity-50"
  >
    {enabling ? m.queries_enabling() : m.debug_enable()}
  </button>
</div>
