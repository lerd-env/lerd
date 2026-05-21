<script lang="ts">
  import { onMount } from 'svelte';
  import { profilerEnabled, loadProfilerStatus, setProfiler } from '$stores/profiler';
  import { dashboardIconSvg } from '$lib/dashboardIcons';
  import { m } from '../paraglide/messages.js';
  import PulseToggle from './PulseToggle.svelte';

  // Antenna-style toggle for the global SPX profiler. When on, every
  // PHP-FPM site's HTTP requests are profiled into flame graphs; the
  // pulsing dot advertises that profiling is live.

  let busy = $state(false);
  const enabled = $derived($profilerEnabled);

  onMount(() => {
    void loadProfilerStatus();
  });

  async function onclick(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (busy) return;
    busy = true;
    try {
      await setProfiler(!enabled);
    } finally {
      busy = false;
    }
  }

  const title = $derived(
    busy ? m.profiler_toggle_busy() : enabled ? m.profiler_toggle_on() : m.profiler_toggle_off()
  );
</script>

<PulseToggle {enabled} {busy} {title} {onclick}>
  <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    {@html dashboardIconSvg('profiler')}
  </svg>
</PulseToggle>
