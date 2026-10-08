import { describe, it, expect, beforeEach, vi } from 'vitest';
// The script lives with the Go package that embeds it.
import script from '../../../../browserlogs/browser.js?raw';

// Runs the capture script lerd injects into site pages against jsdom and reads
// what it would have posted.
type Report = { type: string; nav?: string; message: string; page: string; level?: string };

// Every load is a fresh script instance in the same jsdom, and earlier ones keep
// their history hooks, so each gets its own endpoint and only its posts count.
let instance = 0;
function load(cfg: Record<string, unknown>): Report[] {
  const sent: Report[] = [];
  const endpoint = `/_lerd/browser-${++instance}`;
  delete (window as unknown as Record<string, unknown>).__lerdBrowserLogs;
  (globalThis as unknown as { Blob: unknown }).Blob = class {
    constructor(public parts: string[]) {}
  };
  const prev = navigator.sendBeacon as unknown as ((url: string, blob: { parts: string[] }) => boolean) | undefined;
  Object.defineProperty(navigator, 'sendBeacon', {
    configurable: true,
    value: (url: string, blob: { parts: string[] }) => {
      if (url === endpoint) sent.push(...(JSON.parse(blob.parts[0]) as Report[]));
      else prev?.(url, blob);
      return true;
    }
  });
  const full = { console: [], network: [], navigation: true, endpoint, ...cfg };
  new Function(script.replace('__LERD_CONFIG__', JSON.stringify(full)))();
  return sent;
}

function settle() {
  vi.advanceTimersByTime(400);
}

