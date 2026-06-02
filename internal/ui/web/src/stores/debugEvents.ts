import { derived, type Readable } from 'svelte/store';
import type { DumpEvent } from '$lib/dumpsStream';
import { dumps } from '$stores/dumps';

// Generic per-request grouping shared by the non-dump/non-query Debug lenses
// (jobs, views, mail, cache, events). Mirrors the query grouping: prefer the
// per-request id, then method+path+pid, then a 5s CLI bucket.
export interface DebugGroup {
  key: string;
  label: string;
  ts: string;
  events: DumpEvent[];
  worker: string;
}

function groupKey(ev: DumpEvent): string {
  if (ev.ctx.rid) return `rid:${ev.ctx.rid}`;
  if (ev.ctx.type === 'fpm') {
    return `fpm:${ev.ctx.site ?? ''}:${ev.ctx.request ?? ''}:${ev.ctx.pid ?? ''}`;
  }
  const bucket = Math.floor(new Date(ev.ts).getTime() / 5000);
  return `cli:${ev.ctx.site ?? ''}:${ev.ctx.pid ?? ''}:${bucket}`;
}

function groupLabel(ev: DumpEvent, hideSitePrefix: boolean): string {
  const prefix = !hideSitePrefix && ev.ctx.site ? `[${ev.ctx.site}] ` : '';
  if (ev.ctx.worker) return prefix + ev.ctx.worker;
  if (ev.ctx.type === 'fpm') return prefix + (ev.ctx.request || '(request)');
  return `${prefix}cli (pid ${ev.ctx.pid ?? '?'})`;
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
    if (needle) {
      const hay = (JSON.stringify(ev.data ?? {}) + ' ' + (ev.ctx.worker ?? '')).toLowerCase();
      if (!hay.includes(needle)) continue;
    }
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
