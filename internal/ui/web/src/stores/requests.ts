import { apiJson } from '$lib/api';
import type { DumpEvent } from '$lib/dumpsStream';

// A request as lerd links it: one PHP request (or page view) with what
// happened in it, the page view that sent it and the requests it sent.
export interface RequestLink {
  rid: string;
  site?: string;
  url?: string;
  via?: string;
  status?: number;
  cross_origin?: boolean;
  at?: string;
  duration_ms?: number;
  timing?: Record<string, number>;
}

export interface RequestSummary {
  rid: string;
  type: 'page' | 'fetch' | 'xhr' | 'request' | 'cli' | 'worker' | 'job';
  site?: string;
  branch?: string;
  method?: string;
  uri?: string;
  route?: string;
  operation?: string;
  operations?: string[];
  nginx_ms?: number;
  queue_ms?: number;
  job?: string;
  job_status?: string;
  status?: number;
  time_ms?: number;
  started: string;
  worker?: string;
  command?: string;
  counts: Record<string, number>;
  problems: string[];
  parent?: RequestLink;
  children?: RequestLink[];
}

export interface QueryFinding {
  fingerprint?: string;
  count?: number;
  total_time_ms?: number;
  sample_sql?: string;
  sql?: string;
  example_sql?: string;
  time_ms?: number;
  caller: { file: string; line: number };
  ids?: string[];
}

export interface RequestDetail extends RequestSummary {
  events: Record<string, DumpEvent[]>;
  queries?: { query_count: number; total_time_ms: number; n_plus_one?: QueryFinding[]; slow?: QueryFinding[] };
}

export function loadRequests(site: string): Promise<RequestSummary[]> {
  const q = site ? `?site=${encodeURIComponent(site)}&limit=200` : '?limit=200';
  return apiJson<RequestSummary[]>(`/api/requests${q}`);
}

// summary asks for the counts and only the events the debug bar's chips read.
export function loadRequest(rid: string, summary = false): Promise<RequestDetail> {
  return apiJson<RequestDetail>(`/api/requests/${encodeURIComponent(rid)}${summary ? '?summary=1' : ''}`).then(withTraces);
}

// withTraces puts each event's stack trace back from the table the detail
// sends every distinct trace in once, so a query run a thousand times carries
// its trace once over the wire and the views read it where they always did.
export function withTraces(d: RequestDetail): RequestDetail {
  const traces = (d as RequestDetail & { traces?: unknown[] }).traces;
  if (!traces?.length) return d;
  for (const list of Object.values(d.events ?? {})) {
    for (const e of list as { data?: Record<string, unknown> }[]) {
      const ref = e.data?.trace_ref;
      if (typeof ref === 'number' && traces[ref]) {
        e.data!.trace = traces[ref];
        delete e.data!.trace_ref;
      }
    }
  }
  return d;
}

// nestRequests puts each request a page sent right under that page, keeping
// newest first, and leaves a request whose page is not in the list on its own.
export function nestRequests(list: RequestSummary[]): Array<RequestSummary & { depth: number }> {
  const byRid = new Map(list.map((r) => [r.rid, r]));
  const out: Array<RequestSummary & { depth: number }> = [];
  const placed = new Set<string>();
  for (const r of list) {
    if (placed.has(r.rid) || (r.parent && byRid.has(r.parent.rid))) continue;
    out.push({ ...r, depth: 0 });
    placed.add(r.rid);
    for (const c of r.children ?? []) {
      const child = byRid.get(c.rid);
      if (child && !placed.has(child.rid)) {
        out.push({ ...child, depth: 1 });
        placed.add(child.rid);
      }
    }
  }
  return out;
}
