import { describe, it, expect } from 'vitest';
import { nestRequests, type RequestSummary } from './requests';

const r = (rid: string, extra: Partial<RequestSummary> = {}): RequestSummary => ({ rid, type: 'request', started: rid, counts: {}, problems: [], ...extra });

describe('nestRequests', () => {
  it('lists the requests a page sent right under it, even when they are newer', () => {
    const list = [
      r('api2', { type: 'fetch', parent: { rid: 'page' } }),
      r('other'),
      r('api1', { type: 'xhr', parent: { rid: 'page' } }),
      r('page', { type: 'page', children: [{ rid: 'api1' }, { rid: 'api2' }] }),
      r('orphan', { type: 'fetch', parent: { rid: 'gone' } })
    ];
    expect(nestRequests(list).map((x) => `${x.depth}:${x.rid}`)).toEqual(['0:other', '0:page', '1:api1', '1:api2', '0:orphan']);
  });
});

describe('withTraces', () => {
  it('puts each event its trace back from the shared table', async () => {
    const { withTraces } = await import('./requests');
    const frames = [{ file: '/app/Loop.php', line: 3, func: 'run' }];
    const d = withTraces({ rid: 'r', type: 'page', started: '', counts: {}, problems: [], traces: [frames], events: { query: [{ data: { sql: 'a', trace_ref: 0 } }, { data: { sql: 'b', trace_ref: 0 } }, { data: { sql: 'c' } }] } } as never);
    const qs = (d.events as Record<string, { data: Record<string, unknown> }[]>).query;
    expect(qs[0].data.trace).toBe(frames);
    expect(qs[1].data.trace).toBe(frames);
    expect(qs[0].data.trace_ref).toBeUndefined();
    expect(qs[2].data.trace).toBeUndefined();
  });
});
