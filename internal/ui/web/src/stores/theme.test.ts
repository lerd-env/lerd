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
