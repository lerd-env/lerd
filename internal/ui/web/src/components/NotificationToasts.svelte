<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import { inAppNotifications, dismissInApp, type InAppNotification } from '$lib/notify';
  import { m } from '../paraglide/messages.js';

  // A failure stays until it is dismissed; anything else is informational and
  // clears itself, so a burst of finished operations never piles up on screen.
  const autoDismissMs = 6000;
  const timers = new Map<number, ReturnType<typeof setTimeout>>();

  $effect(() => {
    for (const n of $inAppNotifications) {
      if (n.failed || timers.has(n.id)) continue;
      timers.set(
        n.id,
        setTimeout(() => {
          timers.delete(n.id);
          dismissInApp(n.id);
        }, autoDismissMs)
      );
    }
    return () => {
      for (const t of timers.values()) clearTimeout(t);
      timers.clear();
    };
  });

  function open(n: InAppNotification) {
    if (n.url) location.hash = n.url.startsWith('#') ? n.url.slice(1) : n.url;
    dismissInApp(n.id);
  }
</script>

{#if $inAppNotifications.length > 0}
  <div class="fixed bottom-3 right-3 z-60 flex w-[min(92vw,380px)] flex-col gap-2">
    {#each $inAppNotifications as n (n.id)}
      <div
        role={n.failed ? 'alert' : 'status'}
        class="rounded-lg border border-l-4 bg-white/90 dark:bg-lerd-card/90 backdrop-blur-md shadow-2xl {n.failed
          ? 'border-red-300 dark:border-red-500/40 border-l-red-500'
          : 'border-gray-200 dark:border-lerd-border border-l-sky-500'}"
      >
        <div class="flex items-start gap-2.5 px-3 py-2.5">
          <Icon
            name={n.failed ? 'alert' : 'check'}
            class="mt-0.5 h-4 w-4 shrink-0 {n.failed
              ? 'text-red-500'
              : 'text-sky-600 dark:text-sky-400'}"
          />
          <div class="min-w-0 flex-1">
            <p class="text-xs font-semibold text-gray-800 dark:text-gray-100">{n.title}</p>
            {#if n.body}
              <p class="mt-0.5 text-[11px] text-gray-600 dark:text-gray-400">{n.body}</p>
            {/if}
            {#if n.url}
              <button
                type="button"
                onclick={() => open(n)}
                class="mt-1 text-[11px] font-medium text-sky-600 dark:text-sky-400 hover:underline"
              >{m.common_open()}</button>
            {/if}
          </div>
          <button
            type="button"
            aria-label={m.common_close()}
            onclick={() => dismissInApp(n.id)}
            class="flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-gray-400 hover:bg-gray-100 hover:text-lerd-red dark:hover:bg-white/5 transition-colors"
          >
            <Icon name="close" class="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
    {/each}
  </div>
{/if}
