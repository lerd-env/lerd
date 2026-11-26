import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

function sseResponse(body: string): Response {
  return {
    ok: true,
    body: new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(body));
        controller.close();
      }
    })
  } as unknown as Response;
}

describe('wizard store', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  // The wizard never sends a command line: it names a kind of work and the
  // daemon decides what runs.
  it('starts a run and returns the handle to reattach with', async () => {
    const calls: Array<{ url: string; body: string }> = [];
    globalThis.fetch = vi.fn(async (url: unknown, init?: RequestInit) => {
      calls.push({ url: String(url), body: String(init?.body ?? '') });
      return {
        ok: true,
        status: 200,
        statusText: 'OK',
        text: async () => JSON.stringify({ run: { id: 'abc', kind: 'scaffold', dir: '/p', status: 'running', started: 1 } })
      } as unknown as Response;
    }) as unknown as typeof fetch;

    const { startRun } = await import('./wizard');
    const run = await startRun({ kind: 'scaffold', dir: '/p', name: 'shop', framework: 'laravel' });

    expect(run.id).toBe('abc');
    expect(calls[0].url).toContain('/api/runs');
    expect(JSON.parse(calls[0].body)).toMatchObject({ kind: 'scaffold', name: 'shop' });
  });

  it('reports why a run failed rather than a bare failure', async () => {
    globalThis.fetch = vi.fn(async () =>
      sseResponse('data: composer install\n\nevent: done\ndata: {"ok":false,"error":"no space left"}\n\n')
    ) as unknown as typeof fetch;

    const { streamRun } = await import('./wizard');
    const events: unknown[] = [];
    await streamRun('abc', (e) => events.push(e));

    expect(events[0]).toEqual({ line: 'composer install' });
    expect(events[1]).toEqual({ done: true, ok: false, error: 'no space left' });
  });

  it('turns a refused answer into an error the wizard can show', async () => {
    globalThis.fetch = vi.fn(async () => ({
      ok: false,
      status: 403,
      statusText: 'Forbidden',
      text: async () => 'forbidden'
    })) as unknown as typeof fetch;

    const { saveProjectAnswers } = await import('./wizard');
    await expect(
      saveProjectAnswers('/p', { kind: 'php', secured: false, services: [], workers: [] })
    ).rejects.toThrow();
  });
});

describe('background run feedback', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  // The modal is closed, so nothing on screen was watching the run: its ending
  // has to reach the user as a notification rather than a silent stop.
  it('announces a run that finished while the wizard was closed', async () => {
    const runs = [
      { id: 'r1', kind: 'setup', dir: '/home/u/shop', status: 'running', started: 0 }
    ];
    globalThis.fetch = vi.fn(async () => ({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => ({ runs })
    })) as unknown as typeof fetch;

    const { refreshActiveRun, activeRun } = await import('./wizard');
    const { inAppNotifications } = await import('$lib/notify');

    await refreshActiveRun();
    expect(get(activeRun)?.id).toBe('r1');
    expect(get(inAppNotifications)).toHaveLength(0);

    runs[0].status = 'done';
    await refreshActiveRun();

    expect(get(activeRun)).toBeNull();
    expect(get(inAppNotifications)).toHaveLength(1);
    expect(get(inAppNotifications)[0].body).toContain('shop');
  });
});

describe('startersFor', () => {
  const laravel = {
    name: 'laravel',
    label: 'Laravel',
    versions: ['13', '12', '11'],
    latest: '13',
    starters: [
      { name: 'filament', label: 'Filament', versions: ['13', '12', '11'] },
      { name: 'react', label: 'React', versions: ['13'] }
    ]
  };

  // An empty version is the latest, which is what a scaffold without a pin gets.
  it('reads an unpinned version as the latest', async () => {
    const { startersFor } = await import('./wizard');
    expect(startersFor(laravel, '').map((s) => s.name)).toEqual(['filament', 'react']);
  });

  it('offers only the starters supporting the major picked', async () => {
    const { startersFor } = await import('./wizard');
    expect(startersFor(laravel, '12').map((s) => s.name)).toEqual(['filament']);
    expect(startersFor({ ...laravel, starters: undefined }, '13')).toEqual([]);
    expect(startersFor(undefined, '13')).toEqual([]);
  });
});
