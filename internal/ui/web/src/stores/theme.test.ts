import { describe, it, expect, beforeEach, vi } from 'vitest';

// Controllable matchMedia mock: setDark flips the preference and fires 'change'.
function mockMatchMedia(initialDark: boolean) {
  let dark = initialDark;
  const listeners: Array<() => void> = [];
  const mql = {
    get matches() {
      return dark;
    },
    addEventListener: (_: string, cb: () => void) => listeners.push(cb),
    removeEventListener: () => {}
  };
  (window as unknown as { matchMedia: unknown }).matchMedia = vi.fn(() => mql);
  return {
    setDark(v: boolean) {
      dark = v;
      listeners.forEach((cb) => cb());
    }
  };
}

describe('theme store', () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.className = '';
    document.documentElement.removeAttribute('style');
    document.head.innerHTML = '';
    vi.resetModules();
  });

  it('auto follows a live system light/dark change', async () => {
    const media = mockMatchMedia(false);
    localStorage.setItem('lerd-theme', 'auto');
    const { initTheme } = await import('./theme');
    initTheme();
    expect(document.documentElement.classList.contains('dark')).toBe(false);

    media.setDark(true);
    expect(document.documentElement.classList.contains('dark')).toBe(true);

    media.setDark(false);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('paints the chosen theme onto the root element and remembers it', async () => {
    mockMatchMedia(false);
    localStorage.setItem('lerd-theme', 'light');
    const { initTheme, palette } = await import('./theme');
    initTheme();
    expect(document.documentElement.style.getPropertyValue('--lerd-accent')).toBe('#ff2d20');

    palette.set('muted');
    expect(document.documentElement.style.getPropertyValue('--lerd-accent')).toBe('#b04a42');
    expect(document.documentElement.style.getPropertyValue('--lerd-card')).toBe('#1a1a1c');
    expect(localStorage.getItem('lerd-palette')).toBe('muted');
  });

  it('swaps the accent for the tone that reads on the mode in effect', async () => {
    const media = mockMatchMedia(false);
    localStorage.setItem('lerd-theme', 'auto');
    localStorage.setItem('lerd-palette', 'muted');
    const { initTheme } = await import('./theme');
    initTheme();
    expect(document.documentElement.style.getPropertyValue('--lerd-accent')).toBe('#b04a42');

    media.setDark(true);
    expect(document.documentElement.style.getPropertyValue('--lerd-accent')).toBe('#d98d84');
  });

  it('falls back to the default when the chosen theme is gone', async () => {
    mockMatchMedia(false);
    localStorage.setItem('lerd-palette', 'deleted-by-hand');
    const { initTheme } = await import('./theme');
    initTheme();
    expect(document.documentElement.style.getPropertyValue('--lerd-accent')).toBe('#ff2d20');
  });

  it('repaints when a theme arrives from the daemon after the first paint', async () => {
    mockMatchMedia(false);
    localStorage.setItem('lerd-theme', 'light');
    localStorage.setItem('lerd-palette', 'lagoon');
    const { initTheme, palettes } = await import('./theme');
    const { BUILTIN_PALETTES, resolvePalette } = await import('$lib/palettes');
    initTheme();
    expect(document.documentElement.style.getPropertyValue('--lerd-accent')).toBe('#ff2d20');

    palettes.set([
      ...BUILTIN_PALETTES,
      resolvePalette({ id: 'lagoon', name: 'Lagoon', accent: '#3b7ea1' })!
    ]);
    expect(document.documentElement.style.getPropertyValue('--lerd-accent')).toBe('#3b7ea1');
  });

  it('paints the installed app chrome from the theme in effect', async () => {
    const media = mockMatchMedia(false);
    document.head.innerHTML =
      '<meta name="theme-color" content="#FF2D20"><link rel="manifest" href="/manifest.webmanifest">';
    localStorage.setItem('lerd-theme', 'auto');
    localStorage.setItem('lerd-palette', 'muted');
    const { initTheme } = await import('./theme');
    initTheme();

    const meta = () => document.querySelector('meta[name="theme-color"]')!.getAttribute('content');
    const manifest = () =>
      document.querySelector<HTMLLinkElement>('link[rel="manifest"]')!.getAttribute('href')!;

    expect(meta()).toBe('#b04a42');
    expect(manifest()).toContain('theme_color=%23b04a42');
    expect(manifest()).toContain('background_color=%23ffffff');

    media.setDark(true);
    expect(meta()).toBe('#d98d84');
    expect(manifest()).toContain('background_color=%23111113');
  });

  it('writes a choice back to the config, but not the one the config gave it', async () => {
    mockMatchMedia(false);
    const fetchMock = vi.fn(async () => new Response('{"ok":true}', { status: 200 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { initTheme, palette, adoptTheme } = await import('./theme');
    initTheme();

    adoptTheme('nord');
    expect(fetchMock).not.toHaveBeenCalled();

    palette.set('muted');
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0][0]).toContain('/api/settings/theme');
  });

  it('explicit dark ignores the system preference', async () => {
    const media = mockMatchMedia(false);
    localStorage.setItem('lerd-theme', 'dark');
    const { initTheme } = await import('./theme');
    initTheme();
    expect(document.documentElement.classList.contains('dark')).toBe(true);

    media.setDark(true);
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });
});
