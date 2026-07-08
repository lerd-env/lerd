import { describe, it, expect, vi, beforeEach } from 'vitest';

const apiJson = vi.fn();
vi.mock('$lib/api', () => ({ apiJson: (...args: unknown[]) => apiJson(...args) }));

import { loadSiteAnalytics, loadRouteQueries } from './analytics';

describe('loadSiteAnalytics', () => {
  beforeEach(() => apiJson.mockReset().mockResolvedValue({}));

  it('requests the analytics endpoint with the range', async () => {
    await loadSiteAnalytics('acme.test', '24h');
    expect(apiJson).toHaveBeenCalledWith('/api/sites/acme.test/analytics?range=24h');
  });

  it('includes the worktree branch when given', async () => {
    await loadSiteAnalytics('acme.test', '1h', 'staging');
    expect(apiJson).toHaveBeenCalledWith('/api/sites/acme.test/analytics?range=1h&branch=staging');
  });

  it('encodes a domain with unusual characters', async () => {
    await loadSiteAnalytics('a b.test', '15m');
    expect(apiJson).toHaveBeenCalledWith('/api/sites/a%20b.test/analytics?range=15m');
  });

  it('requests route queries with the encoded route key', async () => {
    await loadRouteQueries('acme.test', 'GET /account/profiles/:id');
    expect(apiJson).toHaveBeenCalledWith(
      '/api/sites/acme.test/route-queries?route=GET+%2Faccount%2Fprofiles%2F%3Aid'
    );
  });

  it('includes the branch for route queries when given', async () => {
    await loadRouteQueries('acme.test', 'GET /x', 'staging');
    expect(apiJson).toHaveBeenCalledWith('/api/sites/acme.test/route-queries?route=GET+%2Fx&branch=staging');
  });
});
