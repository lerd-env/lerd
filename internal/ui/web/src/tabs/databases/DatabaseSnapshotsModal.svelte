<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { formatBytes } from '$lib/bytes';
  import { parseSnapshotTimestamp, snapshotBaseName } from '$lib/snapshots';
  import { snapshotExportUrl, type ImportIssue, type Snapshot } from '$stores/databases';
  import {
    takeSnapshot,
    restoreSnapshot,
    deleteSnapshot,
    keepSnapshot,
    type DatabaseEngine,
    type DatabaseEntry
  } from '$stores/databases';
  import DatabaseOpStatus from './DatabaseOpStatus.svelte';
  import Toggle from '$components/Toggle.svelte';
  import SnapshotSettingsModal from '../../modals/SnapshotSettingsModal.svelte';
  import {
    autoSnapshot,
    loadAutoSnapshot,
    setSiteAutoSnapshot,
    type AutoSnapshotSite
  } from '$stores/autoSnapshot';
  import { onMount } from 'svelte';
  import ImportIssuesModal from './ImportIssuesModal.svelte';
  import { m } from '../../paraglide/messages.js';

  type Result = {
    ok: boolean;
    error?: string;
    errors?: number;
    issues?: ImportIssue[];
    omitted?: number;
  };

  interface Props {
    engine: DatabaseEngine;
    entry: DatabaseEntry;
    onclose: () => void;
  }
  let { engine, entry, onclose }: Props = $props();

  let name = $state('');
  let busy = $state('');
  let showSettings = $state(false);

  onMount(loadAutoSnapshot);

  // This database's own standing with the schedule, resolved through the site
  // that owns it: the policy is global, the opt-in is the site's.
  const scheduled = $derived<AutoSnapshotSite | undefined>(
    $autoSnapshot.sites.find((s) => s.service === engine.service && s.database === entry.name)
  );
  function when(iso?: string): string {
    if (!iso) return m.snapshots_auto_never();
    return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
  }

  // An explicit click is an explicit choice, so it writes "on" or "off" rather
  // than clearing back to the policy: switching the global mode later must not
  // move a database the user has already decided about.
  async function toggleScheduled() {
    if (!scheduled) return;
    const res = await setSiteAutoSnapshot(scheduled.site, scheduled.covered ? 'off' : 'on');
    if (!res.ok) status = { tone: 'error', message: res.error || m.common_failed() };
  }
  // The running or last-finished operation, so a restore that takes a while
  // says what it is doing instead of only spinning on its button.
  let status = $state<{
    tone: 'busy' | 'done' | 'warn' | 'error';
    message: string;
    issues?: ImportIssue[];
    omitted?: number;
  } | null>(null);
  let showIssues = $state(false);
  let clearDone: ReturnType<typeof setTimeout> | undefined;
  $effect(() => () => clearTimeout(clearDone));
  // The snapshot + action pending confirmation. Restore overwrites data and
  // delete is irreversible, so each takes a second click.
  let confirmName = $state('');
  let confirmAction = $state<'restore' | 'delete' | ''>('');

  function ask(snapshot: string, action: 'restore' | 'delete') {
    confirmName = snapshot;
    confirmAction = action;
  }

  // Prefer the time stamped into the name, falling back to the recorded created
  // time for snapshots taken before names carried a stamp.
  function snapDate(snap: Snapshot): Date | null {
    return parseSnapshotTimestamp(snap.name) ?? (snap.created ? new Date(snap.created) : null);
  }
  function snapDateLabel(snap: Snapshot): string {
    const d = snapDate(snap);
    return d ? d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : '';
  }
  const snapshots = $derived(
    [...(entry.snapshots ?? [])].sort(
      (a, b) => (snapDate(b)?.getTime() ?? 0) - (snapDate(a)?.getTime() ?? 0)
    )
  );

  function safeClose() {
    if (busy) return;
    onclose();
  }

  // run drives every snapshot operation through the same status line: running
  // while it works, the engine's error when it fails, a confirmation when it
  // lands that clears itself a few seconds later.
  async function run(
    key: string,
    running: string,
    done: string,
    op: () => Promise<Result>,
    warned?: (count: number) => string
  ): Promise<boolean> {
    busy = key;
    status = { tone: 'busy', message: running };
    const res = await op();
    busy = '';
    if (!res.ok) {
      status = { tone: 'error', message: res.error || m.common_failed() };
      return false;
    }
    // A load the engine only half swallowed still comes back ok, so its counted
    // complaints are what stands between that and a false all-clear.
    if (res.errors && warned) {
      status = { tone: 'warn', message: warned(res.errors), issues: res.issues, omitted: res.omitted };
      showIssues = (res.issues ?? []).length > 0;
      return true;
    }
    status = { tone: 'done', message: done };
    clearTimeout(clearDone);
    clearDone = setTimeout(() => {
      if (status?.tone === 'done' && status.message === done) status = null;
    }, 4000);
    return true;
  }

  async function take() {
    const label = name.trim();
    const ok = await run(
      'take',
      m.databases_takingSnapshot({ name: entry.name }),
      m.databases_snapshotTaken(),
      () => takeSnapshot(engine.service, entry.name, label)
    );
    if (ok) name = '';
  }

  async function restore(snapshot: string) {
    await run(
      snapshot,
      m.databases_restoring({ name: snapshotBaseName(snapshot) }),
      m.databases_restored({ name: snapshotBaseName(snapshot) }),
      () => restoreSnapshot(engine.service, entry.name, snapshot),
      (count) => m.databases_restoredWithErrors({ name: snapshotBaseName(snapshot), count })
    );
    confirmName = '';
    confirmAction = '';
  }

  // An automatic snapshot says when retention drops it, and a kept one says it
  // never will, so the list answers "will this still be here tomorrow" on sight.
  function expiryLabel(snap: Snapshot): string {
    if (!snap.auto) return '';
    if (snap.kept) return m.databases_snapshotKept();
    if (!snap.expires_at) return m.databases_snapshotAuto();
    const when = new Date(snap.expires_at).toLocaleString(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short'
    });
    return snap.estimated
      ? m.databases_snapshotExpiresAbout({ when })
      : m.databases_snapshotExpires({ when });
  }

  async function toggleKeep(snap: Snapshot) {
    const kept = !snap.kept;
    await run(
      snap.name,
      kept ? m.databases_keepingSnapshot() : m.databases_releasingSnapshot(),
      kept ? m.databases_snapshotKeptDone() : m.databases_snapshotReleasedDone(),
      () => keepSnapshot(engine.service, entry.name, snap.name, kept)
    );
  }

  async function remove(snapshot: string) {
    await run(
      snapshot,
      m.databases_deletingSnapshot({ name: snapshotBaseName(snapshot) }),
      m.databases_snapshotDeleted({ name: snapshotBaseName(snapshot) }),
      () => deleteSnapshot(engine.service, entry.name, snapshot)
    );
    confirmName = '';
    confirmAction = '';
  }
