import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';
import whenCases from '../../../../plugins/testdata/when_cases.json';
import type { Checkout, PluginCond } from './plugins';
import type { Site } from './sites';

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

// The cases lerd's own tests run, with the condition as lerd hands it over, so
// the dashboard and lerd judge every tab the same way.
describe('whenApplies', () => {
  const cases = whenCases as { when: string; cond: PluginCond | null; applies: Checkout[]; skips: Checkout[] }[];

  it.each(cases)('judges "$when" as lerd does', async ({ cond, applies, skips }) => {
    const { whenApplies } = await import('./plugins');
    for (const co of applies) expect(whenApplies(cond ?? undefined, co), JSON.stringify(co)).toBe(true);
    for (const co of skips) expect(whenApplies(cond ?? undefined, co), JSON.stringify(co)).toBe(false);
  });
});

describe('checkoutOf', () => {
  it('reads a worktree on its own major and database, and the main checkout otherwise', async () => {
    const { checkoutOf } = await import('./plugins');
    const site = {
      name: 'shop',
      framework: 'laravel',
      framework_version: '10',
      has_database: true,
      worktrees: [{ branch: 'upgrade', framework_version: '11' }]
    } as unknown as Site;
    expect(checkoutOf(site, 'upgrade')).toEqual({ has_database: undefined, framework: 'laravel', framework_version: '11' });
    expect(checkoutOf(site, '')).toBe(site);
    expect(checkoutOf(site, 'gone')).toBe(site);
  });
});
