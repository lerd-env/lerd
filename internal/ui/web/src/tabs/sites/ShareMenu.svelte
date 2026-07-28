<script lang="ts">
  import {
    type Site,
    type ShareToolsInfo,
    loadShareTools,
    loadSites,
    saveShareDomain,
    startTunnel,
    stopTunnel
  } from '$stores/sites';
  import Icon from '$components/Icon.svelte';
  import ShareDomainModal from './ShareDomainModal.svelte';
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

  // A worktree is served on its own subdomain and tunnels it separately, so
  // the section reads and acts on the active branch's tunnel, not the site's.
  const activeWorktree = $derived(
    activeWorktreeBranch
      ? (site.worktrees || []).find((w) => w.branch === activeWorktreeBranch)
      : undefined
  );
  const tunnelUrl = $derived(
    (activeWorktreeBranch ? activeWorktree?.tunnel_url : site.tunnel_url) ?? ''
  );
  const tunnelTool = $derived(
    (activeWorktreeBranch ? activeWorktree?.tunnel_tool : site.tunnel_tool) ?? ''
  );
  const tunnelOn = $derived(Boolean(tunnelUrl));
  const tunnelExternal = $derived(
    Boolean(activeWorktreeBranch ? activeWorktree?.tunnel_external : site.tunnel_external)
  );
  const autoTool = $derived(toolsInfo?.tools.find((t) => t.name === toolsInfo?.auto));

  // The label a Cloudflare named tunnel serves the site on, mirroring the
  // hostname the backend composes: the site's own domain without the TLD, so
  // it follows the domain the user picked rather than the project folder.
  const hostLabel = $derived(
    labelFor((activeWorktreeBranch ? activeWorktree?.domain : site.domain) || site.domain)
  );

  function labelFor(domain: string): string {
    const parts = domain.toLowerCase().split('.').filter(Boolean);
    return (parts.length > 1 ? parts.slice(0, -1) : parts).join('-');
  }

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

  // Cloudflare is the only tool that can serve a site on your own domain, so
  // picking it asks for one first, until the answer has been remembered.
  const CLOUDFLARE = 'cloudflare';
  let domainModal = $state<'start' | 'configure' | ''>('');
  let domainError = $state('');
  let domainBusy = $state(false);

  function pickTool(tool: string, label: string) {
    if (tool === CLOUDFLARE && !toolsInfo?.base_domain_answered) {
      startingLabel = label;
      domainError = '';
      domainModal = 'start';
      return;
    }
    startTool(tool, label);
  }

  async function submitDomain(domain: string, remember: boolean) {
    const starting = domainModal === 'start';
    domainBusy = true;
    domainError = '';
    try {
      const res = await saveShareDomain(domain, remember);
      if (!res.ok) {
        domainError = res.error || m.common_requestFailed();
        return;
      }
      toolsInfo = await loadShareTools().catch(() => toolsInfo);
      domainModal = '';
      if (starting) await startTool(CLOUDFLARE, startingLabel, domain);
    } finally {
      domainBusy = false;
    }
  }

  async function startTool(tool: string, label: string, domain: string = '') {
    if (tunnelBusy) return;
    tunnelBusy = true;
    startingLabel = label;
    tunnelError = '';
    cancelRequested = false;
    try {
      const res = await startTunnel(site, tool, activeWorktreeBranch, domain);
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
    await stopTunnel(site, activeWorktreeBranch);
  }

  let stopBusy = $state(false);

  async function stopT() {
    if (stopBusy) return;
    stopBusy = true;
    try {
      await stopTunnel(site, activeWorktreeBranch);
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
            <span class="block font-medium text-gray-700 dark:text-gray-200">
              {tunnelExternal
                ? m.share_tunnelViaCli({ tool: toolLabel(tunnelTool) })
                : m.share_tunnelVia({ tool: toolLabel(tunnelTool) })}
            </span>
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
            onclick={() => pickTool(autoTool?.name === CLOUDFLARE ? CLOUDFLARE : '', m.share_autoLabel())}
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
            <div class="flex items-stretch">
              <button
                type="button"
                role="menuitem"
                disabled={!tool.installed}
                onclick={() => pickTool(tool.name, tool.label)}
                class="{itemClass} {tool.installed ? itemIdle : 'text-gray-400 dark:text-gray-600'}"
              >
                <Icon name="globe" class="w-3.5 h-3.5 shrink-0" />
                <span class="flex-1 min-w-0 text-left">
                  <span class="block font-medium">{tool.label}</span>
                  <span class="block text-[10px] {tool.installed ? 'text-gray-500 dark:text-gray-400' : ''}">
                    {!tool.installed
                      ? installHint(tool)
                      : tool.name === CLOUDFLARE && toolsInfo.base_domain
                        ? hostLabel + '.' + toolsInfo.base_domain
                        : tool.binary}
                  </span>
                </span>
              </button>
              {#if tool.name === CLOUDFLARE && tool.installed}
                <button
                  type="button"
                  role="menuitem"
                  title={m.shareDomain_configure()}
                  aria-label={m.shareDomain_configure()}
                  data-testid="share-domain-cog"
                  onclick={() => ((domainError = ''), (domainModal = 'configure'))}
                  class="shrink-0 px-2.5 flex items-center text-gray-400 hover:text-lerd-red dark:text-gray-500 dark:hover:text-lerd-red transition-colors"
                >
                  <Icon name="system" class="w-3.5 h-3.5" />
                </button>
              {/if}
            </div>
          {/each}
        {/if}
      {/if}
    </div>
  {/if}
</div>

<!-- Outside the menu: moving the pointer onto the modal closes the hover menu,
     which would take the modal with it. -->
<ShareDomainModal
  open={domainModal !== ''}
  mode={domainModal === 'configure' ? 'configure' : 'start'}
  label={hostLabel}
  baseDomain={toolsInfo?.base_domain ?? ''}
  remembered={toolsInfo?.base_domain_answered ?? false}
  error={domainError}
  busy={domainBusy}
  onclose={() => (domainModal = '')}
  onsubmit={submitDomain}
/>
