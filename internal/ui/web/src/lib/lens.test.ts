import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

const calls: string[] = [];
let answer: (path: string) => unknown = () => ({ groups: [], next: 0 });
vi.mock('$lib/api', async (orig) => ({
  ...(await orig<typeof import('$lib/api')>()),
  apiJson: (path: string) => {
    calls.push(path);
    return Promise.resolve(answer(path));
  }
}));

// A stand-in for the notify stream that a test pushes nudges through.
class FakeSource {
  static last: FakeSource | null = null;
  listeners: Array<(e: { data: string }) => void> = [];
  closed = false;
  constructor(public url: string) {
    FakeSource.last = this;
  }
  addEventListener(_: string, fn: (e: { data: string }) => void) {
    this.listeners.push(fn);
  }
  close() {
    this.closed = true;
  }
  push(n: object) {
    for (const l of this.listeners) l({ data: JSON.stringify(n) });
  }
}

import { createLens, createCounts, matches, queryString, LIVE_MS } from './lens';

const group = (key: string, rows = 1, count = rows) => ({
  key,
  count,
  total_ms: 0,
  slow_count: 0,
  n_plus_one: false,
  rows: Array.from({ length: rows }, (_, i) => ({ event: { id: `${key}-${i}` }, dup: 1 }))
});

beforeEach(() => {
  calls.length = 0;
  vi.stubGlobal('EventSource', FakeSource);
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('queryString', () => {
  it('sends only what narrows: tests when on, workers when off', () => {
    expect(queryString({ site: 'shop', q: '', tests: false, workers: true, kind: 'query' })).toBe('site=shop&kind=query');
    expect(queryString({ tests: true, workers: false })).toBe('tests=1&workers=0');
  });
});

describe('matches', () => {
  it('takes a nudge for the lens kind and site, test runs only when shown', () => {
    expect(matches({ kind: 'query', site: 'shop' }, { site: 'shop' }, 'query')).toBe(true);
    expect(matches({ kind: 'view', site: 'shop' }, { site: 'shop' }, 'query')).toBe(false);
    expect(matches({ kind: 'query', site: 'blog' }, { site: 'shop' }, 'query')).toBe(false);
    expect(matches({ kind: 'query', site: 'shop', test: true }, { site: 'shop' }, 'query')).toBe(false);
    expect(matches({ kind: 'query', site: 'shop', test: true }, { site: 'shop', tests: true }, 'query')).toBe(true);
  });
});

describe('createLens', () => {
  it('reads the first page, then pages back from where it ended', async () => {
    answer = (p) => (p.includes('before=') ? { groups: [group('b')], next: 0 } : { groups: [group('a')], next: 42 });
    const lens = createLens();
    lens.set({ kind: 'query', site: 'shop' });
    await vi.runAllTimersAsync();
    expect(get(lens.groups).map((g) => g.key)).toEqual(['a']);
    expect(get(lens.hasMore)).toBe(true);
    await lens.loadMore();
    expect(calls.at(-1)).toContain('before=42');
    expect(get(lens.groups).map((g) => g.key)).toEqual(['a', 'b']);
    expect(get(lens.hasMore)).toBe(false);
    lens.destroy();
  });

  it('reads more rows of a group from where its rows end', async () => {
    answer = (p) => (p.includes('/rows?') ? [{ event: { id: 'x' }, dup: 1 }] : { groups: [group('a', 2, 3)], next: 0 });
    const lens = createLens();
    lens.set({ kind: 'view', site: 'shop' });
    await vi.runAllTimersAsync();
    await lens.loadRows('a');
    expect(calls.at(-1)).toContain('key=a');
    expect(calls.at(-1)).toContain('offset=2');
    expect(get(lens.groups)[0].rows).toHaveLength(3);
    lens.destroy();
  });

  it('rereads at most twice a second while at the top, and counts arrivals while scrolled away', async () => {
    answer = () => ({ groups: [group('a')], next: 0 });
    const lens = createLens();
    lens.set({ kind: 'query', site: 'shop' });
    await vi.runAllTimersAsync();
    const before = calls.length;
    for (let i = 0; i < 20; i++) FakeSource.last!.push({ kind: 'query', site: 'shop' });
    await vi.advanceTimersByTimeAsync(LIVE_MS);
    expect(calls.length - before).toBe(1);

    lens.setAtTop(false);
    FakeSource.last!.push({ kind: 'query', site: 'shop' });
    FakeSource.last!.push({ kind: 'view', site: 'shop' });
    await vi.advanceTimersByTimeAsync(LIVE_MS);
    expect(get(lens.fresh)).toBe(1);
    lens.setAtTop(true);
    await vi.runAllTimersAsync();
    expect(get(lens.fresh)).toBe(0);
    lens.destroy();
  });

  it('leaves one request pinned still as events arrive', async () => {
    answer = () => ({ groups: [], next: 0 });
    FakeSource.last = null;
    const lens = createLens();
    lens.set({ kind: 'query', site: 'shop', rid: 'r1' });
    await vi.runAllTimersAsync();
    expect(FakeSource.last).toBeNull();
    lens.destroy();
  });
});

describe('createCounts', () => {
  it('reads the badges and rereads them as events arrive', async () => {
    answer = () => ({ counts: { query: 3 }, hidden_tests: 1 });
    const c = createCounts();
    c.set({ site: 'shop' });
    await vi.runAllTimersAsync();
    expect(get(c.counts)).toEqual({ query: 3 });
    expect(get(c.hiddenTests)).toBe(1);
    answer = () => ({ counts: { query: 4 }, hidden_tests: 1 });
    FakeSource.last!.push({ kind: 'query', site: 'shop' });
    await vi.runAllTimersAsync();
    expect(get(c.counts)).toEqual({ query: 4 });
    c.destroy();
  });
});
