import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { entities, loadEntities, runEntityAction, kindLabel } from './entities';

describe('kindLabel', () => {
  it('prefers the declared label', () => {
    expect(kindLabel({ kind: 'buckets', label: 'S3 buckets' })).toBe('S3 buckets');
  });

  it('capitalizes the kind slug when no label is declared', () => {
    expect(kindLabel({ kind: 'buckets' })).toBe('Buckets');
  });
});

describe('loadEntities', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    entities.set({});
  });

  it('merges the fetched kinds under the service name', async () => {
    const kinds = [{ kind: 'buckets', columns: [], actions: [], rows: [{ name: 'lerd' }] }];
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify(kinds), { status: 200 }))
    );
    await loadEntities('rustfs');
    expect(get(entities)['rustfs']?.[0]?.rows?.[0]?.name).toBe('lerd');
  });

  it('keeps the last good copy when the fetch fails', async () => {
    entities.set({ rustfs: [{ kind: 'buckets', columns: [], actions: [], rows: [] }] });
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('boom', { status: 500 }))
    );
    await loadEntities('rustfs');
    expect(get(entities)['rustfs']).toHaveLength(1);
  });
});

describe('runEntityAction', () => {
  beforeEach(() => entities.set({}));
  afterEach(() => {
    vi.unstubAllGlobals();
    entities.set({});
  });

  it('posts the entity name and reloads the service on success', async () => {
    const calls: string[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        calls.push(String(url));
        if (String(url).endsWith('/buckets/delete')) {
          return new Response('{"ok":true}', { status: 200 });
        }
        return new Response('[]', { status: 200 });
      })
    );
    const res = await runEntityAction('rustfs', 'buckets', 'delete', 'old-bucket');
    expect(res.ok).toBe(true);
    expect(calls[0]).toContain('/api/entities/rustfs/buckets/delete');
    // The follow-up reload keeps the rows in step with what just ran.
    expect(calls[1]).toContain('/api/entities/rustfs');
  });

  it('surfaces the daemon error without reloading', async () => {
    const fetchMock = vi.fn(
      async () => new Response('{"ok":false,"error":"bucket not empty"}', { status: 200 })
    );
    vi.stubGlobal('fetch', fetchMock);
    const res = await runEntityAction('rustfs', 'buckets', 'delete', 'full-bucket');
    expect(res).toEqual({ ok: false, error: 'bucket not empty' });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
