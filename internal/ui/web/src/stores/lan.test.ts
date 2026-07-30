import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { get } from 'svelte/store';
import { lan, loadLANStatus, toggleLANServices } from './lan';

describe('LAN store managed-service exposure', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    lan.update((state) => ({
      ...state,
      exposed: false,
      servicesEnabled: false,
      servicesReachable: false,
      loaded: false,
      servicesLoading: false,
      servicesError: '',
      error: ''
    }));
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('loads the managed-service exposure setting', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          exposed: true,
          services_enabled: true,
          services_reachable: true,
          lan_ip: '192.168.1.10',
          macos: false
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    ) as unknown as typeof fetch;

    await loadLANStatus();
    expect(get(lan)).toMatchObject({
      exposed: true,
      servicesEnabled: true,
      servicesReachable: true,
      loaded: true
    });
  });

  it('toggles managed-service exposure through the LAN endpoint', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(JSON.parse(String(init?.body))).toEqual({ action: 'services_on' });
      return new Response(
        '{"step":"Saving managed service exposure"}\n{"result":"ok","services_enabled":true,"services_reachable":true}\n',
        {
          status: 200,
          headers: { 'Content-Type': 'application/x-ndjson' }
        }
      );
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await toggleLANServices(true);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(get(lan)).toMatchObject({
      servicesEnabled: true,
      servicesReachable: true,
      servicesLoading: false,
      servicesError: ''
    });
  });

  it('keeps managed-service errors on the managed-service state', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response('service bind failed', { status: 500 })
    ) as unknown as typeof fetch;
    lan.update((state) => ({ ...state, error: 'site exposure error' }));

    await toggleLANServices(true);

    expect(get(lan)).toMatchObject({
      servicesLoading: false,
      servicesError: 'service bind failed',
      error: 'site exposure error'
    });
  });
});
