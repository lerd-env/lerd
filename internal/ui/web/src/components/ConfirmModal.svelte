<script lang="ts">
  import Modal from './Modal.svelte';
  import DetailButton from './DetailButton.svelte';
  import { m } from '../paraglide/messages.js';

  interface Props {
    open: boolean;
    title: string;
    body: string;
    confirmLabel: string;
    danger?: boolean;
    loading?: boolean;
    onconfirm: () => void;
    onclose: () => void;
  }
  let { open, title, body, confirmLabel, danger = false, loading = false, onconfirm, onclose }: Props = $props();
</script>

<Modal {open} {title} onclose={() => { if (!loading) onclose(); }} size="sm">
  <div class="px-5 py-4">
    <p class="text-sm text-gray-700 dark:text-gray-300">{body}</p>
  </div>

  {#snippet footer()}
    <DetailButton onclick={onclose} disabled={loading}>{m.common_cancel()}</DetailButton>
    <DetailButton tone={danger ? 'danger' : 'primary'} onclick={onconfirm} loading={loading} disabled={loading}>
      {confirmLabel}
    </DetailButton>
  {/snippet}
</Modal>
