import { describe, it, expect } from 'vitest';
import { buildQueryGroups, normalizeSql, SLOW_MS } from './queries';
import type { DumpEvent, QueryData } from '$lib/dumpsStream';

function q(over: { id: string; ts: string; data: QueryData; request?: string; file?: string; rid?: string; worker?: string }): DumpEvent {
  return {
    v: 1,
    id: over.id,
    ts: over.ts,
    kind: 'query',
    ctx: { type: over.worker ? 'cli' : 'fpm', site: 'acme', request: over.request ?? 'GET /', pid: 1, rid: over.rid, worker: over.worker },
    src: { file: over.file ?? '/app/Models/User.php', line: 30 },
    data: over.data
  };
}

describe('normalizeSql', () => {
  it('collapses literals to a shared fingerprint', () => {
    expect(normalizeSql('SELECT * FROM users WHERE id = 1')).toBe(
      normalizeSql('select * from users where id = 42')
    );
    expect(normalizeSql("select * from t where name = 'bob'")).toBe(
      normalizeSql("SELECT * FROM t WHERE name = 'alice'")
    );
  });
});

describe('buildQueryGroups', () => {
  it('ignores non-query events and groups by request', () => {
    const events: DumpEvent[] = [
      q({ id: 'a', ts: '2026-06-01T10:00:00.000Z', data: { sql: 'select 1', time_ms: 2 } }),
      { ...q({ id: 'd', ts: '2026-06-01T10:00:01.000Z', data: { sql: 'x', time_ms: 1 } }), kind: 'dump' },
      q({ id: 'b', ts: '2026-06-01T10:00:02.000Z', data: { sql: 'select 2', time_ms: 3 } })
    ];
    const groups = buildQueryGroups(events);
    expect(groups).toHaveLength(1);
    expect(groups[0].count).toBe(2);
    expect(groups[0].totalMs).toBe(5);
  });

  it('tags slow queries at the threshold', () => {
    const groups = buildQueryGroups([
      q({ id: 'a', ts: '2026-06-01T10:00:00.000Z', data: { sql: 'select 1', time_ms: SLOW_MS } }),
      q({ id: 'b', ts: '2026-06-01T10:00:00.001Z', data: { sql: 'select 2', time_ms: 5 } })
    ]);
    expect(groups[0].slowCount).toBe(1);
    const slowRow = groups[0].rows.find((r) => r.event.id === 'a');
    expect(slowRow?.slow).toBe(true);
  });

  it('flags duplicates and escalates N+1 on repeated fingerprints', () => {
    const events = Array.from({ length: 4 }, (_, i) =>
      q({ id: `r${i}`, ts: `2026-06-01T10:00:0${i}.000Z`, data: { sql: `select * from posts where user_id = ${i}`, time_ms: 1 } })
    );
    const groups = buildQueryGroups(events);
    expect(groups[0].nPlusOne).toBe(true);
    expect(groups[0].rows.every((r) => r.duplicate)).toBe(true);
    expect(groups[0].rows[0].dupCount).toBe(4);
  });

  it('does not count the same query across different requests', () => {
    // The same SQL in two separate requests must not be flagged as a
    // duplicate/N+1 — each request is its own group. Regression guard for the
    // extension not emitting request/pid (everything collapsing into one group).
    const sql = 'select * from `users` where `id` = ?';
    const groups = buildQueryGroups([
      q({ id: 'a', ts: '2026-06-01T10:00:00.000Z', request: 'GET /a', data: { sql, time_ms: 1 } }),
      q({ id: 'b', ts: '2026-06-01T10:00:01.000Z', request: 'GET /b', data: { sql, time_ms: 1 } })
    ]);
    expect(groups).toHaveLength(2);
    for (const g of groups) {
      expect(g.count).toBe(1);
      expect(g.nPlusOne).toBe(false);
      expect(g.rows[0].duplicate).toBe(false);
      expect(g.rows[0].dupCount).toBe(1);
    }
  });

  it('groups by request id, splitting same-URL/same-pid requests apart', () => {
    // Two requests to the same URL on the same pool worker (identical
    // request+pid) but distinct rid must be two groups, not one merged group
    // that double-counts the shared query.
    const sql = 'select * from `users` where `id` = ?';
    const groups = buildQueryGroups([
      q({ id: 'a', ts: '2026-06-01T10:00:00.000Z', request: 'GET /', rid: 'req-1', data: { sql, time_ms: 1 } }),
      q({ id: 'b', ts: '2026-06-01T10:00:01.000Z', request: 'GET /', rid: 'req-2', data: { sql, time_ms: 1 } })
    ]);
    expect(groups).toHaveLength(2);
    for (const g of groups) {
      expect(g.count).toBe(1);
      expect(g.rows[0].duplicate).toBe(false);
    }
  });

  it('labels worker groups by command and filters by it', () => {
    const events = [
      q({ id: 'w1', ts: '2026-06-01T10:00:00.000Z', rid: 'r1', worker: 'queue:work', data: { sql: 'select 1', time_ms: 1 } }),
      q({ id: 'w2', ts: '2026-06-01T10:00:01.000Z', rid: 'r2', worker: 'scrape:rtb-data', data: { sql: 'select 2', time_ms: 1 } }),
      q({ id: 'web', ts: '2026-06-01T10:00:02.000Z', rid: 'r3', request: 'GET /', data: { sql: 'select 3', time_ms: 1 } })
    ];
    const all = buildQueryGroups(events);
    const wq = all.find((g) => g.rows[0].event.id === 'w1');
    expect(wq?.worker).toBe('queue:work');
    expect(wq?.label).toContain('queue:work');

    // Filter to one worker command.
    const onlyScrape = buildQueryGroups(events, '', '', false, 'scrape:rtb-data');
    expect(onlyScrape).toHaveLength(1);
    expect(onlyScrape[0].worker).toBe('scrape:rtb-data');
  });

  it('hides all worker groups when showWorkers is off, keeping web requests', () => {
    const events = [
      q({ id: 'w1', ts: '2026-06-01T10:00:00.000Z', rid: 'r1', worker: 'queue:work', data: { sql: 'select 1', time_ms: 1 } }),
      q({ id: 'w2', ts: '2026-06-01T10:00:01.000Z', rid: 'r2', worker: 'scrape:rtb-data', data: { sql: 'select 2', time_ms: 1 } }),
      q({ id: 'web', ts: '2026-06-01T10:00:02.000Z', rid: 'r3', request: 'GET /', data: { sql: 'select 3', time_ms: 1 } })
    ];
    // showWorkers=false drops buffered worker queries from the view, not just
    // future capture, leaving only the web request.
    const noWorkers = buildQueryGroups(events, '', '', false, '', false);
    expect(noWorkers).toHaveLength(1);
    expect(noWorkers[0].worker).toBe('');
    expect(noWorkers[0].rows[0].event.id).toBe('web');
  });

  it('does not flag N+1 for distinct queries', () => {
    const groups = buildQueryGroups([
      q({ id: 'a', ts: '2026-06-01T10:00:00.000Z', data: { sql: 'select * from users', time_ms: 1 } }),
      q({ id: 'b', ts: '2026-06-01T10:00:01.000Z', data: { sql: 'select * from posts', time_ms: 1 } })
    ]);
    expect(groups[0].nPlusOne).toBe(false);
    expect(groups[0].rows.every((r) => !r.duplicate)).toBe(true);
  });

  it('filters by search text against sql and file', () => {
    const events = [
      q({ id: 'a', ts: '2026-06-01T10:00:00.000Z', data: { sql: 'select * from orders', time_ms: 1 } }),
      q({ id: 'b', ts: '2026-06-01T10:00:01.000Z', data: { sql: 'select * from users', time_ms: 1 } })
    ];
    const groups = buildQueryGroups(events, '', 'orders');
    expect(groups[0].count).toBe(1);
    expect(groups[0].rows[0].data.sql).toContain('orders');
  });
});
