<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal, modal } from '$stores/modals';
  import { resetSiteNginx } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.nginxReset);

  let busy = $state(false);
  let error = $state('');

  function safeClose() {
    if (busy) return;
    closeModal();
  }

  async function confirm() {
    if (!target) return;
    const onSuccess = $modal.onSuccess;
    busy = true;
    error = '';
    try {
      const res = await resetSiteNginx(target.domain);
      if (!res.ok) {
        error = res.error || m.nginxEditor_resetFailed();
        return;
      }
      closeModal();
      // Same async-onSuccess rejection guard as the save/restore modals
      // so a failed post-reset refresh never becomes an unhandled
      // promise rejection.
      try {
        await onSuccess?.();
      } catch {
        /* surfaced in tab state by the caller's refreshAfterAction */
      }
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal open title={m.nginxEditor_resetTitle()} onclose={safeClose} size="md">
  <div class="px-5 py-4 space-y-3">
    {#if !target}
      <p class="text-sm text-gray-500 dark:text-gray-400">{m.common_loading()}</p>
    {:else}
      <p class="text-sm text-gray-700 dark:text-gray-300">
        {m.nginxEditor_resetBody({ domain: target.domain })}
      </p>
      <p class="text-[10px] text-gray-400 dark:text-gray-600 font-mono break-all">{target.path}</p>

      {#if error}
        <p class="text-xs text-red-500">{error}</p>
      {/if}
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={busy}>{m.common_cancel()}</DetailButton>
    {#if target}
      <DetailButton tone="primary" onclick={confirm} loading={busy} disabled={busy}>
        {busy ? m.nginxEditor_resetting() : m.nginxEditor_resetAccept()}
      </DetailButton>
    {/if}
  {/snippet}
</Modal>
