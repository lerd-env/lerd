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

  it('captureCount asks for one host and route and survives a failure', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ count: 3 }), { status: 200 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { captureCount } = await import('./profiler');
    expect(await captureCount('acme.test', 'GET /users/:id')).toBe(3);
    const [url] = fetchMock.mock.calls[0] as unknown as [string];
    expect(url).toBe('/api/profiler/captures?host=acme.test&route=GET+%2Fusers%2F%3Aid');

    globalThis.fetch = vi.fn(async () => new Response('nope', { status: 500 })) as unknown as typeof fetch;
    expect(await captureCount('acme.test', 'GET /users/:id')).toBe(0);
  });

  it('waitForCapture resolves once the count rises, and gives up if it never does', async () => {
    let count = 2;
    globalThis.fetch = vi.fn(
      async () => new Response(JSON.stringify({ count }), { status: 200 })
    ) as unknown as typeof fetch;
    const { waitForCapture } = await import('./profiler');

    const landing = waitForCapture('acme.test', 'GET /users/:id', 2, 1000, 5);
    setTimeout(() => (count = 3), 20);
    expect(await landing).toBe(true);

    // A request the profiler never saw leaves the count where it was.
    expect(await waitForCapture('acme.test', 'GET /users/:id', 3, 30, 5)).toBe(false);
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
