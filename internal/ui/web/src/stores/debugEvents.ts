import { derived, type Readable } from 'svelte/store';
import type { DumpEvent } from '$lib/dumpsStream';
import { groupKey, groupLabel, type GroupLabel } from '$lib/eventGroup';
import { kindHaystack } from '$lib/eventSearch';
import { dumps } from '$stores/dumps';
import { showTests } from '$stores/debugLens';

// Generic per-request grouping shared by the non-dump/non-query Debug lenses
// (jobs, views, mail, cache, events). Mirrors the query grouping: prefer the
// per-request id, then method+path+pid, then a 5s CLI bucket.
export interface DebugGroup {
  key: string;
  label: GroupLabel;
  ts: string;
  events: DumpEvent[];
  worker: string;
}

// buildKindGroups filters the shared event stream to one kind and groups it by
// request, newest-first. Search matches the event's data payload and worker.
export function buildKindGroups(
  events: DumpEvent[],
  kind: string,
  site = '',
  text = '',
  hideSitePrefix = false,
  worker = '',
  showWorkers = true
): DebugGroup[] {
  const needle = text ? text.toLowerCase() : '';
  const groups = new Map<string, DebugGroup>();
  for (const ev of events) {
    if (ev.kind !== kind) continue;
    if (site && ev.ctx.site !== site) continue;
    // "Show worker queries" off hides worker-emitted events from the view,
    // not just future capture, matching buildQueryGroups.
    if (!showWorkers && ev.ctx.worker) continue;
    if (worker && ev.ctx.worker !== worker) continue;
    if (needle && !kindHaystack(ev).includes(needle)) continue;
    const key = groupKey(ev);
    let g = groups.get(key);
    if (!g) {
      g = { key, label: groupLabel(ev, hideSitePrefix), ts: ev.ts, events: [], worker: ev.ctx.worker ?? '' };
      groups.set(key, g);
    }
    g.events.push(ev);
    if (ev.ts > g.ts) g.ts = ev.ts;
  }
  const out = Array.from(groups.values()).sort((a, b) => b.ts.localeCompare(a.ts));
  for (const g of out) g.events.reverse();
  return out;
}

// countKinds tallies buffered events per wire-kind (optionally scoped to a
// site), for the per-tab item counters.
export function countKinds(events: DumpEvent[], site = ''): Record<string, number> {
  const c: Record<string, number> = {};
  for (const ev of events) {
    if (site && ev.ctx.site !== site) continue;
    c[ev.kind] = (c[ev.kind] ?? 0) + 1;
  }
  return c;
}

// Sites seen across all captured events (any kind), for the shared site filter.
export const knownDebugSites: Readable<string[]> = derived(dumps, ($dumps) => {
  const set = new Set<string>();
  for (const ev of $dumps) set.add(ev.ctx.site || '');
  return Array.from(set).sort();
});

// debugEvents is what every lens renders: the captured stream minus test-run
// events unless the user asks for them. Filtering here rather than in each
// build* keeps the tab counters and the lists agreeing on what is visible.
export const debugEvents: Readable<DumpEvent[]> = derived(
  [dumps, showTests],
  ([$dumps, $showTests]) => ($showTests ? $dumps : $dumps.filter((ev) => !ev.ctx.test))
);

// hiddenTestCount drives the "N hidden" hint next to the toggle, so a dump
// added inside a test that never appears has a visible explanation.
export const hiddenTestCount: Readable<number> = derived([dumps, showTests], ([$dumps, $showTests]) =>
  $showTests ? 0 : $dumps.reduce((n, ev) => (ev.ctx.test ? n + 1 : n), 0)
);
