import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

const realFetch = globalThis.fetch;

// A daemon holding `setup` in its config, recording what the page posts back.
function daemon(setup: string) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'POST') return new Response('{"ok":true}', { status: 200 });
    return new Response(JSON.stringify({ setup, theme: '' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
  });
}

const posted = (f: ReturnType<typeof daemon>) =>
  f.mock.calls.filter(([, init]) => init?.method === 'POST').map(([, init]) => JSON.parse(String(init!.body)).state);

async function fresh(opts: { setup?: string; sites?: number; streaming?: boolean; legacyDismissed?: boolean; desktop?: boolean } = {}) {
  vi.resetModules();
  localStorage.clear();
  if (opts.legacyDismissed) localStorage.setItem('lerd-onboarding-dismissed', '1');
  const fetchMock = daemon(opts.setup ?? '');
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  const setup = await import('./setup');
  const { sites, sitesLoaded } = await import('./sites');
  const { status } = await import('./status');
  const { services } = await import('./services');
  const { startOnDashboardOpen } = await import('./autostart');
  const { idleEnabled } = await import('./idle');
  const { permissionState, notifyDelivery } = await import('$lib/notify');
  const { palettes } = await import('./theme');
  const { configTheme } = await import('./palettes');
  const { BUILTIN_PALETTES } = await import('$lib/palettes');
  permissionState.set('default');
  notifyDelivery.set('browser');
  configTheme.set('');
  if (opts.desktop) palettes.set([...BUILTIN_PALETTES, { ...BUILTIN_PALETTES.find((p) => p.id === 'adwaita')!, source: 'desktop' }]);
  sites.set(Array.from({ length: opts.sites ?? 0 }, (_, i) => ({ name: `s${i}` }) as never));
  sitesLoaded.set(true);
  status.set({ streaming_mode: !!opts.streaming } as never);
  await setup.loadSetup();
  return { ...setup, sites, services, startOnDashboardOpen, idleEnabled, permissionState, notifyDelivery, configTheme, fetchMock };
}

describe('setup checklist', () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('starts on an install that has no sites yet', async () => {
    const { setupVisible } = await fresh();
    expect(get(setupVisible)).toBe(true);
  });

  // An install that already has sites is past setup, so an upgrade must not
  // swap its Lerd card for a checklist it never asked for.
  it('never starts on an install that already has sites', async () => {
    const { setupVisible } = await fresh({ sites: 2 });
    expect(get(setupVisible)).toBe(false);
  });

  it('stays away when streaming mode is what emptied the list', async () => {
    const { setupVisible } = await fresh({ streaming: true });
    expect(get(setupVisible)).toBe(false);
  });

  it('stays hidden until the daemon has said where setup stands', async () => {
    vi.resetModules();
    const { setupVisible } = await import('./setup');
    const { sitesLoaded } = await import('./sites');
    sitesLoaded.set(true);
    expect(get(setupVisible)).toBe(false);
  });

  // The config is shared, so a desktop app opened after the browser picks up
  // the checklist where the browser left it, sites and all.
  it('carries on in any window once the config says it started', async () => {
    const { setupVisible } = await fresh({ setup: 'active', sites: 1 });
    expect(get(setupVisible)).toBe(true);
  });

  it('records the start in the config', async () => {
    const { startSetup, fetchMock } = await fresh();
    startSetup();
    expect(posted(fetchMock)).toEqual(['active']);
  });

  it('records the old welcome card having been dismissed as done', async () => {
    const { setupVisible, fetchMock } = await fresh({ legacyDismissed: true });
    expect(get(setupVisible)).toBe(false);
    expect(posted(fetchMock)).toEqual(['done']);
  });

  it('follows another window finishing setup', async () => {
    const { setupVisible, watchSetup } = await fresh({ setup: 'active' });
    const { wsMessage } = await import('$lib/ws');
    const stop = watchSetup();
    globalThis.fetch = daemon('done') as unknown as typeof fetch;
    wsMessage.set({ type: 'setup' });
    await vi.waitFor(() => expect(get(setupVisible)).toBe(false));
    stop();
  });

  it('ticks each step off from the state it reads', async () => {
    const f = await fresh({ desktop: true });
    const ids = () => get(f.setupSteps).map((s) => s.id);
    const done = () => get(f.setupSteps).map((s) => s.done);
    expect(ids()).toEqual(['site', 'service', 'notify', 'theme', 'open', 'idle']);
    expect(done()).toEqual([false, false, false, false, false, false]);

    f.sites.set([{ name: 'a' } as never]);
    f.services.set([{ name: 'mysql', status: 'active' } as never]);
    f.permissionState.set('granted');
    f.configTheme.set('adwaita');
    f.startOnDashboardOpen.set(true);
    f.idleEnabled.set(true);

    expect(done()).toEqual([true, true, true, true, true, true]);
    expect(get(f.setupDone)).toBe(true);
  });

  it('counts native notifications as done', async () => {
    const f = await fresh();
    f.notifyDelivery.set('native');
    expect(get(f.setupSteps).find((s) => s.id === 'notify')?.done).toBe(true);
  });

  // With no desktop to match there is nothing to offer, so the step is left out.
  it('leaves the theme step out where there is no desktop', async () => {
    const f = await fresh();
    expect(get(f.setupSteps).map((s) => s.id)).not.toContain('theme');
  });

  // The banners ask the same two questions, so they belong to installs that
  // never went through Get started, and to nobody while or after it runs.
  it('keeps the banners for installs that never started setup', async () => {
    expect(get((await fresh({ sites: 2 })).bannersAllowed)).toBe(true);
    expect(get((await fresh()).bannersAllowed)).toBe(false);
    expect(get((await fresh({ setup: 'active', sites: 1 })).bannersAllowed)).toBe(false);
    expect(get((await fresh({ setup: 'done', sites: 1 })).bannersAllowed)).toBe(false);
  });

  it('does not count a stopped service', async () => {
    const { setupSteps, services } = await fresh();
    services.set([{ name: 'mysql', status: 'inactive' } as never]);
    expect(get(setupSteps)[1].done).toBe(false);
  });

  it('hands the slot back to the Lerd card for good once finished', async () => {
    const { setupVisible, finishSetup, fetchMock } = await fresh({ setup: 'active' });
    finishSetup();
    expect(get(setupVisible)).toBe(false);
    expect(posted(fetchMock)).toEqual(['done']);
  });
});
