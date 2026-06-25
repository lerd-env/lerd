<script lang="ts">
  import { hostDB } from '$stores/dbBackend';
  import { dbEngineDisplay, dbServiceUnit } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  let dismissed = $state(false);
  // Show once probed and the host database server is not live (absent, or a stale
  // socket with nothing accepting). null = not probed yet → stay hidden.
  const visible = $derived(!dismissed && $hostDB !== null && !$hostDB.live);
  // Engine name + systemd unit for the setup copy, from the probed service.
  const engine = $derived(dbEngineDisplay($hostDB?.service_name ?? 'mysql'));
  const unit = $derived(dbServiceUnit($hostDB?.service_name ?? 'mysql'));
</script>

{#if visible}
  <div
    class="rounded-lg border border-l-4 border-amber-300 dark:border-amber-500/40 border-l-amber-500 bg-amber-50/70 dark:bg-amber-500/10 px-3 py-2.5"
  >
    <div class="flex items-start gap-3">
      <svg
        class="w-5 h-5 shrink-0 text-amber-600 dark:text-amber-400 mt-0.5"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        viewBox="0 0 24 24"
        aria-hidden="true"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M12 9v3.75m0 3.75h.008M10.34 3.94 1.82 18a1.5 1.5 0 0 0 1.29 2.25h17.78A1.5 1.5 0 0 0 22.18 18L13.66 3.94a1.5 1.5 0 0 0-2.32 0Z"
        />
      </svg>
      <div class="flex-1 min-w-0">
        <p class="text-xs font-semibold text-amber-900 dark:text-amber-200">
          {m.services_hostSetup_title({ engine })}
        </p>
        <p class="text-[11px] text-amber-700 dark:text-amber-300/80 mt-0.5">
          {m.services_hostSetup_subtitle({ engine, unit })}
        </p>
      </div>
      <button
        onclick={() => (dismissed = true)}
        title={m.services_hostSetup_dismiss()}
        aria-label={m.services_hostSetup_dismiss()}
        class="shrink-0 text-amber-600/60 hover:text-amber-700 dark:text-amber-400/60 dark:hover:text-amber-300 transition-colors"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  </div>
{/if}
