<script lang="ts">
  import { onMount } from 'svelte';
  import { status, refreshStatus, toggleDumps } from '$stores/dumps';
  import { m } from '../paraglide/messages.js';
  import PulseToggle from './PulseToggle.svelte';

  // Antenna-style toggle for the lerd dump bridge. When on, every PHP-FPM
  // container has dump()/dd() shipped into the dashboard; the pulsing dot
  // advertises that captures are live.

  let busy = $state(false);
  const enabled = $derived(Boolean($status?.enabled));

  onMount(() => {
    void refreshStatus();
  });

  async function onclick(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (busy) return;
    busy = true;
    try {
      await toggleDumps(!enabled);
      await refreshStatus();
    } finally {
      busy = false;
    }
  }

  const title = $derived(
    busy ? m.dumps_toggle_busy() : enabled ? m.dumps_toggle_on() : m.dumps_toggle_off()
  );
</script>

<PulseToggle {enabled} {busy} {title} {onclick}>
  <!-- Antenna tower: triangular mast, two cross-struts, transmitter tip. -->
  <svg
    class="w-3.5 h-3.5"
    fill="none"
    stroke="currentColor"
    stroke-width="1.75"
    stroke-linecap="round"
    stroke-linejoin="round"
    viewBox="0 0 24 24"
  >
    <path d="M12 4 L6 21" />
    <path d="M12 4 L18 21" />
    <path d="M9 12 L15 12" />
    <path d="M7.5 17 L16.5 17" />
    <circle cx="12" cy="4" r="1.25" fill="currentColor" stroke="none" />
  </svg>
</PulseToggle>
