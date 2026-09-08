<script lang="ts">
  import ButtonMenu, { type ButtonMenuAction } from '$components/ButtonMenu.svelte';
  import DetailTabs, { type TabItem } from '$components/DetailTabs.svelte';
  import LogViewer from '$components/LogViewer.svelte';
  import PhpIniTab from './PhpIniTab.svelte';
  import PhpPortsTab from './PhpPortsTab.svelte';
  import PhpExtensionsTab from './PhpExtensionsTab.svelte';
  import SitesPopover from '$components/SitesPopover.svelte';
  import { status, loadStatus } from '$stores/status';
  import {
    setDefaultPhp,
    startPhp,
    stopPhp,
    checkPhpUpdates,
    confirmPhpDownload,
    openPhpShell
  } from '$stores/phpVersions';
  import { sites, sitesByPhp } from '$stores/sites';
  import { xdebugOn, xdebugOff, XDEBUG_MODES, type XdebugMode } from '$stores/xdebug';
  import { openPhpRemoveModal, openPhpRebuildModal } from '$stores/modals';
  import { phpRuntime, loadPHPRuntime } from '$stores/phpRuntime';
  import { notifyLocalInfo } from '$lib/notify';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    version: string;
  }
  let { version }: Props = $props();

  const isDefault = $derived($status.php_default === version);
  const siteCount = $derived($sitesByPhp.get(version) ?? 0);
  const fpm = $derived($status.php_fpms.find((f) => f.version === version));
  const running = $derived(Boolean(fpm?.running));
  const xdebugEnabled = $derived(Boolean(fpm?.xdebug_enabled));
  const xdebugMode = $derived<XdebugMode>((fpm?.xdebug_mode as XdebugMode) || 'debug');
  const container = $derived('lerd-php' + version.replace('.', '') + '-fpm');
  // Under the native runtime the version's output comes from the launchd pool
  // on the host. Streaming the container instead reported it as not running,
  // which is true and useless: nothing has served from it since the switch.
  const logUnit = $derived(
    $phpRuntime === 'native' ? 'lerd-native-php' + version.replace('.', '') : container
  );
  const sitesUsing = $derived($sites.filter((s) => s.php_version === version).map((s) => s.domain));
  const baseUpdate = $derived(Boolean(fpm?.update_available));

  let defaultBusy = $state(false);
  let fpmBusy = $state(false);
  let xdebugBusy = $state(false);
  let xdebugMenuOpen = $state(false);
  let xdebugRootEl: HTMLDivElement | undefined = $state();
  let checking = $state(false);

  // The parent (PhpPage) no longer wraps us in {#key version}; reset
  // per-version transient state when the version prop changes so a stale
  // open xdebug menu doesn't leak across tabs.
  $effect(() => {
    version;
    xdebugMenuOpen = false;
  });

  function closeXdebugMenu() {
    xdebugMenuOpen = false;
  }

  function onXdebugDocClick(e: MouseEvent) {
    if (!xdebugRootEl) return;
    if (!xdebugRootEl.contains(e.target as Node)) closeXdebugMenu();
  }

  function onXdebugDocKey(e: KeyboardEvent) {
    if (e.key === 'Escape') closeXdebugMenu();
  }

  $effect(() => {
    if (!xdebugMenuOpen) return;
    document.addEventListener('mousedown', onXdebugDocClick);
    document.addEventListener('keydown', onXdebugDocKey);
    return () => {
      document.removeEventListener('mousedown', onXdebugDocClick);
      document.removeEventListener('keydown', onXdebugDocKey);
    };
  });

  type TabId = 'logs' | 'config' | 'ports' | 'extensions';
  let active = $state<TabId>('logs');
  const tabs = $derived<TabItem<TabId>[]>([
    { id: 'logs', label: m.services_tabs_logs(), hidden: !running },
    { id: 'config', label: m.system_php_iniTab() },
    { id: 'ports', label: m.system_php_portsTab() },
    { id: 'extensions', label: m.system_php_extensionsTab() }
  ]);

  $effect(() => {
    if (active === 'logs' && !running) active = 'config';
  });

  async function onSetDefault() {
    defaultBusy = true;
    try {
      await setDefaultPhp(version);
      await loadStatus();
    } finally {
      defaultBusy = false;
    }
  }

  async function onToggleFpm() {
    fpmBusy = true;
    try {
      await (running ? stopPhp(version) : startPhp(version));
      await loadStatus();
    } finally {
      fpmBusy = false;
    }
  }

  async function onToggleXdebug() {
    xdebugBusy = true;
    try {
      if (xdebugEnabled) {
        await xdebugOff(version);
      } else {
        await xdebugOn(version, xdebugMode);
      }
      await loadStatus();
    } finally {
      xdebugBusy = false;
    }
  }

  async function onSetXdebugMode(e: Event) {
    const mode = (e.target as HTMLSelectElement).value as XdebugMode;
    if (mode === xdebugMode) return;
    xdebugBusy = true;
    try {
      await xdebugOn(version, mode);
      await loadStatus();
    } finally {
      xdebugBusy = false;
    }
  }

  // Reads through to the registry, so it answers even when the cached digest is
  // hours old. The result goes to the toast surface rather than a line in the
  // action row, which would either push the controls out of line or sit on top
  // of whatever the tab below is showing. The badge follows from the status.
  async function runCheckUpdates() {
    checking = true;
    try {
      const res = await checkPhpUpdates(version);
      const label = 'PHP ' + version;
      if (!res.ok) {
        notifyLocalInfo('update_check', label, m.system_php_checkUpdatesFailed());
        return;
      }
      notifyLocalInfo(
        'update_check',
        label,
        res.status?.stale ? m.system_php_checkUpdatesFound() : m.system_php_checkUpdatesUpToDate()
      );
      await loadStatus();
    } finally {
      checking = false;
    }
  }

  // The shell runs in the host's terminal emulator, so the only thing the tab
  // can report is a launcher that never came up; the session itself lives out
  // there. A stopped container has nothing to exec into, hence the guard.
  async function openShell() {
    const res = await openPhpShell(version);
    if (!res.ok) {
      notifyLocalInfo('php_shell', 'PHP ' + version, res.error || m.common_failed());
    }
  }

  const versionBusy = $derived(fpmBusy || defaultBusy || checking);

  const rebuildAction = $derived<ButtonMenuAction>({
    id: 'rebuild',
    tone: baseUpdate ? 'success' : undefined,
    icon: rebuildIcon,
    label: baseUpdate ? m.system_php_rebuildUpdate() : m.system_php_rebuild(),
    title: baseUpdate ? m.system_php_baseUpdateHint() : m.system_php_rebuildHint(),
    onclick: async () => {
      if (await confirmPhpDownload(version)) openPhpRebuildModal(version);
    }
  });

  // On the native runtime there is no image to rebuild and no container to
  // enter, and an update is a newer build being published rather than a base
  // image moving. Offering those actions there names work that cannot be done.
  const native = $derived($phpRuntime === 'native');

  const updateAction = $derived<ButtonMenuAction>({
    id: 'update',
    icon: rebuildIcon,
    label: m.system_php_updateBuild(),
    title: m.system_php_updateBuildTitle(),
    disabled: !baseUpdate,
    onclick: () => openPhpRebuildModal(version)
  });

  const versionActions = $derived.by<ButtonMenuAction[]>(() => {
    if (native) {
      const nativeActs: ButtonMenuAction[] = [updateAction];
      if (!isDefault) {
        nativeActs.push({
          id: 'remove',
          tone: 'danger',
          icon: trashIcon,
          label: m.common_remove(),
          title: siteCount > 0 ? m.system_php_removeWarn({ count: siteCount }) : m.system_php_removeTitle(),
          onclick: () => openPhpRemoveModal({ version, siteCount })
        });
      }
      return nativeActs;
    }
    const acts: ButtonMenuAction[] = [];
    // Rebuild is only worth a button of its own when the base has actually
    // moved, the way an available service update is; with nothing to pick up it
    // stays in the menu rather than sitting there inviting a five-minute build.
    if (baseUpdate) {
      acts.push(rebuildAction);
    }
    const tail: ButtonMenuAction[] = [
      {
        id: 'shell',
        icon: terminalIcon,
        label: m.common_terminal(),
        title: m.system_php_shellTitle(),
        disabled: !running,
        onclick: openShell
      },
      {
        id: 'check-updates',
        icon: checkUpdatesIcon,
        label: checking ? m.services_checkUpdatesChecking() : m.system_php_checkUpdates(),
        title: m.system_php_checkUpdatesTitle(),
        disabled: checking,
        onclick: runCheckUpdates
      }
    ];
    if (!baseUpdate) {
      tail.push(rebuildAction);
    }
    if (isDefault) {
      return [...acts, ...tail];
    }
    if (running) {
      acts.push({
        id: 'stop',
        icon: stopIcon,
        label: m.common_stop(),
        title: siteCount > 0 ? m.system_php_stopWarn({ count: siteCount }) : m.system_php_stopTitle(),
        disabled: fpmBusy,
        onclick: onToggleFpm
      });
    } else {
      acts.push({
        id: 'start',
        tone: 'success',
        icon: startIcon,
        label: m.common_start(),
        title: m.system_php_startTitle(),
        disabled: fpmBusy,
        onclick: onToggleFpm
      });
    }
    acts.push({
      id: 'set-default',
      icon: starIcon,
      label: m.system_php_setDefault(),
      disabled: defaultBusy,
      onclick: onSetDefault
    });
    acts.push({
      id: 'remove',
      tone: 'danger',
      icon: trashIcon,
      label: m.common_remove(),
      title: siteCount > 0 ? m.system_php_removeWarn({ count: siteCount }) : m.system_php_removeTitle(),
      onclick: () => openPhpRemoveModal({ version, siteCount })
    });
    return [...acts, ...tail];
  });
