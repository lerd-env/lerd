<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { tooltip } from '$lib/tooltip';
  import { formatBytes } from '$lib/bytes';
  import {
    dsnFor,
    exportUrl,
    dropDatabase,
    importDatabase,
    type DatabaseEngine,
    type DatabaseEntry
  } from '$stores/databases';
  import { services, serviceLabel } from '$stores/services';
  import { databaseAdminFor, openDatabaseAdmin } from '$stores/dashboard';
  import { goToTab } from '$stores/route';
  import DatabaseSnapshotsModal from './DatabaseSnapshotsModal.svelte';
  import SegmentedControl from '$components/SegmentedControl.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    engine: DatabaseEngine;
    entry: DatabaseEntry;
    // The entry's "<name>_testing" sibling, folded into this card when it exists.
    testing?: DatabaseEntry;
  }
  let { engine, entry, testing }: Props = $props();

  let target = $state<'app' | 'testing'>('app');
  // Everything below acts on the half the segment points at, and falls back to
  // the parent so dropping the testing database can't leave the card orphaned.
  const active = $derived(target === 'testing' && testing ? testing : entry);

  let copied = $state(false);
  let showSnapshots = $state(false);
  let showDrop = $state(false);
  let dropBusy = $state(false);
  let dropError = $state('');
  let importBusy = $state(false);
  let fileInput = $state<HTMLInputElement | null>(null);

  const sqlOps = $derived(engine.supports_snapshot);
  const snapshotCount = $derived(active.snapshots?.length ?? 0);
  // The installed admin tool that can open this specific database (phpMyAdmin,
  // Adminer, Mongo Express); null when none is installed or can't deep-link.
  const admin = $derived.by(() => {
    void $services;
    return databaseAdminFor(engine.service);
  });

  async function copyDsn() {
    const dsn = dsnFor(engine, active.name);
    if (!dsn) return;
    await navigator.clipboard.writeText(dsn);
    copied = true;
    setTimeout(() => (copied = false), 1200);
  }

  async function onImport(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file) return;
    importBusy = true;
    await importDatabase(engine.service, active.name, file);
    importBusy = false;
  }

  async function confirmDrop() {
    dropBusy = true;
    dropError = '';
    const res = await dropDatabase(engine.service, active.name);
    dropBusy = false;
    if (!res.ok) {
      dropError = res.error || m.common_failed();
      return;
    }
    showDrop = false;
  }
</script>

<div class="flex flex-col rounded-xl border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-3 transition duration-150 hover:border-gray-300 dark:hover:border-white/15 hover:shadow-sm">
  <div class="flex items-start gap-2">
    <Icon name="database" class="w-4 h-4 mt-0.5 shrink-0 text-gray-300 dark:text-gray-600" />
    <div class="min-w-0 flex-1">
      <p class="truncate text-sm font-semibold text-gray-800 dark:text-gray-100" title={active.name}>{active.name}</p>
      <p class="flex items-center gap-1.5 text-xs text-gray-400 dark:text-gray-500">
        <span class="tabular-nums">{formatBytes(active.size_bytes)}</span>
        {#if active.site}
          <span aria-hidden="true">·</span>
          <button
            type="button"
            onclick={() => goToTab('sites', active.site ?? '')}
            class="min-w-0 truncate text-sky-600 dark:text-sky-400 hover:underline"
            title={active.site}
          >{active.site}</button>
        {/if}
      </p>
    </div>
    {#if testing}
      <SegmentedControl
        label={m.databases_targetLabel()}
        value={target}
        options={[
          { value: 'app', label: m.databases_targetApp(), title: entry.name },
          { value: 'testing', label: m.databases_targetTesting(), title: testing.name }
        ]}
        onchange={(v) => (target = v)}
      />
    {/if}
  </div>

  <div class="flex items-center gap-0.5 mt-2 pt-2 border-t border-gray-100 dark:border-lerd-border/60">
    {#if admin}
      <button
        type="button"
        use:tooltip={m.databases_openIn({ name: serviceLabel(admin.name) })}
        aria-label={m.databases_openIn({ name: serviceLabel(admin.name) })}
        onclick={() => openDatabaseAdmin(engine.service, active.name)}
        class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
      >
        <Icon name="external" class="w-3.5 h-3.5" />
      </button>
    {/if}
    <button
      type="button"
      use:tooltip={copied ? m.databases_copied() : m.databases_copyDsn()}
      aria-label={m.databases_copyDsn()}
      onclick={copyDsn}
      class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
    >
      <Icon name={copied ? 'check' : 'clipboard'} class="w-3.5 h-3.5" />
    </button>

    {#if sqlOps}
      <a
        href={exportUrl(engine.service, active.name)}
        use:tooltip={m.databases_export()}
        aria-label={m.databases_export()}
        class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
      >
        <Icon name="download" class="w-3.5 h-3.5" />
      </a>

      <button
        type="button"
        use:tooltip={m.databases_import()}
        aria-label={m.databases_import()}
        onclick={() => fileInput?.click()}
        class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
      >
        <Icon name={importBusy ? 'spinner' : 'upload'} class="w-3.5 h-3.5 {importBusy ? 'animate-spin' : ''}" />
      </button>
      <input bind:this={fileInput} type="file" accept=".sql,.txt" class="hidden" onchange={onImport} />

      <button
        type="button"
        use:tooltip={m.databases_snapshots()}
        aria-label={m.databases_snapshots()}
        onclick={() => (showSnapshots = true)}
        class="relative flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
      >
        <Icon name="camera" class="w-3.5 h-3.5" />
        {#if snapshotCount > 0}
          <span
            class="absolute -top-0.5 -right-0.5 min-w-3.5 h-3.5 px-0.5 rounded-full bg-lerd-red text-white text-[9px] font-semibold flex items-center justify-center"
          >{snapshotCount}</span>
        {/if}
      </button>

      <button
        type="button"
        use:tooltip={m.databases_drop()}
        aria-label={m.databases_drop()}
        onclick={() => (showDrop = true)}
        class="ml-auto flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-500/10 transition-colors"
      >
        <Icon name="trash" class="w-3.5 h-3.5" />
      </button>
    {/if}
  </div>
</div>

{#if showSnapshots}
  <DatabaseSnapshotsModal {engine} entry={active} onclose={() => (showSnapshots = false)} />
{/if}

{#if showDrop}
  <Modal open title={m.databases_dropTitle({ name: active.name })} onclose={() => !dropBusy && (showDrop = false)} size="sm">
    <div class="px-5 py-4 space-y-2">
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.databases_dropBody()}</p>
      {#if dropError}
        <p class="text-xs text-red-500">{dropError}</p>
      {/if}
    </div>
    {#snippet footer()}
      <DetailButton onclick={() => (showDrop = false)} disabled={dropBusy}>{m.common_cancel()}</DetailButton>
      <DetailButton tone="danger" onclick={confirmDrop} loading={dropBusy} disabled={dropBusy}>
        {m.databases_drop()}
      </DetailButton>
    {/snippet}
  </Modal>
{/if}
