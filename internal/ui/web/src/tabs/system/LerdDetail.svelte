<script lang="ts">
  import { onMount } from 'svelte';
  import CheckUpdatesButton from '$components/CheckUpdatesButton.svelte';
  import { version, loadVersion } from '$stores/version';
  import { accessMode } from '$stores/accessMode';
  import { lan, loadLANStatus, toggleLAN, generateRemoteSetupCode, copySetupCurl } from '$stores/lan';
  import { status } from '$stores/status';
  import {
    remoteControl,
    loadRemoteControl,
    disableRemoteControl,
    setRemoteFullAccess
  } from '$stores/remoteControl';
  import { openRemoteControlModal, openLANProgressModal, openChangelogModal, type LANAction } from '$stores/modals';
  import {
    autostartEnabled,
    loadAutostart,
    toggleAutostart,
    trayEnabled,
    toggleTray,
    startOnDashboardOpen,
    toggleStartOnDashboardOpen
  } from '$stores/autostart';
  import { idleEnabled, idleTimeoutMinutes, loadIdle, saveIdle } from '$stores/idle';
  import Toggle from '$components/Toggle.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import Icon from '$components/Icon.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import SettingsCard from '$components/SettingsCard.svelte';
  import LANServicesSetting from './LANServicesSetting.svelte';
  import LanguageSwitcher from '$components/LanguageSwitcher.svelte';
  import { apiFetch, apiBase } from '$lib/api';
  import { escapeHtml } from '$lib/html';
  import { m } from '../../paraglide/messages.js';

  // The remote dashboard always binds :7073; when LAN-exposed we surface the
  // address plus a scannable QR so a phone can jump straight in.
  const dashboardURL = $derived('http://' + $lan.lanIP + ':7073');
  const dashboardQRSrc = $derived(apiBase + '/api/dashboard-qr?v=' + encodeURIComponent($lan.lanIP));

  onMount(() => {
    loadLANStatus();
    loadRemoteControl();
    loadAutostart();
    loadIdle();
  });

  let idleBusy = $state(false);
  let idleMinutesInput = $state(30);
  // Mirror the store into the editable field: fires on load and after a save
  // (the only times the store changes), never while the user is typing.
  $effect(() => {
    idleMinutesInput = $idleTimeoutMinutes;
  });
  async function onToggleIdle() {
    idleBusy = true;
    try {
      await saveIdle(!$idleEnabled, $idleTimeoutMinutes);
    } finally {
      idleBusy = false;
    }
  }
  async function onSaveIdleTimeout() {
    const v = Math.max(1, Math.floor(Number(idleMinutesInput) || 0));
    if (v === $idleTimeoutMinutes) return;
    idleBusy = true;
    try {
      await saveIdle($idleEnabled, v);
    } finally {
      idleBusy = false;
    }
  }

  function startLAN(action: LANAction) {
    openLANProgressModal(action);
    toggleLAN(action);
  }

  function exposeDashboardForLAN() {
    if ($remoteControl.enabled) {
      startLAN('expose');
    } else {
      openRemoteControlModal(() => startLAN('expose'));
    }
  }

  let trayBusy = $state(false);
  async function onToggleTray() {
    trayBusy = true;
    try {
      await toggleTray(!$trayEnabled);
    } finally {
      trayBusy = false;
    }
  }

  let autostartBusy = $state(false);
  async function onToggleAutostart() {
    autostartBusy = true;
    try {
      await toggleAutostart(!$autostartEnabled);
    } finally {
      autostartBusy = false;
    }
  }

  let startOnOpenBusy = $state(false);
  async function onToggleStartOnOpen() {
    startOnOpenBusy = true;
    try {
      await toggleStartOnDashboardOpen(!$startOnDashboardOpen);
    } finally {
      startOnOpenBusy = false;
    }
  }

  let updateTerminalLoading = $state(false);
  let updateTerminalError = $state('');

  // There is no remote session to widen while lerd is loopback-only, so the
  // setting stays out of the way. An already enabled setting keeps showing, so
  // that re-exposing does not silently hand host access back out.
  const fullAccessHidden = $derived(!$lan.exposed && !$remoteControl.fullAccess);

  // Dashboard credentials are equally inert while lerd is loopback-only, so the
  // card goes too. Configured credentials keep it visible so they can be
  // rotated or cleared, and disabled-DNS mode keeps it as its only route to
  // LAN exposure at all.
  const remoteCardHidden = $derived(
    !$lan.exposed && !$remoteControl.enabled && $status.dns?.enabled !== false
  );
  async function openUpdateTerminal() {
    updateTerminalLoading = true;
    updateTerminalError = '';
    try {
      const res = await apiFetch('/api/lerd/update-terminal', { method: 'POST' });
      const data = (await res.json()) as { ok?: boolean; error?: string };
      if (!data.ok) updateTerminalError = data.error || m.common_failed();
    } catch (e) {
      updateTerminalError = e instanceof Error ? e.message : m.common_failed();
    } finally {
      updateTerminalLoading = false;
    }
  }

  async function doDisableRemoteControl() {
    await disableRemoteControl();
  }
