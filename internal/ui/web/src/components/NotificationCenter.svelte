<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import Popover from '$components/Popover.svelte';
  import {
    notificationHistory,
    unreadNotifications,
    markNotificationsRead,
    clearNotificationHistory,
    type NotificationRecord
  } from '$lib/notify';
  import { m } from '../paraglide/messages.js';

  interface Props {
    size?: 'sm' | 'md';
    align?: 'left' | 'right';
  }
  let { size = 'sm', align = 'left' }: Props = $props();

  function activate(n: NotificationRecord, close: () => void) {
    if (n.url) location.hash = n.url.startsWith('#') ? n.url.slice(1) : n.url;
    close();
  }

  // Relative time keeps the list readable without a date library; anything
  // older than a day is rare here since the list is capped.
  function ago(at: number): string {
    const s = Math.max(0, Math.round((Date.now() - at) / 1000));
    if (s < 60) return s + 's';
    if (s < 3600) return Math.round(s / 60) + 'm';
    if (s < 86400) return Math.round(s / 3600) + 'h';
    return Math.round(s / 86400) + 'd';
  }
</script>

<Popover label={m.nav_notifications()} {size} {align} onopen={markNotificationsRead}>
  {#snippet trigger()}
    <span class="relative flex items-center justify-center">
      <Icon name="bell" class="w-5 h-5" />
      {#if $unreadNotifications > 0}
        <span
          class="absolute -top-1 -right-1 min-w-3.5 h-3.5 px-0.5 rounded-full bg-lerd-red text-white text-[9px] font-semibold flex items-center justify-center"
        >{$unreadNotifications > 9 ? '9+' : $unreadNotifications}</span>
      {/if}
    </span>
  {/snippet}

  {#snippet children(close: () => void)}
    <div
      class="flex items-center justify-between gap-2 border-b border-gray-100 dark:border-lerd-border/60 px-3 py-2"
    >
      <p class="text-xs font-semibold text-gray-800 dark:text-gray-100">{m.notify_center_title()}</p>
      {#if $notificationHistory.length > 0}
        <button
          type="button"
          onclick={clearNotificationHistory}
          class="text-[11px] text-gray-400 hover:text-lerd-red transition-colors"
        >{m.common_clear()}</button>
      {/if}
    </div>

    {#if $notificationHistory.length === 0}
      <p class="px-3 py-4 text-[11px] text-gray-400 dark:text-gray-500">{m.notify_center_empty()}</p>
    {:else}
      <ul class="max-h-[60vh] overflow-y-auto divide-y divide-gray-100 dark:divide-lerd-border/60">
        {#each $notificationHistory as n (n.id)}
          <li>
            <button
              type="button"
              onclick={() => activate(n, close)}
              class="flex w-full items-start gap-2 px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-white/5 transition-colors"
            >
              <Icon
                name={n.failed ? 'alert' : 'check'}
                class="mt-0.5 h-3.5 w-3.5 shrink-0 {n.failed
                  ? 'text-red-500'
                  : 'text-gray-300 dark:text-gray-600'}"
              />
              <span class="min-w-0 flex-1">
                <span class="block truncate text-xs font-medium text-gray-800 dark:text-gray-100"
                  >{n.title}</span
                >
                {#if n.body}
                  <span class="mt-0.5 block text-[11px] text-gray-500 dark:text-gray-400 line-clamp-2"
                    >{n.body}</span
                  >
                {/if}
              </span>
              <span class="shrink-0 text-[10px] tabular-nums text-gray-400">{ago(n.at)}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  {/snippet}
</Popover>
