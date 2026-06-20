<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    domain: string;
    target: 'host' | 'container';
    onclose: () => void;
    onconfirm: () => Promise<{ ok: boolean; error?: string }>;
  }
  let { open, domain, target, onclose, onconfirm }: Props = $props();

  let busy = $state(false);
  let error = $state('');

  const targetLabel = $derived(
    target === 'host'
      ? m.services_divergence_targetHost()
      : m.services_divergence_targetContainer()
  );

  function safeClose() {
    if (busy) return;
    error = '';
    onclose();
  }

  async function confirm() {
    busy = true;
    error = '';
    try {
      const res = await onconfirm();
      if (!res.ok) {
        error = m.services_backend_switchFailed({ error: res.error || 'unknown error' });
        return;
      }
      onclose();
    } finally {
      busy = false;
    }
  }
</script>

<Modal {open} title={m.services_divergence_title()} onclose={safeClose} size="md">
  <div class="px-5 py-4 space-y-3">
    <p class="text-sm text-gray-700 dark:text-gray-300">
      {m.services_divergence_body({ domain, target: targetLabel })}
    </p>
    {#if error}
      <p class="text-xs text-red-500">{error}</p>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={busy}>{m.common_cancel()}</DetailButton>
    <DetailButton tone="primary" onclick={confirm} loading={busy} disabled={busy}>
      {busy ? m.services_divergence_switching() : m.services_divergence_confirm()}
    </DetailButton>
  {/snippet}
</Modal>