</script>

<div class="flex-1 overflow-y-auto">
  <div class="flex flex-wrap items-center justify-between gap-y-2 p-3 border-b border-gray-100 dark:border-lerd-border">
    <span class="font-semibold text-gray-900 dark:text-white text-base">{m.system_lerd()}</span>
    <span class="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full bg-gray-100 dark:bg-white/5 text-gray-600 dark:text-gray-400 font-mono">v{$version.current}</span>
  </div>

  <div class="p-3 space-y-3">
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
    <SettingsCard>
      <div class="flex items-center justify-between gap-3">
        <div class="min-w-0 text-sm">
          {#if $version.checked && !$version.hasUpdate}
            <span class="inline-flex items-center gap-2 text-emerald-600 dark:text-emerald-500">
              <svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7"/>
              </svg>
              {m.system_lerd_latest()}
            </span>
          {:else if $version.hasUpdate}
            <span class="inline-flex items-center gap-1.5 font-medium text-yellow-700 dark:text-yellow-400">
              <svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"/>
              </svg>
              {m.system_lerd_available({ version: $version.latest })}
            </span>
          {/if}
        </div>
        <CheckUpdatesButton onclick={() => loadVersion(true)} checking={$version.checking} />
      </div>

      {#if $version.hasUpdate}
        <div class="space-y-3 mt-3">
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {@html m.system_lerd_updateHint({ cmd: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">lerd update</code>' })}
          </p>
          <div class="flex flex-wrap items-center gap-2">
            {#if $accessMode.localControl}
              <button
                onclick={openUpdateTerminal}
                disabled={updateTerminalLoading}
                class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-300 disabled:opacity-50 transition-colors"
              >
                {#if updateTerminalLoading}
                  <svg class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
                  </svg>
                  {m.system_lerd_openingTerminal()}
                {:else}
                  <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/>
                  </svg>
                  {m.system_lerd_openTerminal()}
                {/if}
              </button>
            {/if}
            {#if $version.changelog}
              <DetailButton onclick={openChangelogModal}>
                {#snippet icon()}
                  <Icon name="docs" class="w-3.5 h-3.5" />
                {/snippet}
                {m.system_lerd_whatsNew()}
              </DetailButton>
            {/if}
          </div>
          {#if updateTerminalError}
            <p class="text-xs text-red-500">{updateTerminalError}</p>
          {/if}
        </div>
      {/if}

      <div class="flex items-start gap-2 text-xs text-gray-500 dark:text-gray-400 mt-3">
        <svg class="w-3.5 h-3.5 shrink-0 mt-0.5 text-yellow-500" fill="currentColor" viewBox="0 0 20 20">
          <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.286 3.957a1 1 0 00.95.69h4.162c.969 0 1.371 1.24.588 1.81l-3.367 2.446a1 1 0 00-.364 1.118l1.287 3.957c.3.922-.755 1.688-1.54 1.118l-3.366-2.446a1 1 0 00-1.176 0l-3.366 2.446c-.784.57-1.838-.196-1.54-1.118l1.287-3.957a1 1 0 00-.364-1.118L2.098 9.384c-.783-.57-.38-1.81.588-1.81h4.162a1 1 0 00.95-.69l1.286-3.957z"/>
        </svg>
        <p class="leading-relaxed">
          {m.system_lerd_starBlurb()}
          <a href="https://github.com/lerd-env/lerd" target="_blank" rel="noopener" class="font-medium text-lerd-red hover:text-lerd-redhov underline-offset-2 hover:underline">{m.system_lerd_starCta()}</a>
          {m.system_lerd_starAfter()}
        </p>
      </div>
    </SettingsCard>

    <SettingsCard>
      <div class="flex items-center justify-between mb-2">
        <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.system_language_title()}</span>
      </div>
      <div class="flex items-center justify-between gap-4">
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_language_description()}</p>
        <LanguageSwitcher />
      </div>
    </SettingsCard>
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
    <SettingsCard>
      <div class="flex items-center justify-between gap-3 mb-2">
        <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.system_autostart_title()}</span>
        {#if $accessMode.localControl}
          <Toggle
            on={$autostartEnabled}
            loading={autostartBusy}
            onclick={onToggleAutostart}
            title={$autostartEnabled ? m.system_autostart_toggleOff() : m.system_autostart_toggleOn()}
          />
        {:else}
          <StatusPill
            size="sm"
            tone={$autostartEnabled ? 'ok' : 'muted'}
            label={$autostartEnabled ? m.common_enabled() : m.common_disabled()}
          />
        {/if}
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_autostart_description()}</p>
    </SettingsCard>

    <SettingsCard>
      <div class="flex items-center justify-between gap-3 mb-2">
        <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.system_startOnOpen_title()}</span>
        {#if $accessMode.localControl}
          <Toggle
            on={$startOnDashboardOpen}
            loading={startOnOpenBusy}
            onclick={onToggleStartOnOpen}
            title={$startOnDashboardOpen ? m.system_startOnOpen_toggleOff() : m.system_startOnOpen_toggleOn()}
          />
        {:else}
          <StatusPill
            size="sm"
            tone={$startOnDashboardOpen ? 'ok' : 'muted'}
            label={$startOnDashboardOpen ? m.common_enabled() : m.common_disabled()}
          />
        {/if}
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_startOnOpen_description()}</p>
    </SettingsCard>

    <SettingsCard>
      <div class="flex items-center justify-between gap-3 mb-2">
        <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.system_tray_title()}</span>
        {#if $accessMode.localControl}
          <Toggle
            on={$trayEnabled}
            loading={trayBusy}
            onclick={onToggleTray}
            title={$trayEnabled ? m.system_tray_toggleOff() : m.system_tray_toggleOn()}
          />
        {:else}
          <StatusPill
            size="sm"
            tone={$trayEnabled ? 'ok' : 'muted'}
            label={$trayEnabled ? m.common_enabled() : m.common_disabled()}
          />
        {/if}
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_tray_description()}</p>
    </SettingsCard>

    <SettingsCard>
      <div class="flex items-center justify-between gap-3 mb-2">
        <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.system_idle_title()}</span>
        {#if $accessMode.localControl}
          <Toggle
            on={$idleEnabled}
            loading={idleBusy}
            onclick={onToggleIdle}
            title={$idleEnabled ? m.system_idle_toggleOff() : m.system_idle_toggleOn()}
          />
        {:else}
          <StatusPill
            size="sm"
            tone={$idleEnabled ? 'ok' : 'muted'}
            label={$idleEnabled ? m.common_enabled() : m.common_disabled()}
          />
        {/if}
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_idle_description()}</p>
      <div class="flex items-center justify-between gap-4 mt-3">
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_idle_timeoutLabel()}</p>
        {#if $accessMode.localControl}
          <div class="flex items-center gap-2">
            <input
              type="number"
              min="1"
              bind:value={idleMinutesInput}
              onblur={onSaveIdleTimeout}
              onkeydown={(e) => e.key === 'Enter' && onSaveIdleTimeout()}
              disabled={idleBusy}
              class="text-sm bg-white dark:bg-lerd-card border border-gray-200 dark:border-lerd-border rounded-lg px-3 py-1.5 w-20 text-gray-700 dark:text-gray-200 focus:outline-hidden focus:border-lerd-red/50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            />
            <span class="text-xs text-gray-500 dark:text-gray-400">{m.system_idle_minutes()}</span>
          </div>
        {:else}
          <span class="text-xs text-gray-500 dark:text-gray-400">{idleMinutesInput} {m.system_idle_minutes()}</span>
        {/if}
      </div>
    </SettingsCard>
    </div>

    {#if $status.dns?.enabled !== false}
    <SettingsCard>
      <div class="flex items-center justify-between mb-2">
        <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.system_lan_title()}</span>
        <StatusPill
          size="sm"
          tone={$lan.exposed ? 'ok' : 'muted'}
          label={$lan.exposed ? m.system_lan_exposed() : m.system_lan_loopback()}
        />
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
        {#if $lan.exposed}
          {@html m.system_lan_exposedDescription({
            ip: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">' + escapeHtml($lan.lanIP) + '</code>',
            pattern: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">*.test</code>'
          })}
        {:else}
          {@html m.system_lan_loopbackDescription({
            loop4: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">127.0.0.1</code>',
            loop6: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">::1</code>'
          })}
        {/if}
      </p>

      {#if $lan.macos}
        <p class="text-xs text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-500/30 rounded-lg px-3 py-2 mb-3">
          {@html m.system_lan_macosWarning({ pattern: '<code class="font-mono">*.test</code>' })}
        </p>
      {/if}

      {#if $accessMode.localControl}
        <div class="flex items-center gap-2">
          {#if !$lan.exposed}
            <button
              onclick={() => startLAN('expose')}
              disabled={$lan.loading}
              class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-300 disabled:opacity-50 transition-colors"
            >{m.system_lan_expose()}</button>
          {:else}
            <button
              onclick={() => startLAN('unexpose')}
              disabled={$lan.loading}
              class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-300 disabled:opacity-50 transition-colors"
            >{m.system_lan_stop()}</button>
          {/if}
        </div>
      {/if}

      {#if $lan.error}<p class="text-xs text-red-500 mt-2">{$lan.error}</p>{/if}

      {#if $lan.exposed && $accessMode.localControl}
        <div class="mt-3 space-y-3">
          <div class="text-xs text-gray-600 dark:text-gray-400 bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-500/30 rounded-lg p-3 space-y-1">
            <p>{@html m.system_lan_postExpose_resolver({ addr: '<code class="bg-white/60 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">' + escapeHtml($lan.lanIP) + ':5300</code>', unit: '<code class="bg-white/60 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">lerd-dns-forwarder.service</code>' })}</p>
            <p>{@html m.system_lan_postExpose_dnsmasq({ pattern: '<code class="bg-white/60 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">*.test</code>', ip: escapeHtml($lan.lanIP) })}</p>
            <p><strong>{m.system_lan_postExpose_firewall()}</strong></p>
          </div>

          <div>
            <p class="text-xs font-semibold text-gray-700 dark:text-gray-300 mb-2">{m.system_lan_remote_title()}</p>
            <p class="text-xs text-gray-500 dark:text-gray-400 mb-2">{m.system_lan_remote_hint()}</p>

            {#if !$lan.setupCode}
              <button
                onclick={generateRemoteSetupCode}
                disabled={$lan.setupLoading}
                class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-300 disabled:opacity-50 transition-colors"
              >
                {#if $lan.setupLoading}
                  <svg class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
                  </svg>
                  {m.system_lan_remote_generating()}
                {:else}
                  {m.system_lan_remote_generate()}
                {/if}
              </button>
            {:else}
              <div class="space-y-2">
                <div class="flex items-center justify-between gap-3 text-xs text-gray-600 dark:text-gray-400">
                  <span>{@html m.system_lan_remote_codeLabel({ code: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono text-sm">' + escapeHtml($lan.setupCode) + '</code>' })}</span>
                  {#if $lan.setupExpiresIn}<span>{m.system_lan_remote_expiresIn({ time: $lan.setupExpiresIn })}</span>{/if}
                </div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_lan_remote_runOnMachine()}</p>
                <ul class="text-[11px] text-gray-500 dark:text-gray-400 list-disc pl-4 space-y-0.5">
                  <li>{@html m.system_lan_remote_bullet1({ mkcert: '<code class="font-mono">mkcert</code>' })}</li>
                  <li>{@html m.system_lan_remote_bullet2({ resolver: '<code class="font-mono">/etc/resolver</code>', test: '<code class="font-mono">.test</code>' })}</li>
                  <li>{m.system_lan_remote_bullet3()}</li>
                </ul>
                <div class="relative">
                  <pre class="text-xs text-gray-700 dark:text-gray-300 bg-gray-50 dark:bg-white/3 border border-gray-100 dark:border-lerd-border rounded-lg p-3 pr-12 overflow-x-auto font-mono whitespace-pre">{$lan.setupCurl}</pre>
                  <button
                    onclick={copySetupCurl}
                    title={$lan.setupCopied ? m.system_lan_remote_copyTooltip_copied() : m.system_lan_remote_copyTooltip_copy()}
                    class="absolute top-2 right-2 inline-flex items-center justify-center w-7 h-7 rounded-sm text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-white/10 transition-colors"
                  >
                    {#if $lan.setupCopied}
                      <svg class="w-4 h-4 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                      </svg>
                    {:else}
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/>
                      </svg>
                    {/if}
                  </button>
                </div>
                <p class="text-[11px] text-gray-500 dark:text-gray-400">
                  {@html m.system_lan_remote_footer({ generate: '<em>' + m.system_lan_remote_generate() + '</em>' })}
                </p>
                <button onclick={generateRemoteSetupCode} disabled={$lan.setupLoading} class="text-xs text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 underline">{m.system_lan_remote_newCode()}</button>
              </div>
            {/if}
            {#if $lan.setupError}<p class="text-xs text-red-500 mt-2">{$lan.setupError}</p>{/if}
          </div>
        </div>
      {/if}
      <LANServicesSetting nested />
    </SettingsCard>
    {:else}
    <LANServicesSetting />
    {/if}

    {#if !remoteCardHidden}
    <SettingsCard>
      <div class="flex items-center justify-between mb-2">
        <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.system_remote_title()}</span>
        <StatusPill
          size="sm"
          tone={$remoteControl.enabled ? ($lan.exposed ? 'ok' : 'warn') : 'muted'}
          label={$remoteControl.enabled
            ? $lan.exposed
              ? m.system_remote_status_active()
              : m.system_remote_status_inert()
            : m.system_remote_status_disabled()}
        />
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
        {#if $status.dns?.enabled === false}
          {@html m.system_remote_descriptionNoDns({
            addr: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">' + ($lan.lanIP ? escapeHtml($lan.lanIP) : '&lt;lan-ip&gt;') + ':7073</code>',
            cmd: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">lerd lan:share</code>'
          })}
        {:else}
          {@html m.system_remote_description({ loop4: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">127.0.0.1</code>', loop6: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">::1</code>' })}
        {/if}
      </p>

      {#if $accessMode.localControl}
      {#if $remoteControl.enabled}
        <div class="space-y-2">
          {#if $lan.exposed}
            <div class="flex items-center justify-between gap-3 p-3 rounded-lg bg-gray-50 dark:bg-white/3 border border-gray-100 dark:border-lerd-border">
              <div class="min-w-0">
                <p class="text-[10px] font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide mb-1">{m.system_remote_address()}</p>
                <a href={dashboardURL} target="_blank" rel="noopener" class="text-sm text-teal-600 dark:text-teal-400 font-mono hover:underline break-all">{dashboardURL}</a>
              </div>
              <img src={dashboardQRSrc} width="112" height="112" alt={m.lanShare_qrAlt()} class="shrink-0 rounded-sm bg-white p-1" />
            </div>
          {/if}
          <p class="text-xs text-gray-600 dark:text-gray-400">
            {@html m.system_remote_usernameRow({ username: '<code class="bg-gray-100 dark:bg-white/10 px-1.5 py-0.5 rounded-sm font-mono">' + escapeHtml($remoteControl.username) + '</code>' })}
          </p>
          {#if !$lan.exposed && $status.dns?.enabled !== false}
            <p class="text-xs text-amber-600 dark:text-amber-400">
              {@html m.system_remote_inertWarning({ cmd: '<code class="font-mono">lerd lan:expose</code>', btn: '<em>' + m.system_lan_expose() + '</em>' })}
            </p>
          {/if}
          {#if !fullAccessHidden}
          <div class="pt-1">
            <div class="flex items-center justify-between gap-3">
              <span class="text-xs font-semibold text-gray-700 dark:text-gray-300">{m.system_remote_fullAccess_title()}</span>
              <Toggle
                on={$remoteControl.fullAccess}
                tone="amber"
                loading={$remoteControl.fullAccessLoading}
                title={m.system_remote_fullAccess_title()}
                onclick={() => setRemoteFullAccess(!$remoteControl.fullAccess)}
              />
            </div>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">{m.system_remote_fullAccess_description()}</p>
            {#if $remoteControl.fullAccess}
              <p class="text-xs text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-500/30 rounded-lg px-3 py-2 mt-2">
                {m.system_remote_fullAccess_warning()}
              </p>
            {/if}
          </div>
          {/if}
          <div class="flex flex-wrap gap-2">
            <button
              onclick={() => openRemoteControlModal()}
              disabled={$remoteControl.loading}
              class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-300 disabled:opacity-50 transition-colors"
            >{m.system_remote_changeCredentials()}</button>
            <button
              onclick={doDisableRemoteControl}
              disabled={$remoteControl.loading}
              class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-red-50 hover:bg-red-100 dark:bg-red-500/10 dark:hover:bg-red-500/20 text-red-700 dark:text-red-400 disabled:opacity-50 transition-colors"
            >{m.system_remote_disable()}</button>
          </div>
        </div>
      {:else if $status.dns?.enabled === false}
        <div>
          <button
            onclick={exposeDashboardForLAN}
            disabled={$lan.loading}
            class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-300 disabled:opacity-50 transition-colors"
          >{m.system_remote_enableDashboardLan()}</button>
        </div>
      {:else}
        <div>
          <button
            onclick={() => openRemoteControlModal()}
            disabled={!$lan.exposed}
            title={$lan.exposed ? '' : m.system_remote_enableDisabledHint()}
            class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >{m.system_remote_enable()}</button>
          {#if !$lan.exposed}
            <p class="text-xs text-gray-400 dark:text-gray-500 mt-2">{m.system_remote_exposeFirst()}</p>
          {/if}
        </div>
      {/if}
      {/if}
      {#if $remoteControl.error}<p class="text-xs text-red-500 mt-2">{$remoteControl.error}</p>{/if}
    </SettingsCard>
    {/if}
  </div>
</div>
