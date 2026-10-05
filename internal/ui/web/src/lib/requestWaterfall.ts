import type { RequestDetail } from '$stores/requests';
import { inlineBindings } from '$lib/sqlInline';
import { callerClass, type Frame } from '$lib/sourceLabel';

// One row of a request's waterfall: a span (end > start) or a moment (end ===
// start), in milliseconds from the first row.
export type Layer = 'browser' | 'server' | 'app' | 'framework' | 'query' | 'component' | 'view' | 'cache' | 'redis' | 'filesystem' | 'http' | 'log' | 'dump' | 'error' | 'custom';
export interface WaterfallRow {
  label: string;
  layer: Layer;
  start: number;
  end: number;
  note: string;
  // items holds the moments a condensed row stands for.
  items?: WaterfallRow[];
  // code is the row's full text where it is code, a query with its bindings in
  // place, and sections are what the row's popover lists beneath it.
  code?: string;
  sections?: Array<{ title: string; values: Record<string, string> }>;
  // source is where in the code the row came from, opened in the editor.
  source?: { file: string; line?: number; label?: string; trace?: Frame[] };
  // category and color are what an app gave a row it wrote itself.
  category?: string;
  color?: string;
  // warn marks a row that went wrong without leaving its layer, a failed call.
  warn?: boolean;
}
export interface Waterfall {
  total: number;
  rows: WaterfallRow[];
}

// Browser phases as the Navigation and Resource Timing APIs name their edges,
// and the titles of a row's popover sections.
export interface Phases {
  dns: string;
  connect: string;
  wait: string;
  download: string;
  dom: string;
  load: string;
  bindings: string;
  queries: string;
  details: string;
  state: string;
  source: string;
  timing: string;
  requestHeaders: string;
  responseHeaders: string;
}
type Timing = Record<string, number>;
const PHASES: Array<[keyof Phases, string, string]> = [
  ['dns', 'domainLookupStart', 'domainLookupEnd'],
  ['connect', 'connectStart', 'connectEnd'],
  ['wait', 'requestStart', 'responseStart'],
  ['download', 'responseStart', 'responseEnd'],
  ['dom', 'responseEnd', 'domContentLoadedEventEnd'],
  ['load', 'domContentLoadedEventEnd', 'loadEventEnd']
];

const MAX_QUERIES = 25;
const at = (ts: string) => Date.parse(ts);
const ms = (n: number) => `${n < 10 ? n.toFixed(1) : Math.round(n)} ms`;
export const bytes = (n: number) => (n < 1024 ? `${n} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1048576).toFixed(1)} MB`);

