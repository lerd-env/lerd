import { describe, it, expect, vi } from 'vitest';
import { pluginApiPath, answerPluginRequest, pluginTheme, pluginDiffRequest } from './pluginBridge';

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

describe('pluginTheme', () => {
  it('hands a page the dashboard mode and palette under the names it reads', () => {
    const root = document.createElement('div');
    root.classList.add('dark');
    root.style.setProperty('--color-lerd-red', '#7aa2f7');
    root.style.setProperty('--color-lerd-border', '#333');
    root.style.setProperty('--color-white', '#fdf6e3');
    root.style.setProperty('--color-gray-200', '#eee8d5');
    document.body.appendChild(root);
    const theme = pluginTheme(root);
    expect(theme.type).toBe('lerd:theme');
    expect(theme.dark).toBe(true);
    expect(theme.vars['--lerd-accent']).toBe('#7aa2f7');
    expect(theme.vars['--lerd-border']).toBe('#333');
    expect(theme.vars['--lerd-light-card']).toBe('#fdf6e3');
    expect(theme.vars['--lerd-light-border']).toBe('#eee8d5');
    expect(theme.vars['--lerd-on-accent']).toBeUndefined();
    root.remove();
  });
});

describe('pluginDiffRequest', () => {
  const frame = {} as Window;
  const ask = (data: unknown, source: unknown = frame) => pluginDiffRequest({ data, source } as MessageEvent, frame);

  it('takes two versions of a file from the page to show in the dashboard diff', () => {
    expect(ask({ type: 'lerd:diff', path: 'wp-content/a.php', original: 'a', modified: 'b', before: 'production', after: 'This site' })).toEqual({
      path: 'wp-content/a.php',
      original: 'a',
      modified: 'b',
      before: 'production',
      after: 'This site'
    });
  });

  it('ignores other frames, other messages and anything not text', () => {
    expect(ask({ type: 'lerd:diff', path: 'a', original: 'a', modified: 'b' }, {})).toBeNull();
    expect(ask({ type: 'lerd:api', path: 'a' })).toBeNull();
    expect(ask({ type: 'lerd:diff', path: 'a', original: 1, modified: 'b' })).toBeNull();
    expect(ask({ type: 'lerd:diff', original: 'a', modified: 'b' })).toBeNull();
  });

  it('refuses a version too large to draw', () => {
    const big = 'x'.repeat(2 * 1024 * 1024 + 1);
    expect(ask({ type: 'lerd:diff', path: 'a', original: big, modified: 'b' })).toBeNull();
  });
});

describe('pluginDiffRequest for a single file', () => {
  const frame = {} as Window;
  const ask = (data: unknown) => pluginDiffRequest({ data, source: frame } as MessageEvent, frame);

  it('takes one file to show on its own, with no other side', () => {
    expect(ask({ type: 'lerd:view', path: 'a.css', content: 'body{}', label: 'production' })).toEqual({
      path: 'a.css',
      modified: 'body{}',
      after: 'production'
    });
    expect(ask({ type: 'lerd:view', path: 'a.css', content: 5 })).toBeNull();
  });
});
