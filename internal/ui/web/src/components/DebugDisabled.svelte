<script lang="ts">
  import { setDebugCapture } from '$stores/queries';
  import CaptureOffNotice from '$components/CaptureOffNotice.svelte';
  import { m } from '../paraglide/messages.js';
</script>

<CaptureOffNotice title={m.debug_disabled_title()} body={m.debug_disabled_body()} onenable={() => setDebugCapture(true)} />
