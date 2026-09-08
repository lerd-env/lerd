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
          themes: [{ id: 'lagoon', name: 'Lagoon', accent: '#3b7ea1' }],
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
    expect(get(palettes).at(-1)).toMatchObject({ id: 'lagoon', accent: '#3b7ea1', source: 'user' });
    expect(get(paletteErrors)).toHaveLength(1);
  });

  it('lets a file replace the built-in it is named after', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ themes: [{ id: 'nord', name: 'My Nord', accent: '#112233' }] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { loadPalettes } = await import('./palettes');
    const { palettes } = await import('./theme');
    const { BUILTIN_PALETTES } = await import('$lib/palettes');

    await loadPalettes();

    const nord = get(palettes).filter((p) => p.id === 'nord');
    expect(nord).toHaveLength(1);
    expect(nord[0].name).toBe('My Nord');
    expect(get(palettes)).toHaveLength(BUILTIN_PALETTES.length);
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

    expect(await importPalette('lagoon', 'name: Lagoon')).toBe('accent: not a hex colour');
  });

  it('reloads the list after an import succeeds', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') return new Response(JSON.stringify({ ok: true }), { status: 200 });
      return new Response(
        JSON.stringify({ themes: [{ id: 'lagoon', name: 'Lagoon', accent: '#3b7ea1' }] }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      );
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { importPalette } = await import('./palettes');
    const { palettes } = await import('./theme');

    expect(await importPalette('lagoon', 'name: Lagoon\naccent: "#3b7ea1"')).toBe('');
    expect(get(palettes).some((p) => p.id === 'lagoon')).toBe(true);
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(true);
  });

  it('adopts a theme another device switched to, without writing it back', async () => {
    const fetchMock = vi.fn(async () => new Response('{"ok":true}', { status: 200 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    // initTheme paints, so it needs the media query the jsdom default lacks.
    (window as unknown as { matchMedia: unknown }).matchMedia = vi.fn(() => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {}
    }));
    const { watchThemeChanges } = await import('./palettes');
    const { initTheme, palette } = await import('./theme');
    const { wsMessage } = await import('$lib/ws');
    initTheme();
    const stop = watchThemeChanges();

    wsMessage.set({ type: 'theme', theme: 'gruvbox' });

    expect(get(palette)).toBe('gruvbox');
    expect(fetchMock).not.toHaveBeenCalled();
    stop();
  });
});
