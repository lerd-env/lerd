import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { onFallbackOrigin, handOverToVhost, VHOST_URL } from './vhost';

const realLocation = window.location;
const realFetch = globalThis.fetch;

function opaque(): Response {
  return { type: 'opaque', ok: false, status: 0 } as Response;
}

function at(hostname: string, port: string, hash = '') {
  const href = { hostname, port, hash, href: `http://${hostname}:${port}/${hash}` };
  Object.defineProperty(window, 'location', { value: href, writable: true, configurable: true });
}

describe('vhost handover', () => {
  beforeEach(() => {
    // A real cross-origin no-cors reply from nginx is opaque.
    globalThis.fetch = vi.fn(async () => opaque()) as unknown as typeof fetch;
  });

  afterEach(() => {
    Object.defineProperty(window, 'location', { value: realLocation, writable: true, configurable: true });
    globalThis.fetch = realFetch;
  });

  it('recognises the loopback fallback', () => {
    at('127.0.0.1', '7073');
    expect(onFallbackOrigin()).toBe(true);
    at('localhost', '7073');
    expect(onFallbackOrigin()).toBe(true);
  });

  it('is not the fallback on the vhost', () => {
    at('lerd.localhost', '');
    expect(onFallbackOrigin()).toBe(false);
  });

  // A LAN session is served on the same port from the host's own IP, and
  // lerd.localhost does not resolve on the visiting machine.
  it('is not the fallback for a LAN session on the same port', () => {
    at('192.168.1.20', '7073');
    expect(onFallbackOrigin()).toBe(false);
  });

  it('moves to the vhost once it answers', async () => {
    at('127.0.0.1', '7073');
    await handOverToVhost();
    expect(window.location.href).toBe(VHOST_URL + '/');
  });

  // Every notification deep link lands here with its route in the hash (the
  // desktop app always loads the loopback address), so the handover has to
  // carry it across or the click ends on the plain dashboard.
  it('carries the deep-link hash across to the vhost', async () => {
    at('127.0.0.1', '7073', '#service/mailpit/view/abc123');
    await handOverToVhost();
    expect(window.location.href).toBe(VHOST_URL + '/#service/mailpit/view/abc123');
  });

  // nginx refusing the connection means the stack is still down, so staying
  // put is the only page the user can actually see.
  it('stays put when the vhost is unreachable', async () => {
    at('127.0.0.1', '7073');
    globalThis.fetch = vi.fn(async () => {
      throw new TypeError('failed to fetch');
    }) as unknown as typeof fetch;
    await handOverToVhost();
    expect(window.location.href).toBe('http://127.0.0.1:7073/');
  });

  // The service worker answers a failed request with a synthesized Response, so
  // a probe that only checks for a thrown error reads a stopped nginx as up.
  it('stays put when a service worker fakes a response for a dead vhost', async () => {
    at('127.0.0.1', '7073');
    globalThis.fetch = vi.fn(async () => new Response('', { status: 503 })) as unknown as typeof fetch;
    await handOverToVhost();
    expect(window.location.href).toBe('http://127.0.0.1:7073/');
  });

  it('never redirects a LAN session', async () => {
    at('192.168.1.20', '7073');
    await handOverToVhost();
    expect(window.location.href).toBe('http://192.168.1.20:7073/');
    expect(globalThis.fetch).not.toHaveBeenCalled();
  });
});
