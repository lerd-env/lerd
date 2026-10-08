import { describe, it, expect } from 'vitest';
import { dumpHaystack, kindHaystack, queryHaystack } from './eventSearch';
import type { DumpEvent, QueryData } from '$lib/dumpsStream';

const ev = (kind: string) => ({ v: 1, id: kind, ts: '2026-10-07T10:00:00Z', kind, ctx: { type: 'fpm', site: 'shop', request: 'GET /', rid: '0065d3ba3290d4825' }, src: {}, data: {} }) as unknown as DumpEvent;

// A request id copied from a group header finds that request in every lens.
describe('lens search', () => {
  it('matches the request id in every lens', () => {
    expect(dumpHaystack(ev('dump'))).toContain('0065d3ba3290d4825');
    expect(kindHaystack(ev('view'))).toContain('0065d3ba3290d4825');
    expect(queryHaystack(ev('query'), { sql: 'select 1' } as QueryData)).toContain('0065d3ba3290d4825');
  });
});

describe('lens search by route', () => {
  it('matches the route a request ran under, ids collapsed as the timing view lists it', () => {
    const e = { ...ev('query'), ctx: { type: 'fpm', site: 'shop', request: 'GET /users/5?tab=1' } } as unknown as DumpEvent;
    expect(queryHaystack(e, { sql: 'select 1' } as QueryData)).toContain('get /users/:id');
    expect(kindHaystack({ ...e, kind: 'view' } as DumpEvent)).toContain('get /users/:id');
    expect(dumpHaystack({ ...e, kind: 'dump' } as DumpEvent)).toContain('get /users/:id');
  });
});
