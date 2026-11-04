import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setDefaultNode } from './nodeVersions';

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }));
vi.mock('$lib/api', async (orig) => ({ ...((await orig()) as object), apiFetch, apiJson: async () => [] }));

describe('setDefaultNode', () => {
  beforeEach(() => apiFetch.mockReset());

  it('reports a change the server refused, even with a 200', async () => {
    apiFetch.mockResolvedValue(new Response(JSON.stringify({ ok: false, error: 'not installed' })));
    expect(await setDefaultNode('24')).toBe(false);
  });

  it('reports a saved default', async () => {
    apiFetch.mockResolvedValue(new Response(JSON.stringify({ ok: true })));
    expect(await setDefaultNode('24')).toBe(true);
  });
});
