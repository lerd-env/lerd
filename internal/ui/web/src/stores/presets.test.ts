import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

function readerFrom(chunks: string[]): ReadableStream<Uint8Array> {
  const enc = new TextEncoder();
  let i = 0;
  return new ReadableStream<Uint8Array>({
    pull(ctrl) {
      if (i >= chunks.length) {
        ctrl.close();
        return;
      }
      ctrl.enqueue(enc.encode(chunks[i++]));
    }
  });
}

describe('presets store', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('loadPresets populates with default selected_version', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify([
          {
            name: 'mysql',
            versions: [{ tag: '5.7' }, { tag: '8.0' }],
            default_version: '8.0',
            installed_tags: []
          }
        ]),
        { status: 200 }
      )
    ) as unknown as typeof fetch;
    const { presets, loadPresets } = await import('./presets');
    await loadPresets();
    const list = get(presets);
    expect(list[0].selected_version).toBe('8.0');
  });

  it('installablePresets hides presets with missing deps', async () => {
    const { presets, installablePresets } = await import('./presets');
    presets.set([
      { name: 'a', site_count: 0 } as never,
      { name: 'b', missing_deps: ['x'], site_count: 0 } as never
    ]);
    expect(get(installablePresets).map((p) => p.name)).toEqual(['a']);
  });

  it('installablePresets only shows versioned when new versions exist', async () => {
    const { presets, installablePresets } = await import('./presets');
    presets.set([
      { name: 'full', versions: [{ tag: '1' }], installed_tags: ['1'] } as never,
      { name: 'part', versions: [{ tag: '1' }, { tag: '2' }], installed_tags: ['1'] } as never
    ]);
    expect(get(installablePresets).map((p) => p.name)).toEqual(['part']);
  });

  it('discoverablePresets hides any preset with an installed version', async () => {
    const { presets, discoverablePresets } = await import('./presets');
    presets.set([
      // mysql already installed at 8.4 — must not be promoted for 5.7 / 9.7
      { name: 'mysql', installed: true, versions: [{ tag: '8.4' }, { tag: '5.7' }], installed_tags: ['8.4'] } as never,
      // mariadb installed — hidden too
      { name: 'mariadb', installed: true, versions: [{ tag: '11' }], installed_tags: ['11'] } as never,
      // mongo not installed yet — discoverable
      { name: 'mongo', installed: false, versions: [{ tag: '7' }], installed_tags: [] } as never,
      // single-version service, not installed — discoverable
      { name: 'mailpit', installed: false } as never
    ]);
    expect(get(discoverablePresets).map((p) => p.name)).toEqual(['mongo', 'mailpit']);
  });

  it('discoverablePresets still hides presets with missing deps', async () => {
    const { presets, discoverablePresets } = await import('./presets');
    presets.set([
      { name: 'pgadmin', installed: false, missing_deps: ['postgres'] } as never,
      { name: 'redis', installed: false } as never
    ]);
    expect(get(discoverablePresets).map((p) => p.name)).toEqual(['redis']);
  });

  it('matchesPresetQuery matches name, description, and image, ANDing terms', async () => {
    const { matchesPresetQuery } = await import('./presets');
    const redis = { name: 'redis', description: 'In-memory cache', image: 'docker.io/redis:7' } as never;
    expect(matchesPresetQuery(redis, '')).toBe(true);
    expect(matchesPresetQuery(redis, 'red')).toBe(true);
    expect(matchesPresetQuery(redis, 'cache')).toBe(true);
    expect(matchesPresetQuery(redis, 'redis:7')).toBe(true);
    expect(matchesPresetQuery(redis, 'redis cache')).toBe(true);
    expect(matchesPresetQuery(redis, 'redis postgres')).toBe(false);
    expect(matchesPresetQuery(redis, 'postgres')).toBe(false);
  });

  it('presetAddLabel maps installing phases', async () => {
    const { presetAddLabel } = await import('./presets');
    expect(presetAddLabel({ name: 'x' } as never)).toBe('Add');
    expect(presetAddLabel({ name: 'x', installing: true } as never)).toBe('Adding...');
    expect(
      presetAddLabel({ name: 'x', installing: true, installingPhase: 'pulling_image' } as never)
    ).toBe('Pulling image...');
  });

  it('installPreset streams NDJSON events and sets phases', async () => {
    const body = readerFrom([
      '{"phase":"pulling_image","image":"mysql:8"}\n',
      '{"phase":"starting_unit"}\n',
      '{"phase":"done","name":"mysql"}\n'
    ]);
    globalThis.fetch = vi.fn(async () => new Response(body, { status: 200 })) as unknown as typeof fetch;
    const { installPreset } = await import('./presets');
    const r = await installPreset({ name: 'mysql', site_count: 0 } as never);
    expect(r.ok).toBe(true);
    expect(r.name).toBe('mysql');
  });

  it('installPreset surfaces error from final event', async () => {
    const body = readerFrom(['{"phase":"error","error":"boom"}\n']);
    globalThis.fetch = vi.fn(async () => new Response(body, { status: 200 })) as unknown as typeof fetch;
    const { installPreset } = await import('./presets');
    const r = await installPreset({ name: 'bad' } as never);
    expect(r.ok).toBe(false);
    expect(r.error).toBe('boom');
  });

  it('installPreset short-circuits when missing_deps is non-empty', async () => {
    globalThis.fetch = vi.fn(async () => {
      throw new Error('should not be called');
    }) as unknown as typeof fetch;
    const { installPreset } = await import('./presets');
    const r = await installPreset({ name: 'x', missing_deps: ['mysql'] } as never);
    expect(r.ok).toBe(false);
    expect(r.error).toMatch(/mysql first/);
  });

  it('installPresetAndOpen installs, refreshes services + presets, runs onSuccess, and navigates', async () => {
    location.hash = '';
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: unknown) => {
      const u = String(url);
      calls.push(u);
      if (u.includes('/api/services/presets/')) {
        return new Response(readerFrom(['{"phase":"done","name":"mysql"}\n']), { status: 200 });
      }
      return new Response('[]', { status: 200 });
    }) as unknown as typeof fetch;
    const { installPresetAndOpen } = await import('./presets');
    let opened = '';
    const r = await installPresetAndOpen({ name: 'mysql', site_count: 0 } as never, {
      onSuccess: (n) => (opened = n)
    });
    expect(r.ok).toBe(true);
    expect(opened).toBe('mysql');
    expect(location.hash).toBe('#services/mysql');
    expect(calls.some((c) => c.endsWith('/api/services'))).toBe(true);
    expect(calls.some((c) => c.endsWith('/api/services/presets'))).toBe(true);
  });

  it('installPresetAndOpen does not navigate when the install fails', async () => {
    location.hash = '';
    globalThis.fetch = vi.fn(async (url: unknown) => {
      const u = String(url);
      if (u.includes('/api/services/presets/')) {
        return new Response(readerFrom(['{"phase":"error","error":"boom"}\n']), { status: 200 });
      }
      return new Response('[]', { status: 200 });
    }) as unknown as typeof fetch;
    const { installPresetAndOpen } = await import('./presets');
    const r = await installPresetAndOpen({ name: 'bad', site_count: 0 } as never);
    expect(r.ok).toBe(false);
    expect(location.hash).toBe('');
  });
});
