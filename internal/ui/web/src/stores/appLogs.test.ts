import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

describe('appLogs store', () => {
  const realFetch = globalThis.fetch;
  let calls: string[];

  beforeEach(() => {
    vi.resetModules();
    calls = [];
    globalThis.fetch = vi.fn(async (url: string) => {
      calls.push(url);
      return new Response(JSON.stringify({ files: [], entries: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      });
    }) as unknown as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('listAppLogFiles omits ?branch when not provided', async () => {
    const { listAppLogFiles } = await import('./appLogs');
    await listAppLogFiles('acme.test');
    expect(calls).toHaveLength(1);
    expect(calls[0]).toContain('/api/app-logs/acme.test');
    expect(calls[0]).not.toContain('branch=');
  });

  it('listAppLogFiles includes ?branch= when provided', async () => {
    const { listAppLogFiles } = await import('./appLogs');
    await listAppLogFiles('acme.test', 'feat-a');
    expect(calls[0]).toMatch(/[?&]branch=feat-a(&|$)/);
  });

  it('loadAppLogEntries omits branch when blank', async () => {
    const { loadAppLogEntries } = await import('./appLogs');
    await loadAppLogEntries('acme.test', 'laravel.log', 0);
    expect(calls[0]).toContain('limit=200');
    expect(calls[0]).toContain('offset=0');
    expect(calls[0]).not.toContain('branch=');
  });

  it('loadAppLogEntries asks for the page at the offset, with the branch', async () => {
    const { loadAppLogEntries } = await import('./appLogs');
    await loadAppLogEntries('acme.test', 'laravel.log', 400, 'feat-a');
    expect(calls[0]).toContain('offset=400');
    expect(calls[0]).toMatch(/[?&]branch=feat-a(&|$)/);
  });

  it('loadAppLogEntries url-encodes filenames containing dots', async () => {
    const { loadAppLogEntries } = await import('./appLogs');
    await loadAppLogEntries('acme.test', 'laravel-2026-05-02.log', 0, 'feat-a');
    expect(calls[0]).toContain('laravel-2026-05-02.log');
  });
});

describe('mergeNewest', () => {
  const e = (message: string) => ({ date: '2026-10-01', level: 'INFO', message });

  it('puts only the new entries in front and keeps the history loaded', async () => {
    const { mergeNewest } = await import('./appLogs');
    const r = mergeNewest([e('b'), e('a')], [e('d'), e('c'), e('b')]);
    expect(r.entries.map((x) => x.message)).toEqual(['d', 'c', 'b', 'a']);
    expect(r.replaced).toBe(false);
  });

  it('replaces the list when the page no longer reaches what is loaded', async () => {
    const { mergeNewest } = await import('./appLogs');
    const r = mergeNewest([e('b'), e('a')], [e('z'), e('y')]);
    expect(r.entries.map((x) => x.message)).toEqual(['z', 'y']);
    expect(r.replaced).toBe(true);
  });
});
