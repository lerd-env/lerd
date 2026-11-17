import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

describe('plugins store', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('reloads both plugin lists after the catalog check', async () => {
    const fetchSpy = vi.fn(
      async () =>
        new Response(JSON.stringify([{ name: 'notes', update: true }]), { status: 200, headers: { 'Content-Type': 'application/json' } })
    );
    globalThis.fetch = fetchSpy as unknown as typeof fetch;
    const { pluginUpdates, managedPlugins } = await import('./plugins');
    const { wsMessage } = await import('$lib/ws');

    wsMessage.set({ type: 'plugins' });
    await vi.waitFor(() => expect(get(pluginUpdates)).toBe(1));

    expect(get(managedPlugins)).toHaveLength(1);
  });

  it('leaves the lists alone for other messages', async () => {
    const fetchSpy = vi.fn(async () => new Response('[]', { status: 200 }));
    globalThis.fetch = fetchSpy as unknown as typeof fetch;
    await import('./plugins');
    const { wsMessage } = await import('$lib/ws');

    wsMessage.set({ type: 'notification', notification: { kind: 'update_available', data: { version: '1.38.0' } } });

    expect(fetchSpy).not.toHaveBeenCalled();
  });
});

describe('whenApplies', () => {
  it('reads the condition lerd parsed the way lerd does', async () => {
    const { whenApplies } = await import('./plugins');
    const laravel12 = { has_database: true, framework: 'laravel', framework_version: '12' };
    const legacy = { framework: 'laravel-legacy' };
    const wordpress = { has_database: true, framework: 'wordpress', framework_version: '6' };
    const plain = { has_database: true };

    expect(whenApplies(undefined, {})).toBe(true);
    expect(whenApplies({ is: 'database' }, plain)).toBe(true);
    expect(whenApplies({ is: 'database' }, legacy)).toBe(false);
    expect(whenApplies({ is: 'framework', name: 'laravel*' }, legacy)).toBe(true);
    expect(whenApplies({ is: 'framework', name: 'laravel.x' }, { framework: 'laravelax' })).toBe(false);
    expect(whenApplies({ is: 'framework', name: '*' }, plain)).toBe(false);
    expect(whenApplies({ is: 'framework', name: 'laravel', version: 12 }, laravel12)).toBe(true);
    expect(whenApplies({ is: 'framework', name: 'laravel', cmp: '<', version: 12 }, laravel12)).toBe(false);
    expect(whenApplies({ is: 'framework', name: 'laravel*', cmp: '>=', version: 1 }, legacy)).toBe(false);

    // (database and not framework:wordpress) or framework:laravel*@>=11
    const cond = {
      op: 'or' as const,
      args: [
        { op: 'and' as const, args: [{ is: 'database' as const }, { op: 'not' as const, args: [{ is: 'framework' as const, name: 'wordpress' }] }] },
        { is: 'framework' as const, name: 'laravel*', cmp: '>=' as const, version: 11 }
      ]
    };
    expect(whenApplies(cond, laravel12)).toBe(true);
    expect(whenApplies(cond, plain)).toBe(true);
    expect(whenApplies(cond, wordpress)).toBe(false);
    expect(whenApplies(cond, legacy)).toBe(false);
  });
});
