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

  // Split by owner so it is obvious how much is lerd cleaning up after itself
  // and how much is everything else the deep tier reaches on this machine.
  const groups = $derived(
    [
      { label: m.dashboard_disk_ownerLerd(), items: images.filter((i) => i.owner !== 'other') },
      { label: m.dashboard_disk_ownerOther(), items: images.filter((i) => i.owner === 'other') }
    ].filter((g) => g.items.length > 0)
  );
</script>

<Modal {open} title={m.dashboard_disk_modalTitle()} onclose={onclose} size="md">
  <div class="px-5 py-4 space-y-3">
    <p class="text-sm text-gray-700 dark:text-gray-300">
      {m.dashboard_disk_modalBody({ size: formatBytes(reclaimableBytes) })}
    </p>

    {#if images.length > 0}
      <div class="rounded-lg border border-gray-200 dark:border-lerd-border divide-y divide-gray-100 dark:divide-lerd-border max-h-56 overflow-y-auto">
        {#each groups as group (group.label)}
          <div class="px-3 py-1 bg-gray-50 dark:bg-lerd-bg text-[10px] font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
            {group.label} ({group.items.length})
          </div>
          {#each group.items as img (img.id)}
            <div class="flex items-center gap-2 px-3 py-1.5 text-xs">
              <span class="flex-1 min-w-0">
                <span class="block truncate text-gray-700 dark:text-gray-200">{img.id}</span>
                <span class="block truncate text-[10px] text-gray-400 dark:text-gray-500">{img.desc}</span>
              </span>
              <span class="shrink-0 font-mono tabular-nums text-gray-500 dark:text-gray-400">{formatBytes(img.bytes)}</span>
            </div>
          {/each}
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
