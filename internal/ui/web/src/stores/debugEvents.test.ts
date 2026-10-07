import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { DumpEvent } from '$lib/dumpsStream';
import { dumps } from './dumps';
import { showTests } from './debugLens';
import { debugEvents, hiddenTestCount, countKinds, buildKindGroups, isPageView, ofRequest, requestEvents } from './debugEvents';

function ev(id: string, test = false): DumpEvent {
  return {
    v: 1,
    id,
    ts: '2026-07-21T10:00:00.000Z',
    kind: 'dump',
    ctx: { type: 'cli', site: 'acme', test: test || undefined },
    src: { file: '/app/Http/Kernel.php', line: 10 },
    text: id
  };
}

describe('debugEvents', () => {
  beforeEach(() => {
    dumps.set([ev('a'), ev('t1', true), ev('b'), ev('t2', true)]);
    showTests.set(false);
  });

  it('hides test-run events by default', () => {
    expect(get(debugEvents).map((e) => e.id)).toEqual(['a', 'b']);
    expect(get(hiddenTestCount)).toBe(2);
  });

  it('includes them once the toggle is on', () => {
    showTests.set(true);
    expect(get(debugEvents).map((e) => e.id)).toEqual(['a', 't1', 'b', 't2']);
    expect(get(hiddenTestCount)).toBe(0);
  });

  it('keeps the tab counters agreeing with the visible list', () => {
    expect(countKinds(get(debugEvents), 'acme')['dump']).toBe(2);
    showTests.set(true);
    expect(countKinds(get(debugEvents), 'acme')['dump']).toBe(4);
  });
});

function job(id: string, status: string, worker = ''): DumpEvent {
  return {
    v: 1,
    id,
    ts: '2026-07-21T10:00:0' + id.slice(-1) + '.000Z',
    kind: 'job',
    ctx: { type: 'cli', site: 'acme', rid: id, worker: worker || undefined },
    src: { file: '/app/Jobs/SendInvoice.php', line: 20 },
    data: { class: 'App\\Jobs\\SendInvoice', status }
  };
}

function log(id: string, level: string): DumpEvent {
  return {
    v: 1,
    id,
    ts: '2026-07-21T10:00:0' + id.slice(-1) + '.000Z',
    kind: 'log',
    ctx: { type: 'fpm', site: 'acme', rid: id },
    src: { file: '/app/Http/Controllers/X.php', line: 30 },
    data: { level, channel: 'app', message: 'the gateway refused' }
  };
}

describe('buildKindGroups', () => {
  it('keeps a worker\'s jobs on screen with worker capture off', () => {
    const events = [job('1', 'processed', 'queue:work'), job('2', 'failed', 'queue:work')];
    expect(buildKindGroups(events, 'job', '', '', false, '', false)).toHaveLength(2);
  });

  it('still hides a worker\'s other events with worker capture off', () => {
    const query = { ...job('3', 'processed', 'queue:work'), kind: 'query' } as DumpEvent;
    expect(buildKindGroups([query], 'query', '', '', false, '', false)).toHaveLength(0);
  });

  it('narrows to one job status when the filter is set', () => {
    const events = [job('4', 'processing'), job('5', 'failed')];
    const groups = buildKindGroups(events, 'job', '', '', false, '', true, 'failed');
    expect(groups).toHaveLength(1);
    expect(groups[0].events[0].id).toBe('5');
  });

  it('narrows to one log level with the same filter', () => {
    const events = [log('6', 'debug'), log('7', 'error')];
    const groups = buildKindGroups(events, 'log', '', '', false, '', true, 'error');
    expect(groups).toHaveLength(1);
    expect(groups[0].events[0].id).toBe('7');
  });
});

describe('browser type filter', () => {
  const browser = (id: string, data: Record<string, unknown>): DumpEvent => ({
    v: 1,
    id,
    ts: '2026-07-21T10:00:00.000Z',
    kind: 'browser',
    ctx: { type: 'browser', site: 'acme', rid: 'p1' },
    src: { file: '', line: 0 },
    data
  });
  const events = [
    browser('a', { type: 'navigation', nav: 'load' }),
    browser('b', { type: 'console', level: 'error' }),
    browser('c', { type: 'console', level: 'warn' }),
    browser('d', { type: 'error' })
  ];

  it('splits console messages by level', () => {
    const ids = (facet: string) => buildKindGroups(events, 'browser', '', '', false, '', true, facet).flatMap((g) => g.events.map((e) => e.id));
    expect(ids('console.warn')).toEqual(['c']);
    expect(ids('error')).toEqual(['d']);
    expect(ids('navigation')).toEqual(['a']);
  });
});

describe('browser page views', () => {
  const browser = (id: string, type: string): DumpEvent =>
    ({ v: 1, id, ts: '2026-07-21T10:00:00.000Z', kind: 'browser', ctx: { type: 'browser', site: 'acme', rid: 'p1' }, src: {}, data: { type, message: id } }) as unknown as DumpEvent;

  it('counts what happened on a page, not the page loads', () => {
    expect(countKinds([browser('load', 'navigation'), browser('boom', 'error')], 'acme')['browser']).toBe(1);
  });

  it('recognises a page view', () => {
    expect(isPageView(browser('load', 'navigation'))).toBe(true);
    expect(isPageView(browser('boom', 'error'))).toBe(false);
  });
});

describe('one request', () => {
  const at = (id: string, kind: string, ctx: Record<string, unknown>, data: unknown = {}) =>
    ({ v: 1, id, ts: `2026-10-07T10:00:0${id}.000Z`, kind, ctx: { site: 'acme', ...ctx }, src: {}, data }) as unknown as DumpEvent;

  it('holds what it ran and the browser failures that reached it', () => {
    expect(ofRequest(at('1', 'query', { type: 'fpm', rid: 'r1' }), 'r1')).toBe(true);
    expect(ofRequest(at('2', 'browser', { type: 'browser', rid: 'page' }, { type: 'network', rid: 'r1' }), 'r1')).toBe(true);
    expect(ofRequest(at('3', 'query', { type: 'fpm', rid: 'r2' }), 'r1')).toBe(false);
  });

});

describe('a pinned request', () => {
  const at = (id: string, test = false) =>
    ({ v: 1, id, ts: `2026-10-07T10:00:0${id}.000Z`, kind: 'query', ctx: { type: 'fpm', site: 'acme', rid: 'r1', test }, src: {}, data: {} }) as unknown as DumpEvent;

  it('starts from what the server kept and adds what the stream brought since', () => {
    const got = requestEvents([at('2'), at('3')], 'r1', [at('1'), at('2'), at('4', true)], false);
    expect(got.map((e) => e.id)).toEqual(['1', '2', '3']);
  });
});

describe('route scope', () => {
  it('keeps only the route asked for, so the home page does not match every path', async () => {
    const { lensRouteFilter } = await import('./debugEvents');
    const q = (id: string, request: string) => ({ v: 1, id, ts: '2026-10-07T10:00:00Z', kind: 'query', ctx: { type: 'fpm', site: 'acme', request }, src: {} }) as DumpEvent;
    const events = [q('a', 'GET /'), q('b', 'GET /users/5'), q('c', 'GET /?page=2'), q('d', 'POST /')];
    expect(lensRouteFilter(events, 'GET /').map((e) => e.id)).toEqual(['a', 'c']);
    expect(lensRouteFilter(events, '').map((e) => e.id)).toEqual(['a', 'b', 'c', 'd']);
  });
});