</script>

{#snippet startIcon()}
  <svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg>
{/snippet}
{#snippet stopIcon()}
  <svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>
{/snippet}
{#snippet starIcon()}
  <svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20"><path d="M10 1.5l2.6 5.27 5.82.85-4.21 4.1.99 5.78L10 14.77l-5.2 2.73.99-5.78L1.58 7.62l5.82-.85L10 1.5z"/></svg>
{/snippet}
{#snippet trashIcon()}
  <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
{/snippet}
{#snippet rebuildIcon()}
  <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
{/snippet}
{#snippet terminalIcon()}
  <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24"><path d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
{/snippet}
{#snippet checkUpdatesIcon()}
  <svg class={`w-3.5 h-3.5 ${checking ? 'animate-spin' : ''}`} fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-3-6.7"/><polyline points="21 4 21 10 15 10"/></svg>
{/snippet}

{#snippet detailActions()}
  <div bind:this={xdebugRootEl} class="relative inline-flex">
    <button
      type="button"
      onclick={onToggleXdebug}
      disabled={xdebugBusy}
      aria-pressed={xdebugEnabled}
      title={(xdebugEnabled ? m.common_disable() : m.common_enable()) + ' ' + m.sites_badges_xdebug()}
      class="inline-flex items-center gap-1.5 px-3 py-1.5 border border-gray-200 dark:border-lerd-border transition-colors text-xs font-medium text-gray-700 dark:text-gray-200 disabled:opacity-50 {xdebugEnabled
        ? 'rounded-l-lg border-r-0 bg-emerald-50/60 dark:bg-emerald-900/15 hover:bg-emerald-50 dark:hover:bg-emerald-900/25'
        : 'rounded-lg bg-white dark:bg-lerd-card hover:bg-gray-50 dark:hover:bg-white/5'}"
    >
      {#if xdebugBusy}
        <svg class="w-2.5 h-2.5 animate-spin text-amber-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-30" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-90" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
        </svg>
      {:else}
        <span class="shrink-0 w-2 h-2 rounded-full {xdebugEnabled ? 'bg-emerald-500' : 'border border-gray-300 dark:border-gray-600 bg-transparent'}"></span>
      {/if}
      <span>{m.system_php_xdebug()}</span>
    </button>
    {#if xdebugEnabled}
      <button
        type="button"
        onclick={() => (xdebugMenuOpen = !xdebugMenuOpen)}
        disabled={xdebugBusy}
        aria-haspopup="menu"
        aria-expanded={xdebugMenuOpen}
        title={m.system_php_xdebugModeTitle()}
        class="inline-flex items-center gap-1 px-3 py-1.5 rounded-r-lg border border-gray-200 dark:border-lerd-border transition-colors text-xs font-medium text-gray-700 dark:text-gray-200 bg-emerald-50/60 dark:bg-emerald-900/15 hover:bg-emerald-50 dark:hover:bg-emerald-900/25 disabled:opacity-50"
      >
        <span class="font-mono">{xdebugMode}</span>
        <svg class="w-3 h-3 transition-transform {xdebugMenuOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="6 9 12 15 18 9"/>
        </svg>
      </button>
      {#if xdebugMenuOpen}
        <div
          role="menu"
          class="absolute right-0 top-full mt-1 z-50 min-w-40 rounded-xl bg-white dark:bg-lerd-card border border-gray-200 dark:border-lerd-border shadow-xl py-1"
        >
          {#each XDEBUG_MODES as mode (mode)}
            {@const selected = mode === xdebugMode}
            <button
              type="button"
              role="menuitem"
              onclick={() => {
                xdebugMenuOpen = false;
                onSetXdebugMode({ target: { value: mode } } as unknown as Event);
              }}
              class="w-full text-left px-3 py-1.5 text-xs font-mono hover:bg-gray-50 dark:hover:bg-white/5 transition-colors {selected ? 'text-lerd-red font-semibold' : 'text-gray-700 dark:text-gray-200'}"
            >
              {mode}
            </button>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
  <SitesPopover domains={sitesUsing} />
  <ButtonMenu actions={versionActions} busy={versionBusy} />
{/snippet}

<DetailTabs {tabs} {active} onchange={(id) => (active = id)} actions={detailActions} />
{#if active === 'logs' && running}
  <LogViewer path={'/api/logs/' + logUnit} />
{:else if active === 'config'}
  <PhpIniTab {version} />
{:else if active === 'ports'}
  <PhpPortsTab {version} />
{:else if active === 'extensions'}
  <PhpExtensionsTab {version} />
{/if}
