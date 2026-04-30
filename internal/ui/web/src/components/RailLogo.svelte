<script lang="ts">
  import { version } from '$stores/version';
  import { goToTab } from '$stores/route';
  import { m } from '../paraglide/messages.js';
</script>

<button
  class="mb-4 flex items-center justify-center relative rounded-lg p-0.5 transition-colors hover:bg-gray-100 dark:hover:bg-white/5"
  title={$version.hasUpdate ? m.system_lerd_updateAvailable() : m.nav_dashboard()}
  onclick={() => goToTab('dashboard')}
>
  <img src="/icons/icon.svg" class="w-7 h-7 rounded-lg" alt="Lerd" />
  {#if $version.hasUpdate}
    <span
      class="absolute -top-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-yellow-400 ring-2 ring-white dark:ring-lerd-card"
    ></span>
  {/if}
</button>
