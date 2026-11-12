import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

function ndjsonResponse(lines: string[]): Response {
  const enc = new TextEncoder();
  const body = new ReadableStream({
    start(controller) {
      for (const l of lines) controller.enqueue(enc.encode(l + '\n'));
      controller.close();
    }
  });
  return new Response(body, { status: 200 });
}

describe('lerdLifecycle store', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  // Status refreshes land continuously while a start runs, so the guard is what
  // stops the dashboard from firing a second `lerd start` on top of the first.
  it('autoStartOnce starts lerd only once per page load', async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      ndjsonResponse(['{"phase":"done"}'])
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { autoStartOnce } = await import('./lerdLifecycle');

    await autoStartOnce();
    await autoStartOnce();

    const starts = fetchMock.mock.calls.filter((c) => c[0] === '/api/lerd/start');
    expect(starts).toHaveLength(1);
  });

  // A stop from the tray, CLI or dashboard has to hold: an open tab must not
  // bring the stack straight back up behind the user's back.
  it('autoStartOnce leaves a stack the user stopped alone', async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      ndjsonResponse(['{"phase":"done"}'])
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { status } = await import('./status');
    status.update((s) => ({ ...s, stopped: true }));
    const { autoStartOnce } = await import('./lerdLifecycle');

    await autoStartOnce();

    const starts = fetchMock.mock.calls.filter((c) => c[0] === '/api/lerd/start');
    expect(starts).toHaveLength(0);
  });

  it('does not retry after a failed start', async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      new Response('nope', { status: 500 })
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const { autoStartOnce } = await import('./lerdLifecycle');

    await autoStartOnce();
    await autoStartOnce();

    const starts = fetchMock.mock.calls.filter((c) => c[0] === '/api/lerd/start');
    expect(starts).toHaveLength(1);
  });

  // The button reports progress off this stream, so the counters have to move
  // as events land rather than only settle once the request returns.
  it('tracks the step, the last unit and the unit count while starting', async () => {
    globalThis.fetch = vi.fn(async () =>
      ndjsonResponse([
        '{"phase":"step","step":"images"}',
        '{"phase":"step","step":"units","total":3}',
        '{"phase":"unit","unit":"nginx"}',
        '{"phase":"unit","unit":"mysql"}',
        '{"phase":"done"}'
      ])
    ) as unknown as typeof fetch;
    const { lerdStart, lerdStartStep, lerdStartUnit, lerdStartDone, lerdStartTotal } =
      await import('./lerdLifecycle');

    const steps: string[] = [];
    const units: string[] = [];
    const counts: number[] = [];
    const totals: number[] = [];
    const stop = [
      lerdStartStep.subscribe((v) => steps.push(v)),
      lerdStartUnit.subscribe((v) => units.push(v)),
      lerdStartDone.subscribe((v) => counts.push(v)),
      lerdStartTotal.subscribe((v) => totals.push(v))
    ];
    await lerdStart();
    stop.forEach((s) => s());

    expect(steps).toContain('images');
    expect(steps).toContain('units');
    expect(units).toContain('nginx');
    expect(units).toContain('mysql');
    expect(counts).toContain(2);
    expect(totals).toContain(3);
  });

  it('clears the progress counters once the start finishes', async () => {
    globalThis.fetch = vi.fn(async () =>
      ndjsonResponse(['{"phase":"step","step":"units","total":2}', '{"phase":"unit","unit":"nginx"}'])
    ) as unknown as typeof fetch;
    const { lerdStart, lerdStarting, lerdStartTotal, lerdStartUnit } = await import('./lerdLifecycle');

    await lerdStart();

    expect(get(lerdStarting)).toBe(false);
    expect(get(lerdStartTotal)).toBe(0);
    expect(get(lerdStartUnit)).toBe('');
  });

  it('reports failure when the stream ends on a failed phase', async () => {
    globalThis.fetch = vi.fn(async () =>
      ndjsonResponse(['{"phase":"failed","error":"podman machine is not running"}'])
    ) as unknown as typeof fetch;
    const { lerdStart } = await import('./lerdLifecycle');

    expect(await lerdStart()).toBe(false);
  });
});
