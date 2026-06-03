import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

describe('horizonReload store', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('loads horizon_reload from /api/settings', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ horizon_reload: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { horizonReload, loadHorizonReload } = await import('./horizonReload');
    await loadHorizonReload();
    expect(get(horizonReload)).toBe(true);
  });

  it('setHorizonReload POSTs and flips the store on success', async () => {
    const fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ ok: true, horizon_reload: true }), { status: 200 })
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { horizonReload, setHorizonReload } = await import('./horizonReload');
    expect(get(horizonReload)).toBe(false);

    const r = await setHorizonReload(true);
    expect(r.ok).toBe(true);
    expect(get(horizonReload)).toBe(true);

    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/settings/horizon-reload');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ enabled: true }));
  });

  it('does not flip the store on failure', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response('nope', { status: 500 })
    ) as unknown as typeof fetch;
    const { horizonReload, setHorizonReload } = await import('./horizonReload');
    const r = await setHorizonReload(true);
    expect(r.ok).toBe(false);
    expect(get(horizonReload)).toBe(false);
  });
});
