import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

describe('palettes store', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('adds the daemon themes to the built-in ones and keeps the reported errors', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          themes: [{ id: 'ocean', name: 'Ocean', accent: '#3b7ea1' }],
          errors: [{ file: 'broken.yaml', error: 'accent: not a hex colour' }]
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    ) as unknown as typeof fetch;
    const { loadPalettes, paletteErrors } = await import('./palettes');
    const { palettes } = await import('./theme');
    const { BUILTIN_PALETTES } = await import('$lib/palettes');

    await loadPalettes();

    expect(get(palettes)).toHaveLength(BUILTIN_PALETTES.length + 1);
    expect(get(palettes).at(-1)).toMatchObject({ id: 'ocean', accent: '#3b7ea1', source: 'user' });
    expect(get(paletteErrors)).toHaveLength(1);
  });

  it('drops a theme the daemon sent with an unusable colour', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ themes: [{ id: 'x', name: 'X', accent: 'nope' }] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { loadPalettes } = await import('./palettes');
    const { palettes } = await import('./theme');
    const { BUILTIN_PALETTES } = await import('$lib/palettes');

    await loadPalettes();

    expect(get(palettes)).toHaveLength(BUILTIN_PALETTES.length);
  });

  it('returns the daemon message when an import is refused', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ ok: false, error: 'accent: not a hex colour' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { importPalette } = await import('./palettes');

    expect(await importPalette('ocean', 'name: Ocean')).toBe('accent: not a hex colour');
  });

  it('reloads the list after an import succeeds', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') return new Response(JSON.stringify({ ok: true }), { status: 200 });
      return new Response(
        JSON.stringify({ themes: [{ id: 'ocean', name: 'Ocean', accent: '#3b7ea1' }] }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      );
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { importPalette } = await import('./palettes');
    const { palettes } = await import('./theme');

    expect(await importPalette('ocean', 'name: Ocean\naccent: "#3b7ea1"')).toBe('');
    expect(get(palettes).some((p) => p.id === 'ocean')).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
