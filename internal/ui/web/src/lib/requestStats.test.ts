import { describe, it, expect } from 'vitest';
import { requestStats } from './requestStats';
import type { DumpEvent } from './dumpEvent';

const e = (kind: string, data: unknown): DumpEvent => ({ v: 1, id: kind, ts: '2026-10-07T10:00:00.000Z', kind, ctx: { type: 'fpm' }, src: { file: '', line: 0 }, data }) as DumpEvent;

describe('requestStats', () => {
  it('splits what PHP spent between the app and its queries', () => {
    const s = requestStats(
      [e('query', { sql: 'a', time_ms: 10 }), e('query', { sql: 'b', time_ms: 5 }), e('request', { time_ms: 100, memory_peak: 4194304, queue_ms: 1.2 }), e('view', { name: 'home' })],
      104
    );
    expect(s).toEqual({ response: 104, memory: 4194304, app: 85, database: 15, queue: 1.2 });
  });

  it('keeps only the response time and queries when PHP did not report the request', () => {
    expect(requestStats([e('query', { sql: 'a', time_ms: 7 })], 40)).toEqual({ response: 40, database: 7 });
  });

  it('leaves the database out when the request ran no queries', () => {
    expect(requestStats([e('request', { time_ms: 30, memory_peak: 2097152 })], 31)).toEqual({ response: 31, memory: 2097152, app: 30 });
  });
});
