import { describe, it, expect, vi, afterEach } from 'vitest';
import { get } from 'svelte/store';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});

function stubMedia(matches: boolean) {
  const listeners: Array<(e: { matches: boolean }) => void> = [];
  vi.stubGlobal('matchMedia', (q: string) => ({
    matches,
    media: q,
    addEventListener: (_: string, fn: (e: { matches: boolean }) => void) => listeners.push(fn),
    removeEventListener: () => {}
  }));
  return (m: boolean) => listeners.forEach((l) => l({ matches: m }));
}

describe('isDesktop', () => {
  it('follows the md breakpoint as the window crosses it', async () => {
    const cross = stubMedia(false);
    const { isDesktop } = await import('./media');
    const seen: boolean[] = [];
    const stop = isDesktop.subscribe((v) => seen.push(v));
    expect(get(isDesktop)).toBe(false);
    cross(true);
    expect(seen.at(-1)).toBe(true);
    stop();
  });
});
