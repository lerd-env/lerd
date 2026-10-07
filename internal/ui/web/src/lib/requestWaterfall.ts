import type { DumpEvent } from './dumpsStream';
import { inlineBindings } from './sqlInline';

// One row of a request's timeline: a span (end > start) or a moment (end ===
// start), in milliseconds from the first row.
export type Layer = 'request' | 'query' | 'view' | 'cache' | 'http' | 'job' | 'mail' | 'message' | 'event' | 'log' | 'dump' | 'error';
export interface WaterfallRow {
  label: string;
  layer: Layer;
  start: number;
  end: number;
  note: string;
  // items holds the rows a condensed row stands for.
  items?: WaterfallRow[];
  // code is the row's full text where it is code: a query with its bindings.
  code?: string;
  source?: { file: string; line?: number };
}
export interface Waterfall {
  total: number;
  rows: WaterfallRow[];
}
// ServedRequest is the request as nginx logged it: when it started, in Unix
// milliseconds, and how long it took.
export interface ServedRequest {
  label: string;
  start: number;
  millis: number;
}

type Data = Record<string, unknown> & { time_ms?: number };
const ms = (n: number) => `${n < 10 ? n.toFixed(1) : Math.round(n)} ms`;

// row places one captured event: a timed one as a span ending when it was
// reported, anything else as the moment it happened.
function row(ev: DumpEvent): Omit<WaterfallRow, 'start' | 'end'> & { took: number } {
  const d = (ev.data ?? {}) as Data;
  const s = (v: unknown) => (v == null ? '' : String(v));
  const source = ev.src?.file ? { source: { file: ev.src.file, line: ev.src.line } } : {};
  const took = Number(d.time_ms ?? 0);
  const base = { took, note: took ? ms(took) : '', ...source };
  switch (ev.kind) {
    case 'query':
      return { ...base, label: s(d.sql), layer: 'query', code: inlineBindings(s(d.sql), d.bindings as unknown[] | undefined) };
    case 'view':
      return { ...base, label: s(d.name), layer: 'view', ...(d.path ? { source: { file: s(d.path) } } : {}) };
    case 'cache':
      return { ...base, label: `${s(d.op)} ${s(d.key)}`.trim(), layer: 'cache', note: base.note || s(d.store) };
    case 'http':
      return { ...base, label: `${s(d.method)} ${s(d.url)}`.trim(), layer: d.failed ? 'error' : 'http' };
    case 'job':
      return { ...base, label: s(d.class), layer: d.status === 'failed' ? 'error' : 'job', note: base.note || s(d.status) };
    case 'mail':
      return { ...base, label: s(d.subject), layer: 'mail' };
    case 'message':
      return { ...base, label: s(d.body || d.notification), layer: 'message' };
    case 'event':
      return { ...base, label: s(d.name), layer: 'event' };
    case 'log': {
      const bad = ['error', 'critical', 'alert', 'emergency'].includes(s(d.level));
      return { ...base, label: s(d.message), layer: bad ? 'error' : 'log', note: s(d.level) };
    }
    case 'exception':
      return { ...base, label: `${d.type && d.type !== 'message' ? s(d.type) + ': ' : ''}${s(d.message)}`, layer: 'error' };
    default:
      return { ...base, label: ev.label || ev.text?.split('\n')[0] || 'dump', layer: 'dump' };
  }
}

// buildWaterfall lays a request's captured events on one chronological list,
// under the request itself when nginx timed it. Browser events happen on the
// page after the response, so they stay in the Browser lens.
export function buildWaterfall(events: DumpEvent[], served?: ServedRequest): Waterfall {
  const rows: WaterfallRow[] = [];
  const t0 = served?.start ?? Math.min(...events.map((ev) => Date.parse(ev.ts)));
  if (served) rows.push({ label: served.label, layer: 'request', start: 0, end: served.millis, note: ms(served.millis) });
  for (const ev of events) {
    if (ev.kind === 'browser') continue;
    const { took, ...r } = row(ev);
    const end = Date.parse(ev.ts) - t0;
    rows.push({ ...r, start: end - took, end });
  }
  // A span that starts with another one encloses it, so the longer goes first.
  rows.sort((a, b) => a.start - b.start || b.end - a.end);
  const origin = Math.min(0, ...rows.map((r) => r.start));
  for (const r of rows) {
    r.start -= origin;
    r.end -= origin;
  }
  return { total: Math.max(1, ...rows.map((r) => r.end)), rows };
}

// condense folds rows of one layer that follow each other into a single row
// spanning them all, the way a busy request's queries would take a row each.
// The request and its errors always keep a row of their own.
export function condense(rows: WaterfallRow[]): WaterfallRow[] {
  const out: WaterfallRow[] = [];
  for (const r of rows) {
    const last = out[out.length - 1];
    if (last && last.layer === r.layer && r.layer !== 'request' && r.layer !== 'error') {
      if (!last.items) out[out.length - 1] = { label: '', layer: r.layer, start: last.start, end: last.end, note: '', items: [last] };
      const fold = out[out.length - 1];
      fold.items!.push(r);
      fold.start = Math.min(fold.start, r.start);
      fold.end = Math.max(fold.end, r.end);
      continue;
    }
    out.push(r);
  }
  return out;
}
