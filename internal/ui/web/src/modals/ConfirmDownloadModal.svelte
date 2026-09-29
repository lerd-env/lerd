<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { downloadConfirm, answerDownloadConfirm } from '$stores/downloadConfirm';
  import { formatBytes } from '$lib/bytes';
  import { m } from '../paraglide/messages.js';
</script>

<Modal
  open={$downloadConfirm.open}
  title={m.services_download_title()}
  onclose={() => answerDownloadConfirm(false)}
  size="sm"
>
  <div class="px-5 py-4 space-y-2">
    <p class="text-sm text-gray-700 dark:text-gray-300">
      {#if ($downloadConfirm.download?.bytes ?? 0) > 0}
        {m.services_download_body({
          name: $downloadConfirm.name,
          size: formatBytes($downloadConfirm.download?.bytes ?? 0)
        })}
      {:else}
        {m.services_download_bodyUnknown({ name: $downloadConfirm.name })}
      {/if}
    </p>
    <div class="text-[11px] text-gray-500 dark:text-gray-400 font-mono truncate">
      {$downloadConfirm.download?.image ?? ''}
    </div>
  </div>

  {#snippet footer()}
    <DetailButton onclick={() => answerDownloadConfirm(false)}>{m.common_cancel()}</DetailButton>
    <DetailButton tone="primary" onclick={() => answerDownloadConfirm(true)}>
      {m.services_download_confirm()}
    </DetailButton>
  {/snippet}
</Modal>
