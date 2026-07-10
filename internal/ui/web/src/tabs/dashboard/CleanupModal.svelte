<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { formatBytes } from '$stores/stats';
  import type { DiskImage } from '$stores/disk';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    images: DiskImage[];
    reclaimableBytes: number;
    loading?: boolean;
    error?: string;
    onconfirm: () => void;
    onclose: () => void;
  }
  let { open, images, reclaimableBytes, loading = false, error, onconfirm, onclose }: Props =
    $props();
</script>

<Modal {open} title={m.dashboard_disk_modalTitle()} onclose={onclose} size="md">
  <div class="px-5 py-4 space-y-3">
    <p class="text-sm text-gray-700 dark:text-gray-300">
      {m.dashboard_disk_modalBody({ size: formatBytes(reclaimableBytes) })}
    </p>

    {#if images.length > 0}
      <div class="rounded-lg border border-gray-200 dark:border-lerd-border divide-y divide-gray-100 dark:divide-lerd-border max-h-56 overflow-y-auto">
        {#each images as img (img.id)}
          <div class="flex items-center gap-2 px-3 py-1.5 text-xs">
            <span class="flex-1 truncate text-gray-600 dark:text-gray-300">{img.desc}</span>
            <span class="shrink-0 font-mono tabular-nums text-gray-500 dark:text-gray-400">{formatBytes(img.bytes)}</span>
          </div>
        {/each}
      </div>
    {/if}

    <p class="text-xs text-gray-500 dark:text-gray-400">{m.dashboard_disk_modalDeepNote()}</p>

    {#if error}
      <p class="text-xs text-red-600 dark:text-red-400">{error}</p>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={onclose} disabled={loading}>{m.common_cancel()}</DetailButton>
    <DetailButton tone="danger" onclick={onconfirm} loading={loading} disabled={loading}>
      {loading ? m.dashboard_disk_cleaning() : m.dashboard_disk_modalConfirm()}
    </DetailButton>
  {/snippet}
</Modal>
