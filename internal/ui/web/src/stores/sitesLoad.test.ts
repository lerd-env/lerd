import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { get } from 'svelte/store';
import { sites, sitesLoaded, loadSites } from './sites';

describe('loadSites recovery', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.useFakeTimers();
    sites.set([]);
    sitesLoaded.set(false);
  });

  afterEach(() => {
    vi.useRealTimers();
    globalThis.fetch = realFetch;
  });

  function respond(...responses: Array<() => Response>) {
    let i = 0;
    const mock = vi.fn(async () => responses[Math.min(i++, responses.length - 1)]());
    globalThis.fetch = mock as unknown as typeof fetch;
    return mock;
  }

  const ok = () =>
    new Response(JSON.stringify([{ name: 'acme' }]), {
      status: 200,
      headers: { 'Content-Type': 'application/json' }
    });
  const unavailable = () => new Response('sites are temporarily unavailable', { status: 503 });

  // The cold-cache window is short, so a retry is all it takes. Without one the
  // list stays empty until an unrelated websocket event arrives.
  it('retries until it has a list to show', async () => {
    const mock = respond(unavailable, ok);

    await loadSites();
    expect(get(sitesLoaded)).toBe(false);

    await vi.advanceTimersByTimeAsync(600);
    expect(mock).toHaveBeenCalledTimes(2);
    expect(get(sitesLoaded)).toBe(true);
    expect(get(sites)).toHaveLength(1);
  });

  it('gives up after the last delay instead of retrying forever', async () => {
    const mock = respond(unavailable);

    await loadSites();
    await vi.advanceTimersByTimeAsync(10_000);

    expect(mock).toHaveBeenCalledTimes(4); // initial attempt plus three retries
    expect(get(sitesLoaded)).toBe(false);
  });

  // Once a list has been shown, a later blip must not blank it or start
  // chasing the endpoint.
  it('leaves an already loaded list alone when a refresh fails', async () => {
    respond(ok);
    await loadSites();
    expect(get(sites)).toHaveLength(1);

    const mock = respond(unavailable);
    await loadSites();
    await vi.advanceTimersByTimeAsync(10_000);

    expect(mock).toHaveBeenCalledTimes(1);
    expect(get(sites)).toHaveLength(1);
    expect(get(sitesLoaded)).toBe(true);
  });
});
