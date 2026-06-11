<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    sizeLabel: string;
    loading?: boolean;
    onconfirm: () => void;
    onclose: () => void;
  }
  let { open, sizeLabel, loading = false, onconfirm, onclose }: Props = $props();
</script>

<Modal {open} title={m.sites_appLogs_clearModalTitle()} onclose={onclose} size="md">
  <div class="px-5 py-4">
    <p class="text-sm text-gray-700 dark:text-gray-300">
      {m.sites_appLogs_clearModalBody({ size: sizeLabel })}
    </p>
  </div>

  {#snippet footer()}
    <DetailButton onclick={onclose} disabled={loading}>{m.common_cancel()}</DetailButton>
    <DetailButton tone="danger" onclick={onconfirm} loading={loading} disabled={loading}>
      {loading ? m.sites_appLogs_clearing() : m.sites_appLogs_clearModalConfirm()}
    </DetailButton>
  {/snippet}
</Modal>
