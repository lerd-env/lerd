import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { setDefaultNode, watchNodeVersions, nodeVersionStatuses } from './nodeVersions';

const { apiFetch, apiJson } = vi.hoisted(() => ({ apiFetch: vi.fn(), apiJson: vi.fn() }));
vi.mock('$lib/api', async (orig) => ({ ...((await orig()) as object), apiFetch, apiJson }));

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

// `lerd node:update` or another browser changes the versions without telling
// this tab, so an open dashboard has to ask again or keep offering stale updates.
describe('watchNodeVersions', () => {
  const behind = { major: '24', installed: '24.1.0', latest: '24.2.0', update_available: true };
  const current = { ...behind, installed: '24.2.0', update_available: false };

  beforeEach(() => {
    vi.useFakeTimers();
    apiJson.mockReset();
  });
  afterEach(() => vi.useRealTimers());

  it('picks up a change made elsewhere and stops when told', async () => {
    apiJson.mockImplementation(async (url: string) => (url.endsWith('/status') ? [behind] : ['24']));
    const stop = watchNodeVersions();
    await vi.advanceTimersByTimeAsync(0);
    expect(get(nodeVersionStatuses)).toEqual([behind]);

    apiJson.mockImplementation(async (url: string) => (url.endsWith('/status') ? [current] : ['24']));
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    await vi.advanceTimersByTimeAsync(5 * 60_000);
    expect(get(nodeVersionStatuses)).toEqual([current]);

    stop();
    apiJson.mockClear();
    await vi.advanceTimersByTimeAsync(5 * 60_000);
    expect(apiJson).not.toHaveBeenCalled();
  });
});
