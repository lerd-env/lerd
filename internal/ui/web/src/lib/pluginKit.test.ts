import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// The boilerplate's ui/lerd.js, which every plugin page loads, run as a page would.
const kit = readFileSync(resolve(__dirname, '../../../../plugins/skeleton/ui/lerd.js'), 'utf8');
type Kit = { maximize(on: boolean): Promise<boolean> };

describe('lerd.maximize', () => {
  let lerd: Kit;
  const answer = (on: boolean) =>
    window.dispatchEvent(new MessageEvent('message', { source: window.parent, data: { type: 'lerd:maximized', on } }));

  beforeEach(() => {
    vi.useFakeTimers();
    globalThis.ResizeObserver ??= class {
      observe() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
    vi.spyOn(window.parent, 'postMessage').mockImplementation(() => {});
    lerd = new Function(kit + '\nreturn window.lerd;')() as Kit;
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('resolves with where the page ended up', async () => {
    const asked = lerd.maximize(true);
    answer(true);
    await expect(asked).resolves.toBe(true);
  });

  // The first call's timer used to clear the second call's wait, so an answer
  // arriving after it was dropped.
  it('a superseded call does not cost the newer one its answer', async () => {
    const first = lerd.maximize(true).catch((e: Error) => e.message);
    vi.advanceTimersByTime(1000);
    const second = lerd.maximize(false);
    await expect(first).resolves.toBe('superseded');
    vi.advanceTimersByTime(600);
    answer(false);
    vi.advanceTimersByTime(2000);
    await expect(second).resolves.toBe(false);
  });

  it('rejects when no lerd answers', async () => {
    const asked = lerd.maximize(true);
    vi.advanceTimersByTime(1600);
    await expect(asked).rejects.toThrow('cannot maximize');
  });
});
