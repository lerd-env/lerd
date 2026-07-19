<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { formatBytes } from '$lib/bytes';
  import { parseSnapshotTimestamp, snapshotBaseName } from '$lib/snapshots';
  import { snapshotExportUrl, type Snapshot } from '$stores/databases';
  import {
    takeSnapshot,
    restoreSnapshot,
    deleteSnapshot,
    type DatabaseEngine,
    type DatabaseEntry
  } from '$stores/databases';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    engine: DatabaseEngine;
    entry: DatabaseEntry;
    onclose: () => void;
  }
  let { engine, entry, onclose }: Props = $props();

  let name = $state('');
  let busy = $state('');
  let error = $state('');
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

  async function take() {
    busy = 'take';
    error = '';
    const res = await takeSnapshot(engine.service, entry.name, name.trim());
    busy = '';
    if (!res.ok) {
      error = res.error || m.common_failed();
      return;
    }
    name = '';
  }

  async function restore(snapshot: string) {
    busy = snapshot;
    error = '';
    const res = await restoreSnapshot(engine.service, entry.name, snapshot);
    busy = '';
    confirmName = '';
    confirmAction = '';
    if (!res.ok) error = res.error || m.common_failed();
  }

  async function remove(snapshot: string) {
    busy = snapshot;
    error = '';
    const res = await deleteSnapshot(engine.service, entry.name, snapshot);
    busy = '';
    confirmName = '';
    confirmAction = '';
    if (!res.ok) error = res.error || m.common_failed();
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

    {#if error}
      <p class="text-xs text-red-500">{error}</p>
    {/if}

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
                {#if snapDateLabel(snap)}{snapDateLabel(snap)} · {/if}{formatBytes(snap.size_bytes)}
              </p>
            </div>
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
