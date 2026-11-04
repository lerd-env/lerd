<script lang="ts">
  import DetailHeader from '$components/DetailHeader.svelte';
  import { onMount } from 'svelte';
  import { status, loadStatus } from '$stores/status';
  import { nodeVersions, nodeVersionStatuses, loadNodeVersions, setDefaultNode, removeNode, installNode, updateNodeVersion, manageNode, unmanageNode, setNodeManager, checkNodeUpdates, type NodeVersionStatus } from '$stores/nodeVersions';
  import { accessMode } from '$stores/accessMode';
  import NodeUpdateModal from '$components/NodeUpdateModal.svelte';
  import VersionCard from '$components/VersionCard.svelte';
  import Icon from '$components/Icon.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import CheckUpdatesButton from '$components/CheckUpdatesButton.svelte';
  import { notifyLocalInfo } from '$lib/notify';
  import { sites, sitesByNode } from '$stores/sites';
  import DetailButton from '$components/DetailButton.svelte';
  import SitesPopover from '$components/SitesPopover.svelte';
  import SegmentedControl from '$components/SegmentedControl.svelte';
  import { m } from '../../paraglide/messages.js';

  // Mirror the store value into local $state via subscribe so the
  // reactivity is direct (bypasses any auto-subscription edge case).
  let nodeDefault = $state('');
  $effect(() => status.subscribe((s) => { nodeDefault = s.node_default || ''; }));

  // The list is otherwise fetched once at app start, so a version installed
  // from the CLI stays invisible in an open dashboard.
  onMount(loadNodeVersions);

  let defaultBusy = $state<string | null>(null);
  let saveError = $state('');
  let removeBusy = $state<string | null>(null);
  let removeError = $state<Record<string, string>>({});

  let newVersion = $state('');
  let installBusy = $state(false);
  let installDone = $state(false);
  let installError = $state('');

  // The pick shows while it saves, then the status decides: a failed save falls
  // back to the real default rather than leaving the wrong radio lit.
  const selectedDefault = $derived(defaultBusy ?? nodeDefault);

  async function onPickDefault(v: string) {
    if (v === nodeDefault || defaultBusy !== null) return;
    defaultBusy = v;
    saveError = '';
    try {
      if (!(await setDefaultNode(v))) saveError = m.common_failed();
      await loadStatus();
      await loadNodeVersions();
    } catch (e) {
      saveError = e instanceof Error ? e.message : m.common_failed();
    } finally {
      defaultBusy = null;
    }
  }

  async function onRemove(v: string) {
    removeBusy = v;
    removeError = { ...removeError, [v]: '' };
    try {
      const ok = await removeNode(v);
      if (!ok) removeError = { ...removeError, [v]: m.common_failed() };
      await loadStatus();
      await loadNodeVersions();
    } finally {
      removeBusy = null;
    }
  }

  const outdated = $derived($nodeVersionStatuses.filter((s) => s.update_available));
  // What was outdated when the dialog opened, null while it is closed.
  let updateBatch = $state<NodeVersionStatus[] | null>(null);
  let updateBusy = $state<string | null>(null);
  let updateError = $state<Record<string, string>>({});

  // The result stays on this card: a failed download names the version it hit.
  async function onUpdate(major: string) {
    updateBusy = major;
    updateError = { ...updateError, [major]: '' };
    const res = await updateNodeVersion(major);
    if (!res.ok) updateError = { ...updateError, [major]: res.error || m.common_failed() };
    await loadNodeVersions();
    updateBusy = null;
  }

  let checking = $state(false);

  // The toast answers the click; the cards follow from the versions it returns.
  async function runCheck() {
    checking = true;
    try {
      const res = await checkNodeUpdates();
      const found = res.versions?.some((s) => s.update_available);
      notifyLocalInfo(
        'update_check',
        m.system_nodeJs(),
        !res.ok
          ? m.system_tools_checkFailed()
          : found
            ? m.system_tools_checkFound()
            : m.system_node_checkUpToDate()
      );
    } finally {
      checking = false;
    }
  }

  async function onInstall() {
    if (!newVersion.trim()) return;
    installBusy = true;
    installError = '';
    installDone = false;
    try {
      const ok = await installNode(newVersion.trim());
      if (ok) {
        installDone = true;
        newVersion = '';
        setTimeout(() => (installDone = false), 2000);
      } else {
        installError = m.system_node_installFailed();
      }
    } catch (e) {
      installError = e instanceof Error ? e.message : m.common_failed();
    } finally {
      installBusy = false;
    }
  }

  // A site with no version of its own runs on the default, so the default's
  // list owns it too. Only that card ever picks these up, and it is never
  // removable, so the list can hold more than the removal guard's count.
  function sitesForVersion(v: string) {
    return $sites
      .filter((s) => s.node_version === v || (!s.node_version && v === nodeDefault))
      .map((s) => s.domain);
  }

  let manageBusy = $state(false);
  let managerBusy = $state(false);
  let manageError = $state('');

  const managerOptions = [
    { value: 'mise' as const, label: 'mise', title: m.system_node_managerMise() },
    { value: 'fnm' as const, label: 'fnm', title: m.system_node_managerFnm() },
    {
      value: 'nvm' as const,
      label: 'nvm',
      title: $status.nvm_available ? m.system_node_managerNvm() : m.system_node_managerNvmMissing(),
      disabled: !$status.nvm_available
    }
  ];

  async function onSwitchManager(manager: 'mise' | 'fnm' | 'nvm') {
    if (managerBusy || manageBusy || manager === $status.node_manager) return;
    if (manager === 'nvm' && !$status.nvm_available) return;
    managerBusy = true;
    manageError = '';
    try {
      const res = await setNodeManager(manager);
      if (!res.ok) manageError = res.error || m.common_failed();
      await loadStatus();
      await loadNodeVersions();
    } catch (e) {
      manageError = e instanceof Error ? e.message : m.common_failed();
    } finally {
      managerBusy = false;
    }
  }

  async function onToggleManage(manage: boolean) {
    manageBusy = true;
    manageError = '';
    try {
      const ok = manage ? await manageNode() : await unmanageNode();
      if (!ok) manageError = m.common_failed();
      await loadStatus();
      await loadNodeVersions();
    } catch (e) {
      manageError = e instanceof Error ? e.message : m.common_failed();
    } finally {
      manageBusy = false;
    }
  }
