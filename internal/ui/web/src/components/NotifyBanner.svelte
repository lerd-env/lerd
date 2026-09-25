<script lang="ts">
  import {
    permissionState,
    dismissed,
    notifyDelivery,
    enableNotifications,
    dismissNotifyBanner
  } from '$lib/notify';
  import { onFallbackOrigin } from '$lib/vhost';
  import { m } from '../paraglide/messages.js';
  import BottomBanner from './BottomBanner.svelte';

  // The banner asks for browser notification permission, which is meaningless
  // when the daemon is delivering natively, so it only shows on the browser
  // sink. It also stays away from the loopback fallback origin: permission is
  // per-origin, and that page hands over to the vhost as soon as nginx is back,
  // so granting it there would leave a second browser subscribed for good.
  const visible = $derived(
    $permissionState === 'default' &&
      !$dismissed &&
      $notifyDelivery !== 'native' &&
      !onFallbackOrigin()
  );

  async function onEnable() {
    await enableNotifications();
  }
  function onDismiss() {
    dismissNotifyBanner();
  }
</script>

{#if visible}
  <BottomBanner
    title={m.notify_banner_title()}
    subtitle={m.notify_banner_subtitle()}
    actionLabel={m.notify_banner_enable()}
    dismissLabel={m.notify_banner_dismiss()}
    onAction={onEnable}
    onDismiss={onDismiss}
  >
    {#snippet icon()}
      <svg class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" d="M15 17h5l-1.4-1.4A2 2 0 0118 14.2V11a6 6 0 10-12 0v3.2a2 2 0 01-.6 1.4L4 17h5m6 0a3 3 0 11-6 0"/>
      </svg>
    {/snippet}
  </BottomBanner>
{/if}
