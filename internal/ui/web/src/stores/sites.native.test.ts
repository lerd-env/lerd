import { describe, it, expect, vi } from 'vitest';

describe('native runtime', () => {
  it('setSiteRuntime posts the target', async () => {
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: unknown) => {
      calls.push(String(url));
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const { setSiteRuntime } = await import('./sites');
    await setSiteRuntime({ domain: 'a.test' }, 'native');
    await setSiteRuntime({ domain: 'a.test' }, 'fpm');
    expect(calls[0]).toBe('/api/sites/a.test/runtime?target=native');
    expect(calls[1]).toBe('/api/sites/a.test/runtime?target=fpm');
  });

  it('surfaces a refusal from the daemon', async () => {
    globalThis.fetch = vi.fn(
      async () => new Response('{"error":"macOS only"}', { status: 200 })
    ) as unknown as typeof fetch;
    const { setSiteRuntime } = await import('./sites');
    const res = await setSiteRuntime({ domain: 'a.test' }, 'native');
    expect(res.ok).toBe(false);
    expect(res.error).toContain('macOS');
  });

  // A native site has no FPM container, so anything keyed on one must not
  // invent a name that does not exist.
  it('a native site reports no fpm container and its own label', async () => {
    const { fpmContainer, fpmTabLabel } = await import('./sites');
    expect(fpmContainer({ domain: 'a.test', name: 'a', runtime: 'native', php_version: '8.4' })).toBe('');
    expect(fpmTabLabel({ domain: 'a.test', runtime: 'native' })).toBe('Native PHP');
  });
});