</script>

<div class="flex-1 overflow-y-auto">
  <DetailHeader title={$status.using_system_bun ? m.dashboard_health_jsRuntime() : m.system_nodeJs()}>
    {#snippet beside()}
      {#if !$status.node_managed_by_lerd}
        <span class="text-[10px] font-medium text-blue-500 dark:text-blue-400 bg-blue-50 dark:bg-blue-500/10 rounded-sm px-1.5 py-0.5">{m.system_system()}</span>
      {/if}
      {#if $status.bun_available}
        <span
          class="inline-flex items-center gap-1 text-[10px] font-medium rounded-sm px-1.5 py-0.5
            {$status.using_system_bun
              ? 'text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-500/10'
              : 'text-gray-500 dark:text-gray-400 bg-gray-100 dark:bg-white/5'}"
          title={$status.using_system_bun ? m.system_node_usingBunHint() : ''}
        >🥟 bun {$status.bun_version}</span>
      {/if}
    {/snippet}
    {#snippet trailing()}
    <div class="flex items-center gap-2">
      {#if manageError}<span class="text-xs text-red-500">{manageError}</span>{/if}
      {#if outdated.length > 0 && $accessMode.localControl}
        <DetailButton tone="success" onclick={() => (updateBatch = [...outdated])}>{m.services_updateAll()}</DetailButton>
      {/if}
      <!-- mise and fnm are always both on offer; nvm is only ever a choice on a
           host that has one, and the option disables itself when it does not. -->
      {#if $status.node_managed_by_lerd}
        <CheckUpdatesButton onclick={runCheck} {checking} title={m.system_node_checkTitle()} />
      {/if}
      {#if !$status.using_system_bun}
        <SegmentedControl
          options={managerOptions}
          value={$status.node_manager}
          label={m.system_node_manager()}
          onchange={onSwitchManager}
        />
      {/if}
      {#if $status.node_managed_by_lerd}
        <DetailButton
          tone="warn"
          onclick={() => onToggleManage(false)}
          disabled={manageBusy}
          loading={manageBusy}
          title={m.system_node_unmanageHint()}
        >{m.system_node_unmanage()}</DetailButton>
      {:else}
        <DetailButton
          tone="primary"
          onclick={() => onToggleManage(true)}
          disabled={manageBusy}
          loading={manageBusy}
          title={m.system_node_manageHint()}
        >{m.system_node_manage()}</DetailButton>
      {/if}
    </div>
    {/snippet}
  </DetailHeader>

  <div class="p-3 space-y-3">
    {#if $status.using_system_bun}
      <div class="text-sm text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-500/20 rounded-lg px-3 py-2.5">
        <span class="font-medium">🥟 {m.system_node_usingBun()} {$status.bun_version}.</span> {m.system_node_usingBunHint()}
      </div>
    {:else if !$status.node_managed_by_lerd}
      <div class="text-sm text-blue-700 dark:text-blue-300 bg-blue-50 dark:bg-blue-500/10 border border-blue-200 dark:border-blue-500/20 rounded-lg px-3 py-2.5">
        <span class="font-medium">{m.system_node_managedBySystem()}</span> {m.system_node_managedBySystemHint()}
      </div>
    {/if}

    {#if $nodeVersions.length === 0}
      <p class="text-sm text-gray-400">{m.system_node_noneInstalled()}</p>
    {/if}
    {#if saveError}<p class="text-xs text-red-500">{saveError}</p>{/if}
    <div class="flex flex-wrap gap-3">
      {#each $nodeVersions as v (v)}
        {@const detail = $nodeVersionStatuses.find((s) => s.major === v)}
        {@const siteDomains = sitesForVersion(v)}
        {@const siteCount = $sitesByNode.get(v) ?? 0}
        {@const isDefault = v === nodeDefault}
        {@const isSelected = v === selectedDefault}
        {@const canRemove = siteCount === 0 && $status.node_managed_by_lerd && !isDefault}
        <VersionCard value={detail?.installed ?? v} wide highlight={isSelected && !isDefault}>
          {#snippet heading()}
            <div class="flex items-center gap-2 min-w-0">
              <button
                type="button"
                onclick={() => onPickDefault(v)}
                disabled={defaultBusy !== null}
                aria-pressed={isSelected}
                aria-label={'Node ' + v}
                class="flex items-center gap-2 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <span
                  class="relative w-4 h-4 rounded-full border-2 shrink-0 transition-colors
                    {isSelected
                      ? 'border-lerd-red'
                      : 'border-gray-400 dark:border-gray-500 hover:border-gray-500 dark:hover:border-gray-400'}"
                >
                  {#if isSelected}
                    <span class="absolute inset-[2px] rounded-full bg-lerd-red"></span>
                  {/if}
                </span>
                <span class="font-mono text-xs font-semibold text-gray-700 dark:text-gray-300">Node {v}</span>
              </button>
              {#if isDefault}
                <span class="text-[10px] font-medium text-lerd-red bg-red-50 dark:bg-red-900/20 px-1.5 py-0.5 rounded-sm">{m.common_default()}</span>
              {/if}
            </div>
          {/snippet}
          {#snippet pill()}
            {#if detail?.update_available}
              <StatusPill tone="warn" label={m.system_lerd_updateTag()} />
            {:else if detail?.latest}
              <StatusPill tone="ok" label={m.system_tools_current()} />
            {/if}
          {/snippet}
          {#if detail?.update_available}
            <span class="text-xs font-medium text-yellow-700 dark:text-yellow-400 truncate">{m.system_tools_updateAvailable({ version: detail.latest ?? '' })}</span>
            {#if $accessMode.localControl}
              <DetailButton
                tone="success"
                block
                onclick={() => onUpdate(v)}
                disabled={updateBusy !== null}
                loading={updateBusy === v}
              >{m.services_update()}</DetailButton>
            {/if}
          {/if}
          <div class="flex items-center justify-between gap-2">
            <SitesPopover domains={siteDomains} />
            <div class="ml-auto">
              <DetailButton
                tone="danger"
                onclick={() => onRemove(v)}
                disabled={!canRemove || removeBusy === v}
                loading={removeBusy === v}
                title={!$status.node_managed_by_lerd
                  ? m.system_node_cannotRemoveSystem()
                  : isDefault
                    ? m.system_node_cannotRemoveDefault()
                    : siteCount > 0
                      ? m.system_node_cannotRemove()
                      : m.system_node_removeTitle()}
                icon={trashIcon}
              />
            </div>
          </div>
          {#if removeError[v]}
            <p class="text-xs text-red-500">{removeError[v]}</p>
          {/if}
          {#if updateError[v]}
            <p class="text-xs text-red-500 break-words">{updateError[v]}</p>
          {/if}
        </VersionCard>
      {/each}

      <div class="shrink-0 w-[15rem] flex flex-col gap-2.5 rounded-2xl border border-dashed border-gray-200 dark:border-lerd-border bg-gray-50/50 dark:bg-white/2 p-3">
        <p class="text-xs font-semibold text-gray-700 dark:text-gray-300">{m.system_node_installNewTitle()}</p>
        <p class="text-xs text-gray-400">
          {@html m.system_node_installNewHint({ major: '<code class="font-mono bg-gray-100 dark:bg-white/5 px-1 rounded-sm">22</code>', specific: '<code class="font-mono bg-gray-100 dark:bg-white/5 px-1 rounded-sm">22.12.0</code>', manager: $status.node_manager || 'mise' })}
        </p>
        <div class="flex items-center gap-2">
          <input
            type="text"
            bind:value={newVersion}
            onkeydown={(e) => e.key === 'Enter' && onInstall()}
            placeholder={m.system_node_installPlaceholder()}
            disabled={installBusy || !$status.node_managed_by_lerd}
            title={!$status.node_managed_by_lerd ? m.system_node_managedBySystem() : ''}
            class="text-sm bg-white dark:bg-lerd-card border border-gray-200 dark:border-lerd-border rounded-lg px-3 py-1.5 w-full min-w-0 text-gray-700 dark:text-gray-200 placeholder-gray-400 dark:placeholder-gray-600 focus:outline-hidden focus:border-lerd-red/50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          />
          <DetailButton
            tone="primary"
            onclick={onInstall}
            disabled={installBusy || !newVersion.trim() || !$status.node_managed_by_lerd}
            loading={installBusy}
          >{m.common_install()}</DetailButton>
        </div>
        {#if installDone}<span class="text-xs text-emerald-600 dark:text-emerald-500">{m.system_node_installed()}</span>{/if}
        {#if installError}<span class="text-xs text-red-500 break-words">{installError}</span>{/if}
      </div>
    </div>
  </div>
</div>

{#snippet trashIcon()}
  <Icon name="trash" class="w-4 h-4" />
{/snippet}

{#if updateBatch}
  <NodeUpdateModal versions={updateBatch} onclose={() => (updateBatch = null)} />
{/if}
