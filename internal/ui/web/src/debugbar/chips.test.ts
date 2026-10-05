import { describe, it, expect } from 'vitest';
import { summarize, path, popoverPlace } from './chips';
import type { RequestDetail } from '$stores/requests';

const detail = {
  rid: 'r1', type: 'page', method: 'GET', uri: '/cart', status: 200, time_ms: 90, nginx_ms: 4, queue_ms: 6, started: '', counts: {}, problems: ['N+1'],
  queries: { total_time_ms: 20 },
  children: [{ rid: 'c1', url: 'https://shop.test/api/cart?x=1', status: 500, duration_ms: 38 }, { rid: 'c2', url: '/api/stock', status: 200 }, { rid: 'j1', url: 'App\\Jobs\\SendInvoice', via: 'job' }],
  events: {
    request: [{ data: { memory_peak: 4194304 } }],
    query: [{}, {}, {}],
    view: [{}],
    log: [{}, {}],
    auth: [{ data: { id: 7, name: 'Demo' } }],
    tab: [{ data: { id: 'cart', title: 'Cart' } }, { data: { id: 'cart', title: 'Cart' } }, { data: { id: 'flags', title: 'Flags' } }]
  }
} as unknown as RequestDetail;

describe('summarize', () => {
  it('reads every chip from the request detail the dashboard shows', () => {
    const s = summarize(detail);
    expect([s.method, s.uri, s.status, s.failed, s.memory, s.queries, s.nPlusOne, s.views, s.cache, s.logs, s.user]).toEqual(['GET', '/cart', 200, false, '4.0 MB', 3, true, 1, 0, 2, 'Demo']);
    expect(s.appTabs).toEqual([{ id: 'custom:cart', title: 'Cart' }, { id: 'custom:flags', title: 'Flags' }]);
    expect(s.children.map((c) => c.rid)).toEqual(['c1', 'c2']);
    expect(s.childFailures).toBe(1);
  });
  it('splits the time into before PHP, PHP and the database', () => {
    const { server, app, db } = summarize(detail).phases;
    expect([server, app, db].map((n) => Math.round(n * 100))).toEqual([10, 70, 20]);
  });
});

describe('summarize, a page no PHP request served', () => {
  it('reads as the path the browser showed, with no status or time', () => {
    const s = summarize({ rid: 'p1', type: 'page', uri: 'https://spa.test/orders?x=1', started: '', counts: {}, problems: [], events: {} } as unknown as RequestDetail);
    expect([s.method, s.uri, s.status, s.timeMs]).toEqual(['', '/orders?x=1', 0, 0]);
  });
});

describe('path', () => {
  it('keeps the path and query of a URL', () => {
    expect(path('https://shop.test/api/cart?x=1')).toBe('/api/cart?x=1');
    expect(path('/api/stock')).toBe('/api/stock');
  });
});

describe('popoverPlace', () => {
  it('keeps a list inside the viewport wherever its chip sits', () => {
    expect(popoverPlace({ left: 20, top: 640, bottom: 670 }, 1400, 700)).toBe('bottom:68px;left:20px');
    expect(popoverPlace({ left: 1300, top: 0, bottom: 28 }, 1400, 700)).toBe('top:36px;left:908px');
    expect(popoverPlace({ left: 5, top: 0, bottom: 28 }, 360, 700)).toBe('top:36px;left:12px');
  });
});
