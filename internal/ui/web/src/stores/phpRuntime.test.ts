import { describe, it, expect, vi } from 'vitest';
import { get } from 'svelte/store';

describe('php runtime store', () => {
  it('loads the runtime and whether it applies', async () => {
    globalThis.fetch = vi.fn(
      async () =>
        new Response('{"php_runtime":"native","php_runtime_applies":true}', { status: 200 })
    ) as unknown as typeof fetch;
    const { loadPHPRuntime, phpRuntime, phpRuntimeApplies } = await import('./phpRuntime');
    await loadPHPRuntime();
    expect(get(phpRuntime)).toBe('native');
    expect(get(phpRuntimeApplies)).toBe(true);
  });

  it('treats an unknown runtime as container', async () => {
    globalThis.fetch = vi.fn(
      async () => new Response('{"php_runtime":"nonsense"}', { status: 200 })
    ) as unknown as typeof fetch;
    const { loadPHPRuntime, phpRuntime } = await import('./phpRuntime');
    await loadPHPRuntime();
    expect(get(phpRuntime)).toBe('container');
  });

  it('posts the mode and surfaces a refusal', async () => {
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: unknown, init?: RequestInit) => {
      calls.push(String(url) + ':' + String(init?.body));
      return new Response('{"ok":false,"error":"macOS only"}', { status: 200 });
    }) as unknown as typeof fetch;
    const { setPHPRuntime, phpRuntime } = await import('./phpRuntime');
    const res = await setPHPRuntime('native');
    expect(calls[0]).toContain('/api/settings/php-runtime');
    expect(calls[0]).toContain('"mode":"native"');
    expect(res.ok).toBe(false);
    // A refused switch must not leave the toggle showing the new value.
    expect(get(phpRuntime)).toBe('container');
  });
});
