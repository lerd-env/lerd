<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal, modal } from '$stores/modals';
  import { deleteWorkspace } from '$stores/workspaces';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.workspaceDelete);

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
    const res = await deleteWorkspace(target.name);
    busy = false;
    if (!res.ok) {
      error = res.error || m.common_failed();
      return;
    }
    closeModal();
  }
</script>

<Modal open title={target ? m.workspaces_deleteTitle({ name: target.name }) : ''} onclose={safeClose} size="sm">
  <div class="px-5 py-4 space-y-3">
    {#if target}
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.workspaces_deleteBody()}</p>
      {#if target.siteCount > 0}
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {m.workspaces_deleteSiteCount({ count: target.siteCount })}
        </p>
      {/if}
      {#if error}
        <p class="text-xs text-red-500">{error}</p>
      {/if}
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={busy}>{m.common_cancel()}</DetailButton>
    {#if target}
      <DetailButton tone="danger" onclick={confirm} loading={busy} disabled={busy}>{m.common_remove()}</DetailButton>
    {/if}
  {/snippet}
</Modal>
