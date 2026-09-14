<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { formatBytes } from '$stores/stats';
  import type { UsedImage } from '$stores/disk';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    images: UsedImage[];
    usedBytes: number;
    onclose: () => void;
  }
  let { open, images, usedBytes, onclose }: Props = $props();
</script>

<Modal {open} title={m.dashboard_disk_usageTitle()} onclose={onclose} size="md">
  <div class="px-5 py-4 space-y-3">
    <p class="text-sm text-gray-700 dark:text-gray-300">
      {m.dashboard_disk_usageBody({ size: formatBytes(usedBytes), count: images.length })}
    </p>

    {#if images.length > 0}
      <div class="rounded-lg border border-gray-200 dark:border-lerd-border divide-y divide-gray-100 dark:divide-lerd-border max-h-72 overflow-y-auto">
        {#each images as img (img.ref)}
          <div class="flex items-center gap-2 px-3 py-1.5 text-xs">
            <span class="flex-1 min-w-0">
              <span class="block truncate text-gray-700 dark:text-gray-200">{img.ref}</span>
              <span class="block text-[10px] text-gray-400 dark:text-gray-500">
                {img.in_use ? m.dashboard_disk_inUse() : m.dashboard_disk_idle()}
              </span>
            </span>
            <span class="shrink-0 font-mono tabular-nums text-gray-500 dark:text-gray-400">{formatBytes(img.bytes)}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={onclose}>{m.common_close()}</DetailButton>
  {/snippet}
</Modal>
