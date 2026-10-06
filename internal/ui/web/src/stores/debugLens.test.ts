import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

describe('debugLens', () => {
  beforeEach(() => {
    localStorage.clear();
    // Re-import fresh each test so initial() re-reads localStorage.
    vi.resetModules();
  });

  it('defaults to dumps with no stored value', async () => {
    const { debugLens } = await import('./debugLens');
    expect(get(debugLens)).toBe('dumps');
  });

  it('restores a previously stored lens', async () => {
    localStorage.setItem('lerd:debugLens', 'queries');
    const { debugLens } = await import('./debugLens');
    expect(get(debugLens)).toBe('queries');
  });

  it('persists changes to localStorage', async () => {
    const { debugLens } = await import('./debugLens');
    debugLens.set('queries');
    expect(localStorage.getItem('lerd:debugLens')).toBe('queries');
  });
});

describe('isDebugLens', () => {
  it('accepts a lens a deep link can open', async () => {
    const { isDebugLens } = await import('./debugLens');
    expect(isDebugLens('messages')).toBe(true);
    expect(isDebugLens('queries')).toBe(true);
  });

  it('rejects anything else, so a stray segment leaves the lens alone', async () => {
    const { isDebugLens } = await import('./debugLens');
    expect(isDebugLens('nginx')).toBe(false);
    expect(isDebugLens('')).toBe(false);
    expect(isDebugLens(undefined)).toBe(false);
  });

  it('orders the lens tabs by category, dumps first, then errors, the request, and background work', async () => {
    const { debugLensTabs } = await import('./debugLens');
    const tabs = debugLensTabs({ exception: 2 }, true);
    expect(tabs.map((t) => t.id)).toEqual(['dumps', 'exceptions', 'logs', 'browser', 'queries', 'views', 'cache', 'http', 'jobs', 'messages', 'events', 'mail']);
    expect(new Set(tabs.map((t) => t.group)).size).toBe(4);
    expect(tabs.find((t) => t.id === 'exceptions')?.count).toBe(2);
  });

  it('hides cache when no Laravel site feeds it', async () => {
    const { debugLensTabs } = await import('./debugLens');
    expect(debugLensTabs({}, false).find((t) => t.id === 'cache')?.hidden).toBe(true);
  });
});
