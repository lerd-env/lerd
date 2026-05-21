<script lang="ts">
  import { get } from 'svelte/store';
  import {
    notifyPrefs,
    permissionState,
    autoSubscribeDisabled,
    setNotifyMaster,
    enableNotifications
  } from '$lib/notify';
  import { m } from '../paraglide/messages.js';
  import PulseToggle from './PulseToggle.svelte';

  // Bell toggle for lerd notifications, matching the dump bridge and
  // profiler toggles. A browser-denied permission can't be flipped here;
  // the toggle dims and System > Notifications carries the recovery flow.

  let busy = $state(false);

  const enabled = $derived(
    $permissionState === 'granted' && !$autoSubscribeDisabled && $notifyPrefs.enabled
  );
  const blocked = $derived($permissionState === 'denied' || $permissionState === 'unsupported');

  async function onclick(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (busy || blocked) return;
    busy = true;
    try {
      // 'default' and a forgotten-but-granted browser both need the permission
      // prompt / re-subscribe before the master flag means anything.
      if ($permissionState === 'default') {
        await enableNotifications();
        return;
      }
      if ($permissionState === 'granted' && $autoSubscribeDisabled) {
        await enableNotifications();
        if (!get(notifyPrefs).enabled) setNotifyMaster(true);
        return;
      }
      setNotifyMaster(!$notifyPrefs.enabled);
    } finally {
      busy = false;
    }
  }

  const title = $derived(
    busy
      ? m.notify_toggle_busy()
      : blocked
        ? m.notify_toggle_blocked()
        : enabled
          ? m.notify_toggle_on()
          : m.notify_toggle_off()
  );
</script>

<PulseToggle {enabled} busy={busy || blocked} {title} {onclick}>
  <!-- Bell. -->
  <svg
    class="w-3.5 h-3.5"
    fill="none"
    stroke="currentColor"
    stroke-width="1.75"
    stroke-linecap="round"
    stroke-linejoin="round"
    viewBox="0 0 24 24"
  >
    <path
      d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"
    />
  </svg>
</PulseToggle>
