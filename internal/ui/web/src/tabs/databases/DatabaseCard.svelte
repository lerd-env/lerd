<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import Modal from '$components/Modal.svelte';
  import ActionCard from '$components/ActionCard.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { tooltip } from '$lib/tooltip';
  import { formatBytes } from '$lib/bytes';
  import {
    dsnFor,
    exportUrl,
    dropDatabase,
    importDatabase,
    type ImportIssue,
    type DatabaseEngine,
    type DatabaseEntry
  } from '$stores/databases';
  import { services, serviceLabel } from '$stores/services';
  import { autoSnapshot, setSiteAutoSnapshot } from '$stores/autoSnapshot';
  import { databaseAdminFor, openDatabaseAdmin } from '$stores/dashboard';
  import { goToTab } from '$stores/route';
  import DatabaseSnapshotsModal from './DatabaseSnapshotsModal.svelte';
  import DatabaseOpStatus from './DatabaseOpStatus.svelte';
  import ImportIssuesModal from './ImportIssuesModal.svelte';
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
  // The pair the drop can take along, offered only when the card is pointing at
  // the database being tested; the testing half is nobody's pair.
  const dropTesting = $derived(target === 'app' ? testing : undefined);
  // Ticked to match how the pair was created, since dropping one half and
  // leaving the other is almost never what was meant.
  let dropPaired = $state(true);
  let fileInput = $state<HTMLInputElement | null>(null);
  // The dump being imported, from the first byte uploaded to the confirmation
  // or the engine's error; null whenever no import has run on this card.
  let importOp = $state<{
    tone: 'busy' | 'done' | 'warn' | 'error';
    file: string;
    percent: number | null;
    error?: string;
    errors?: number;
    issues?: ImportIssue[];
    omitted?: number;
    skipped?: ImportIssue[];
    created?: ImportIssue[];
  } | null>(null);
  // The dump waiting on the confirmation, and whether to empty the database
  // before it loads.
  let pending = $state<File | null>(null);
  let importFresh = $state(false);
  const importBusy = $derived(importOp?.tone === 'busy');
  // The engine's complaints open over the page rather than inside the card,
  // which has no room for a list and belongs to one database in a grid.
  let showIssues = $state(false);
  let clearDone: ReturnType<typeof setTimeout> | undefined;
  $effect(() => () => clearTimeout(clearDone));
  const importMessage = $derived.by(() => {
    if (!importOp) return '';
    if (importOp.tone === 'error') return m.databases_importFailed({ error: importOp.error ?? '' });
    if (importOp.tone === 'warn')
      return m.databases_importedWithErrors({ file: importOp.file, count: importOp.errors ?? 0 });
    if (importOp.tone === 'done')
      return importOp.created?.length
        ? m.databases_importedWithExtensions({
            file: importOp.file,
            names: importOp.created.map((c) => c.message).join(', ')
          })
        : m.databases_imported({ file: importOp.file });
    return importOp.percent === null
      ? m.databases_importing({ file: importOp.file })
      : m.databases_importingPercent({
          file: importOp.file,
          percent: Math.round(importOp.percent * 100)
        });
  });

  // The engines' dumps travel as SQL text unless the preset declares another
  // format, in which case any file may be a valid archive.
  const importAccept = $derived(engine.dump_format === 'sql' ? '.sql,.txt,.gz' : '');
  // A worktree's isolated database is shown under the branch's own domain, so it
  // reads as staging's data rather than as another database of the parent site.
  const ownerDomain = $derived(active.branch ? `${active.branch}.${active.site}` : active.site);
  const snapshotCount = $derived(active.snapshots?.length ?? 0);
  // Shown only while the schedule actually covers this database, so the icon's
  // presence is the answer rather than something to decode.
  const scheduled = $derived(
    $autoSnapshot.sites.find((s) => s.service === engine.service && s.database === active.name)
  );
  let schedulingBusy = $state(false);

  // The indicator is the control: one click from the grid includes or excludes
  // this database, which is the whole per-database choice.
  async function toggleScheduled() {
    if (!scheduled || schedulingBusy) return;
    schedulingBusy = true;
    await setSiteAutoSnapshot(scheduled.site, scheduled.covered ? 'off' : 'on');
    schedulingBusy = false;
  }
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

  // Picking a file opens the confirmation rather than loading straight away, so
  // the choice between loading on top and replacing is made before anything runs.
  function onFilePicked(e: Event) {
    const input = e.target as HTMLInputElement;
    pending = input.files?.[0] ?? null;
    input.value = '';
    importFresh = false;
  }

  async function startImport() {
    const file = pending;
    pending = null;
    if (!file) return;
    const fresh = importFresh;
    importOp = { tone: 'busy', file: file.name, percent: 0 };
    const res = await importDatabase(engine.service, active.name, file, (p) => {
      if (importOp?.tone !== 'busy') return;
      // Once the last byte is out the engine is still replaying the dump, and
      // there is nothing left to measure, so the bar gives way to a spinner.
      importOp = { ...importOp, percent: p.uploaded ? null : p.percent };
    }, fresh);
    if (!res.ok) {
      importOp = { tone: 'error', file: file.name, percent: null, error: res.error || m.common_failed() };
      return;
    }
    // A load the engine only half swallowed still returns ok, so the counted
    // complaints are the only thing standing between that and a false all-clear.
    if (res.errors) {
      importOp = {
        tone: 'warn',
        file: file.name,
        percent: null,
        errors: res.errors,
        issues: res.issues,
        omitted: res.omitted,
        skipped: res.skipped,
        created: res.created
      };
      showIssues = (res.issues ?? []).length > 0;
      return;
    }
    importOp = { tone: 'done', file: file.name, percent: null, created: res.created };
    clearTimeout(clearDone);
    clearDone = setTimeout(() => {
      if (importOp?.tone === 'done') importOp = null;
    }, 4000);
  }

  function openDrop() {
    dropError = '';
    dropPaired = true;
    showDrop = true;
  }

  async function confirmDrop() {
    dropBusy = true;
    dropError = '';
    const res = await dropDatabase(engine.service, active.name, Boolean(dropTesting) && dropPaired);
    dropBusy = false;
    if (!res.ok) {
      dropError = res.error || m.common_failed();
      return;
    }
    showDrop = false;
  }
