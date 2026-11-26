import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

describe('sites store', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it('derives php and node site counts', async () => {
    const { sites, sitesByPhp, sitesByNode, phpSiteCount, nodeSiteCount } = await import('./sites');
    sites.set([
      { domain: 'a.test', php_version: '8.4', node_version: '22' },
      { domain: 'b.test', php_version: '8.5', node_version: '22' },
      { domain: 'c.test', php_version: '8.5' }
    ]);
    expect(get(sitesByPhp).get('8.5')).toBe(2);
    expect(get(sitesByPhp).get('8.4')).toBe(1);
    expect(phpSiteCount('8.5')).toBe(2);
    expect(get(sitesByNode).get('22')).toBe(2);
    expect(nodeSiteCount('22')).toBe(2);
    expect(nodeSiteCount('24')).toBe(0);
  });

  it('does not count a paused site as using its php version', async () => {
    const { sites, phpSiteCount } = await import('./sites');
    sites.set([
      { domain: 'a.test', php_version: '8.3', paused: true },
      { domain: 'b.test', php_version: '8.4' }
    ]);
    expect(phpSiteCount('8.3')).toBe(0);
    expect(phpSiteCount('8.4')).toBe(1);
  });

  it('does not count a paused site as using its node version', async () => {
    const { sites, nodeSiteCount } = await import('./sites');
    sites.set([
      { domain: 'a.test', node_version: '20', paused: true },
      { domain: 'b.test', node_version: '22' }
    ]);
    expect(nodeSiteCount('20')).toBe(0);
    expect(nodeSiteCount('22')).toBe(1);
  });

  it('activeWorktreeDomain returns the parent domain when branch is empty', async () => {
    const { activeWorktreeDomain } = await import('./sites');
    const s = {
      domain: 'acme.test',
      worktrees: [{ branch: 'feat-a', domain: 'feat-a.acme.test' }]
    };
    expect(activeWorktreeDomain(s, '')).toBe('acme.test');
  });

  it('activeWorktreeDomain returns the worktree domain when branch matches', async () => {
    const { activeWorktreeDomain } = await import('./sites');
    const s = {
      domain: 'acme.test',
      worktrees: [{ branch: 'feat-a', domain: 'feat-a.acme.test' }]
    };
    expect(activeWorktreeDomain(s, 'feat-a')).toBe('feat-a.acme.test');
  });

  it('activeWorktreeDomain falls back to the parent when branch is unknown', async () => {
    const { activeWorktreeDomain } = await import('./sites');
    const s = { domain: 'acme.test', worktrees: [] };
    expect(activeWorktreeDomain(s, 'mystery')).toBe('acme.test');
  });

  it('siteDomainForName maps a site name back to its domain', async () => {
    const { siteDomainForName } = await import('./sites');
    const list = [
      { domain: 'acme.test', name: 'acme' },
      { domain: 'admin-astrolov.test', name: 'admin-astrolov' }
    ];
    expect(siteDomainForName(list, 'admin-astrolov')).toBe('admin-astrolov.test');
    expect(siteDomainForName(list, 'mystery')).toBe('');
    expect(siteDomainForName(list, '')).toBe('');
  });

  it('siteDotColor reports serving, paused and stopped, and never guesses for an unknown site', async () => {
    const { siteDotColor } = await import('./sites');
    expect(siteDotColor({ domain: 'a.test', fpm_running: true })).toBe('green');
    expect(siteDotColor({ domain: 'a.test', fpm_running: true, paused: true })).toBe('amber');
    expect(siteDotColor({ domain: 'a.test' })).toBe('gray');
    expect(siteDotColor(undefined)).toBe('gray');
  });

  it('siteBroken flags a site lerd should serve but cannot, never one stopped on purpose', async () => {
    const { siteBroken } = await import('./sites');
    expect(siteBroken({ domain: 'a.test', uses_php: true })).toBe(true);
    expect(siteBroken({ domain: 'a.test', custom_container: true, container_port: 8000 })).toBe(true);
    expect(siteBroken({ domain: 'a.test', uses_php: true, fpm_running: true, container_build_failed: true })).toBe(true);
    expect(siteBroken({ domain: 'a.test', host_proxy: true, host_has_dev_server: true })).toBe(true);

    expect(siteBroken({ domain: 'a.test', uses_php: true, fpm_running: true })).toBe(false);
    expect(siteBroken({ domain: 'a.test', uses_php: true, paused: true })).toBe(false);
    expect(siteBroken({ domain: 'a.test', host_proxy: true, host_has_dev_server: true, idle_suspended: true })).toBe(false);
    expect(siteBroken({ domain: 'a.test', host_proxy: true })).toBe(false);
    expect(siteBroken({ domain: 'a.test' })).toBe(false);
  });

  it('brokenSites lists only the broken sites', async () => {
    const { sites, brokenSites } = await import('./sites');
    sites.set([
      { domain: 'up.test', uses_php: true, fpm_running: true },
      { domain: 'down.test', uses_php: true },
      { domain: 'paused.test', uses_php: true, paused: true }
    ]);
    expect(get(brokenSites).map((s) => s.domain)).toEqual(['down.test']);
  });

  it('saveSiteEnv PUTs JSON body to the site env endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true, backup_path: '.env.20260528-103045' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    );
    vi.stubGlobal('fetch', fetchMock);
    const { saveSiteEnv } = await import('./sites');

    const res = await saveSiteEnv('acme.test', '', 'FOO=bar\n', true);

    expect(res.ok).toBe(true);
    expect(res.backupPath).toBe('.env.20260528-103045');
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/api\/sites\/acme\.test\/env$/);
    expect(init.method).toBe('PUT');
    const sentHeaders = new Headers(init.headers);
    expect(sentHeaders.get('Content-Type')).toBe('application/json');
    expect(sentHeaders.get('X-Lerd-CSRF')).toBe('1');
    expect(JSON.parse(init.body as string)).toEqual({ content: 'FOO=bar\n', backup: true });
    vi.unstubAllGlobals();
  });

  it('saveSiteEnv appends branch query param when set', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    );
    vi.stubGlobal('fetch', fetchMock);
    const { saveSiteEnv } = await import('./sites');

    await saveSiteEnv('acme.test', 'feature/x', 'A=1', false);

    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/api\/sites\/acme\.test\/env\?branch=feature%2Fx$/);
    vi.unstubAllGlobals();
  });

  it('loadSiteEnvFiles GETs /env/files and returns the parsed list', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(['.env', '.env.local', '.env.testing']), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    );
    vi.stubGlobal('fetch', fetchMock);
    const { loadSiteEnvFiles } = await import('./sites');

    const list = await loadSiteEnvFiles('acme.test', '');

    expect(list).toEqual(['.env', '.env.local', '.env.testing']);
    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/api\/sites\/acme\.test\/env\/files$/);
    vi.unstubAllGlobals();
  });

  it('loadSiteEnvFiles returns an empty list on error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 500 })));
    const { loadSiteEnvFiles } = await import('./sites');
    expect(await loadSiteEnvFiles('acme.test', '')).toEqual([]);
    vi.unstubAllGlobals();
  });

  it('loadSiteEnvFiles passes an empty server list through rather than inventing .env', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify([]), { status: 200, headers: { 'Content-Type': 'application/json' } })
      )
    );
    const { loadSiteEnvFiles } = await import('./sites');
    expect(await loadSiteEnvFiles('acme.test', '')).toEqual([]);
    vi.unstubAllGlobals();
  });

  it('saveSiteEnv appends ?file= when targeting a non-default env file', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true, backup_path: '.env.testing.bkp.20260528-103045' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    );
    vi.stubGlobal('fetch', fetchMock);
    const { saveSiteEnv } = await import('./sites');

    await saveSiteEnv('acme.test', '', 'X=1\n', true, '.env.testing');

    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/api\/sites\/acme\.test\/env\?file=\.env\.testing$/);
    vi.unstubAllGlobals();
  });

  it('saveSiteEnv sends a nested framework dotenv encoded', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    );
    vi.stubGlobal('fetch', fetchMock);
    const { saveSiteEnv } = await import('./sites');

    await saveSiteEnv('acme.test', '', 'X=1\n', false, 'config/.env');

    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/api\/sites\/acme\.test\/env\?file=config%2F\.env$/);
    vi.unstubAllGlobals();
  });

  it('saveSiteEnv omits ?file= when no file is given, letting the server resolve the framework dotenv', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    );
    vi.stubGlobal('fetch', fetchMock);
    const { saveSiteEnv } = await import('./sites');

    await saveSiteEnv('acme.test', '', 'X=1\n', false);

    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/api\/sites\/acme\.test\/env$/);
    vi.unstubAllGlobals();
  });

  it('loadSiteEnvBackups GETs /env/backups and returns the parsed list', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          { name: '.env.20260528-103045', mtime_unix: 1779973000 },
          { name: '.env.20260101-100000', mtime_unix: 1767369600 }
        ]),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    );
    vi.stubGlobal('fetch', fetchMock);
    const { loadSiteEnvBackups } = await import('./sites');

    const list = await loadSiteEnvBackups('acme.test', '');

    expect(list).toHaveLength(2);
    expect(list[0].name).toBe('.env.20260528-103045');
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/api\/sites\/acme\.test\/env\/backups$/);
    expect(init?.method).toBeUndefined();
    vi.unstubAllGlobals();
  });

  it('loadSiteEnvBackups returns an empty list on error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 500 })));
    const { loadSiteEnvBackups } = await import('./sites');
    expect(await loadSiteEnvBackups('acme.test', '')).toEqual([]);
    vi.unstubAllGlobals();
  });

  it('restoreSiteEnv POSTs to /env/restore and returns parsed payload', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true, restored: '.env.20260528-103045', content: 'OLD=1\n' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    );
    vi.stubGlobal('fetch', fetchMock);
    const { restoreSiteEnv } = await import('./sites');

    const res = await restoreSiteEnv('acme.test', 'feature/x', '.env', '.env.20260528-103045');

    expect(res.ok).toBe(true);
    expect(res.restored).toBe('.env.20260528-103045');
    expect(res.content).toBe('OLD=1\n');
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/api\/sites\/acme\.test\/env\/restore\?branch=feature%2Fx&file=\.env$/);
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body as string)).toEqual({ name: '.env.20260528-103045' });
    vi.unstubAllGlobals();
  });

  it('saveSiteEnv surfaces backend errors verbatim', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ ok: false, error: 'writing temp file: no space' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' }
        })
      )
    );
    const { saveSiteEnv } = await import('./sites');

    const res = await saveSiteEnv('acme.test', '', '', true);

    expect(res.ok).toBe(false);
    expect(res.error).toBe('writing temp file: no space');
    vi.unstubAllGlobals();
  });

  it('siteHasLogSources sees a proxy-only host site whose only source is a stripe listener', async () => {
    const { siteHasLogSources } = await import('./sites');
    const proxyOnly = {
      domain: 'gonitro.test',
      host_proxy: true,
      host_has_dev_server: false,
      has_app_logs: false,
      uses_php: false,
      stripe_running: true
    } as unknown as import('./sites').Site;
    expect(siteHasLogSources(proxyOnly)).toBe(true);
  });

  it('siteHasLogSources covers running and failing workers, and is false with no source', async () => {
    const { siteHasLogSources } = await import('./sites');
    const base = { domain: 'x.test', host_proxy: true } as unknown as import('./sites').Site;
    expect(siteHasLogSources(base)).toBe(false);
    expect(siteHasLogSources({ ...base, queue_running: true })).toBe(true);
    expect(siteHasLogSources({ ...base, schedule_failing: true })).toBe(true);
    expect(siteHasLogSources({ ...base, host_has_dev_server: true })).toBe(true);
    expect(
      siteHasLogSources({ ...base, framework_workers: [{ name: 'vite', running: true }] })
    ).toBe(true);
  });
});
