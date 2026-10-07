import { describe, it, expect } from 'vitest';
import { normalizeRoute, routeQuery, routeOf } from './route';
import type { DumpEvent } from '$lib/dumpsStream';

// The same cases internal/reqstats/route_test.go holds, so a route clicked in the
// timing view names exactly the requests the watcher grouped under it.
describe('normalizeRoute', () => {
  it.each([
    ['GET', '/users/123', 'GET /users/:id'],
    ['get', '/users/123/posts/456', 'GET /users/:id/posts/:id'],
    ['GET', '/reports/5?page=2&sort=asc', 'GET /reports/:id'],
    ['POST', '/import', 'POST /import'],
    ['GET', '/', 'GET /'],
    ['GET', '', 'GET /'],
    ['GET', '/users/550e8400-e29b-41d4-a716-446655440000', 'GET /users/:id'],
    ['GET', '/assets/a1b2c3d4e5f6a7b8/app.css', 'GET /assets/:id/app.css'],
    ['GET', '/v1/orders/42', 'GET /v1/orders/:id'],
    ['GET', '/users/123/', 'GET /users/:id'],
    ['GET', '/search#frag', 'GET /search'],
    ['GET', '/api/v2/dashboard', 'GET /api/v2/dashboard']
  ])('%s %s is %s', (method, uri, want) => {
    expect(normalizeRoute(method, uri)).toBe(want);
  });
});

describe('routeQuery', () => {
  it('reads a search shaped like a route as that route', () => {
    expect(routeQuery('GET /users/:id')).toBe('GET /users/:id');
    expect(routeQuery(' post /import ')).toBe('POST /import');
    expect(routeQuery('GET /')).toBe('GET /');
  });

  it('leaves any other search as plain text', () => {
    expect(routeQuery('select * from users')).toBe('');
    expect(routeQuery('/users')).toBe('');
    expect(routeQuery('GET')).toBe('');
  });
});

describe('routeOf', () => {
  it('is the route a web request event ran under, and nothing for the rest', () => {
    const ev = (request?: string) => ({ ctx: { type: 'fpm', request } }) as unknown as DumpEvent;
    expect(routeOf(ev('GET /users/5?tab=1'))).toBe('GET /users/:id');
    expect(routeOf(ev(undefined))).toBe('');
  });
});

describe('routeOf a browser event', () => {
  it('reads the page it happened on as a GET of that path', () => {
    const ev = { ctx: { type: 'browser', request: 'https://shop.test/users/5?tab=1' } } as unknown as DumpEvent;
    expect(routeOf(ev)).toBe('GET /users/:id');
  });
});
