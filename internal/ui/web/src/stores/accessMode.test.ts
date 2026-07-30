import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

describe('accessMode store', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('defaults to loopback peer without assuming local control', async () => {
    const { accessMode } = await import('./accessMode');
    expect(get(accessMode)).toEqual({
      localControl: false,
      lanExposed: false,
      checked: false
    });
  });

  it('maps API response', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ local_control: true, lan_exposed: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { accessMode, loadAccessMode } = await import('./accessMode');
    await loadAccessMode();
    expect(get(accessMode)).toEqual({
      localControl: true,
      lanExposed: true,
      checked: true
    });
  });

  it('marks checked even on error', async () => {
    globalThis.fetch = vi.fn(async () => {
      throw new Error('down');
    }) as unknown as typeof fetch;
    const { accessMode, loadAccessMode } = await import('./accessMode');
    await loadAccessMode();
    expect(get(accessMode).checked).toBe(true);
  });
});
