import { writable, get, type Readable } from 'svelte/store';
import { apiJson, apiUrl } from '$lib/api';
import type { DumpEvent } from '$lib/dumpEvent';

// The Debug lenses read their rows from lerd-ui, which groups, searches and
// pages the debug buffer in SQLite. A lens holds one page, and the stream only
// nudges it to read again, so a tab never carries events it does not show.

export interface LensScope {
  site?: string;
  branch?: string;
  // rid pins the lens to one request, as a request's inspector does.
  rid?: string;
  // ctx narrows to web ("fpm") or console ("cli") events.
  ctx?: string;
  // tests shows events captured inside a test run.
  tests?: boolean;
  // route keeps one route, read from a search shaped like "GET /path".
  route?: string;
}

export interface LensQuery extends LensScope {
  kind: string;
  q?: string;
  worker?: string;
  // workers false leaves out what worker processes emitted, jobs aside.
  workers?: boolean;
  facet?: string;
}

export interface LensRow {
  event: DumpEvent;
  // dup is how often the row's query shape ran in its request.
  dup: number;
}

export interface LensGroup {
  key: string;
  count: number;
  total_ms: number;
  slow_count: number;
  n_plus_one: boolean;
  rows: LensRow[];
}

interface GroupPage {
  groups: LensGroup[];
  next: number;
  total: number;
}

// queryString renders a lens query the way the endpoints read it.
export function queryString(q: Record<string, string | boolean | number | undefined>): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) {
    if (k === 'tests') {
      if (v) p.set('tests', '1');
    } else if (k === 'workers') {
      if (v === false) p.set('workers', '0');
    } else if (v !== undefined && v !== '' && v !== false) {
      p.set(k, String(v));
    }
  }
  return p.toString();
}

// Nudges come from one notify stream per site and branch, shared by every
// lens and counter open on them.
export interface Nudge {
  kind: string;
  site: string;
  branch?: string;
  test?: boolean;
}

const streams = new Map<string, { source: EventSource; subs: Set<(n: Nudge) => void> }>();

export function onNudge(scope: LensScope, fn: (n: Nudge) => void): () => void {
  const key = queryString({ site: scope.site, branch: scope.branch });
  let entry = streams.get(key);
  if (!entry) {
    const subs = new Set<(n: Nudge) => void>();
    const source = new EventSource(apiUrl(`/api/dumps/stream?${queryString({ notify: '1', site: scope.site, branch: scope.branch })}`));
    source.addEventListener('message', (e) => {
      try {
        const n = JSON.parse((e as MessageEvent).data) as Nudge;
        for (const s of subs) s(n);
      } catch {
        // A proxy could rewrite the stream; a line we cannot read is skipped.
      }
    });
    entry = { source, subs };
    streams.set(key, entry);
  }
  entry.subs.add(fn);
  return () => {
    const e = streams.get(key);
    if (!e) return;
    e.subs.delete(fn);
    if (e.subs.size === 0) {
      e.source.close();
      streams.delete(key);
    }
  };
}

// matches reports whether a nudge concerns what a lens or counter shows.
export function matches(n: Nudge, scope: LensScope, kind = ''): boolean {
  if (kind && n.kind !== kind) return false;
  if (scope.site && n.site !== scope.site) return false;
  if (scope.branch && n.branch !== scope.branch) return false;
  return Boolean(scope.tests) || !n.test;
}

// throttle runs fn at most once per wait, on the trailing edge, so a burst of
// events costs one read.
export function throttle(fn: () => void, wait: number): { call: () => void; cancel: () => void } {
  let timer: ReturnType<typeof setTimeout> | null = null;
  return {
    call() {
      if (timer) return;
      timer = setTimeout(() => {
        timer = null;
        fn();
      }, wait);
    },
    cancel() {
      if (timer) clearTimeout(timer);
      timer = null;
    }
  };
}

// LIVE_MS spaces a lens's rereads while events pour in: about two a second.
export const LIVE_MS = 500;

export interface Lens {
  groups: Readable<LensGroup[]>;
  loading: Readable<boolean>;
  hasMore: Readable<boolean>;
  // total is every event the lens matches, across all its pages.
  total: Readable<number>;
  // fresh counts events that arrived while the list was scrolled away from
  // the top, for the "N new" pill.
  fresh: Readable<number>;
  set(q: LensQuery | null): void;
  loadMore(): Promise<void>;
  loadRows(key: string): Promise<void>;
  setAtTop(atTop: boolean): void;
  refresh(): Promise<void>;
  destroy(): void;
}

