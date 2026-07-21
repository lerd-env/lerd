import { derived, writable, type Readable } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';
import type { DumpEvent, QueryData } from '$lib/dumpsStream';
import { groupKey, groupLabel, type GroupLabel } from '$lib/eventGroup';
import { dumps, toggleDumps, status as dumpsStatus, type DumpsStatus } from '$stores/dumps';
import { wsMessage } from '$lib/ws';

// Queries reuse the dumps receiver/stream: the lerd_devtools extension ships
// kind === 'query' events to the same socket, so they arrive in the shared
// `dumps` store. This module derives a query-only, request-grouped view with
// N+1 detection, slow tagging, and per-request rollups computed client-side.

// SLOW_MS tags any single query at or above this duration, matching
// Telescope's default slow-query threshold.
export const SLOW_MS = 100;

// A SQL fingerprint repeated at least DUPLICATE_AT times in one request marks
// those rows as duplicates; NPLUSONE_AT repeats escalate the whole request to
// an N+1 warning (the classic "same query in a loop" storm).
const DUPLICATE_AT = 2;
const NPLUSONE_AT = 3;

export interface DevtoolsStatus {
  enabled: boolean;
  workers: boolean;
}

export const devtoolsStatus = writable<DevtoolsStatus | null>(null);

export interface QueryRow {
  event: DumpEvent;
  data: QueryData;
  duplicate: boolean;
  dupCount: number;
  slow: boolean;
}

export interface QueryGroup {
  key: string;
  label: GroupLabel;
  ts: string;
  rows: QueryRow[];
  count: number;
  totalMs: number;
  slowCount: number;
  nPlusOne: boolean;
  // worker is the queue/scheduler command name when this group came from a
  // worker process; empty for web/CLI requests.
  worker: string;
}

// normalizeSql collapses literal values so structurally-identical queries
// share a fingerprint: quoted strings and numbers become `?`, whitespace
// collapses, case folds. Prepared statements already parameterize, so this
// mostly catches inlined PDO::query/exec literals.
export function normalizeSql(sql: string): string {
  return sql
    .replace(/'(?:[^'\\]|\\.)*'/g, '?')
    .replace(/"(?:[^"\\]|\\.)*"/g, '?')
    .replace(/\b\d+\b/g, '?')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase();
}

function queryData(ev: DumpEvent): QueryData | null {
  const d = ev.data as QueryData | undefined;
  if (!d || typeof d !== 'object' || typeof d.sql !== 'string') return null;
  return d;
}

// buildQueryGroups is the pure derivation, exported for unit tests and so a
// site-scoped view can pass a pre-filtered slice without touching globals.
export function buildQueryGroups(events: DumpEvent[], site = '', text = '', hideSitePrefix = false, worker = '', showWorkers = true): QueryGroup[] {
  const needle = text ? text.toLowerCase() : '';
  const groups = new Map<string, QueryGroup>();

  for (const ev of events) {
    if (ev.kind !== 'query') continue;
    if (site && ev.ctx.site !== site) continue;
    // "Show worker queries" off hides everything a queue/scheduler process
    // emitted from the view, not just future capture, so unchecking it
    // clears worker queries already buffered in the stream.
    if (!showWorkers && ev.ctx.worker) continue;
    if (worker && ev.ctx.worker !== worker) continue;
    const data = queryData(ev);
    if (!data) continue;
    if (
      needle &&
      !(
        data.sql.toLowerCase().includes(needle) ||
        (ev.ctx.request ?? '').toLowerCase().includes(needle) ||
        (ev.src.file ?? '').toLowerCase().includes(needle) ||
        (ev.ctx.worker ?? '').toLowerCase().includes(needle) ||
        (ev.ctx.branch ?? '').toLowerCase().includes(needle)
      )
    ) {
      continue;
    }
    const key = groupKey(ev);
    let g = groups.get(key);
    if (!g) {
      g = { key, label: groupLabel(ev, hideSitePrefix), ts: ev.ts, rows: [], count: 0, totalMs: 0, slowCount: 0, nPlusOne: false, worker: ev.ctx.worker ?? '' };
      groups.set(key, g);
    }
    const slow = data.time_ms >= SLOW_MS;
    g.rows.push({ event: ev, data, duplicate: false, dupCount: 1, slow });
    g.count++;
    g.totalMs += data.time_ms || 0;
    if (slow) g.slowCount++;
    if (ev.ts > g.ts) g.ts = ev.ts;
  }

  // Second pass per group: fingerprint counts drive duplicate/N+1 flags.
  for (const g of groups.values()) {
    const counts = new Map<string, number>();
    for (const row of g.rows) {
      const fp = normalizeSql(row.data.sql);
      counts.set(fp, (counts.get(fp) ?? 0) + 1);
    }
    let maxDup = 1;
    for (const row of g.rows) {
      const c = counts.get(normalizeSql(row.data.sql)) ?? 1;
      row.dupCount = c;
      row.duplicate = c >= DUPLICATE_AT;
      if (c > maxDup) maxDup = c;
    }
    g.nPlusOne = maxDup >= NPLUSONE_AT;
    // Newest query at the top of each request card.
    g.rows.reverse();
  }

  return Array.from(groups.values()).sort((a, b) => b.ts.localeCompare(a.ts));
}

