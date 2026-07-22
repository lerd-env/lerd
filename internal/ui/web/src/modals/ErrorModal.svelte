<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal, modal } from '$stores/modals';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.error);
</script>

<Modal open title={target?.title || m.common_errorTitle()} onclose={closeModal} size="sm">
  <div class="px-5 py-4">
    <p class="text-sm text-gray-700 dark:text-gray-300 whitespace-pre-wrap break-words">
      {target?.message}
    </p>
  </div>

  {#snippet footer()}
    <DetailButton onclick={closeModal}>{m.common_close()}</DetailButton>
  {/snippet}
</Modal>
