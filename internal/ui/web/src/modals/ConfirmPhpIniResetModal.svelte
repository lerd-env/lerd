<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal, modal } from '$stores/modals';
  import { resetPhpIni } from '$stores/phpVersions';
  import { notifyLocalFailure } from '$lib/notify';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.phpIniReset);

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
      const res = await resetPhpIni(target.version);
      if (!res.ok) {
        error = res.error || m.nginxEditor_resetFailed();
        // A reset that cannot restart FPM leaves the version down, so say so
        // somewhere that outlives this modal.
        notifyLocalFailure('php_ini', m.phpIniEditor_restartFailed({ scope: target.label }), error);
        return;
      }
      closeModal();
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

<Modal open title={m.phpIniEditor_resetTitle()} onclose={safeClose} size="md">
  <div class="px-5 py-4 space-y-3">
    {#if !target}
      <p class="text-sm text-gray-500 dark:text-gray-400">{m.common_loading()}</p>
    {:else}
      <p class="text-sm text-gray-700 dark:text-gray-300">
        {m.phpIniEditor_resetBody({ scope: target.label })}
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
        {busy ? m.phpIniEditor_restarting() : m.nginxEditor_resetAccept()}
      </DetailButton>
    {/if}
  {/snippet}
</Modal>
