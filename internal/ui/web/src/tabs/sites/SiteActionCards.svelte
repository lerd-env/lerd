<script lang="ts">
  import SiteDoctorModal from './SiteDoctorModal.svelte';
  import {
    loadCommands,
    launchCommand,
    commandIconPath,
    runningName,
    type Command
  } from '$stores/commands';
  import type { Site } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: Site;
    branch?: string;
  }
  let { site, branch = '' }: Props = $props();

  let commands = $state<Command[]>([]);
  let doctorOpen = $state(false);

  async function refresh() {
    if (!site.domain) return;
    try {
      commands = await loadCommands(site.domain, branch);
    } catch {
      commands = [];
    }
  }

  $effect(() => {
    void site.domain;
    void branch;
    void refresh();
  });

  const busy = $derived($runningName !== null);
</script>

<div class="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-4 gap-3">
  <button
    type="button"
    onclick={() => (doctorOpen = true)}
    title={m.sites_doctor_title()}
    class="group flex items-center gap-2.5 rounded-lg border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-2.5 text-left transition duration-150 hover:border-gray-300 dark:hover:border-white/15 hover:shadow-sm"
  >
    <span class="shrink-0 inline-flex items-center justify-center w-8 h-8 rounded-lg bg-gray-100 text-gray-500 dark:bg-white/5 dark:text-gray-400 group-hover:text-lerd-red">
      <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M4.8 2.3A.3.3 0 1 0 5 2H4a2 2 0 00-2 2v5a6 6 0 006 6 6 6 0 006-6V4a2 2 0 00-2-2h-1a.3.3 0 10.2.3" />
        <path d="M8 15v1a6 6 0 006 6 6 6 0 006-6v-4" />
        <circle cx="20" cy="10" r="2" />
      </svg>
    </span>
    <span class="min-w-0 flex-1">
      <span class="block text-xs font-semibold text-gray-800 dark:text-gray-100 truncate">{m.sites_doctor_title()}</span>
      <span class="block text-[10px] text-gray-500 dark:text-gray-400 truncate">{m.sites_doctor_runChecks()}</span>
    </span>
  </button>

  {#each commands as c (c.name)}
    <button
      type="button"
      onclick={() => launchCommand(site.domain, c, { branch })}
      disabled={busy}
      title={(c.description ?? '') + (c.description ? '\n\n' : '') + '$ ' + c.command}
      class="group flex items-center gap-2.5 rounded-lg border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-2.5 text-left transition duration-150 hover:border-gray-300 dark:hover:border-white/15 hover:shadow-sm disabled:opacity-50"
    >
      <span class="shrink-0 inline-flex items-center justify-center w-8 h-8 rounded-lg bg-gray-100 text-gray-500 dark:bg-white/5 dark:text-gray-400 group-hover:text-lerd-red">
        {#if $runningName === c.name}
          <svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
          </svg>
        {:else}
          <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" d={commandIconPath(c.icon)} />
          </svg>
        {/if}
      </span>
      <span class="min-w-0 flex-1">
        <span class="flex items-center gap-1.5">
          <span class="text-xs font-semibold text-gray-800 dark:text-gray-100 truncate">{c.label || c.name}</span>
          {#if c.confirm}
            <span class="shrink-0 w-1.5 h-1.5 rounded-full bg-amber-500" title={m.cmd_asksBeforeRunning()}></span>
          {/if}
        </span>
        <span class="block text-[10px] text-gray-500 dark:text-gray-400 font-mono truncate">{c.command}</span>
      </span>
    </button>
  {/each}
</div>

<SiteDoctorModal open={doctorOpen} {site} {branch} onclose={() => (doctorOpen = false)} />
