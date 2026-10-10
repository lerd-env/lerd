import { vi } from 'vitest';
import type { DumpEvent } from '$lib/dumpEvent';

// A test double for lerd-ui's Debug lens endpoints and its nudge stream, for
// component tests. It groups by request id, filters and pages the way the Go
// side does closely enough to exercise a lens; the real grouping, search and
// N+1 rules are tested in internal/dumps.

export interface FakeLensApi {
  events: DumpEvent[];
  requests: string[];
  // enabled is what /api/dumps/status reports for the debug bridge.
  enabled: boolean;
  // nudge tells every open lens an event of kind arrived for site.
  nudge(kind: string, site: string): void;
  restore(): void;
}

function facetOf(ev: DumpEvent): string {
  const d = (ev.data ?? {}) as { status?: string; level?: string; type?: string; channel?: string };
  if (ev.kind === 'message') return d.channel ?? '';
  if (ev.kind === 'browser') return d.type === 'console' ? `console.${d.level}` : (d.type ?? '');
  return d.status ?? d.level ?? '';
}

const isNav = (ev: DumpEvent) => ev.kind === 'browser' && (ev.data as { type?: string } | undefined)?.type === 'navigation';
const reached = (ev: DumpEvent) => (ev.kind === 'browser' ? ((ev.data as { rid?: string } | undefined)?.rid ?? '') : '');
const groupKey = (ev: DumpEvent) => (ev.ctx.rid && ev.kind !== 'dump' ? `rid:${ev.ctx.rid}` : `req:${ev.ctx.site}:${ev.ctx.request}:${ev.ctx.pid ?? ''}`);

// fakeLensAnswer is what the fake answers for one request path, for a test
// that mocks $lib/api itself rather than fetch.
export function fakeLensAnswer(events: DumpEvent[], url: string, enabled = true): unknown {
  const u = new URL(url, 'http://lerd.test');
  function select(p: URLSearchParams): DumpEvent[] {
    return events.filter((ev, i) => {
      const before = Number(p.get('before') || 0);
      if (before && i + 1 >= before) return false;
      if (p.get('kind') && ev.kind !== p.get('kind')) return false;
      if (p.get('site') && ev.ctx.site !== p.get('site')) return false;
      if (p.get('ctx') && ev.ctx.type !== p.get('ctx')) return false;
      if (p.get('rid') && ev.ctx.rid !== p.get('rid') && reached(ev) !== p.get('rid')) return false;
      if (p.get('facet') && facetOf(ev) !== p.get('facet')) return false;
      if (p.get('tests') !== '1' && ev.ctx.test) return false;
      const q = (p.get('q') ?? '').toLowerCase();
      if (q && !JSON.stringify(ev).toLowerCase().includes(q)) return false;
      return true;
    });
  }

  function groups(p: URLSearchParams) {
    const limit = Number(p.get('limit') || 30);
    const matched = select(p).filter((ev) => !isNav(ev));
    const byKey = new Map<string, { key: string; last: number; rows: DumpEvent[] }>();
    for (const ev of matched) {
      const key = groupKey(ev);
      const g = byKey.get(key) ?? { key, last: 0, rows: [] };
      g.rows.unshift(ev);
      g.last = events.indexOf(ev) + 1;
      byKey.set(key, g);
    }
    const all = [...byKey.values()].sort((a, b) => b.last - a.last);
    const page = all.slice(0, limit);
    return {
      groups: page.map((g) => ({
        key: g.key,
        count: g.rows.length,
        total_ms: 0,
        slow_count: 0,
        n_plus_one: false,
        rows: g.rows.slice(0, 100).map((event) => ({ event, dup: 1, seq: events.indexOf(event) + 1 }))
      })),
      next: all.length > limit ? page[page.length - 1].last : 0,
      total: matched.length
    };
  }

  function answer(path: string, p: URLSearchParams): unknown {
    if (path === '/api/dumps/groups') return groups(p);
    if (path === '/api/dumps/groups/rows') {
      const all = new URLSearchParams({ ...Object.fromEntries(p), limit: '1000' });
      all.delete('before');
      const g = groups(all).groups.find((x) => x.key === p.get('key'));
      const before = Number(p.get('before') || 0);
      return (g?.rows ?? []).filter((r) => !before || r.seq < before);
    }
    if (path === '/api/dumps/counts') {
      const counts: Record<string, number> = {};
      for (const ev of select(p)) if (!isNav(ev)) counts[ev.kind] = (counts[ev.kind] ?? 0) + 1;
      return { counts, hidden_tests: 0 };
    }
    if (path === '/api/dumps/facets') {
      const scoped = select(new URLSearchParams({ ...Object.fromEntries(p), kind: '' }));
      return {
        sites: [...new Set(events.map((e) => e.ctx.site ?? ''))].sort(),
        workers: [...new Set(scoped.map((e) => e.ctx.worker ?? '').filter(Boolean))].sort(),
        values: [...new Set(select(p).map(facetOf).filter(Boolean))].sort()
      };
    }
    if (path === '/api/dumps/event') return events.find((e) => e.id === p.get('id')) ?? {};
    if (path === '/api/dumps/status') return { enabled, passthrough: false, listening: true, addr: '', count: events.length, subscribers: 0, last_ts: '' };
    return { enabled };
  }
  return answer(u.pathname, u.searchParams);
}

export function installFakeLensApi(events: DumpEvent[] = []): FakeLensApi {
  const api: FakeLensApi = { events, requests: [], enabled: true, nudge: () => {}, restore: () => {} };
  const sources = new Set<{ url: string; fire: (data: string) => void }>();

  const realFetch = globalThis.fetch;
  const realES = globalThis.EventSource;
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), 'http://lerd.test');
    api.requests.push(url.pathname + url.search);
    return new Response(JSON.stringify(fakeLensAnswer(api.events, url.pathname + url.search, api.enabled)), { status: 200 });
  }) as unknown as typeof fetch;

  class FakeSource {
    listeners: Array<(e: { data: string }) => void> = [];
    entry: { url: string; fire: (data: string) => void };
    constructor(url: string) {
      this.entry = { url, fire: (data) => this.listeners.forEach((l) => l({ data })) };
      sources.add(this.entry);
    }
    addEventListener(type: string, fn: (e: { data: string }) => void) {
      if (type === 'message') this.listeners.push(fn);
    }
    close() {
      sources.delete(this.entry);
    }
  }
  // @ts-expect-error test double
  globalThis.EventSource = FakeSource;

  api.nudge = (kind, site) => {
    for (const s of sources) s.fire(JSON.stringify({ kind, site }));
  };
  api.restore = () => {
    globalThis.fetch = realFetch;
    globalThis.EventSource = realES;
  };
  return api;
}
