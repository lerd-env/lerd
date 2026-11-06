<script lang="ts">
  import DetailButton from '$components/DetailButton.svelte';
  import ServiceCardShell from '$components/ServiceCardShell.svelte';
  import ServiceIcon from '$components/ServiceIcon.svelte';
  import { openExternalDbModal } from '$stores/modals';
  import { m } from '../../paraglide/messages.js';
</script>

{#snippet plusIcon()}
  <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24" aria-hidden="true">
    <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
  </svg>
{/snippet}

<ServiceCardShell>
  <ServiceIcon name="external" category="databases" icon="database" />
  <div class="min-w-0 flex-1">
    <div class="text-sm font-semibold text-gray-900 dark:text-white truncate">{m.externalDb_rowTitle()}</div>
    <p class="text-[11px] leading-snug text-gray-500 dark:text-gray-400 truncate" title={m.externalDb_hint()}>{m.externalDb_hint()}</p>
  </div>
  <div class="shrink-0">
    <DetailButton tone="secondary" onclick={openExternalDbModal} title={m.externalDb_add()} icon={plusIcon} />
  </div>
</ServiceCardShell>
