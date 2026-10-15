<script lang="ts">
  import { onMount } from 'svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import ConfirmModal from '$components/ConfirmModal.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import PluginRequirements from '$components/PluginRequirements.svelte';
  import PluginBrowse from './PluginBrowse.svelte';
  import { managedPlugins, loadManagedPlugins, installPlugin, removePlugin, updatePlugin } from '$stores/plugins';
  import { routeRest, goToTab } from '$stores/route';
  import { notifyLocalFailure, notifyLocalInfo } from '$lib/notify';
  import { m } from '../../paraglide/messages.js';

  let source = $state('');
  let installing = $state(false);
  let removing = $state('');
  let updating = $state('');
  let confirmName = $state('');

  // #system/plugins is the installed list; #system/plugins/browse[/<name>] the
  // catalog, which plugins.lerd.sh links straight into.
  const parts = $derived($routeRest.split('/'));
  const browsing = $derived(parts[1] === 'browse');
  const focus = $derived(browsing ? decodeURIComponent(parts[2] ?? '') : '');

  const tabBtn = (active: boolean) =>
    'pb-1 text-xs font-medium border-b-2 transition-colors ' +
    (active ? 'border-lerd-red text-lerd-red' : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300');

  async function update(name: string) {
    updating = name;
    try {
      const err = await updatePlugin(name);
      if (err) notifyLocalFailure('plugin_update', m.system_plugins_title(), err);
    } finally {
      updating = '';
    }
  }

  onMount(() => {
    void loadManagedPlugins();
  });

  async function install(e: SubmitEvent) {
    e.preventDefault();
    const src = source.trim();
    if (!src) return;
    installing = true;
    try {
      const err = await installPlugin(src);
      if (err) {
        notifyLocalFailure('plugin_install', m.system_plugins_installFailed(), err);
        return;
      }
      source = '';
      notifyLocalInfo('plugin_install', m.system_plugins_title(), m.system_plugins_installed());
    } finally {
      installing = false;
    }
  }

  async function remove() {
    const name = confirmName;
    removing = name;
    try {
      const err = await removePlugin(name);
      if (err) notifyLocalFailure('plugin_remove', m.system_plugins_title(), err);
    } finally {
      removing = '';
      confirmName = '';
    }
  }
</script>

<div class="flex-1 overflow-y-auto">
  <div class="flex items-center gap-5 px-3 py-1.5 page-header">
    <span class="font-semibold text-gray-900 dark:text-white text-base">{m.system_plugins_title()}</span>
    <div class="flex gap-4" role="tablist">
      <button role="tab" aria-selected={!browsing} class={tabBtn(!browsing)} onclick={() => goToTab('system', 'plugins')}>{m.system_plugins_tabInstalled()}</button>
      <button role="tab" aria-selected={browsing} class={tabBtn(browsing)} onclick={() => goToTab('system', 'plugins/browse')}>{m.system_plugins_tabBrowse()}</button>
    </div>
  </div>

  <div class="p-3 space-y-3">
    <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_plugins_description()}</p>

    {#if browsing}
      {#key focus}<PluginBrowse {focus} />{/key}
    {:else}
    <form class="flex gap-2" onsubmit={install}>
      <input
        type="text"
        bind:value={source}
        placeholder={m.system_plugins_sourcePlaceholder()}
        aria-label={m.system_plugins_sourcePlaceholder()}
        disabled={installing}
        class="flex-1 min-w-0 text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-2.5 py-1.5 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red/50"
      />
      <DetailButton tone="primary" loading={installing} disabled={installing || !source.trim()}>
        {m.common_install()}
      </DetailButton>
    </form>

    {#if $managedPlugins.length === 0}
      <EmptyState title={m.system_plugins_empty()} size="sm" />
    {:else}
      <ul class="divide-y divide-gray-100 dark:divide-lerd-border border border-gray-200 dark:border-lerd-border rounded-md">
        {#each $managedPlugins as p (p.name)}
          <li class="flex items-start gap-3 px-3 py-2.5">
            <div class="flex-1 min-w-0">
              <div class="flex items-baseline gap-2">
                <span class="text-sm font-medium text-gray-900 dark:text-white">{p.name}</span>
                {#if p.version}<span class="text-xs text-gray-400 tabular-nums">{p.version}</span>{/if}
              </div>
              {#if p.error}
                <p class="text-xs text-red-600 dark:text-red-400 break-words">{m.system_plugins_broken()}: {p.error}</p>
              {:else}
                {#if p.description}<p class="text-xs text-gray-500 dark:text-gray-400">{p.description}</p>{/if}
                <p class="text-[11px] text-gray-400 mt-0.5">
                  {#if p.commit}{m.system_plugins_fromCatalog({ commit: p.commit.slice(0, 7) })}{:else}{m.system_plugins_notListed()}{/if}
                </p>
                {#if p.adds?.length}
                  <div class="flex flex-wrap gap-1 mt-1">
                    {#each p.adds as surface (surface)}
                      <span class="text-[10px] font-mono px-1.5 py-0.5 rounded-sm bg-gray-100 dark:bg-white/5 text-gray-500 dark:text-gray-400">{surface}</span>
                    {/each}
                  </div>
                {/if}
                {#if p.requires?.length}<PluginRequirements requires={p.requires} />{/if}
                {#if p.failure}
                  <div class="mt-1.5 flex items-start gap-2">
                    <p class="flex-1 min-w-0 text-xs text-red-600 dark:text-red-400 break-words">
                      {m.system_plugins_lastFailure({ what: p.failure.what })}: {p.failure.error.split('\n')[0]}
                    </p>
                    {#if p.failure.report_url}
                      <a href={p.failure.report_url} target="_blank" rel="noopener noreferrer" class="shrink-0 text-xs font-medium text-lerd-red hover:underline">{m.system_plugins_report()}</a>
                    {/if}
                  </div>
                {/if}
              {/if}
            </div>
            <div class="flex gap-2 shrink-0">
              {#if p.update}
                <DetailButton tone="primary" onclick={() => update(p.name)} loading={updating === p.name} disabled={updating !== ''} title={p.update.slice(0, 7)}>
                  {m.system_plugins_update()}
                </DetailButton>
              {/if}
              <DetailButton tone="danger" onclick={() => (confirmName = p.name)} loading={removing === p.name} disabled={removing !== ''}>
                {m.common_remove()}
              </DetailButton>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
    {/if}
  </div>
</div>

<ConfirmModal
  open={confirmName !== ''}
  title={m.system_plugins_removeTitle({ name: confirmName })}
  body={m.system_plugins_removeBody()}
  confirmLabel={m.common_remove()}
  danger
  loading={removing !== ''}
  onconfirm={remove}
  onclose={() => (confirmName = '')}
/>
