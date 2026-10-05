import type { RequestDetail } from '$stores/requests';
import { bytes } from '$lib/requestWaterfall';

// What the bar shows for one request, each chip the RequestDetail tab it opens.
// A count of nothing gets no chip, so a quiet request makes a short bar.

export interface BarSummary {
  method: string;
  uri: string;
  status: number;
  failed: boolean;
  timeMs: number;
  // phases is how the time split: before PHP, PHP itself, and the database.
  phases: { server: number; app: number; db: number };
  memory: string;
  queries: number;
  nPlusOne: boolean;
  views: number;
  cache: number;
  logs: number;
  appTabs: { id: string; title: string }[];
  user: string;
  children: { rid: string; url: string; status: number; ms: number; at: string }[];
  childFailures: number;
}

type Ev = { data?: unknown };
const data = (e: Ev | undefined) => (e?.data ?? {}) as Record<string, any>;

export function summarize(d: RequestDetail): BarSummary {
  const ev = (kind: string): Ev[] => (d.events?.[kind] as Ev[] | undefined) ?? [];
  const req = data(ev('request')[0]);
  const timeMs = Number(d.time_ms ?? req.time_ms ?? 0);
  const server = Number(d.nginx_ms ?? 0) + Number(d.queue_ms ?? 0);
  const db = Number(d.queries?.total_time_ms ?? 0);
  const total = Math.max(timeMs + server, 0.001);
  const auth = data(ev('auth')[0]);
  const tabs = new Map<string, string>();
  for (const e of ev('tab')) {
    const t = data(e);
    if (t.id !== undefined && !tabs.has(String(t.id))) tabs.set(String(t.id), String(t.title ?? t.id));
  }
  // Jobs the request ran have their own tab; the chip is for what the page sent.
  const children = (d.children ?? []).filter((c) => c.via !== 'job').map((c) => ({ rid: c.rid, url: c.url ?? '', status: Number(c.status ?? 0), ms: Number(c.duration_ms ?? 0), at: c.at ?? '' }));
  return {
    method: d.method ?? req.method ?? '',
    uri: path(d.uri ?? req.uri ?? ''),
    status: Number(d.status ?? req.status ?? 0),
    failed: Number(d.status ?? 0) >= 400 || d.problems.includes('exception'),
    timeMs,
    phases: { server: server / total, app: Math.max(timeMs - db, 0) / total, db: db / total },
    memory: req.memory_peak ? bytes(Number(req.memory_peak)) : '',
    queries: ev('query').length,
    nPlusOne: d.problems.includes('N+1'),
    views: ev('view').length,
    cache: ev('cache').length,
    logs: ev('log').length,
    appTabs: [...tabs].map(([id, title]) => ({ id: `custom:${id}`, title })),
    user: String(auth.name || auth.email || auth.id || ''),
    children,
    childFailures: children.filter((c) => c.status >= 400 || c.status === 0).length
  };
}

// path is a URL's path and query, which is what a request reads as on a bar.
export function path(url: string): string {
  try {
    const u = new URL(url, 'http://x');
    return u.pathname + u.search;
  } catch {
    return url;
  }
}

// popoverPlace puts a list a chip opens on the side of the chip away from the
// edge the bar sits on, starting under the chip and kept inside the viewport
// however far along a full-width strip the chip is.
export function popoverPlace(r: { left: number; top: number; bottom: number }, vw: number, vh: number, width = 480, margin = 12): string {
  const w = Math.min(width, vw - margin * 2);
  const left = Math.max(margin, Math.min(r.left, vw - w - margin));
  const y = r.top < vh / 2 ? `top:${r.bottom + 8}px` : `bottom:${vh - r.top + 8}px`;
  return `${y};left:${left}px`;
}
