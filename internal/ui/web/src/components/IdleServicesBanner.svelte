<script lang="ts">
  import { idleEnabled, idleServices } from '$stores/idle';
  import { answerIdleSuggestion, idleSuggestion, loadIdleSuggestion } from '$stores/idleSuggestion';
  import { accessMode } from '$stores/accessMode';
  import { setupVisible } from '$stores/setup';
  import { formatBytes } from '$stores/stats';
  import { pollWhileVisible } from '$lib/pollWhileVisible';
  import { m } from '../paraglide/messages.js';
  import BottomBanner from './BottomBanner.svelte';

  const POLL_INTERVAL_MS = 5 * 60_000;

  let busy = $state(false);

  // Only an unanswered question is worth asking the daemon about.
  const askable = $derived($idleEnabled && !$idleServices && $accessMode.localControl && !$setupVisible);

  $effect(() => {
    if (askable) return pollWhileVisible(loadIdleSuggestion, POLL_INTERVAL_MS);
  });

  async function answer(sleep: boolean) {
    if (busy) return;
    busy = true;
    await answerIdleSuggestion(sleep);
    busy = false;
  }
</script>

{#if askable && $idleSuggestion.services.length > 0}
  <BottomBanner
    title={m.idle_suggest_title({ count: $idleSuggestion.services.length, size: formatBytes($idleSuggestion.mem_bytes) })}
    subtitle={m.idle_suggest_subtitle({ services: $idleSuggestion.services.join(', ') })}
    actionLabel={m.idle_suggest_enable()}
    dismissLabel={m.idle_suggest_dismiss()}
    onAction={() => answer(true)}
    onDismiss={() => answer(false)}
  >
    {#snippet icon()}
      <svg class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/>
      </svg>
    {/snippet}
  </BottomBanner>
{/if}
