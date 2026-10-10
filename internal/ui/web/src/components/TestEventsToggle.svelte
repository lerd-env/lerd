<script lang="ts">
  import { onMount } from 'svelte';
  import { readable } from 'svelte/store';
  import LensToggle from '$components/LensToggle.svelte';
  import { showTests } from '$stores/debugLens';
  import { lensCounts } from '$stores/debugEvents';
  import { refreshDevtoolsStatus, toggleDevtoolsTests } from '$stores/queries';
  import { m } from '../paraglide/messages.js';

  const hidden = lensCounts()?.hiddenTests ?? readable(0);

  onMount(() => {
    void refreshDevtoolsStatus();
  });
</script>

<LensToggle
  label={m.debug_show_tests()}
  checked={$showTests}
  hint={$hidden > 0 ? m.debug_tests_hidden({ count: $hidden }) : ''}
  onchange={(v) => void toggleDevtoolsTests(v)}
/>
