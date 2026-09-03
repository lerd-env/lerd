<script lang="ts">
  import SettingsCard from '$components/SettingsCard.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import Icon from '$components/Icon.svelte';
  import Modal from '$components/Modal.svelte';
  import SnapshotSettingsModal from '../../modals/SnapshotSettingsModal.svelte';
  import { tooltip } from '$lib/tooltip';
  import { formatBytes } from '$lib/bytes';
  import { parseSnapshotTimestamp, snapshotBaseName, intervalParts } from '$lib/snapshots';
  import {
    databases,
    loadEngine,
    keepSnapshot,
    deleteSnapshot,
    restoreSnapshot,
    snapshotExportUrl,
    type Snapshot
  } from '$stores/databases';
  import { autoSnapshot, loadAutoSnapshot, nextSnapshotAt } from '$stores/autoSnapshot';
  import type { Service } from '$stores/services';
  import { onMount } from 'svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();

  onMount(loadAutoSnapshot);
  // The snapshots ride along with the engine's databases, so the tab reads the
  // same listing the Databases tab does rather than fetching its own.
  $effect(() => {
    void loadEngine(svc.name);
  });

  type Row = { snap: Snapshot; database: string; site: string; key: string; at: Date | null };

  let error = $state('');
  let busy = $state('');
  let siteFilter = $state('');
  let timeFilter = $state('');
  let page = $state(0);
  let showSettings = $state(false);
  let selected = $state(new Set<string>());
  // What a confirmation is standing in front of: a removal covering one or many
  // rows, or a restore, which overwrites the database it came from.
  let pending = $state<{ kind: 'remove' | 'restore'; rows: Row[] } | null>(null);

  const engine = $derived($databases.find((e) => e.service === svc.name));
  const covered = $derived(
    $autoSnapshot.sites.filter((s) => s.service === svc.name && s.covered).length
  );

  function snapDate(snap: Snapshot): Date | null {
    return parseSnapshotTimestamp(snap.name) ?? (snap.created ? new Date(snap.created) : null);
  }

  // One column, one spelling: the site that owns the database now wins, so a
  // snapshot that recorded a name and one that recorded nothing (or a domain,
  // from an older build) all read the same. Only a database no site claims any
  // more falls back to what the snapshot itself recorded.
  function siteOf(snap: Snapshot, database: string): string {
    const owner = $autoSnapshot.sites.find(
      (site) => site.service === svc.name && site.database === database
    );
    return owner?.site ?? snap.site ?? '';
  }

  const rows = $derived<Row[]>(
    (engine?.databases ?? [])
      .flatMap((db) =>
        (db.snapshots ?? []).map((snap) => ({
          snap,
          database: db.name,
          site: siteOf(snap, db.name),
          key: db.name + '/' + snap.name,
          at: snapDate(snap)
        }))
      )
      .sort((a, b) => (b.at?.getTime() ?? 0) - (a.at?.getTime() ?? 0))
  );

  const totalBytes = $derived(rows.reduce((sum, r) => sum + (r.snap.size_bytes ?? 0), 0));

  // The same count-and-unit split the settings dialog edits, rendered as text.
  // Scoped to this engine: the soonest run among the databases it holds.
  const nextRun = $derived(nextSnapshotAt($autoSnapshot, svc.name));
  const interval = $derived(intervalParts($autoSnapshot.every));
  const unitLabels: Record<string, [string, string]> = {
    hour: [m.snapshots_unitHour(), m.snapshots_unitHours()],
    day: [m.snapshots_unitDay(), m.snapshots_unitDays()],
    week: [m.snapshots_unitWeek(), m.snapshots_unitWeeks()],
    month: [m.snapshots_unitMonth(), m.snapshots_unitMonths()]
  };
  const intervalLabel = $derived(
    interval.amount + ' ' + unitLabels[interval.unit][interval.amount === 1 ? 0 : 1]
  );
  const keepForLabel = $derived.by(() => {
    if (!$autoSnapshot.keep_for) return m.snapshots_auto_keepForNever();
    const { amount, unit } = intervalParts($autoSnapshot.keep_for);
    return amount + ' ' + unitLabels[unit][amount === 1 ? 0 : 1];
  });

  const siteOptions = $derived([
    { value: '', label: m.snapshots_filterAllSites() },
    ...[...new Set(rows.map((r) => r.site).filter(Boolean))].sort().map((s) => ({ value: s, label: s }))
  ]);
  const timeOptions = [
    { value: '', label: m.snapshots_filterAnyTime() },
    { value: '1', label: m.snapshots_filterLast24h() },
    { value: '7', label: m.snapshots_filterLastDays({ count: 7 }) },
    { value: '30', label: m.snapshots_filterLastDays({ count: 30 }) },
    { value: 'older', label: m.snapshots_filterOlder({ count: 30 }) }
  ];

  function withinFilter(at: Date | null): boolean {
    if (!timeFilter) return true;
    if (!at) return false;
    const days = (Date.now() - at.getTime()) / 86_400_000;
    return timeFilter === 'older' ? days > 30 : days <= Number(timeFilter);
  }

  const filtered = $derived(
    rows.filter((r) => (!siteFilter || r.site === siteFilter) && withinFilter(r.at))
  );
  const selectedRows = $derived(filtered.filter((r) => selected.has(r.key)));

  // A long history is paged rather than scrolled: the engine keeps every
  // snapshot of every database it holds.
  const PAGE_SIZE = 20;
  const maxPage = $derived(Math.max(0, Math.ceil(filtered.length / PAGE_SIZE) - 1));
  // Clamped rather than reset, so deleting the last row of a page steps back
  // instead of showing an empty table.
  const current = $derived(Math.min(page, maxPage));
  const paged = $derived(filtered.slice(current * PAGE_SIZE, current * PAGE_SIZE + PAGE_SIZE));
  const pageSelected = $derived(paged.filter((r) => selected.has(r.key)));
  const allSelected = $derived(paged.length > 0 && pageSelected.length === paged.length);

  function toggleRow(key: string) {
    const next = new Set(selected);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    selected = next;
  }

  // Select-all covers the rows on screen, so it can never reach a row the
  // filters or the paging are hiding. What is already selected on another page
  // stays selected, which is what the removal count reflects.
  function toggleAll() {
    const next = new Set(selected);
    for (const row of paged) {
      if (allSelected) next.delete(row.key);
      else next.add(row.key);
    }
    selected = next;
  }

  function setSiteFilter(v: string) {
    siteFilter = v;
    page = 0;
  }

  function setTimeFilter(v: string) {
    timeFilter = v;
    page = 0;
  }

  function dateLabel(at: Date | null): string {
    return at ? at.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : '';
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

  async function toggleKeep(row: Row) {
    error = '';
    busy = row.key;
    const res = await keepSnapshot(svc.name, row.database, row.snap.name, !row.snap.kept);
    busy = '';
    if (!res.ok) {
      error = res.error || m.common_failed();
      return;
    }
    await loadEngine(svc.name);
  }

  async function removeConfirmed() {
    const targets = pending?.rows ?? [];
    pending = null;
    error = '';
    busy = 'remove';
    const failed: string[] = [];
    for (const row of targets) {
      const res = await deleteSnapshot(svc.name, row.database, row.snap.name);
      if (!res.ok) failed.push(snapshotBaseName(row.snap.name) + (res.error ? ': ' + res.error : ''));
    }
    busy = '';
    // Only the rows that survived stay selected, so a retry acts on what is left.
    const gone = new Set(targets.map((r) => r.key));
    selected = new Set([...selected].filter((k) => !gone.has(k)));
    if (failed.length > 0) error = failed.join(' · ');
    await loadEngine(svc.name);
  }

  async function restoreConfirmed() {
    const row = pending?.rows[0];
    pending = null;
    if (!row) return;
    error = '';
    busy = row.key;
    const res = await restoreSnapshot(svc.name, row.database, row.snap.name);
    busy = '';
    if (!res.ok) {
      error = res.error || m.common_failed();
      return;
    }
    if (res.errors) {
      error = m.databases_restoredWithErrors({
        name: snapshotBaseName(row.snap.name),
        count: res.errors
      });
      return;
    }
    await loadEngine(svc.name);
  }
</script>

<div class="p-3 sm:p-5 space-y-4 overflow-y-auto">
  <SettingsCard>
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <p class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.snapshots_auto_title()}</p>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {$autoSnapshot.enabled
            ? m.snapshots_auto_coveredCount({ count: covered })
            : m.common_disabled()}
        </p>
      </div>
      <DetailButton onclick={() => (showSettings = true)}>{m.snapshots_auto_settings()}</DetailButton>
    </div>

    <!-- A read-only glance: the schedule is changed in the settings dialog, so
         the tab states it rather than offering a second set of controls. -->
    <dl class="mt-3 grid grid-cols-2 sm:grid-cols-4 gap-x-4 gap-y-2 text-xs">
      <div>
        <dt class="text-gray-400 dark:text-gray-500">{m.snapshots_auto_every()}</dt>
        <dd class="text-gray-700 dark:text-gray-300">{intervalLabel}</dd>
      </div>
      <div>
        <dt class="text-gray-400 dark:text-gray-500">{m.snapshots_auto_keep()}</dt>
        <dd class="text-gray-700 dark:text-gray-300 tabular-nums">{$autoSnapshot.keep}</dd>
      </div>
      <div>
        <dt class="text-gray-400 dark:text-gray-500">{m.snapshots_auto_keepFor()}</dt>
        <dd class="text-gray-700 dark:text-gray-300">{keepForLabel}</dd>
      </div>
      <div>
        <dt class="text-gray-400 dark:text-gray-500">{m.snapshots_auto_selection()}</dt>
        <dd class="text-gray-700 dark:text-gray-300">
          {$autoSnapshot.selection === 'opt_in'
            ? m.snapshots_auto_selectionOptIn()
            : m.snapshots_auto_selectionOptOut()}
        </dd>
      </div>
    </dl>

    <!-- The facts run along one line on a wide card and wrap on a narrow one;
         the hint always takes its own row rather than trailing them. -->
    <div
      class="mt-3 pt-3 border-t border-gray-100 dark:border-lerd-border flex flex-wrap items-baseline gap-x-5 gap-y-1 text-xs text-gray-500 dark:text-gray-400"
    >
      {#if $autoSnapshot.enabled && covered > 0}
        <p>
          {m.snapshots_auto_nextRun()}:
          <span class="text-gray-700 dark:text-gray-300">
            {nextRun
              ? nextRun.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
              : m.snapshots_auto_nextCheck()}
          </span>
        </p>
      {/if}
      <p>{m.snapshots_totalUsage({ count: rows.length, size: formatBytes(totalBytes) })}</p>
      <p class="basis-full text-gray-400 dark:text-gray-500">{m.snapshots_auto_perDatabaseHint()}</p>
    </div>
  </SettingsCard>

  <SettingsCard>
    <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
      <p class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.snapshots_title()}</p>
      <div class="flex flex-wrap items-center gap-1.5">
        <Dropdown value={siteFilter} options={siteOptions} onchange={setSiteFilter} />
        <Dropdown value={timeFilter} options={timeOptions} onchange={setTimeFilter} />
        {#if selectedRows.length > 0}
          <DetailButton
            tone="danger"
            loading={busy === 'remove'}
            disabled={Boolean(busy)}
            onclick={() => (pending = { kind: 'remove', rows: selectedRows })}
          >
            {m.snapshots_removeSelected({ count: selectedRows.length })}
          </DetailButton>
        {/if}
      </div>
    </div>

    {#if error}
      <p class="text-xs text-red-500 mb-2">{error}</p>
    {/if}

    {#if rows.length === 0}
      <p class="text-sm text-gray-400 dark:text-gray-500">{m.databases_noSnapshots()}</p>
    {:else if filtered.length === 0}
      <p class="text-sm text-gray-400 dark:text-gray-500">{m.snapshots_noMatches()}</p>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead class="text-[11px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
            <tr class="border-b border-gray-100 dark:border-lerd-border">
              <th class="w-8 py-2 pr-2">
                <input
                  type="checkbox"
                  checked={allSelected}
                  indeterminate={pageSelected.length > 0 && !allSelected}
                  onchange={toggleAll}
                  aria-label={m.snapshots_selectAll()}
                  class="align-middle accent-lerd-red"
                />
              </th>
              <th class="py-2 pr-3 font-medium">{m.snapshots_colName()}</th>
              <th class="py-2 pr-3 font-medium">{m.snapshots_colSite()}</th>
              <th class="py-2 pr-3 font-medium">{m.snapshots_colDatabase()}</th>
              <th class="py-2 pr-3 font-medium">{m.snapshots_colTaken()}</th>
              <th class="py-2 pr-3 font-medium tabular-nums">{m.snapshots_colSize()}</th>
              <th class="py-2 pr-3 font-medium">{m.snapshots_colRetention()}</th>
              <th class="w-28 py-2"></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-lerd-border/60">
            {#each paged as row (row.key)}
              <tr class="text-gray-700 dark:text-gray-300">
                <td class="py-2 pr-2">
                  <input
                    type="checkbox"
                    checked={selected.has(row.key)}
                    onchange={() => toggleRow(row.key)}
                    aria-label={m.snapshots_selectOne({ name: snapshotBaseName(row.snap.name) })}
                    class="align-middle accent-lerd-red"
                  />
                </td>
                <td class="py-2 pr-3 max-w-48 truncate" title={row.snap.name}>
                  {snapshotBaseName(row.snap.name)}
                </td>
                <td class="py-2 pr-3 max-w-40 truncate text-gray-500 dark:text-gray-400" title={row.site}>
                  {row.site || '—'}
                </td>
                <td class="py-2 pr-3 max-w-40 truncate text-gray-500 dark:text-gray-400">{row.database}</td>
                <td class="py-2 pr-3 whitespace-nowrap text-gray-500 dark:text-gray-400">{dateLabel(row.at)}</td>
                <td class="py-2 pr-3 whitespace-nowrap tabular-nums text-gray-500 dark:text-gray-400">
                  {formatBytes(row.snap.size_bytes)}
                </td>
                <td class="py-2 pr-3 whitespace-nowrap text-gray-500 dark:text-gray-400">
                  {expiryLabel(row.snap) || '—'}
                </td>
                <td class="py-2">
                  <div class="flex items-center justify-end gap-1">
                    {#if row.snap.auto}
                      <button
                        type="button"
                        onclick={() => toggleKeep(row)}
                        disabled={Boolean(busy)}
                        use:tooltip={row.snap.kept
                          ? m.databases_snapshotRelease()
                          : m.databases_snapshotKeep()}
                        aria-label={row.snap.kept
                          ? m.databases_snapshotRelease()
                          : m.databases_snapshotKeep()}
                        aria-pressed={Boolean(row.snap.kept)}
                        class="flex items-center justify-center w-7 h-7 rounded-md transition-colors {row.snap
                          .kept
                          ? 'text-lerd-red'
                          : 'text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5'}"
                      >
                        <Icon name="bookmark" class="w-3.5 h-3.5" />
                      </button>
                    {/if}
                    <a
                      href={snapshotExportUrl(svc.name, row.database, row.snap.name)}
                      use:tooltip={m.databases_export()}
                      aria-label={m.databases_export()}
                      class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
                    >
                      <Icon name="download" class="w-3.5 h-3.5" />
                    </a>
                    <button
                      type="button"
                      onclick={() => (pending = { kind: 'restore', rows: [row] })}
                      disabled={Boolean(busy)}
                      use:tooltip={m.databases_restore()}
                      aria-label={m.databases_restore()}
                      class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
                    >
                      <Icon name="refresh" class="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      onclick={() => (pending = { kind: 'remove', rows: [row] })}
                      disabled={Boolean(busy)}
                      use:tooltip={m.databases_delete()}
                      aria-label={m.databases_delete()}
                      class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-red-500 hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
                    >
                      <Icon name="trash" class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      {#if filtered.length > PAGE_SIZE}
        <div class="flex items-center justify-between gap-2 pt-3 text-xs text-gray-500 dark:text-gray-400">
          <span class="tabular-nums">
            {m.snapshots_pageRange({
              from: current * PAGE_SIZE + 1,
              to: current * PAGE_SIZE + paged.length,
              total: filtered.length
            })}
          </span>
          <div class="flex items-center gap-1">
            <button
              type="button"
              onclick={() => (page = Math.max(0, current - 1))}
              disabled={current === 0}
              use:tooltip={m.snapshots_pagePrev()}
              aria-label={m.snapshots_pagePrev()}
              class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 enabled:hover:text-gray-700 dark:enabled:hover:text-gray-200 enabled:hover:bg-gray-100 dark:enabled:hover:bg-white/5 disabled:opacity-40 transition-colors"
            >
              <Icon name="back" class="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              onclick={() => (page = Math.min(maxPage, current + 1))}
              disabled={current === maxPage}
              use:tooltip={m.snapshots_pageNext()}
              aria-label={m.snapshots_pageNext()}
              class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 enabled:hover:text-gray-700 dark:enabled:hover:text-gray-200 enabled:hover:bg-gray-100 dark:enabled:hover:bg-white/5 disabled:opacity-40 transition-colors"
            >
              <Icon name="chevron" class="w-3.5 h-3.5 -rotate-90" />
            </button>
          </div>
        </div>
      {/if}
    {/if}
  </SettingsCard>
</div>

{#if pending}
  {@const restoring = pending.kind === 'restore'}
  <Modal
    open
    title={restoring ? m.databases_restore() : m.databases_delete()}
    onclose={() => (pending = null)}
    size="sm"
  >
    <div class="px-5 py-4 space-y-2">
      <p class="text-sm text-gray-700 dark:text-gray-300">
        {restoring
          ? m.snapshots_restoreConfirmBody({ database: pending.rows[0].database })
          : m.snapshots_removeConfirmBody({ count: pending.rows.length })}
      </p>
      <ul class="text-xs text-gray-500 dark:text-gray-400 space-y-0.5 max-h-40 overflow-y-auto">
        {#each pending.rows.slice(0, 8) as row (row.key)}
          <li class="truncate" title={row.snap.name}>{snapshotBaseName(row.snap.name)} · {row.database}</li>
        {/each}
        {#if pending.rows.length > 8}
          <li>…</li>
        {/if}
      </ul>
    </div>
    {#snippet footer()}
      <DetailButton onclick={() => (pending = null)}>{m.common_cancel()}</DetailButton>
      <DetailButton tone="danger" onclick={restoring ? restoreConfirmed : removeConfirmed}>
        {restoring ? m.databases_restoreConfirm() : m.databases_deleteConfirm()}
      </DetailButton>
    {/snippet}
  </Modal>
{/if}

{#if showSettings}
  <SnapshotSettingsModal onclose={() => (showSettings = false)} />
{/if}
