import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

describe('profiler store', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('loadProfilerStatus reads /api/profiler/status', async () => {
    globalThis.fetch = vi.fn(
      async () =>
        new Response(JSON.stringify({ enabled: true }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' }
        })
    ) as unknown as typeof fetch;
    const { profilerEnabled, loadProfilerStatus } = await import('./profiler');
    await loadProfilerStatus();
    expect(get(profilerEnabled)).toBe(true);
  });

  it('setProfiler POSTs and flips the store on success', async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ enabled: true }), { status: 200 })
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { profilerEnabled, setProfiler } = await import('./profiler');
    expect(get(profilerEnabled)).toBe(false);
    await setProfiler(true);
    expect(get(profilerEnabled)).toBe(true);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/profiler/toggle');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ enable: true }));
  });

  it('clearProfilerData POSTs and returns the removed count', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ removed: 4 }), { status: 200 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { clearProfilerData } = await import('./profiler');
    expect(await clearProfilerData()).toBe(4);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/profiler/clear');
    expect(init.method).toBe('POST');
  });

  it('clearProfilerData throws on a failed response', async () => {
    globalThis.fetch = vi.fn(
      async () => new Response('nope', { status: 500 })
    ) as unknown as typeof fetch;
    const { clearProfilerData } = await import('./profiler');
    await expect(clearProfilerData()).rejects.toThrow();
  });
});