</script>

<Modal open title={m.databases_snapshotsTitle({ name: entry.name })} onclose={safeClose} size="md">
  <div class="px-5 py-4 space-y-4">
    <div class="flex gap-2">
      <input
        bind:value={name}
        placeholder={m.databases_snapshotNamePlaceholder()}
        class="flex-1 min-w-0 rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-lerd-red/30"
      />
      <DetailButton tone="primary" onclick={take} loading={busy === 'take'} disabled={Boolean(busy)}>
        {m.databases_takeSnapshot()}
      </DetailButton>
    </div>

    {#if status}
      <DatabaseOpStatus tone={status.tone} message={status.message}>
        {#if status.tone === 'warn' && (status.issues ?? []).length > 0}
          <button
            type="button"
            onclick={() => (showIssues = true)}
            class="text-xs underline text-amber-600 dark:text-amber-400 hover:no-underline"
          >{m.databases_importIssuesLink()}</button>
        {/if}
      </DatabaseOpStatus>
    {/if}

    <div class="rounded-lg border border-gray-100 dark:border-lerd-border px-3 py-2.5">
      <div class="flex items-center justify-between gap-2">
        <div class="min-w-0">
          <p class="flex items-center gap-1.5 text-sm font-medium text-gray-700 dark:text-gray-300">
            <Icon
              name="clock"
              class="w-3.5 h-3.5 shrink-0 {scheduled?.covered ? 'text-lerd-red' : 'text-gray-300 dark:text-gray-600'}"
            />
            {m.snapshots_auto_title()}
          </p>
          {#if !scheduled}
            <p class="text-[11px] text-gray-400 dark:text-gray-500">{m.snapshots_auto_noSiteForDatabase()}</p>
          {:else if scheduled.covered}
            <!-- Two rows: the pair of full timestamps runs past the dialog on
                 one line, and the block sits next to the mode control. -->
            <p class="text-[11px] text-gray-400 dark:text-gray-500">
              {m.snapshots_auto_last()}: {when(scheduled.last)}
            </p>
            <p class="text-[11px] text-gray-400 dark:text-gray-500">
              {m.snapshots_auto_next()}: {when(scheduled.next)}
            </p>
          {:else}
            <p class="text-[11px] text-gray-400 dark:text-gray-500">{m.common_disabled()}</p>
          {/if}
        </div>
        <div class="flex items-center gap-1.5 shrink-0">
          {#if scheduled}
            <!-- Named for what it does to this database, so it is never read
                 as the schedule's own on/off switch in the settings dialog. -->
            <Toggle
              on={scheduled.covered}
              title={scheduled.covered ? m.snapshots_auto_exclude() : m.snapshots_auto_include()}
              onclick={toggleScheduled}
            />
          {/if}
          <button
            type="button"
            onclick={() => (showSettings = true)}
            use:tooltip={m.snapshots_auto_settings()}
            aria-label={m.snapshots_auto_settings()}
            class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
          >
            <Icon name="system" class="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </div>

    {#if snapshots.length === 0}
      <p class="text-sm text-gray-400 dark:text-gray-500">{m.databases_noSnapshots()}</p>
    {:else}
      <ul class="divide-y divide-gray-100 dark:divide-lerd-border/60 rounded-lg border border-gray-100 dark:border-lerd-border">
        {#each snapshots as snap (snap.name)}
          {@const pending = confirmName === snap.name ? confirmAction : ''}
          <li class="flex items-center gap-2 px-3 py-2">
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-medium text-gray-800 dark:text-gray-200" title={snap.name}>{snapshotBaseName(snap.name)}</p>
              <p class="text-[11px] text-gray-400 dark:text-gray-500">
                {#if snapDateLabel(snap)}{snapDateLabel(snap)} · {/if}{formatBytes(snap.size_bytes)}{#if expiryLabel(snap)} · {expiryLabel(snap)}{/if}
              </p>
            </div>
            {#if snap.auto}
              <button
                type="button"
                onclick={() => toggleKeep(snap)}
                disabled={Boolean(busy)}
                use:tooltip={snap.kept ? m.databases_snapshotRelease() : m.databases_snapshotKeep()}
                aria-label={snap.kept ? m.databases_snapshotRelease() : m.databases_snapshotKeep()}
                aria-pressed={Boolean(snap.kept)}
                class="flex items-center justify-center w-7 h-7 rounded-md transition-colors {snap.kept
                  ? 'text-lerd-red'
                  : 'text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5'}"
              >
                <Icon name="bookmark" class="w-3.5 h-3.5" />
              </button>
            {/if}
            <a
              href={snapshotExportUrl(engine.service, entry.name, snap.name)}
              use:tooltip={m.databases_export()}
              aria-label={m.databases_export()}
              class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
            >
              <Icon name="download" class="w-3.5 h-3.5" />
            </a>
            {#if pending === 'restore'}
              <DetailButton
                tone="danger"
                onclick={() => restore(snap.name)}
                loading={busy === snap.name}
                disabled={Boolean(busy)}
              >
                {m.databases_restoreConfirm()}
              </DetailButton>
            {:else}
              <DetailButton onclick={() => ask(snap.name, 'restore')} disabled={Boolean(busy)}>
                {m.databases_restore()}
              </DetailButton>
            {/if}
            {#if pending === 'delete'}
              <DetailButton
                tone="danger"
                onclick={() => remove(snap.name)}
                loading={busy === snap.name}
                disabled={Boolean(busy)}
              >
                {m.databases_deleteConfirm()}
              </DetailButton>
            {:else}
              <DetailButton tone="danger" onclick={() => ask(snap.name, 'delete')} disabled={Boolean(busy)}>
                {m.databases_delete()}
              </DetailButton>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={Boolean(busy)}>{m.common_cancel()}</DetailButton>
  {/snippet}
</Modal>

{#if showSettings}
  <SnapshotSettingsModal onclose={() => (showSettings = false)} />
{/if}

{#if showIssues && status?.issues}
  <ImportIssuesModal
    title={status.message}
    issues={status.issues}
    omitted={status.omitted}
    onclose={() => (showIssues = false)}
  />
{/if}