// buildWaterfall lays out what a request did in one chronological list: how the
// browser and nginx got it to PHP, the app's work as spans and moments, and the
// page's browser events, with the layer saying which is which. The requests a
// page sent later are listed elsewhere, since minutes can pass before a call.
export function buildWaterfall(d: RequestDetail, phases: Phases): Waterfall {
  const t0 = at(d.started);
  const rows: WaterfallRow[] = [];
  const span = (label: string, layer: Layer, endTs: string, took: number, extra: Partial<WaterfallRow> = {}) => {
    const end = at(endTs) - t0;
    rows.push({ label, layer, start: Math.max(end - took, 0), end, note: ms(took), ...extra });
  };
  const section = (title: string, values: Record<string, unknown> | undefined) =>
    values && Object.keys(values).length ? [{ title, values: Object.fromEntries(Object.entries(values).map(([k, v]) => [k, typeof v === 'string' ? v : JSON.stringify(v)])) }] : [];
  const moment = (label: string, layer: Layer, t: number, note = '', extra: Partial<WaterfallRow> = {}) => rows.push({ label, layer, start: t, end: t, note, ...extra });
  const src = (e: { src?: { file: string; line: number }; data?: unknown }) =>
    e.src?.file ? { source: { file: e.src.file, line: e.src.line, label: callerClass(e.src.file, e.src.line, (e.data as { trace?: Frame[] })?.trace), trace: (e.data as { trace?: Frame[] })?.trace } } : {};
  const at2 = (file?: string, line?: number) => (file ? { source: { file, line } } : {});

  // nginx and the FPM queue come before PHP's own clock starts.
  const queue = d.queue_ms ?? 0;
  const serverStart = -(queue + (d.nginx_ms ?? 0));
  if (d.nginx_ms !== undefined || d.queue_ms !== undefined) {
    rows.push({ label: 'nginx', layer: 'server', start: serverStart, end: -queue, note: ms(d.nginx_ms ?? 0) });
    rows.push({ label: 'FPM queue', layer: 'server', start: -queue, end: 0, note: ms(queue) });
  }
  // The browser's phases are on its own clock, so they are placed with the
  // moment it sent the request on the moment nginx received it.
  const page = (d.events.browser ?? []).map((e) => e.data as { type?: string; timing?: Timing; origin?: number }).find((b) => b.type === 'timing');
  const timing = page?.timing ?? d.parent?.timing;
  const anchor = timing?.requestStart !== undefined ? serverStart - timing.requestStart : undefined;
  if (timing && anchor !== undefined) {
    for (const [key, from, to] of PHASES) {
      const a = timing[from];
      const b = timing[to];
      if (a === undefined || b === undefined || b <= a) continue;
      rows.push({ label: phases[key], layer: 'browser', start: anchor + a, end: anchor + b, note: ms(b - a) });
    }
  }

  if (d.time_ms) rows.push({ label: d.job ?? `${d.method ?? ''} ${d.uri ?? ''}`.trim(), layer: 'app', start: 0, end: d.time_ms, note: ms(d.time_ms) });
  // A view the app timed as a span is drawn once, as that span, and opens at
  // the template the view event named.
  const timed = new Set<string>();
  const viewPaths = new Map((d.events.view ?? []).map((e) => [String((e.data as { name?: string }).name), String((e.data as { path?: string }).path ?? '')]));
  for (const e of d.events.span ?? []) {
    const s = e.data as { label?: string; name?: string; time_ms?: number; status?: string; file?: string; line?: number };
    if (s.name) timed.add(s.name);
    const view = s.label === 'View' ? viewPaths.get(s.name ?? '') : undefined;
    span(`${s.label ?? ''} ${s.name ?? ''}`.trim(), s.status === 'failed' ? 'error' : 'framework', e.ts, s.time_ms ?? 0, at2(s.file ?? view, s.line));
  }
  const queries = d.events.query ?? [];
  for (const e of queries.slice(0, MAX_QUERIES)) {
    const q = e.data as { sql?: string; time_ms?: number; bindings?: unknown[]; connection?: string };
    const bindings = Object.fromEntries((q.bindings ?? []).map((b, i) => [`#${i + 1}`, b]));
    span(q.sql ?? '', 'query', e.ts, q.time_ms ?? 0, {
      code: inlineBindings(q.sql ?? '', q.bindings ?? []),
      sections: [...section(phases.bindings, bindings), ...section(phases.details, q.connection ? { connection: q.connection } : undefined)],
      ...src(e)
    });
  }
  if (queries.length > MAX_QUERIES) {
    moment(`+${queries.length - MAX_QUERIES} more queries`, 'query', Math.max(at(queries[queries.length - 1].ts) - t0, 0));
  }
  for (const e of d.events.component ?? []) {
    const c = e.data as { name?: string; phase?: string; time_ms?: number; status?: string; details?: Record<string, unknown>; state?: Record<string, unknown>; file?: string; line?: number; exception?: string };
    span(`${c.name ?? ''} ${c.phase ?? ''}`.trim(), c.status === 'failed' ? 'error' : 'component', e.ts, c.time_ms ?? 0, {
      sections: [
        ...section(phases.state, c.state),
        ...section(phases.details, { ...(c.details ?? {}), ...(c.exception ? { exception: c.exception } : {}) })
      ],
      ...at2(c.file, c.line)
    });
  }
  for (const e of d.events.redis ?? []) {
    const r = e.data as { command?: string; args?: string; connection?: string; time_ms?: number; status?: string };
    span(`${r.command ?? ''} ${r.args ?? ''}`.trim(), r.status === 'failed' ? 'error' : 'redis', e.ts, r.time_ms ?? 0, {
      sections: section(phases.details, r.connection ? { connection: r.connection } : undefined),
      ...src(e)
    });
  }
  for (const e of d.events.filesystem ?? []) {
    const f = e.data as { op?: string; path?: string; disk?: string; time_ms?: number; status?: string };
    span(`${f.op ?? ''} ${f.path ?? ''}`.trim(), f.status === 'failed' ? 'error' : 'filesystem', e.ts, f.time_ms ?? 0, {
      sections: section(phases.details, f.disk ? { disk: f.disk } : undefined),
      ...src(e)
    });
  }
  // Outgoing requests end when they are reported, so each is drawn back from
  // there by its time; calls a pool made at once overlap.
  for (const e of d.events.http ?? []) {
    const h = e.data as { method?: string; url?: string; status?: number; reason?: string; failed?: boolean; time_ms?: number; timing?: Record<string, number>; request_size?: number; response_size?: number; request_headers?: Record<string, string>; response_headers?: Record<string, string> };
    const timing: Record<string, string> = Object.fromEntries(Object.entries(h.timing ?? {}).map(([k, v]) => [k, ms(v)]));
    const details: Record<string, string> = { status: h.status ? `${h.status}${h.reason ? ` ${h.reason}` : ''}` : h.failed ? 'failed' : '' };
    if (!details.status) delete details.status;
    if (h.request_size !== undefined) details.sent = bytes(h.request_size);
    if (h.response_size !== undefined) details.received = bytes(h.response_size);
    // A failed call stays an HTTP row, so it folds in with its siblings, and
    // carries a warning instead of a colour of its own.
    span(`${h.method ?? ''} ${h.url ?? ''}`.trim(), 'http', e.ts, h.time_ms ?? 0, {
      warn: Boolean(h.failed) || (h.status ?? 0) >= 400,
      note: `${h.status || (h.failed ? 'failed' : '')}${h.time_ms ? ` · ${ms(h.time_ms)}` : ''}`,
      sections: [...section(phases.details, details), ...section(phases.timing, timing), ...section(phases.requestHeaders, h.request_headers), ...section(phases.responseHeaders, h.response_headers)],
      ...src(e)
    });
  }
  // Rows the app wrote itself through lerd/debug, placed by the start it gave.
  for (const e of d.events.timeline ?? []) {
    const c = e.data as { label?: string; category?: string; color?: string; start?: number; duration_ms?: number; details?: Record<string, unknown> };
    const start = c.start ? c.start * 1000 - t0 : Math.max(at(e.ts) - t0, 0);
    const took = c.duration_ms ?? 0;
    rows.push({
      label: c.label ?? '', layer: 'custom', start, end: start + took, note: took ? ms(took) : (c.category ?? ''),
      category: c.category ?? 'app', color: c.color, sections: section(phases.details, c.details), ...src(e)
    });
  }
  const appMoment = (label: string, layer: Layer, e: { ts: string; src?: { file: string; line: number }; data?: unknown }, note = '', extra: Partial<WaterfallRow> = {}) =>
    moment(label, layer, Math.max(at(e.ts) - t0, 0), note, { ...src(e), ...extra });
  for (const e of d.events.view ?? []) {
    const v = e.data as { name?: string; path?: string };
    const name = String(v?.name ?? 'view');
    if (!timed.has(name)) appMoment(name, 'view', e, '', at2(v.path));
  }
  for (const e of d.events.dump ?? []) appMoment(e.label || 'dump', 'dump', e);
  // A timed cache call takes in the queries that ran inside it, a database
  // store's own, so they show in its popover rather than as rows of their own.
  for (const e of d.events.cache ?? []) {
    const c = e.data as { op?: string; key?: string; store?: string; time_ms?: number; file?: string; connection?: string };
    const label = `${c.op ?? ''} ${c.key ?? ''}`.trim();
    const details = section(phases.details, { ...(c.store ? { store: c.store } : {}), ...(c.connection ? { connection: c.connection } : {}), ...(c.file ? { file: c.file } : {}) });
    if (!c.time_ms) {
      appMoment(label, 'cache', e, c.store ?? '', { sections: details });
      continue;
    }
    span(label, 'cache', e.ts, c.time_ms, { sections: details, ...src(e) });
    const call = rows[rows.length - 1];
    const inner = rows.filter((r) => (r.layer === 'query' || r.layer === 'redis' || r.layer === 'filesystem') && r.start >= call.start - 1 && r.end <= call.end + 1);
    if (!inner.length) continue;
    for (const q of inner) rows.splice(rows.indexOf(q), 1);
    call.sections = [...section(phases.queries, Object.fromEntries(inner.map((q, i) => [`#${i + 1} · ${q.note}`, q.code ?? q.label]))), ...(call.sections ?? [])];
  }
  for (const e of d.events.log ?? []) {
    const l = e.data as { level?: string; message?: string };
    const bad = ['error', 'critical', 'alert', 'emergency'].includes(l.level ?? '');
    appMoment(`log ${l.level ?? ''}: ${l.message ?? ''}`, bad ? 'error' : 'log', e, l.level ?? '');
  }
  for (const e of d.events.exception ?? []) {
    const x = e.data as { type?: string; message?: string };
    appMoment(`${x.type ?? 'exception'}: ${x.message ?? ''}`, 'error', e);
  }

  // Browser events after the page finished loading belong to its later life,
  // which the Browser section lists; drawing them would squash the load.
  const loadEnd = timing && anchor !== undefined ? anchor + (timing.loadEventEnd ?? timing.domContentLoadedEventEnd ?? timing.responseEnd ?? 0) : Infinity;
  const cutoff = loadEnd === Infinity ? Infinity : Math.max(loadEnd, ...rows.map((r) => r.end));
  for (const e of d.events.browser ?? []) {
    const b = e.data as { type?: string; message?: string; nav?: string; level?: string; rid?: string; at?: string };
    // A linked call is listed with the requests the page sent.
    if (b.type === 'request' || b.type === 'timing' || (b.type === 'network' && b.rid)) continue;
    const bad = b.type === 'error' || b.type === 'rejection' || b.type === 'network' || b.type === 'resource' || (b.type === 'console' && b.level === 'error');
    const label = b.type === 'navigation' ? `${b.nav ?? 'load'} ${b.message ?? ''}` : (b.message ?? '');
    // On the page's own clock when it reported one, so the event lands among
    // its load phases; otherwise by when it was reported.
    const t = anchor !== undefined && page?.origin && b.at ? anchor + (at(b.at) - page.origin) : at(e.ts) - t0;
    if (t > cutoff + 1) continue;
    moment(label, bad ? 'error' : 'browser', t, b.type === 'console' ? `console.${b.level}` : (b.type ?? ''));
  }

  // One chronological list; a span that starts with another one encloses it,
  // so the longer is drawn first.
  rows.sort((a, b) => a.start - b.start || b.end - a.end);
  const origin = Math.min(0, ...rows.map((r) => r.start));
  for (const r of rows) {
    r.start -= origin;
    r.end -= origin;
  }
  const total = Math.max(1, ...rows.map((r) => r.end));
  return { total, rows };
}

// Groupable are lerd's own layers whose rows come in runs; the request's
// backbone, errors and the categories an app writes itself always keep a row.
const groupable = new Set<Layer>(['browser', 'query', 'component', 'view', 'cache', 'redis', 'filesystem', 'http', 'log', 'dump']);

// condense folds rows of one groupable layer that follow each other into a
// single "N events" row spanning them all, the way a busy request's queries or
// logs would otherwise take a row each.
export function condense(rows: WaterfallRow[], total: number): WaterfallRow[] {
  void total;
  const out: WaterfallRow[] = [];
  for (const r of rows) {
    const last = out[out.length - 1];
    if (last && last.layer === r.layer && groupable.has(r.layer)) {
      const items = last.items ?? [last];
      out[out.length - 1] = { label: '', layer: r.layer, start: Math.min(last.start, r.start), end: Math.max(last.end, r.end), note: '', items: [...items, r] };
      continue;
    }
    out.push(r);
  }
  return out;
}
