<script lang="ts">
  import { onMount } from 'svelte';
  import { siteCaptureOn, loadSiteBrowserLogs, saveSiteBrowserLogs } from '$stores/browserLogs';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  // Turns browser logs on or off for one site from its header; what it
  // reports is set in the site's Debug → Browser settings.

  interface Props {
    site: string;
    class?: string;
  }
  let { site, class: cls = '' }: Props = $props();

  let busy = $state(false);
  const on = $derived($siteCaptureOn[site] === true);

  onMount(() => {
    loadSiteBrowserLogs(site).catch(() => {});
  });

  async function onclick() {
    if (busy) return;
    busy = true;
    try {
      const s = await loadSiteBrowserLogs(site);
      await saveSiteBrowserLogs(site, { ...s, enabled: !s.enabled });
    } finally {
      busy = false;
    }
  }

  const title = $derived(busy ? m.browser_toggle_busy() : on ? m.browser_toggle_on() : m.browser_toggle_off());
</script>

<button
  type="button"
  {onclick}
  disabled={busy}
  aria-label={m.debug_tab_browser()}
  aria-pressed={on}
  use:tooltip={title}
  class="{cls} w-8 h-8 items-center justify-center rounded-md transition-colors disabled:opacity-50 {on
    ? 'text-emerald-500 dark:text-emerald-400 hover:bg-emerald-50 dark:hover:bg-emerald-900/20'
    : 'text-gray-500 dark:text-gray-400 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5'}"
>
  <!-- Browser window with console lines; >_ alone would read as the terminal action. -->
  <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24">
    <rect x="3" y="4" width="18" height="16" rx="2" />
    <path d="M3 9h18" />
    <path d="m6.5 12.5 1.5 1.25-1.5 1.25" />
    <path d="M10 13.75h7" />
    <path d="M6.5 17h10.5" />
  </svg>
</button>
