import { describe, it, expect, vi } from 'vitest';
import { pluginApiPath, answerPluginRequest, pluginTheme, pluginDiffRequest, pluginEditRequest, openPluginTerminal, pluginMaximizeRequest } from './pluginBridge';

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

describe('pluginEditRequest', () => {
  const frame = {} as Window;
  const ask = (data: unknown, source: unknown = frame) => pluginEditRequest({ data, source } as MessageEvent, frame);

  it('takes a file for the dashboard editor, with the id to answer by', () => {
    expect(ask({ type: 'lerd:edit', id: 3, path: '.lerd/scrub.sql', content: 'DELETE FROM x;', label: 'after a pull' })).toEqual({
      id: 3,
      path: '.lerd/scrub.sql',
      content: 'DELETE FROM x;',
      label: 'after a pull'
    });
  });

  it('ignores other frames, a missing id and anything not text', () => {
    expect(ask({ type: 'lerd:edit', id: 1, path: 'a', content: '' }, {})).toBeNull();
    expect(ask({ type: 'lerd:edit', path: 'a', content: '' })).toBeNull();
    expect(ask({ type: 'lerd:edit', id: 1, path: 'a', content: 7 })).toBeNull();
    expect(ask({ type: 'lerd:edit', id: 1, path: 'a', content: 'x'.repeat(2 * 1024 * 1024 + 1) })).toBeNull();
  });
});

describe('openPluginTerminal', () => {
  class FakeSocket {
    static last: FakeSocket;
    static OPEN = 1;
    readyState = 0;
    binaryType = '';
    onopen: (() => void) | null = null;
    sent: unknown[] = [];
    closed = false;
    onmessage: ((e: { data: unknown }) => void) | null = null;
    onclose: (() => void) | null = null;
    constructor(public url: string) {
      FakeSocket.last = this;
    }
    send(d: unknown) {
      this.sent.push(d);
    }
    close() {
      this.closed = true;
    }
  }
  const asSocket = FakeSocket as unknown as typeof WebSocket;
  const ask = (win: Window, path: string) =>
    ({ source: win, data: { type: 'lerd:terminal', id: 3, path } }) as unknown as MessageEvent;

  it('opens the socket for its own endpoint and hands the page a port to it', async () => {
    const win = { postMessage: vi.fn() } as unknown as Window & { postMessage: ReturnType<typeof vi.fn> };
    const close = openPluginTerminal(ask(win, 'shell?site=shop&cols=90&rows=20'), 'term', win, asSocket);
    expect(close).toBeTypeOf('function');
    const ws = FakeSocket.last;
    expect(ws.url).toMatch(/^ws:\/\/[^/]+\/api\/plugins\/term\/shell\?site=shop&cols=90&rows=20$/);
    const [msg, , ports] = win.postMessage.mock.calls[0];
    expect(msg).toEqual({ type: 'lerd:terminal:opened', id: 3 });
    const page: MessagePort = ports[0];

    const got: unknown[] = [];
    page.onmessage = (e) => got.push(e.data);
    page.postMessage('ls\r');
    page.postMessage({ type: 'resize', cols: 100, rows: 30 });
    await new Promise((r) => setTimeout(r, 10));
    expect(ws.sent).toEqual([]);
    ws.readyState = 1;
    ws.onopen?.();
    ws.onmessage?.({ data: '{"type":"session","id":"abc"}' });
    ws.onmessage?.({ data: new Uint8Array([104, 105]).buffer });
    await new Promise((r) => setTimeout(r, 10));
    expect(ws.sent[0]).toEqual(new TextEncoder().encode('ls\r'));
    expect(ws.sent[1]).toBe('{"type":"resize","cols":100,"rows":30}');
    expect(got[0]).toEqual({ type: 'session', id: 'abc' });
    expect(new Uint8Array(got[1] as ArrayBuffer)).toEqual(new Uint8Array([104, 105]));

    // The page going away only lets go: lerd keeps a terminal the user typed in.
    close!();
    expect(ws.closed).toBe(true);
    expect(ws.sent).not.toContain('{"type":"close"}');
    page.close();
  });

  it('ends the terminal when the page closes it', async () => {
    const win = { postMessage: vi.fn() } as unknown as Window & { postMessage: ReturnType<typeof vi.fn> };
    openPluginTerminal(ask(win, 'shell'), 'term', win, asSocket);
    const ws = FakeSocket.last;
    ws.readyState = 1;
    const page: MessagePort = win.postMessage.mock.calls[0][2][0];
    page.postMessage({ type: 'close' });
    await new Promise((r) => setTimeout(r, 10));
    expect(ws.sent).toEqual(['{"type":"close"}']);
    expect(ws.closed).toBe(true);
    page.close();
  });

  it('still ends a terminal closed while its socket was connecting', async () => {
    const win = { postMessage: vi.fn() } as unknown as Window & { postMessage: ReturnType<typeof vi.fn> };
    openPluginTerminal(ask(win, 'shell?session=abc'), 'term', win, asSocket);
    const ws = FakeSocket.last;
    const page: MessagePort = win.postMessage.mock.calls[0][2][0];
    page.postMessage({ type: 'close' });
    await new Promise((r) => setTimeout(r, 10));
    expect(ws.closed).toBe(false);
    ws.readyState = 1;
    ws.onopen?.();
    expect(ws.sent).toEqual(['{"type":"close"}']);
    expect(ws.closed).toBe(true);
    page.close();
  });

  it('refuses a path outside the plugin and opens nothing', () => {
    const win = { postMessage: vi.fn() } as unknown as Window;
    FakeSocket.last = undefined as unknown as FakeSocket;
    expect(openPluginTerminal(ask(win, '../sites'), 'term', win, asSocket)).toBeNull();
    expect(FakeSocket.last).toBeUndefined();
  });

  it('ignores another window asking', () => {
    const win = { postMessage: vi.fn() } as unknown as Window;
    const other = { postMessage: vi.fn() } as unknown as Window;
    expect(openPluginTerminal(ask(other, 'shell'), 'term', win, asSocket)).toBeNull();
  });
});

describe('pluginMaximizeRequest', () => {
  const win = {} as Window;
  const msg = (data: unknown, source: Window = win) => ({ source, data }) as unknown as MessageEvent;

  it('reads whether its own page wants the whole page', () => {
    expect(pluginMaximizeRequest(msg({ type: 'lerd:maximize', on: true }), win)).toBe(true);
    expect(pluginMaximizeRequest(msg({ type: 'lerd:maximize', on: false }), win)).toBe(false);
  });

  it('ignores another window and anything else', () => {
    expect(pluginMaximizeRequest(msg({ type: 'lerd:maximize', on: true }, {} as Window), win)).toBeNull();
    expect(pluginMaximizeRequest(msg({ type: 'lerd:api', on: true }), win)).toBeNull();
    expect(pluginMaximizeRequest(msg({ type: 'lerd:maximize', on: 'yes' }), win)).toBeNull();
  });
});
