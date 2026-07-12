<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal, modal } from '$stores/modals';
  import { unlinkSite, loadSites } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.siteUnlink);

  let busy = $state(false);
  let error = $state('');

  function safeClose() {
    if (busy) return;
    closeModal();
  }

  async function confirm() {
    if (!target) return;
    busy = true;
    error = '';
    const res = await unlinkSite(target.domain);
    busy = false;
    if (!res.ok) {
      error = res.error || m.common_failed();
      return;
    }
    await loadSites();
    closeModal();
  }
</script>

<Modal open title={target ? m.sites_unlinkTitle({ domain: target.domain }) : ''} onclose={safeClose} size="sm">
  <div class="px-5 py-4 space-y-3">
    {#if target}
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.sites_unlinkBody()}</p>
      {#if error}
        <p class="text-xs text-red-500">{error}</p>
      {/if}
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={busy}>{m.common_cancel()}</DetailButton>
    {#if target}
      <DetailButton tone="danger" onclick={confirm} loading={busy} disabled={busy}>{m.sites_unlink()}</DetailButton>
    {/if}
  {/snippet}
</Modal>
