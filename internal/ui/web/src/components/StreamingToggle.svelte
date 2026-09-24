<script lang="ts">
  import Icon from './Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { status } from '$stores/status';
  import { setStreamingMode } from '$stores/workspaces';
  import { m } from '../paraglide/messages.js';

  const on = $derived(Boolean($status.streaming_mode));
  const label = $derived(on ? m.nav_streamingOn() : m.nav_streamingOff());

  async function toggle() {
    const res = await setStreamingMode(!on);
    if (!res.ok) console.error('streaming mode failed:', res.error);
  }
</script>

<button
  type="button"
  onclick={toggle}
  aria-label={label}
  aria-pressed={on}
  use:tooltip={{ label, placement: 'bottom' }}
  class="inline-flex items-center justify-center w-5 h-5 rounded transition-colors {on
    ? 'text-lerd-red'
    : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-200'}"
>
  <Icon name="eyeOff" class="w-3.5 h-3.5" />
</button>
