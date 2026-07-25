<script lang="ts">
  import {
    type Site,
    type ShareToolsInfo,
    loadShareTools,
    loadSites,
    startTunnel,
    stopTunnel
  } from '$stores/sites';
  import Icon from '$components/Icon.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: Site;
    activeWorktreeBranch?: string;
    lanOn: boolean;
    lanBusy: boolean;
    lanUrl?: string;
    onToggleLan: () => void;
    // Visibility classes for the trigger, so the header can keep its
    // responsive slots ("flex" for the remote primary, "hidden @md:flex"
    // for the regular toggle).
    visibleClass?: string;
  }
  let {
    site,
    activeWorktreeBranch = '',
    lanOn,
    lanBusy,
    lanUrl = '',
    onToggleLan,
    visibleClass = 'flex'
  }: Props = $props();

  const MENU_WIDTH = 264;
  let open = $state(false);
  let menuX = $state(0);
  let menuY = $state(0);
  let btnEl: HTMLButtonElement | null = $state(null);
  let closeTimer: ReturnType<typeof setTimeout> | undefined;

  let toolsInfo: ShareToolsInfo | null = $state(null);
  let toolsLoading = $state(false);
  let tunnelBusy = $state(false);
  let startingLabel = $state('');
  let tunnelError = $state('');
  let errorTool = $state('');
  let cancelRequested = false;

  const tunnelUrl = $derived(site.tunnel_url ?? '');
  const tunnelTool = $derived(site.tunnel_tool ?? '');
  const tunnelOn = $derived(Boolean(tunnelUrl));
  // Tunnels are site-level (they front the primary domain), so the section
  // hides while a worktree is the active view.
  const showTunnelSection = $derived(!activeWorktreeBranch);
  const autoTool = $derived(toolsInfo?.tools.find((t) => t.name === toolsInfo?.auto));

  function toolLabel(name: string): string {
    return toolsInfo?.tools.find((t) => t.name === name)?.label || name;
  }

  function installHint(tool: { binary: string; install_url?: string }): string {
    if (tool.binary === 'ssh') return m.share_needsSsh();
    if (!tool.install_url) return m.share_notInstalled();
    return m.share_notInstalled() + ' · ' + new URL(tool.install_url).host;
  }

  function openMenu() {
    clearTimeout(closeTimer);
    if (!open) {
      open = true;
      // Refetch on every open so a tool installed mid-session shows up;
      // stale data stays rendered while the refresh is in flight.
      if (!toolsLoading) {
        toolsLoading = !toolsInfo;
        loadShareTools()
          .then((info) => (toolsInfo = info))
          .catch(() => {})
          .finally(() => (toolsLoading = false));
      }
    }
    if (btnEl) {
      const r = btnEl.getBoundingClientRect();
      menuX = Math.max(8, Math.min(r.right - MENU_WIDTH, window.innerWidth - MENU_WIDTH - 8));
      menuY = r.bottom + 6;
    }
  }

  function scheduleClose() {
    clearTimeout(closeTimer);
    closeTimer = setTimeout(() => (open = false), 250);
  }

  function closeNow() {
    clearTimeout(closeTimer);
    open = false;
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') closeNow();
  }

  async function startTool(tool: string, label: string) {
    if (tunnelBusy) return;
    tunnelBusy = true;
    startingLabel = label;
    tunnelError = '';
    cancelRequested = false;
    try {
      const res = await startTunnel(site, tool);
      if (!res.ok && !cancelRequested) {
        tunnelError = res.error || m.common_requestFailed();
        errorTool = label;
      }
      await loadSites();
    } finally {
      tunnelBusy = false;
    }
  }

  async function cancelStart() {
    cancelRequested = true;
    await stopTunnel(site);
  }

  let stopBusy = $state(false);

  async function stopT() {
    if (stopBusy) return;
    stopBusy = true;
    try {
      await stopTunnel(site);
      await loadSites();
    } finally {
      stopBusy = false;
    }
  }

  // The button acts on the state it shows: a globe (tunnel running) stops
  // the tunnel, a wifi icon toggles the LAN share.
  function onButtonClick() {
    if (tunnelOn) stopT();
    else onToggleLan();
  }

  const itemClass =
    'w-full px-3 py-1.5 text-xs text-left flex items-center gap-2 transition-colors';
  const itemIdle =
    'text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-white/5';
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="relative"
  role="none"
  onmouseenter={openMenu}
  onmouseleave={scheduleClose}
  onfocusin={openMenu}
  onfocusout={scheduleClose}
