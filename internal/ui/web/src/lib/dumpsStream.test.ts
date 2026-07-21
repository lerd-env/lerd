import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  listeners: Record<string, ((e: unknown) => void)[]> = {};
  closed = false;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(ev: string, fn: (e: unknown) => void) {
    (this.listeners[ev] ||= []).push(fn);
  }

  fire(ev: string, data: unknown) {
    for (const fn of this.listeners[ev] || []) fn(data);
  }

  close() {
    this.closed = true;
  }
}

function payload(id: string, extra: Record<string, unknown> = {}) {
  return JSON.stringify({
    v: 1,
    id,
    ts: '2026-05-10T00:00:00.000Z',
    kind: 'dump',
    ctx: { type: 'fpm', site: 'acme' },
    src: { file: '/x.php', line: 1 },
    text: 'value',
    ...extra
  });
}

describe('createDumpsStream', () => {
  const realES = globalThis.EventSource;

  beforeEach(() => {
    MockEventSource.instances = [];
    // @ts-expect-error test double
    globalThis.EventSource = MockEventSource;
  });

  afterEach(() => {
    globalThis.EventSource = realES;
  });

  it('parses message events into typed entries', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('message', { data: payload('a') });
    MockEventSource.instances[0].fire('message', { data: payload('b') });
    s.flush();
    const list = get(s.events);
    expect(list.map((e) => e.id)).toEqual(['a', 'b']);
    expect(list[0].ctx.site).toBe('acme');
  });

  it('de-dupes by id (replay safety)', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('message', { data: payload('a') });
    MockEventSource.instances[0].fire('message', { data: payload('a') });
    s.flush();
    expect(get(s.events).length).toBe(1);
  });

  it('caps at maxEvents (drops oldest)', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream({}, 3);
    s.connect();
    for (let i = 0; i < 5; i++) {
      MockEventSource.instances[0].fire('message', { data: payload('e' + i) });
    }
    s.flush();
    expect(get(s.events).map((e) => e.id)).toEqual(['e2', 'e3', 'e4']);
  });

  it('skips malformed JSON without throwing', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('message', { data: '{not valid json' });
    MockEventSource.instances[0].fire('message', { data: payload('ok') });
    s.flush();
    expect(get(s.events).map((e) => e.id)).toEqual(['ok']);
  });

  it('builds query string from filters', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream({ site: 'acme', ctx: 'fpm' });
    s.connect();
    expect(MockEventSource.instances[0].url).toContain('site=acme');
    expect(MockEventSource.instances[0].url).toContain('ctx=fpm');
  });

  it('clear empties the events store', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('message', { data: payload('a') });
    s.flush();
    s.clear();
    expect(get(s.events)).toEqual([]);
  });

  it('close tears down the EventSource', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    s.close();
    expect(MockEventSource.instances[0].closed).toBe(true);
    expect(get(s.connected)).toBe(false);
  });

  it('open fires connected=true', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('open', {});
    expect(get(s.connected)).toBe(true);
  });

  it('error fires connected=false', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('open', {});
    MockEventSource.instances[0].fire('error', {});
    expect(get(s.connected)).toBe(false);
  });

  it('retains a full session past the old 500-event window', async () => {
    // The shared ring means ~7 events per request, so the dashboard must keep
    // far more than 500 or dumps vanish as queries/etc flow in. Default cap is
    // a high safety ceiling, not the working size.
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    for (let i = 0; i < 600; i++) {
      MockEventSource.instances[0].fire('message', { data: payload('e' + i) });
    }
    s.flush();
    expect(get(s.events).length).toBe(600);
    expect(get(s.events)[0].id).toBe('e0');
  });

  it('coalesces a burst into a single store write', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    let writes = 0;
    const stop = s.events.subscribe(() => writes++);
    writes = 0;
    for (let i = 0; i < 50; i++) {
      MockEventSource.instances[0].fire('message', { data: payload('e' + i) });
    }
    expect(writes).toBe(0);
    s.flush();
    expect(writes).toBe(1);
    expect(get(s.events).length).toBe(50);
    stop();
  });

  it('flushes buffered events on the next frame without a manual flush', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('message', { data: payload('a') });
    await new Promise((r) => requestAnimationFrame(() => r(null)));
    expect(get(s.events).map((e) => e.id)).toEqual(['a']);
  });

  it('clear discards events buffered but not yet flushed', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('message', { data: payload('a') });
    s.clear();
    s.flush();
    expect(get(s.events)).toEqual([]);
  });

  it('close flushes what is still buffered', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('message', { data: payload('a') });
    s.close();
    expect(get(s.events).map((e) => e.id)).toEqual(['a']);
  });

  it('clear resets the de-dup set so a replayed id is welcome again', async () => {
    const { createDumpsStream } = await import('./dumpsStream');
    const s = createDumpsStream();
    s.connect();
    MockEventSource.instances[0].fire('message', { data: payload('a') });
    s.flush();
    s.clear();
    MockEventSource.instances[0].fire('message', { data: payload('a') });
    s.flush();
    expect(get(s.events).map((e) => e.id)).toEqual(['a']);
  });
});
