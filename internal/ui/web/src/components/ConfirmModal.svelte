<script lang="ts">
  import type { Snippet } from 'svelte';
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
    // Holds the confirm back, for a choice in `extra` that leaves nothing to do.
    disabled?: boolean;
    size?: 'sm' | 'md';
    // Extra controls rendered under the body, for a confirmation that also
    // carries a choice (a checkbox, a scope toggle) rather than only a yes/no.
    extra?: Snippet;
    onconfirm: () => void;
    onclose: () => void;
  }
  let { open, title, body, confirmLabel, danger = false, loading = false, disabled = false, size = 'sm', extra, onconfirm, onclose }: Props = $props();
</script>

<Modal {open} {title} onclose={() => { if (!loading) onclose(); }} {size}>
  <div class="px-5 py-4">
    <p class="text-sm text-gray-700 dark:text-gray-300">{body}</p>
    {#if extra}
      <div class="mt-3">{@render extra()}</div>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={onclose} disabled={loading}>{m.common_cancel()}</DetailButton>
    <DetailButton tone={danger ? 'danger' : 'primary'} onclick={onconfirm} loading={loading} disabled={loading || disabled}>
      {confirmLabel}
    </DetailButton>
  {/snippet}
</Modal>
