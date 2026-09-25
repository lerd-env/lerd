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

  it('refetches the list when the daemon says the themes on offer changed', async () => {
    const fetchSpy = vi.fn(
      async () =>
        new Response(JSON.stringify({ themes: [{ id: 'omarchy', name: 'Omarchy (nord)', accent: '#81a1c1' }] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' }
        })
    );
    globalThis.fetch = fetchSpy as unknown as typeof fetch;
    const { watchThemeChanges } = await import('./palettes');
    const { wsMessage } = await import('$lib/ws');

    const stop = watchThemeChanges();
    fetchSpy.mockClear();
    wsMessage.set({ type: 'theme_list' });
    await vi.waitFor(() => expect(fetchSpy).toHaveBeenCalled());
    stop();
  });

  it('lets the desktop entry replace the built-in that imitates it', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          themes: [
            {
              id: 'plasma',
              name: 'Plasma (Breeze Dark)',
              accent: '#3dd425',
              card: '#202326',
              source: 'desktop'
            }
          ]
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    ) as unknown as typeof fetch;
    const { loadPalettes } = await import('./palettes');
    const { palettes } = await import('./theme');

    await loadPalettes();

    const breeze = get(palettes).filter((p) => p.id === 'breeze');
    expect(breeze).toHaveLength(1);
    expect(breeze[0].name).toBe('Breeze');
    expect(breeze[0].accent).toBe('#3dd425');
    expect(get(palettes).some((p) => p.id === 'plasma')).toBe(false);
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
  describe('desktop theme suggestion', () => {
    // The daemon answers the chosen theme and the list from two endpoints, and
    // the POST that records a choice from a third.
    function daemon(chosen: string) {
      return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === 'POST') return new Response('{"ok":true}', { status: 200 });
        const body = String(input).endsWith('/api/settings')
          ? { theme: chosen }
          : { themes: [{ id: 'gnome', name: 'GNOME', accent: '#3584e4', source: 'desktop' }] };
        return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
      });
    }

    async function load(chosen: string) {
      const fetchMock = daemon(chosen);
      globalThis.fetch = fetchMock as unknown as typeof fetch;
      (window as unknown as { matchMedia: unknown }).matchMedia = vi.fn(() => ({
        matches: false,
        addEventListener: () => {},
        removeEventListener: () => {}
      }));
      localStorage.clear();
      const store = await import('./palettes');
      const theme = await import('./theme');
      theme.initTheme();
      await store.loadPalettes();
      fetchMock.mockClear();
      return { ...store, ...theme, fetchMock };
    }

    const posted = (fetchMock: ReturnType<typeof daemon>) =>
      fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST').map(([, init]) => JSON.parse(String(init!.body)));

    it('offers the desktop theme while no theme was ever chosen', async () => {
      const { desktopSuggestion } = await load('');
      expect(get(desktopSuggestion)).toMatchObject({ id: 'adwaita', name: 'Adwaita' });
    });

    it('stays quiet once a theme was chosen, even the default one', async () => {
      const { desktopSuggestion } = await load('lerd');
      expect(get(desktopSuggestion)).toBeNull();
    });

    it('switches to the suggested theme and records it', async () => {
      const { desktopSuggestion, useSuggestedTheme, palette, fetchMock } = await load('');
      useSuggestedTheme();
      expect(get(palette)).toBe('adwaita');
      expect(get(desktopSuggestion)).toBeNull();
      expect(posted(fetchMock)).toEqual([{ theme: 'adwaita' }]);
    });

    it('keeps the current theme by recording it as the choice', async () => {
      const { desktopSuggestion, keepCurrentTheme, palette, fetchMock } = await load('');
      keepCurrentTheme();
      expect(get(palette)).toBe('lerd');
      expect(get(desktopSuggestion)).toBeNull();
      expect(posted(fetchMock)).toEqual([{ theme: 'lerd' }]);
    });

    it('offers it again when the config is cleared on another device', async () => {
      const { desktopSuggestion, watchThemeChanges } = await load('lerd');
      const { wsMessage } = await import('$lib/ws');
      const stop = watchThemeChanges();
      wsMessage.set({ type: 'theme', theme: '' });
      expect(get(desktopSuggestion)).toMatchObject({ id: 'adwaita' });
      stop();
    });

    it('stops offering once another device answers', async () => {
      const { desktopSuggestion, watchThemeChanges } = await load('');
      const { wsMessage } = await import('$lib/ws');
      const stop = watchThemeChanges();
      wsMessage.set({ type: 'theme', theme: 'lerd' });
      expect(get(desktopSuggestion)).toBeNull();
      stop();
    });
  });
});
