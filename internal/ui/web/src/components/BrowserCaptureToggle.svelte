<script lang="ts">
  import { onMount } from 'svelte';
  import { browserCaptureEnabled, loadBrowserCaptureStatus, setBrowserCapture } from '$stores/browserCapture';
  import { m } from '../paraglide/messages.js';
  import PulseToggle from './PulseToggle.svelte';

  // Toggle for browser capture: when on, every PHP-FPM site's pages report
  // their JavaScript errors to the Debug window's Browser lens.

  let busy = $state(false);
  const enabled = $derived($browserCaptureEnabled);

  onMount(() => {
    void loadBrowserCaptureStatus();
  });

  async function onclick(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (busy) return;
    busy = true;
    try {
      await setBrowserCapture(!enabled);
    } finally {
      busy = false;
    }
  }

  const title = $derived(busy ? m.browser_toggle_busy() : enabled ? m.browser_toggle_on() : m.browser_toggle_off());
</script>

<PulseToggle {enabled} {busy} {title} {onclick}>
  <!-- Browser window with an alert mark. -->
  <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24">
    <rect x="3" y="4" width="18" height="16" rx="2" />
    <path d="M3 9h18" />
    <path d="M12 12.5v3" />
    <circle cx="12" cy="17.6" r="0.6" fill="currentColor" stroke="none" />
  </svg>
</PulseToggle>
