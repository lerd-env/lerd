<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import BrowserLogsSettings from '$components/BrowserLogsSettings.svelte';
  import { closeModal } from '$stores/modals';
  import { m } from '../paraglide/messages.js';

  interface Props {
    site: string;
  }
  let { site }: Props = $props();
</script>

<Modal open title={m.browser_settings_title({ site })} onclose={closeModal}>
  <BrowserLogsSettings {site} />
</Modal>