</script>

<ActionCard>
  {#snippet header()}
    <Icon name="database" class="w-4 h-4 mt-0.5 shrink-0 text-gray-300 dark:text-gray-600" />
    <div class="min-w-0 flex-1">
      <p class="truncate text-sm font-semibold text-gray-800 dark:text-gray-100" title={active.name}>{active.name}</p>
      <p class="flex flex-wrap items-center gap-x-1.5 text-xs text-gray-400 dark:text-gray-500">
        <span class="shrink-0 tabular-nums">{formatBytes(active.size_bytes)}</span>
        {#if scheduled}
          <button
            type="button"
            onclick={toggleScheduled}
            disabled={schedulingBusy}
            aria-pressed={scheduled.covered}
            use:tooltip={scheduled.covered ? m.snapshots_auto_exclude() : m.snapshots_auto_include()}
            aria-label={scheduled.covered ? m.snapshots_auto_exclude() : m.snapshots_auto_include()}
            class="shrink-0 rounded-sm transition-colors {scheduled.covered
              ? 'text-emerald-600 dark:text-emerald-500'
              : 'text-gray-300 dark:text-gray-600 hover:text-emerald-600 dark:hover:text-emerald-500'}"
          >
            <Icon name="clock" class="w-3 h-3" />
          </button>
        {/if}
        {#if active.site}
          <!-- The separator travels with the domain so it never strands at the
               end of the size line when the two wrap onto separate rows. -->
          <span class="inline-flex min-w-0 max-w-full items-center gap-1.5">
            <span class="shrink-0" aria-hidden="true">·</span>
            <button
              type="button"
              onclick={() => goToTab('sites', active.site ?? '')}
              class="min-w-0 truncate text-sky-600 dark:text-sky-400 hover:underline"
              title={ownerDomain}
            >{ownerDomain}</button>
          </span>
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
  {/snippet}

  {#snippet actions()}
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

    {#if engine.supports_export}
      <a
        href={exportUrl(engine.service, active.name)}
        use:tooltip={m.databases_export()}
        aria-label={m.databases_export()}
        class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
      >
        <Icon name="download" class="w-3.5 h-3.5" />
      </a>
    {/if}

    {#if engine.supports_import}
      <button
        type="button"
        use:tooltip={m.databases_import()}
        aria-label={m.databases_import()}
        onclick={() => fileInput?.click()}
        disabled={importBusy}
        class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
      >
        <Icon name={importBusy ? 'spinner' : 'upload'} class="w-3.5 h-3.5 {importBusy ? 'animate-spin' : ''}" />
      </button>
      <input bind:this={fileInput} type="file" accept={importAccept} class="hidden" onchange={onFilePicked} />
    {/if}

    {#if engine.supports_snapshot}
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
            class="absolute -top-0.5 -right-0.5 min-w-3.5 h-3.5 px-0.5 rounded-full bg-emerald-500 text-white text-[9px] font-semibold flex items-center justify-center"
          >{snapshotCount}</span>
        {/if}
      </button>
    {/if}

    {#if engine.supports_drop}
      <button
        type="button"
        use:tooltip={m.databases_drop()}
        aria-label={m.databases_drop()}
        onclick={openDrop}
        class="ml-auto flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-500/10 transition-colors"
      >
        <Icon name="trash" class="w-3.5 h-3.5" />
      </button>
    {/if}
  {/snippet}

  {#if importOp}
    <div class="mt-2">
      <DatabaseOpStatus tone={importOp.tone} message={importMessage} percent={importOp.percent}>
        {#if importOp.tone === 'warn' && (importOp.issues ?? []).length > 0}
          <button
            type="button"
            onclick={() => (showIssues = true)}
            class="text-xs underline text-amber-600 dark:text-amber-400 hover:no-underline"
          >{m.databases_importIssuesLink()}</button>
        {/if}
      </DatabaseOpStatus>
    </div>
  {/if}
</ActionCard>

{#if showIssues && importOp?.issues}
  <ImportIssuesModal
    title={importMessage}
    issues={importOp.issues}
    omitted={importOp.omitted}
    skipped={importOp.skipped}
    onclose={() => (showIssues = false)}
  />
{/if}

{#if showSnapshots}
  <DatabaseSnapshotsModal {engine} entry={active} onclose={() => (showSnapshots = false)} />
{/if}

{#if pending}
  <Modal
    open
    title={m.databases_importTitle({ file: pending.name })}
    onclose={() => (pending = null)}
    size="sm"
  >
    <div class="px-5 py-4 space-y-3">
      <p class="text-sm text-gray-700 dark:text-gray-300">
        {m.databases_importBody({ name: active.name })}
      </p>
      <label class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input type="checkbox" bind:checked={importFresh} class="mt-0.5 accent-lerd-red" />
        <span>
          {m.databases_importFresh()}
          <span class="block text-xs text-gray-500 dark:text-gray-400">
            {m.databases_importFreshHint()}
          </span>
        </span>
      </label>
    </div>
    {#snippet footer()}
      <DetailButton onclick={() => (pending = null)}>{m.common_cancel()}</DetailButton>
      <DetailButton tone={importFresh ? 'danger' : 'primary'} onclick={startImport}>
        {m.databases_import()}
      </DetailButton>
    {/snippet}
  </Modal>
{/if}

{#if showDrop}
  <Modal open title={m.databases_dropTitle({ name: active.name })} onclose={() => !dropBusy && (showDrop = false)} size="sm">
    <div class="px-5 py-4 space-y-2">
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.databases_dropBody()}</p>
      {#if dropTesting}
        <label class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input type="checkbox" bind:checked={dropPaired} class="mt-0.5 accent-lerd-red" />
          <span>{m.databases_dropTesting({ name: dropTesting.name })}</span>
        </label>
      {/if}
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