export const queryFilterText = writable<string>('');
export const queryFilterSite = writable<string>('');
export const queryFilterWorker = writable<string>('');

// Worker command names seen in buffered query events, for the command filter
// dropdown shown when worker capture is enabled.
export const knownWorkerCommands: Readable<string[]> = derived(dumps, ($dumps) => {
  const set = new Set<string>();
  for (const ev of $dumps) {
    if (ev.kind === 'query' && ev.ctx.worker) set.add(ev.ctx.worker);
  }
  return Array.from(set).sort();
});

// Sites seen in buffered query events, for the global view's site dropdown.
// Empty-site events (CLI workers) collapse to '' and render as "(unknown)".
export const knownQuerySites: Readable<string[]> = derived(dumps, ($dumps) => {
  const set = new Set<string>();
  for (const ev of $dumps) {
    if (ev.kind === 'query') set.add(ev.ctx.site || '');
  }
  return Array.from(set).sort();
});

export const queryGroups: Readable<QueryGroup[]> = derived(
  [dumps, queryFilterSite, queryFilterText, queryFilterWorker, devtoolsStatus],
  ([$dumps, $site, $text, $worker, $status]) =>
    buildQueryGroups($dumps, $site, $text, false, $worker, Boolean($status?.workers))
);

export async function refreshDevtoolsStatus(): Promise<void> {
  try {
    const data = await apiJson<DevtoolsStatus>('/api/devtools/status');
    devtoolsStatus.set(data);
  } catch {
    devtoolsStatus.set(null);
  }
}

export async function toggleDevtoolsWorkers(enable: boolean): Promise<void> {
  await apiFetch('/api/devtools/workers', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enable })
  });
  void refreshDevtoolsStatus();
}

// debugCaptureEnabled is the whole Debug window's one switch. The debug bridge
// and the devtools collector share a single sentinel, so one flag (the dumps
// enable state) arms both; the sub-tabs gate on this.
export const debugCaptureEnabled: Readable<boolean> = derived(
  dumpsStatus,
  ($d) => Boolean(($d as DumpsStatus | null)?.enabled)
);

// setDebugCapture flips the shared capture flag, arming the entire Debug window
// (dumps + the devtools collector) in one call. refreshDevtoolsStatus keeps the
// worker-capture sub-toggle's state in sync.
export async function setDebugCapture(on: boolean): Promise<void> {
  await toggleDumps(on);
  await refreshDevtoolsStatus();
}

wsMessage.subscribe((msg) => {
  const fresh = msg?.devtools_status as DevtoolsStatus | undefined;
  if (fresh) devtoolsStatus.set(fresh);
});
