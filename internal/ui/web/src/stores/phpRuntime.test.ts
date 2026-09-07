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

  it('asks for the images to be removed only when told to', async () => {
    const bodies: string[] = [];
    globalThis.fetch = vi.fn(async (_url: unknown, init?: RequestInit) => {
      bodies.push(String(init?.body));
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const { setPHPRuntime } = await import('./phpRuntime');

    await setPHPRuntime('native');
    expect(bodies[0]).toContain('"remove_images":false');

    await setPHPRuntime('native', true);
    expect(bodies[1]).toContain('"remove_images":true');
  });

  // The reclaim runs after the switch has already happened, so it has to be
  // reported on its own rather than turning a completed switch into a failure.
  it('reports a failed reclaim separately from the switch', async () => {
    globalThis.fetch = vi.fn(
      async () =>
        new Response('{"ok":true,"images_removed":2,"images_error":"image is in use"}', {
          status: 200
        })
    ) as unknown as typeof fetch;
    const { setPHPRuntime } = await import('./phpRuntime');
    const res = await setPHPRuntime('native', true);
    expect(res.ok).toBe(true);
    expect(res.imagesRemoved).toBe(2);
    expect(res.imagesError).toBe('image is in use');
  });
});
