import { derived, readable, type Readable } from 'svelte/store';
import { getContext, setContext } from 'svelte';
import type { DumpEvent } from '$lib/dumpsStream';
import { groupKey, groupLabel, type GroupLabel } from '$lib/eventGroup';
import { kindHaystack } from '$lib/eventSearch';
import { routeOf } from '$lib/route';
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
// facet is the one value a kind narrows on: a job's status, a log's level.
export function buildKindGroups(
  events: DumpEvent[],
  kind: string,
  site = '',
  text = '',
  hideSitePrefix = false,
  worker = '',
  showWorkers = true,
  facet = ''
): DebugGroup[] {
  const needle = text ? text.toLowerCase() : '';
  const groups = new Map<string, DebugGroup>();
  for (const ev of events) {
    if (ev.kind !== kind) continue;
    if (site && ev.ctx.site !== site) continue;
    // "Show worker queries" off hides worker-emitted events from the view,
    // not just future capture, matching buildQueryGroups. Jobs are exempt: a
    // worker's jobs are captured whatever that toggle says, because they are
    // the only feedback a queue being drained gives.
    if (!showWorkers && kind !== 'job' && ev.ctx.worker) continue;
    if (worker && ev.ctx.worker !== worker) continue;
    if (facet && facetOf(ev) !== facet) continue;
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

// facetOf is the value a kind is narrowed by. A job carries a status and a log
// a level; no kind carries both, so one accessor serves the filter. A browser
// event narrows by its type, with console messages split by level.
export function facetOf(ev: DumpEvent): string {
  const d = ev.data as { status?: string; level?: string; type?: string } | undefined;
  if (ev.kind === 'browser') return d?.type === 'console' ? `console.${d.level}` : (d?.type ?? '');
  return d?.status ?? d?.level ?? '';
}

// countKinds tallies buffered events per wire-kind (optionally scoped to a
// site), for the per-tab item counters.
// isPageView marks a browser page load or SPA navigation. It groups the
// events of one page and tells MCP a page loaded, but the lens header already
// names the page, so it is not a row or a count of its own.
export function isPageView(ev: DumpEvent): boolean {
  return ev.kind === 'browser' && (ev.data as { type?: string } | undefined)?.type === 'navigation';
}

export function countKinds(events: DumpEvent[], site = ''): Record<string, number> {
  const c: Record<string, number> = {};
  for (const ev of events) {
    if (site && ev.ctx.site !== site) continue;
    if (isPageView(ev)) continue;
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

// ofRequest reports whether an event belongs to one request: it ran in it, or
// it is a browser event naming it as the request a fetch reached. It mirrors
// dumps.Event.OfRequest on the Go side.
export function ofRequest(ev: DumpEvent, rid: string): boolean {
  return ev.ctx.rid === rid || (ev.kind === 'browser' && (ev.data as { rid?: string } | undefined)?.rid === rid);
}

// requestEvents is one request's events: what the server returned for it, then
// whatever the stream brought since that the server's answer did not hold.
export function requestEvents(events: DumpEvent[], rid: string, fetched: DumpEvent[], showTests: boolean): DumpEvent[] {
  const live = events.filter((ev) => ofRequest(ev, rid));
  if (fetched.length === 0) return live;
  const seen = new Set(fetched.map((ev) => ev.id));
  return [...fetched.filter((ev) => showTests || !ev.ctx.test), ...live.filter((ev) => !seen.has(ev.id))];
}

const SCOPE = Symbol('lensEvents');

// scopeLensEvents narrows every lens rendered below the calling component to
// the request rid names, or leaves them on every event while it is empty.
// fetched is what the server's ring holds for the request, which reaches back
// further than the stream a tab opened on.
export function scopeLensEvents(rid: Readable<string>, fetched: Readable<DumpEvent[]> = readable([]), route: Readable<string> = readable('')): Readable<DumpEvent[]> {
  const scoped = derived([debugEvents, rid, fetched, showTests, route], ([$events, $rid, $fetched, $showTests, $route]) =>
    $rid ? requestEvents($events, $rid, $fetched, $showTests) : lensRouteFilter($events, $route)
  );
  setContext(SCOPE, scoped);
  return scoped;
}

// lensRouteFilter keeps the events of one route, matched whole rather than as
// text, since "GET /" is a prefix of every other GET.
export function lensRouteFilter(events: DumpEvent[], route: string): DumpEvent[] {
  return route ? events.filter((ev) => routeOf(ev) === route) : events;
}

const PICK = Symbol('pickRequest');

// providePickRequest lets a request id shown in a lens below the caller narrow
// the view to that request; pickRequest is undefined where nothing can.
export function providePickRequest(pick: (rid: string) => void): void {
  setContext(PICK, pick);
}

export function pickRequest(): ((rid: string) => void) | undefined {
  return getContext<((rid: string) => void) | undefined>(PICK);
}

// lensEvents is the stream a lens renders: its scope's, or every event.
export function lensEvents(): Readable<DumpEvent[]> {
  return getContext<Readable<DumpEvent[]> | undefined>(SCOPE) ?? debugEvents;
}
