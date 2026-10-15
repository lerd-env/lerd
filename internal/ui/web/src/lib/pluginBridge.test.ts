import { describe, it, expect, vi } from 'vitest';
import { pluginApiPath, answerPluginRequest } from './pluginBridge';

describe('pluginApiPath', () => {
  it('keeps a plugin inside its own endpoints', () => {
    expect(pluginApiPath('hello', 'greet?site=shop')).toBe('/api/plugins/hello/greet?site=shop');
    expect(pluginApiPath('hello', 'a/b')).toBe('/api/plugins/hello/a/b');
  });

  it('refuses anything that would reach past them', () => {
    for (const p of ['../sites', 'x/../../sites', '/api/sites', '//evil.test/x', 'http://evil.test/', '%2e%2e/sites', '']) {
      expect(pluginApiPath('hello', p), p).toBeNull();
    }
  });
});

describe('answerPluginRequest', () => {
  function frame() {
    return { postMessage: vi.fn() } as unknown as Window & { postMessage: ReturnType<typeof vi.fn> };
  }

  it('relays an API call from its own frame and posts the answer back', async () => {
    const win = frame();
    const fetcher = vi.fn().mockResolvedValue(new Response('hi', { status: 200 }));
    await answerPluginRequest(
      { source: win, data: { type: 'lerd:api', id: 7, method: 'POST', path: 'greet', body: { a: 1 } } } as unknown as MessageEvent,
      'hello',
      win,
      fetcher
    );
    expect(fetcher).toHaveBeenCalledWith('/api/plugins/hello/greet', {
      method: 'POST',
      body: '{"a":1}',
      headers: { 'Content-Type': 'application/json' }
    });
    expect(win.postMessage).toHaveBeenCalledWith({ type: 'lerd:api:result', id: 7, status: 200, body: 'hi' }, '*');
  });

  it('ignores messages from any other window', async () => {
    const fetcher = vi.fn();
    await answerPluginRequest(
      { source: frame(), data: { type: 'lerd:api', id: 1, path: 'greet' } } as unknown as MessageEvent,
      'hello',
      frame(),
      fetcher
    );
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('answers a path outside the plugin with 403 and fetches nothing', async () => {
    const win = frame();
    const fetcher = vi.fn();
    await answerPluginRequest(
      { source: win, data: { type: 'lerd:api', id: 2, path: '../sites' } } as unknown as MessageEvent,
      'hello',
      win,
      fetcher
    );
    expect(fetcher).not.toHaveBeenCalled();
    expect(win.postMessage).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'lerd:api:result', id: 2, status: 403 }),
      '*'
    );
  });
});
