import { describe, it, expect, vi, afterEach } from 'vitest';
import { apiUrl, wsUrl, apiFetch } from './api';

describe('apiUrl', () => {
  it('passes absolute URLs through', () => {
    expect(apiUrl('https://example.com/foo')).toBe('https://example.com/foo');
    expect(apiUrl('http://x/y')).toBe('http://x/y');
  });

  it('prepends apiBase only when non-empty', () => {
    // default hostname in jsdom is "localhost", so apiBase is ''
    expect(apiUrl('/api/version')).toBe('/api/version');
  });
});

describe('wsUrl', () => {
  it('rewrites http to ws', () => {
    const u = wsUrl('/api/ws');
    expect(u.startsWith('ws://') || u.startsWith('wss://')).toBe(true);
    expect(u.endsWith('/api/ws')).toBe(true);
  });
});

describe('apiFetch CSRF header', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  function captureHeaders(): () => Headers | undefined {
    let seen: Headers | undefined;
    vi.spyOn(globalThis, 'fetch').mockImplementation(((_url: unknown, init?: RequestInit) => {
      seen = new Headers(init?.headers);
      return Promise.resolve(new Response(null, { status: 204 }));
    }) as typeof fetch);
    return () => seen;
  }

  for (const method of ['POST', 'PUT', 'DELETE', 'PATCH']) {
    it(`adds X-Lerd-CSRF on ${method}`, async () => {
      const get = captureHeaders();
      await apiFetch('/api/anything', { method });
      expect(get()?.get('X-Lerd-CSRF')).toBe('1');
    });
  }

  for (const method of ['GET', 'HEAD']) {
    it(`omits X-Lerd-CSRF on ${method}`, async () => {
      const get = captureHeaders();
      await apiFetch('/api/anything', { method });
      expect(get()?.get('X-Lerd-CSRF')).toBeNull();
    });
  }

  it('default GET (no method) omits the header', async () => {
    const get = captureHeaders();
    await apiFetch('/api/anything');
    expect(get()?.get('X-Lerd-CSRF')).toBeNull();
  });

  it('preserves an explicit caller value', async () => {
    const get = captureHeaders();
    await apiFetch('/api/anything', { method: 'POST', headers: { 'X-Lerd-CSRF': 'caller-value' } });
    expect(get()?.get('X-Lerd-CSRF')).toBe('caller-value');
  });
});
