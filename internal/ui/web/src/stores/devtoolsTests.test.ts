import { describe, it, expect, vi, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { devtoolsStatus, toggleDevtoolsTests } from './queries';
import { showTests } from './debugLens';

describe('show test runs', () => {
  const realFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('follows the server setting', () => {
    devtoolsStatus.set({ enabled: true, workers: false, tests: true });
    expect(get(showTests)).toBe(true);
    devtoolsStatus.set({ enabled: true, workers: false, tests: false });
    expect(get(showTests)).toBe(false);
  });

  it('posts the switch so the receiver stops recording', async () => {
    const calls: [string, RequestInit | undefined][] = [];
    globalThis.fetch = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      calls.push([String(url), init]);
      return new Response(JSON.stringify({ enabled: true, workers: false, tests: true }), { status: 200 });
    }) as typeof fetch;
    await toggleDevtoolsTests(true);
    expect(get(showTests)).toBe(true);
    const post = calls.find(([u]) => u.endsWith('/api/devtools/tests'));
    expect(post?.[1]?.method).toBe('POST');
    expect(post?.[1]?.body).toBe(JSON.stringify({ enable: true }));
  });
});
