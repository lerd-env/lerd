import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

describe('autostart store', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('loads from /api/settings', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ autostart_on_login: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { autostartEnabled, loadAutostart } = await import('./autostart');
    await loadAutostart();
    expect(get(autostartEnabled)).toBe(true);
  });

  it('toggleAutostart POSTs and flips store on success', async () => {
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { autostartEnabled, toggleAutostart } = await import('./autostart');
    expect(get(autostartEnabled)).toBe(false);
    const ok = await toggleAutostart(true);
    expect(ok).toBe(true);
    expect(get(autostartEnabled)).toBe(true);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/settings/autostart');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ enabled: true }));
  });

  it('loads start_on_dashboard_open from /api/settings', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ start_on_dashboard_open: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { startOnDashboardOpen, loadAutostart } = await import('./autostart');
    await loadAutostart();
    expect(get(startOnDashboardOpen)).toBe(true);
  });

  it('toggleStartOnDashboardOpen POSTs and flips store on success', async () => {
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { startOnDashboardOpen, toggleStartOnDashboardOpen } = await import('./autostart');
    expect(await toggleStartOnDashboardOpen(true)).toBe(true);
    expect(get(startOnDashboardOpen)).toBe(true);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/settings/start-on-open');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ enabled: true }));
  });

  it('keeps the tray on when the API omits the field, as older backends do', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ autostart_on_login: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { trayEnabled, loadAutostart } = await import('./autostart');
    await loadAutostart();
    expect(get(trayEnabled)).toBe(true);
  });

  it('loads tray_enabled from /api/settings', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ tray_enabled: false }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { trayEnabled, loadAutostart } = await import('./autostart');
    await loadAutostart();
    expect(get(trayEnabled)).toBe(false);
  });

  it('toggleTray POSTs and flips store on success', async () => {
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { trayEnabled, toggleTray } = await import('./autostart');
    expect(await toggleTray(false)).toBe(true);
    expect(get(trayEnabled)).toBe(false);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/settings/tray');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ enabled: false }));
  });

  it('does not flip on failure', async () => {
    globalThis.fetch = vi.fn(async () => new Response('nope', { status: 500 })) as unknown as typeof fetch;
    const { autostartEnabled, toggleAutostart } = await import('./autostart');
    const ok = await toggleAutostart(true);
    expect(ok).toBe(false);
    expect(get(autostartEnabled)).toBe(false);
  });

  it('loads beta_updates from /api/settings', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ beta_updates: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;
    const { betaUpdates, loadAutostart } = await import('./autostart');
    await loadAutostart();
    expect(get(betaUpdates)).toBe(true);
  });

  it('toggleBetaUpdates POSTs and flips store on success', async () => {
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { betaUpdates, toggleBetaUpdates } = await import('./autostart');
    expect(get(betaUpdates)).toBe(false);
    const ok = await toggleBetaUpdates(true);
    expect(ok).toBe(true);
    expect(get(betaUpdates)).toBe(true);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/settings/beta-updates');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ enabled: true }));
  });
});
