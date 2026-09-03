<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import SnapshotScheduleFields from '$components/SnapshotScheduleFields.svelte';
  import { m } from '../paraglide/messages.js';

  interface Props {
    onclose: () => void;
  }
  let { onclose }: Props = $props();
</script>

<Modal open title={m.snapshots_auto_settings()} {onclose} size="md">
  <div class="px-5 py-4">
    <SnapshotScheduleFields />
  </div>
  {#snippet footer()}
    <DetailButton onclick={onclose}>{m.common_close()}</DetailButton>
  {/snippet}
</Modal>