export function createLens(): Lens {
  const groups = writable<LensGroup[]>([]);
  const loading = writable(false);
  const hasMore = writable(false);
  const total = writable(0);
  const fresh = writable(0);
  let query: LensQuery | null = null;
  let next = 0;
  // gen drops an answer that a newer query or refresh has overtaken.
  let gen = 0;
  let atTop = true;
  let unsub: (() => void) | null = null;
  // reading names the groups whose next rows are on their way, so a footer
  // that fires twice does not append them twice.
  const reading = new Set<string>();
  const live = throttle(() => void refresh(), LIVE_MS);

  async function read(before: number): Promise<GroupPage | null> {
    if (!query) return null;
    const mine = gen;
    const page = await apiJson<GroupPage>(`/api/dumps/groups?${queryString({ ...query, before: before || undefined })}`);
    return mine === gen ? page : null;
  }

  async function refresh(): Promise<void> {
    gen++;
    fresh.set(0);
    if (!query) {
      groups.set([]);
      hasMore.set(false);
      total.set(0);
      return;
    }
    loading.set(true);
    try {
      const page = await read(0);
      if (!page) return;
      groups.set(page.groups);
      next = page.next;
      hasMore.set(page.next > 0);
      total.set(page.total);
    } finally {
      loading.set(false);
    }
  }

  function listen() {
    unsub?.();
    unsub = null;
    // One request's view is a record of something finished; it does not move.
    if (!query || query.rid) return;
    const q = query;
    unsub = onNudge(q, (n) => {
      if (!matches(n, q, q.kind)) return;
      if (atTop) live.call();
      else fresh.update((c) => c + 1);
    });
  }

  return {
    groups,
    loading,
    hasMore,
    total,
    fresh,
    set(q) {
      const same = JSON.stringify(q) === JSON.stringify(query);
      query = q;
      if (same) return;
      live.cancel();
      listen();
      void refresh();
    },
    async loadMore() {
      if (!next || get(loading)) return;
      loading.set(true);
      try {
        const page = await read(next);
        if (!page) return;
        groups.update((gs) => [...gs, ...page.groups]);
        next = page.next;
        hasMore.set(page.next > 0);
      } finally {
        loading.set(false);
      }
    },
    async loadRows(key) {
      if (!query) return;
      const g = get(groups).find((x) => x.key === key);
      if (!g || g.rows.length >= g.count || reading.has(key)) return;
      const mine = gen;
      reading.add(key);
      try {
        const rows = await apiJson<LensRow[]>(`/api/dumps/groups/rows?${queryString({ ...query, key, offset: g.rows.length })}`);
        if (mine !== gen) return;
        groups.update((gs) => gs.map((x) => (x.key === key ? { ...x, rows: [...x.rows, ...rows] } : x)));
      } finally {
        reading.delete(key);
      }
    },
    setAtTop(v) {
      atTop = v;
      if (v && get(fresh) > 0) void refresh();
    },
    refresh,
    destroy() {
      live.cancel();
      unsub?.();
      unsub = null;
      query = null;
      gen++;
    }
  };
}

export interface Counts {
  counts: Readable<Record<string, number>>;
  hiddenTests: Readable<number>;
  set(scope: LensScope | null): void;
  destroy(): void;
}

// createCounts keeps the lens bar's badges, read from lerd-ui and reread as
// events arrive for the scope.
export function createCounts(): Counts {
  const counts = writable<Record<string, number>>({});
  const hiddenTests = writable(0);
  let scope: LensScope | null = null;
  let unsub: (() => void) | null = null;
  let gen = 0;
  async function refresh() {
    const mine = ++gen;
    if (!scope) {
      counts.set({});
      hiddenTests.set(0);
      return;
    }
    const r = await apiJson<{ counts: Record<string, number>; hidden_tests: number }>(`/api/dumps/counts?${queryString({ ...scope })}`);
    if (mine !== gen) return;
    counts.set(r.counts ?? {});
    hiddenTests.set(r.hidden_tests ?? 0);
  }
  const live = throttle(() => void refresh(), LIVE_MS);
  return {
    counts,
    hiddenTests,
    set(s) {
      if (JSON.stringify(s) === JSON.stringify(scope)) return;
      scope = s;
      live.cancel();
      unsub?.();
      unsub = null;
      void refresh().catch(() => {});
      if (s && !s.rid) {
        const sc = s;
        // Hidden test runs count too, so the hint next to the toggle moves.
        unsub = onNudge(sc, (n) => {
          if (matches(n, { ...sc, tests: true })) live.call();
        });
      }
    },
    destroy() {
      live.cancel();
      unsub?.();
      unsub = null;
      scope = null;
      gen++;
    }
  };
}

export interface FacetLists {
  sites: string[];
  workers: string[];
  values: string[];
}

// fetchFacets reads what a lens can filter by: sites, worker commands and,
// for a kind, its facet values.
export function fetchFacets(scope: LensScope & { kind?: string }): Promise<FacetLists> {
  return apiJson<FacetLists>(`/api/dumps/facets?${queryString({ ...scope })}`);
}

// fetchEvent reads one whole event. A lens lists rows without their call stack
// and a mail's HTML, and reads the event when a row is opened.
export function fetchEvent(id: string): Promise<DumpEvent> {
  return apiJson<DumpEvent>(`/api/dumps/event?${queryString({ id })}`);
}
