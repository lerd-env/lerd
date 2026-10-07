import { describe, it, expect } from 'vitest';
import { buildWaterfall, condense } from './requestWaterfall';
import type { DumpEvent } from './dumpsStream';

const e = (kind: string, ts: string, data: unknown, src = { file: '', line: 0 }): DumpEvent =>
  ({ v: 1, id: ts + kind, ts, kind, ctx: { type: 'fpm', request: 'GET /cart' }, src, data }) as DumpEvent;

describe('buildWaterfall', () => {
  it('lays what a request did on the clock of the request nginx served', () => {
    const w = buildWaterfall(
      [
        e('query', '2026-10-07T10:00:00.050Z', { sql: 'select ?', time_ms: 20, bindings: [1] }),
        e('log', '2026-10-07T10:00:00.060Z', { level: 'error', message: 'boom' }),
        e('view', '2026-10-07T10:00:00.080Z', { name: 'cart', path: '/app/cart.blade.php' }),
        e('browser', '2026-10-07T10:00:05.000Z', { type: 'error', message: 'later' })
      ],
      { label: 'GET /cart', start: Date.parse('2026-10-07T10:00:00.000Z'), millis: 100 }
    );
    expect(w.rows.map((r) => [r.layer, r.start, r.end])).toEqual([
      ['request', 0, 100],
      ['query', 30, 50],
      ['error', 60, 60],
      ['view', 80, 80]
    ]);
    expect(w.rows[1].code).toBe('select 1');
    expect(w.total).toBe(100);
  });

  it('starts at the first event when nginx has not timed the request', () => {
    const w = buildWaterfall([
      e('cache', '2026-10-07T10:00:00.010Z', { op: 'hit', key: 'k' }),
      e('http', '2026-10-07T10:00:00.040Z', { method: 'GET', url: 'https://api.test', time_ms: 25 })
    ]);
    expect(w.rows.map((r) => [r.layer, r.start, r.end])).toEqual([
      ['cache', 0, 0],
      ['http', 5, 30]
    ]);
  });
});

describe('condense', () => {
  it('folds a run of one layer into a row that lists them', () => {
    const rows = condense([
      { label: 'a', layer: 'query', start: 0, end: 1, note: '' },
      { label: 'b', layer: 'query', start: 2, end: 3, note: '' },
      { label: 'c', layer: 'error', start: 4, end: 4, note: '' }
    ]);
    expect(rows).toHaveLength(2);
    expect(rows[0].items?.map((r) => r.label)).toEqual(['a', 'b']);
    expect([rows[0].start, rows[0].end]).toEqual([0, 3]);
  });
});
