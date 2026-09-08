import { describe, it, expect, vi } from 'vitest';
import { get } from 'svelte/store';


// sseResponse frames lines and a done payload the way the switch endpoint does,
// which streams so a rebuild that takes minutes is visible rather than hidden.
function sseResponse(lines: string[], done: object): Response {
  const body = lines.map((l) => `data: ${l}\n\n`).join('') + `event: done\ndata: ${JSON.stringify(done)}\n\n`;
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(body));
      controller.close();
    }
  });
  return new Response(stream, { status: 200 });
}

describe('php runtime store', () => {
  it('loads the runtime and whether it applies', async () => {
    globalThis.fetch = vi.fn(
      async () =>
        new Response('{"php_runtime":"native","php_runtime_applies":true}', { status: 200 })
    ) as unknown as typeof fetch;
    const { loadPHPRuntime, phpRuntime, phpRuntimeApplies } = await import('./phpRuntime');
    await loadPHPRuntime();
    expect(get(phpRuntime)).toBe('native');
    expect(get(phpRuntimeApplies)).toBe(true);
  });

  it('treats an unknown runtime as container', async () => {
    globalThis.fetch = vi.fn(
      async () => new Response('{"php_runtime":"nonsense"}', { status: 200 })
    ) as unknown as typeof fetch;
    const { loadPHPRuntime, phpRuntime } = await import('./phpRuntime');
    await loadPHPRuntime();
    expect(get(phpRuntime)).toBe('container');
  });

  it('posts the mode and surfaces a refusal', async () => {
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: unknown, init?: RequestInit) => {
      calls.push(String(url) + ':' + String(init?.body));
      return sseResponse([], { ok: false, error: 'macOS only' });
    }) as unknown as typeof fetch;
    const { setPHPRuntime, phpRuntime } = await import('./phpRuntime');
    const res = await setPHPRuntime('native');
    expect(calls[0]).toContain('/api/settings/php-runtime');
    expect(calls[0]).toContain('"mode":"native"');
    expect(res.ok).toBe(false);
    // A refused switch must not leave the toggle showing the new value.
    expect(get(phpRuntime)).toBe('container');
  });

  it('asks for the images to be removed only when told to', async () => {
    const bodies: string[] = [];
    globalThis.fetch = vi.fn(async (_url: unknown, init?: RequestInit) => {
      bodies.push(String(init?.body));
      return sseResponse([], { ok: true });
    }) as unknown as typeof fetch;
    const { setPHPRuntime } = await import('./phpRuntime');

    await setPHPRuntime('native');
    expect(bodies[0]).toContain('"remove_images":false');

    await setPHPRuntime('native', true);
    expect(bodies[1]).toContain('"remove_images":true');
  });

  // The reclaim runs after the switch has already happened, so it has to be
  // reported on its own rather than turning a completed switch into a failure.
  it('reports a failed reclaim separately from the switch', async () => {
    globalThis.fetch = vi.fn(async () =>
      sseResponse(['Building PHP 8.2 image...'], {
        ok: true,
        images_removed: 2,
        images_error: 'image is in use'
      })
    ) as unknown as typeof fetch;
    const { setPHPRuntime } = await import('./phpRuntime');
    const res = await setPHPRuntime('native', true);
    expect(res.ok).toBe(true);
    expect(res.imagesRemoved).toBe(2);
    expect(res.imagesError).toBe('image is in use');
  });

  // A switch that has to rebuild images spends minutes doing it, and a spinner
  // with nothing behind it reads as a hang, so the progress is handed to the
  // caller as it arrives rather than only at the end.
  it('hands the progress to the caller as it arrives', async () => {
    globalThis.fetch = vi.fn(async () =>
      sseResponse(['Building PHP 8.2 image...', 'Building PHP 8.3 image...'], { ok: true })
    ) as unknown as typeof fetch;
    const { setPHPRuntime } = await import('./phpRuntime');
    const seen: string[] = [];
    const res = await setPHPRuntime('container', false, (line) => seen.push(line));
    expect(res.ok).toBe(true);
    expect(seen).toEqual(['Building PHP 8.2 image...', 'Building PHP 8.3 image...']);
  });
});
