<script lang="ts">
  import LensToggle from '$components/LensToggle.svelte';
  import { showTests } from '$stores/debugLens';
  import { hiddenTestCount } from '$stores/debugEvents';
  import { m } from '../paraglide/messages.js';
</script>

<LensToggle
  label={m.debug_show_tests()}
  checked={$showTests}
  hint={$hiddenTestCount > 0 ? m.debug_tests_hidden({ count: $hiddenTestCount }) : ''}
  onchange={(v) => showTests.set(v)}
/>