describe('browser logs script', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
    history.replaceState(null, '', '/');
    vi.spyOn(console, 'info').mockImplementation(() => {});
  });

  it('reports the first view and each SPA navigation as its own page view', () => {
    const sent = load({});
    history.pushState(null, '', '/cart');
    history.replaceState({ scroll: 1 }, '', '/cart');
    settle();
    const views = sent.filter((r) => r.type === 'navigation');
    expect(views.map((r) => r.nav)).toEqual(['load', 'push']);
    expect(views[0].page).not.toBe(views[1].page);
  });

  it('files an error under the view it happened in', () => {
    const sent = load({});
    history.pushState(null, '', '/checkout');
    window.dispatchEvent(new ErrorEvent('error', { message: 'boom', error: new Error('boom') }));
    settle();
    const nav = sent.find((r) => r.nav === 'push');
    const err = sent.find((r) => r.type === 'error');
    expect(err?.page).toBe(nav?.page);
  });

  it('reports no page views when the site turned them off', () => {
    const sent = load({ navigation: false });
    history.pushState(null, '', '/elsewhere');
    settle();
    expect(sent.filter((r) => r.type === 'navigation')).toEqual([]);
  });

  it('reports console levels only when asked for', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const sent = load({ console: ['error'], navigation: false });
    console.error('bad thing');
    console.warn('meh');
    settle();
    expect(sent.map((r) => [r.type, r.level, r.message])).toEqual([['console', 'error', 'bad thing']]);
    errorSpy.mockRestore();
    warnSpy.mockRestore();
  });

  it('reports a request that got no response, flagging a cross-origin one', async () => {
    const realFetch = window.fetch;
    window.fetch = (() => Promise.reject(new TypeError('Failed to fetch'))) as typeof fetch;
    const sent = load({ network: ['failed'], navigation: false });
    await expect(fetch('https://api.elsewhere.test/x')).rejects.toThrow('Failed to fetch');
    settle();
    expect(sent.map((r) => [r.type, (r as unknown as { status: number }).status, (r as unknown as { cross: boolean }).cross])).toEqual([['network', 0, true]]);
    window.fetch = realFetch;
  });

  it('leaves an aborted request alone', async () => {
    const realFetch = window.fetch;
    window.fetch = (() => Promise.reject(new DOMException('aborted', 'AbortError'))) as typeof fetch;
    const sent = load({ network: ['failed'], navigation: false });
    await expect(fetch('/x')).rejects.toThrow();
    settle();
    expect(sent).toEqual([]);
    window.fetch = realFetch;
  });

  it('reports a resource that failed to load only when asked for', () => {
    const sent = load({ resources: true, navigation: false });
    const img = document.createElement('img');
    img.setAttribute('src', 'http://localhost/missing.png');
    document.body.appendChild(img);
    img.dispatchEvent(new Event('error'));
    settle();
    expect(sent.map((r) => [r.type, (r as unknown as { tag: string }).tag])).toEqual([['resource', 'img']]);
    img.remove();
  });

  it('reports a configured event, even one that does not bubble, with the value at its path', () => {
    const sent = load({ navigation: false, events: [{ event: 'inertia:invalid', label: 'Inertia invalid response', message: 'detail.response.status' }] });
    document.dispatchEvent(new CustomEvent('inertia:invalid', { detail: { response: { status: 500 } } }));
    settle();
    expect(sent.map((r) => [r.type, r.message])).toEqual([['event', 'Inertia invalid response: 500']]);
  });

  // A loop that throws every frame is held back, but the same error caused
  // again by the user a moment later is reported each time.
  it('drops an identical repeat only within a second of the last one', () => {
    const sent = load({ navigation: false, events: [{ event: 'app:fail', label: 'App failed', message: '' }] });
    const fire = () => document.dispatchEvent(new CustomEvent('app:fail'));
    fire();
    fire();
    settle();
    expect(sent).toHaveLength(1);
    vi.advanceTimersByTime(1000);
    fire();
    settle();
    expect(sent).toHaveLength(2);
  });

  // Inertia hands its events an axios response, a plain object, so it is told
  // apart by shape: the status and the URL it was asked for, nothing else.
  it('describes an axios response by its status, method and URL', () => {
    const sent = load({ navigation: false, events: [{ event: 'inertia:invalid', label: 'Inertia invalid response', message: 'detail.response' }] });
    const response = {
      status: 500,
      data: '<h1>Plain server error</h1>',
      headers: { 'content-type': 'text/html' },
      config: { method: 'get', url: '/lerd-test/broken' },
      request: { responseURL: 'https://shop.test/lerd-test/broken' }
    };
    document.dispatchEvent(new CustomEvent('inertia:invalid', { detail: { response } }));
    settle();
    expect(sent.map((r) => r.message)).toEqual(['Inertia invalid response: 500 GET https://shop.test/lerd-test/broken']);
  });

  it('describes an object at the message path by what identifies its parts', () => {
    const sent = load({ navigation: false, events: [{ event: 'app:fail', label: 'App failed', message: 'detail' }] });
    const el = document.createElement('div');
    el.id = 'cart';
    const cyclic: Record<string, unknown> = { name: 'loop' };
    cyclic.self = cyclic;
    const detail = {
      error: new TypeError('bad cart'),
      target: el,
      seen: new Set(['a', 'b']),
      totals: new Map([['eur', 12]]),
      cyclic,
      deep: { a: { b: { c: { d: 1 } } } }
    };
    document.dispatchEvent(new CustomEvent('app:fail', { detail }));
    settle();
    const msg = sent[0].message;
    expect(msg.startsWith('App failed: ')).toBe(true);
    expect(JSON.parse(msg.slice('App failed: '.length))).toEqual({
      error: 'TypeError: bad cart',
      target: 'div#cart',
      seen: ['a', 'b'],
      totals: { eur: 12 },
      cyclic: { name: 'loop', self: '[Circular]' },
      deep: { a: { b: '[Object]' } }
    });
  });

  it('keeps a long list short', () => {
    const sent = load({ navigation: false, events: [{ event: 'app:list', label: 'List', message: 'detail' }] });
    document.dispatchEvent(new CustomEvent('app:list', { detail: Array.from({ length: 25 }, (_, i) => i) }));
    settle();
    const list = JSON.parse(sent[0].message.slice('List: '.length));
    expect(list).toHaveLength(21);
    expect(list[20]).toBe('… 5 more');
  });
  it('names the first page view after the request that served it', () => {
    const tag = document.createElement('script');
    tag.setAttribute('data-rid', 'php-rid-1');
    Object.defineProperty(document, 'currentScript', { configurable: true, value: tag });
    const sent = load({});
    Object.defineProperty(document, 'currentScript', { configurable: true, value: null });
    history.pushState(null, '', '/next');
    settle();
    const views = sent.filter((r) => r.type === 'navigation');
    expect(views[0].page).toBe('php-rid-1');
    expect(views[1].page).not.toBe('php-rid-1');
  });

  it('names the PHP request a failed fetch reached', async () => {
    const realFetch = window.fetch;
    window.fetch = (() => Promise.resolve({ status: 500, headers: { get: (h: string) => (h === 'X-Lerd-Rid' ? 'api-rid-9' : null) } })) as unknown as typeof fetch;
    const sent = load({ navigation: false, network: ['5xx'] });
    await fetch('/api/orders');
    settle();
    expect(sent.find((r) => r.type === 'network')).toMatchObject({ rid: 'api-rid-9', status: 500 });
    window.fetch = realFetch;
  });
});
