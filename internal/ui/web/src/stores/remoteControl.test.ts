import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { get } from 'svelte/store';
import { remoteControl, loadRemoteControl, setRemoteFullAccess } from './remoteControl';

describe('remote control host-action opt-in', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    remoteControl.set({
      enabled: false,
      username: '',
      fullAccess: false,
      fullAccessLoading: false,
      loading: false,
      error: ''
    });
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('loads the host-action setting alongside the credentials', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ enabled: true, username: 'alice', full_access: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;

    await loadRemoteControl();
    expect(get(remoteControl)).toMatchObject({ enabled: true, username: 'alice', fullAccess: true });
  });

  it('defaults to local-only when the backend omits the setting', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ enabled: true, username: 'alice' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    ) as unknown as typeof fetch;

    await loadRemoteControl();
    expect(get(remoteControl).fullAccess).toBe(false);
  });

  it('posts the full-access action and stores what the backend confirms', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(JSON.parse(String(init?.body))).toEqual({ action: 'full-access', enabled: true });
      return new Response(JSON.stringify({ ok: true, full_access: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      });
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    expect(await setRemoteFullAccess(true)).toBe(true);
    expect(fetchMock).toHaveBeenCalled();
    expect(get(remoteControl)).toMatchObject({ fullAccess: true, fullAccessLoading: false });
  });

  it('surfaces a rejection without flipping the toggle', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response('Forbidden — remote full access can only be changed from the lerd host.', { status: 403 })
    ) as unknown as typeof fetch;

    expect(await setRemoteFullAccess(true)).toBe(false);
    const state = get(remoteControl);
    expect(state.fullAccess).toBe(false);
    expect(state.fullAccessLoading).toBe(false);
    expect(state.error).toContain('only be changed from the lerd host');
  });
});
