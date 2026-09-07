<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal } from '$stores/modals';
  import { version } from '$stores/version';
  import { m } from '../paraglide/messages.js';
</script>

<Modal open title={m.system_lerd_whatsNew()} onclose={closeModal} size="lg">
  <div class="px-5 py-4 max-h-[60vh] overflow-y-auto">
    <pre class="text-xs text-gray-600 dark:text-gray-400 bg-gray-50 dark:bg-white/3 rounded-lg p-3 overflow-x-auto whitespace-pre-wrap font-mono leading-relaxed border border-gray-100 dark:border-lerd-border">{$version.changelog}</pre>
  </div>

  {#snippet footer()}
    <DetailButton onclick={closeModal}>{m.common_close()}</DetailButton>
  {/snippet}
</Modal>
