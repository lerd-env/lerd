<script lang="ts">
  import DashboardCard from './DashboardCard.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import ActivityList from './ActivityList.svelte';
  import { version, loadVersion } from '$stores/version';
  import { autostartEnabled } from '$stores/autostart';
  import { lan } from '$stores/lan';
  import { accessMode } from '$stores/accessMode';
  import { goToTab } from '$stores/route';
  import { apiFetch } from '$lib/api';
  import { m } from '../../paraglide/messages.js';

  let updateTerminalLoading = $state(false);
  let updateTerminalError = $state('');
  let changelogOpen = $state(false);

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
</script>

<DashboardCard title={m.dashboard_lerd_title()} tone={$version.hasUpdate ? 'warn' : 'default'}>
  {#snippet badge()}
    <span class="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full bg-gray-100 dark:bg-white/5 text-gray-600 dark:text-gray-400 font-mono">
      v{$version.current}
    </span>
  {/snippet}

  {#if $version.checked && !$version.hasUpdate}
    <div class="flex items-center gap-2 text-sm text-emerald-600 dark:text-emerald-500">
      <svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7"/>
      </svg>
      {m.system_lerd_latest()}
    </div>
  {/if}

  {#if $version.hasUpdate}
    <div class="flex items-start gap-2 text-sm text-yellow-700 dark:text-yellow-400 bg-yellow-50 dark:bg-yellow-500/10 border border-yellow-200 dark:border-yellow-500/30 rounded-lg px-3 py-2">
      <svg class="w-4 h-4 shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"/>
      </svg>
      <div class="flex-1 space-y-2">
        <span>{m.system_lerd_available({ version: $version.latest })}</span>
        {#if $accessMode.loopback}
          <div>
            <button
              onclick={openUpdateTerminal}
              disabled={updateTerminalLoading}
              class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-white hover:bg-gray-50 dark:bg-white/10 dark:hover:bg-white/20 text-gray-700 dark:text-gray-200 disabled:opacity-50 transition-colors"
            >
              {#if updateTerminalLoading}
                {m.system_lerd_openingTerminal()}
              {:else}
                {m.system_lerd_openTerminal()}
              {/if}
            </button>
          </div>
        {/if}
        {#if updateTerminalError}
          <p class="text-xs text-red-500">{updateTerminalError}</p>
        {/if}
        {#if $version.changelog}
          <button
            type="button"
            onclick={() => (changelogOpen = !changelogOpen)}
            class="inline-flex items-center gap-1 text-[11px] font-medium text-yellow-700/80 dark:text-yellow-300/80 hover:text-yellow-800 dark:hover:text-yellow-200 transition-colors"
            aria-expanded={changelogOpen}
          >
            <svg class="w-3 h-3 transition-transform {changelogOpen ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
            </svg>
            {changelogOpen ? m.dashboard_lerd_hideChangelog() : m.dashboard_lerd_viewChangelog()}
          </button>
          {#if changelogOpen}
            <pre class="text-[11px] leading-relaxed font-mono text-yellow-900/90 dark:text-yellow-100/90 bg-yellow-100/40 dark:bg-yellow-500/10 border border-yellow-200/60 dark:border-yellow-500/20 rounded-md p-2 max-h-[180px] overflow-y-auto whitespace-pre-wrap">{$version.changelog}</pre>
          {/if}
        {/if}
      </div>
    </div>
  {/if}

  <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs">
    <span class="inline-flex items-center gap-1.5">
      <span class="text-gray-500 dark:text-gray-400">{m.system_autostart_title()}</span>
      <StatusPill
        tone={$autostartEnabled ? 'ok' : 'muted'}
        label={$autostartEnabled ? m.system_autostart_enabled() : m.system_autostart_disabled()}
      />
    </span>
    <span class="inline-flex items-center gap-1.5">
      <span class="text-gray-500 dark:text-gray-400">{m.system_lan_title()}</span>
      <StatusPill
        tone={$lan.exposed ? 'ok' : 'muted'}
        label={$lan.exposed ? m.system_lan_exposed() : m.system_lan_loopback()}
      />
    </span>
  </div>

  <div class="pt-2 border-t border-gray-100 dark:border-lerd-border space-y-1.5">
    <div class="text-[10px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wide">{m.dashboard_activity_title()}</div>
    <ActivityList />
  </div>

  {#snippet footer()}
    <div class="flex items-center gap-2">
      <button
        onclick={loadVersion}
        disabled={$version.checking}
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/5 dark:hover:bg-white/10 text-gray-700 dark:text-gray-200 disabled:opacity-50 transition-colors"
      >
        {#if $version.checking}
          <svg class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
          </svg>
          {m.system_lerd_checking()}
        {:else}
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h5M20 20v-5h-5M20.49 9A9 9 0 005.64 5.64L4 4m16 16l-1.64-1.64A9 9 0 014.51 15"/>
          </svg>
          {m.system_lerd_checkForUpdates()}
        {/if}
      </button>
      <button
        onclick={() => goToTab('system', 'lerd')}
        class="ml-auto text-xs font-medium text-lerd-red hover:text-lerd-redhov"
      >{m.dashboard_lerd_manage()}</button>
    </div>
  {/snippet}
</DashboardCard>
