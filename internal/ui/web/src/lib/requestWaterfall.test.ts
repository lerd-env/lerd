import { describe, it, expect } from 'vitest';
import { buildWaterfall, condense } from './requestWaterfall';
import type { RequestDetail } from '$stores/requests';

const phases = { dns: 'DNS', connect: 'Connect', wait: 'Wait', download: 'Download', dom: 'DOM', load: 'Load', bindings: 'Bindings', queries: 'Queries', details: 'Details', state: 'State', source: 'Source' };
const e = (kind: string, ts: string, data: unknown) => ({ v: 1, id: ts, ts, kind, ctx: { type: 'fpm' }, src: { file: '', line: 0 }, data });

describe('buildWaterfall', () => {
  it('lays spans and moments on the request clock and leaves the sent requests out', () => {
    const d: RequestDetail = {
      rid: 'r1', type: 'page', started: '2026-10-04T10:00:00.000Z', method: 'GET', uri: '/cart', time_ms: 100,
      counts: {}, problems: [],
      events: {
        span: [e('span', '2026-10-04T10:00:00.090Z', { label: 'View', name: 'cart', time_ms: 10, status: 'ok' })],
        view: [e('view', '2026-10-04T10:00:00.080Z', { name: 'cart' })],
        query: [e('query', '2026-10-04T10:00:00.050Z', { sql: 'select 1', time_ms: 20 })],
        log: [e('log', '2026-10-04T10:00:00.060Z', { level: 'error', message: 'boom' })],
        browser: [
          e('browser', '2026-10-04T10:00:00.300Z', { type: 'navigation', nav: 'load', message: 'https://shop.test/cart' }),
          e('browser', '2026-10-04T10:00:00.400Z', { type: 'network', rid: 'c1', message: 'fetch /api/x 500' })
        ]
      },
      children: [{ rid: 'c1', url: '/api/x', via: 'fetch', status: 500, at: '2026-10-04T10:00:00.400Z', duration_ms: 50 }]
    };
    const w = buildWaterfall(d, phases);
    expect(w.rows.map((r) => [r.layer, r.start, r.end])).toEqual([
      ['app', 0, 100],
      ['query', 30, 50],
      ['error', 60, 60],
      ['framework', 80, 90],
      ['browser', 300, 300]
    ]);
    expect(w.total).toBe(300);
  });

  it('merges the browser, nginx, the FPM queue and the app into one chronological list', () => {
    const d: RequestDetail = {
      rid: 'r2', type: 'page', started: '2026-10-04T10:00:00.000Z', time_ms: 50, nginx_ms: 2, queue_ms: 3,
      counts: {}, problems: [],
      events: {
        browser: [
          e('browser', '2026-10-04T10:00:00.200Z', { type: 'timing', message: 'page loaded', origin: Date.parse('2026-10-04T10:00:00.000Z'), timing: { connectStart: 1, connectEnd: 4, requestStart: 5, responseStart: 60, responseEnd: 70, domContentLoadedEventEnd: 120 } }),
          e('browser', '2026-10-04T10:00:01.000Z', { type: 'console', level: 'error', message: 'oops', at: '2026-10-04T10:00:00.065Z' }),
          e('browser', '2026-10-04T10:00:03.000Z', { type: 'console', level: 'error', message: 'much later', at: '2026-10-04T10:00:03.000Z' })
        ]
      }
    };
    const w = buildWaterfall(d, phases);
    // The request left the browser at 5 ms on its clock, which is when nginx
    // got it, the chart starts at the first phase, and the console error the
    // page logged 65 ms in lands during the download, while one seconds after
    // the load is left to the Browser section.
    expect(w.rows.map((r) => [r.label, r.start, r.end])).toEqual([
      ['Connect', 0, 3],
      ['Wait', 4, 59],
      ['nginx', 4, 6],
      ['FPM queue', 6, 9],
      ['', 9, 59],
      ['Download', 59, 69],
      ['oops', 64, 64],
      ['DOM', 69, 119]
    ]);
  });

  it('condenses rows of one lerd layer that follow each other, never the app own categories', () => {
    const m = (label: string, layer: 'log' | 'query' | 'custom' | 'framework', start: number, end = start) => ({ label, layer, start, end, note: '' });
    const out = condense(
      [m('a', 'log', 10), m('b', 'log', 11), m('q1', 'query', 12, 30), m('q2', 'query', 400, 420), m('c1', 'custom', 500, 510), m('c2', 'custom', 520, 530), m('f1', 'framework', 0, 5), m('f2', 'framework', 5, 9), m('d', 'log', 900)],
      1000
    );
    expect(out.map((r) => [r.label, r.items?.length ?? 0, r.start, r.end])).toEqual([
      ['', 2, 10, 11],
      ['', 2, 12, 420],
      ['c1', 0, 500, 510],
      ['c2', 0, 520, 530],
      ['f1', 0, 0, 5],
      ['f2', 0, 5, 9],
      ['d', 0, 900, 900]
    ]);
  });

  it('gives a query its bindings and a component its state for the popover', () => {
    const d: RequestDetail = {
      rid: 'r3', type: 'request', started: '2026-10-04T10:00:00.000Z', time_ms: 20, counts: {}, problems: [],
      events: {
        query: [e('query', '2026-10-04T10:00:00.010Z', { sql: 'select * from users where id = ?', bindings: [7], time_ms: 1, connection: 'sqlite' })],
        component: [e('component', '2026-10-04T10:00:00.015Z', { name: 'App\\Livewire\\Cart', phase: 'render', time_ms: 2, state: { count: '3' } })]
      }
    };
    const w = buildWaterfall(d, phases);
    const q = w.rows.find((r) => r.layer === 'query')!;
    expect(q.code).toBe('select * from users where id = 7');
    expect(q.sections).toEqual([{ title: 'Bindings', values: { '#1': '7' } }, { title: 'Details', values: { connection: 'sqlite' } }]);
    expect(w.rows.find((r) => r.layer === 'component')!.sections).toEqual([{ title: 'State', values: { count: '3' } }]);
  });

  it('folds the queries a timed cache call ran into that call', () => {
    const d: RequestDetail = {
      rid: 'r4', type: 'request', started: '2026-10-04T10:00:00.000Z', time_ms: 50, counts: {}, problems: [],
      events: {
        query: [
          e('query', '2026-10-04T10:00:00.012Z', { sql: 'select * from cache where key = ?', bindings: ['k'], time_ms: 2 }),
          e('query', '2026-10-04T10:00:00.030Z', { sql: 'select 1', time_ms: 1 })
        ],
        cache: [e('cache', '2026-10-04T10:00:00.013Z', { op: 'hit', key: 'k', store: 'database', time_ms: 4 })]
      }
    };
    const w = buildWaterfall(d, phases);
    expect(w.rows.filter((r) => r.layer === 'query').map((r) => r.label)).toEqual(['select 1']);
    const hit = w.rows.find((r) => r.layer === 'cache')!;
    expect(hit.sections?.[0]).toEqual({ title: 'Queries', values: { '#1 · 2.0 ms': "select * from cache where key = 'k'" } });
  });
});

