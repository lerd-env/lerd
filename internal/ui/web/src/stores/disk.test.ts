import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

describe('disk store', () => {
  const realFetch = globalThis.fetch;
  let calls: { url: string; method: string }[];

  beforeEach(() => {
    vi.resetModules();
    calls = [];
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, method: (init?.method ?? 'GET').toUpperCase() });
      const body =
        (init?.method ?? 'GET').toUpperCase() === 'POST'
          ? { ok: true, removed: 3, reclaimed_bytes: 4096 }
          : { available: true, reclaimable_bytes: 512, images: [], held_bytes: 0, held_count: 0 };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      });
    }) as unknown as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('loadDisk fetches the preview and marks it loaded', async () => {
    const { loadDisk, disk, diskLoaded, get } = await importDisk();
    await loadDisk();
    expect(calls[0].url).toContain('/api/disk');
    expect(calls[0].method).toBe('GET');
    expect(get(disk).reclaimable_bytes).toBe(512);
    expect(get(diskLoaded)).toBe(true);
  });

  it('runCleanup POSTs, reports freed space, then refreshes the preview', async () => {
    const { runCleanup } = await importDisk();
    const res = await runCleanup();
    expect(res.ok).toBe(true);
    expect(res.removed).toBe(3);
    expect(res.reclaimedBytes).toBe(4096);
    // A POST to reclaim, then a follow-up GET to refresh the snapshot.
    expect(calls[0].method).toBe('POST');
    expect(calls[1].method).toBe('GET');
  });
});

async function importDisk() {
  const mod = await import('./disk');
  const { get } = await import('svelte/store');
  return { ...mod, get };
}