>
  <button
    bind:this={btnEl}
    type="button"
    onclick={onButtonClick}
    aria-label={tunnelOn
      ? m.share_stopTunnel()
      : lanOn
        ? m.sites_controls_lanToggle_on()
        : m.sites_controls_lanToggle_off()}
    aria-haspopup="menu"
    aria-expanded={open}
    class="{visibleClass} w-8 h-8 items-center justify-center rounded-md transition-colors {tunnelOn
      ? 'text-violet-500 dark:text-violet-400 hover:bg-violet-50 dark:hover:bg-violet-900/20'
      : lanOn
        ? 'text-teal-500 dark:text-teal-400 hover:bg-teal-50 dark:hover:bg-teal-900/20'
        : 'text-gray-500 dark:text-gray-400 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5'}"
  >
    {#if lanBusy || tunnelBusy || stopBusy}
      <Icon name="spinner" class="w-4 h-4 animate-spin" />
    {:else if tunnelOn}
      <Icon name="globe" class="w-4 h-4" />
    {:else}
      <Icon name="wifi" class="w-4 h-4" />
    {/if}
  </button>

  {#if open}
    <div
      role="menu"
      aria-label={m.share_menuLabel({ domain: site.domain })}
      data-testid="share-menu"
      style="position:fixed; left:{menuX}px; top:{menuY}px; width:{MENU_WIDTH}px; z-index:40"
      class="rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-bg shadow-lg py-1"
    >
      <div class="px-3 pt-1.5 pb-0.5 text-[9px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500">
        {m.share_localNetwork()}
      </div>
      {#if lanBusy}
        <div class="{itemClass} text-gray-500 dark:text-gray-400">
          <Icon name="spinner" class="w-3.5 h-3.5 shrink-0 animate-spin" />
          {m.share_switching()}
        </div>
      {:else if lanOn}
        <div class="{itemClass} border-l-2 border-teal-500 bg-teal-50/60 dark:bg-teal-900/15">
          <Icon name="wifi" class="w-3.5 h-3.5 shrink-0 text-teal-600 dark:text-teal-400" />
          <span class="flex-1 min-w-0">
            <span class="block font-medium text-gray-700 dark:text-gray-200">{m.share_lanActive()}</span>
            {#if lanUrl}
              <a href={lanUrl} target="_blank" rel="noopener" class="block font-mono text-[10px] text-teal-600 dark:text-teal-400 truncate hover:underline">{lanUrl}</a>
            {/if}
          </span>
          <button
            type="button"
            role="menuitem"
            onclick={onToggleLan}
            class="shrink-0 text-[10px] font-medium px-2 py-0.5 rounded border border-gray-200 dark:border-lerd-border text-gray-500 dark:text-gray-400 hover:text-red-600 hover:border-red-400 dark:hover:text-red-400 transition-colors"
          >{m.share_stop()}</button>
        </div>
      {:else}
        <button type="button" role="menuitem" onclick={onToggleLan} class="{itemClass} {itemIdle}">
          <Icon name="wifi" class="w-3.5 h-3.5 shrink-0" />
          <span class="flex-1 min-w-0 text-left">
            <span class="block font-medium">{m.sites_controls_lanToggle_off()}</span>
            <span class="block text-[10px] text-gray-500 dark:text-gray-400">{m.share_lanDevices()}</span>
          </span>
        </button>
      {/if}

      {#if showTunnelSection}
        <div class="my-1 border-t border-gray-100 dark:border-lerd-border"></div>
        <div class="px-3 pt-0.5 pb-0.5 text-[9px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500">
          {m.share_publicTunnel()}
        </div>

        {#if tunnelBusy}
          <div class="{itemClass} text-gray-700 dark:text-gray-200">
            <Icon name="spinner" class="w-3.5 h-3.5 shrink-0 animate-spin" />
            <span class="flex-1 min-w-0">
              <span class="block font-medium">{m.share_starting({ tool: startingLabel })}</span>
              <span class="block text-[10px] text-gray-500 dark:text-gray-400">{m.share_waitingURL()}</span>
            </span>
            <button
              type="button"
              role="menuitem"
              onclick={cancelStart}
              class="shrink-0 text-[10px] font-medium px-2 py-0.5 rounded border border-gray-200 dark:border-lerd-border text-gray-500 dark:text-gray-400 hover:text-red-600 hover:border-red-400 dark:hover:text-red-400 transition-colors"
            >{m.common_cancel()}</button>
          </div>
        {:else if tunnelOn}
          <div class="{itemClass} border-l-2 border-violet-500 bg-violet-50/60 dark:bg-violet-900/15">
            <Icon name="globe" class="w-3.5 h-3.5 shrink-0 text-violet-600 dark:text-violet-400" />
            <span class="flex-1 min-w-0">
              <span class="block font-medium text-gray-700 dark:text-gray-200">{m.share_tunnelVia({ tool: toolLabel(tunnelTool) })}</span>
              <a href={tunnelUrl} target="_blank" rel="noopener" class="block font-mono text-[10px] text-violet-600 dark:text-violet-400 truncate hover:underline">{tunnelUrl}</a>
            </span>
            <button
              type="button"
              role="menuitem"
              onclick={stopT}
              class="shrink-0 text-[10px] font-medium px-2 py-0.5 rounded border border-gray-200 dark:border-lerd-border text-gray-500 dark:text-gray-400 hover:text-red-600 hover:border-red-400 dark:hover:text-red-400 transition-colors"
            >{m.share_stop()}</button>
          </div>
        {:else}
          {#if tunnelError}
            <button
              type="button"
              role="menuitem"
              onclick={() => (tunnelError = '')}
              class="{itemClass} text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20"
            >
              <Icon name="alert" class="w-3.5 h-3.5 shrink-0" />
              <span class="flex-1 min-w-0">
                <span class="block font-medium">{m.share_startFailed({ tool: errorTool })}</span>
                <span class="block text-[10px] break-words">{tunnelError}</span>
              </span>
            </button>
          {/if}
          {#if toolsLoading}
            <div class="{itemClass} text-gray-500 dark:text-gray-400">
              <Icon name="spinner" class="w-3.5 h-3.5 shrink-0 animate-spin" />
            </div>
          {:else if toolsInfo}
            <button
              type="button"
              role="menuitem"
              disabled={!autoTool}
              onclick={() => startTool('', m.share_autoLabel())}
              class="{itemClass} {autoTool ? itemIdle : 'text-gray-400 dark:text-gray-600'}"
            >
              <Icon name="external" class="w-3.5 h-3.5 shrink-0" />
              <span class="flex-1 min-w-0 text-left">
                <span class="block font-medium">{m.share_viaTunnel()}</span>
                <span class="block text-[10px] {autoTool ? 'text-gray-500 dark:text-gray-400' : ''}">
                  {autoTool ? m.share_autoPicks({ tool: autoTool.label }) : m.share_noTools()}
                </span>
              </span>
            </button>
            {#each toolsInfo.tools as tool (tool.name)}
              <button
                type="button"
                role="menuitem"
                disabled={!tool.installed}
                onclick={() => startTool(tool.name, tool.label)}
                class="{itemClass} {tool.installed ? itemIdle : 'text-gray-400 dark:text-gray-600'}"
              >
                <Icon name="globe" class="w-3.5 h-3.5 shrink-0" />
                <span class="flex-1 min-w-0 text-left">
                  <span class="block font-medium">{tool.label}</span>
                  <span class="block text-[10px] {tool.installed ? 'text-gray-500 dark:text-gray-400' : ''}">
                    {tool.installed ? tool.binary : installHint(tool)}
                  </span>
                </span>
              </button>
            {/each}
          {/if}
        {/if}
      {/if}
    </div>
  {/if}
</div>
