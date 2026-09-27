<script lang="ts">
  import ToggleButton from '$components/ToggleButton.svelte';
  import { m } from '../../paraglide/messages.js';

  // Octane (FrankenPHP worker mode) control. Serving via Octane IS the site
  // runtime, so it can't be turned off here: the left segment is a disabled,
  // always-on ToggleButton that signals "Octane is running", joined to a refresh
  // toggle that flips octane:start --watch. When watch is on the refresh icon
  // lights emerald and glows, mirroring HorizonControl's reload segment.
  interface Props {
    reload?: boolean;
    reloadLoading?: boolean;
    onToggleReload: () => void;
  }
  let { reload = false, reloadLoading = false, onToggleReload }: Props = $props();

  // While a reload toggle is in flight the desired state is in flux, so show the
  // icon neutral (not a misleading green glow that lags intent) until the
  // restart completes.
  const iconClass = $derived(
    reloadLoading
      ? 'text-gray-500 dark:text-gray-400'
      : reload
        ? 'text-emerald-500 drop-shadow-[0_0_4px_rgba(16,185,129,0.9)]'
        : 'text-gray-500 dark:text-gray-400'
  );

  // One full turn of the refresh icon as click feedback, with a timeout fallback
  // in case animationend never fires (reduced motion, hidden element).
  let spinning = $state(false);
  let spinTimer: ReturnType<typeof setTimeout>;
  function clickReload() {
    spinning = true;
    clearTimeout(spinTimer);
    spinTimer = setTimeout(() => (spinning = false), 800);
    onToggleReload();
  }
</script>

<div class="inline-flex items-center">
  <ToggleButton
    label={m.sites_controls_octane()}
    on={true}
    disabled={true}
    rounding="rounded-l-md border-r-0"
    title={m.sites_controls_octaneRunning()}
  />
  <button
    type="button"
    disabled={reloadLoading}
    aria-pressed={reload}
    aria-busy={reloadLoading}
    title={reload
      ? m.sites_controls_octaneReloadToggle_on()
      : m.sites_controls_octaneReloadToggle_off()}
    onclick={clickReload}
    class="inline-flex items-center justify-center h-7 w-8 rounded-r-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card hover:bg-gray-50 dark:hover:bg-white/5 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
  >
    <svg
      class="w-3.5 h-3.5 transition-colors {iconClass} {spinning ? 'animate-[spin_0.6s_ease-in-out]' : ''}"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      onanimationend={() => (spinning = false)}
    >
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
    </svg>
  </button>
</div>
