import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

describe('sites actions', () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('pauseSite POSTs /pause', async () => {
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: unknown) => {
      calls.push(String(url));
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const { pauseSite } = await import('./sites');
    const r = await pauseSite('a.test');
    expect(r.ok).toBe(true);
    expect(calls[0]).toBe('/api/sites/a.test/pause');
  });

  it('getSiteNginx GETs the per-site override', async () => {
    globalThis.fetch = vi.fn(
      async () => new Response(JSON.stringify({ path: '/x/custom.d/a.test.conf', content: '# snippet\n', exists: true }), { status: 200 })
    ) as unknown as typeof fetch;
    const { getSiteNginx } = await import('./sites');
    const res = await getSiteNginx('a.test');
    expect(res.path).toContain('a.test.conf');
    expect(res.content).toContain('# snippet');
    expect(res.exists).toBe(true);
  });

  it('saveSiteNginx POSTs the content and backup flag to /nginx', async () => {
    const calls: Array<[string, RequestInit | undefined]> = [];
    globalThis.fetch = vi.fn(async (url: unknown, init?: RequestInit) => {
      calls.push([String(url), init]);
      return new Response(
        JSON.stringify({
          ok: true,
          backup_name: 'a.test.conf.bkp.20260528-101010',
          validation_output: 'nginx -t ok'
        }),
        { status: 200 }
      );
    }) as unknown as typeof fetch;
    const { saveSiteNginx } = await import('./sites');
    const res = await saveSiteNginx('a.test', 'client_max_body_size 100m;\n', true);
    expect(res.ok).toBe(true);
    expect(res.backupName).toBe('a.test.conf.bkp.20260528-101010');
    expect(res.validationOutput).toBe('nginx -t ok');
    expect(calls[0][0]).toBe('/api/sites/a.test/nginx');
    expect(calls[0][1]?.method).toBe('POST');
    expect(JSON.parse(String(calls[0][1]?.body))).toEqual({
      content: 'client_max_body_size 100m;\n',
      backup: true
    });
  });

  it('saveSiteNginx surfaces validation_output when nginx -t fails', async () => {
    globalThis.fetch = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            ok: false,
            error: 'nginx config invalid, rolled back to previous contents',
            validation_output: 'nginx: [emerg] unknown directive "oops"'
          }),
          { status: 200 }
        )
    ) as unknown as typeof fetch;
    const { saveSiteNginx } = await import('./sites');
    const res = await saveSiteNginx('a.test', 'oops;\n');
    expect(res.ok).toBe(false);
    expect(res.error).toContain('rolled back');
    expect(res.validationOutput).toContain('unknown directive');
  });

  it('saveSiteNginx surfaces content + exists on success so onSuccess can refresh original', async () => {
    globalThis.fetch = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            ok: true,
            content: 'client_max_body_size 100m;\n',
            exists: true,
            validation_output: 'ok'
          }),
          { status: 200 }
        )
    ) as unknown as typeof fetch;
    const { saveSiteNginx } = await import('./sites');
    const res = await saveSiteNginx('a.test', 'client_max_body_size 100m;\n');
    expect(res.ok).toBe(true);
    expect(res.content).toContain('client_max_body_size');
    expect(res.exists).toBe(true);
  });

  it('saveSiteNginx defaults backup to false', async () => {
    const calls: Array<[string, RequestInit | undefined]> = [];
    globalThis.fetch = vi.fn(async (url: unknown, init?: RequestInit) => {
      calls.push([String(url), init]);
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const { saveSiteNginx } = await import('./sites');
    await saveSiteNginx('a.test', 'x;\n');
    expect(JSON.parse(String(calls[0][1]?.body))).toEqual({ content: 'x;\n', backup: false });
  });

  it('loadSiteNginxBackups returns ok=true with the parsed list', async () => {
    globalThis.fetch = vi.fn(
      async () => new Response('[{"name":"a.test.conf.bkp.20260528-101010","mtime_unix":1}]', { status: 200 })
    ) as unknown as typeof fetch;
    const { loadSiteNginxBackups } = await import('./sites');
    const res = await loadSiteNginxBackups('a.test');
    expect(res.ok).toBe(true);
    expect(res.list).toEqual([{ name: 'a.test.conf.bkp.20260528-101010', mtime_unix: 1 }]);
    expect(res.error).toBeUndefined();
  });

  it('loadSiteNginxBackups returns ok=false with error when the server fails', async () => {
    // The earlier shape collapsed errors to [] which made a 500 from the
    // server indistinguishable from "no backups exist" — the Restore
    // button would silently disappear with no signal. The new shape
    // surfaces the failure so the UI can show it.
    globalThis.fetch = vi.fn(
      async () => new Response('internal error', { status: 500 })
    ) as unknown as typeof fetch;
    const { loadSiteNginxBackups } = await import('./sites');
    const res = await loadSiteNginxBackups('a.test');
    expect(res.ok).toBe(false);
    expect(res.list).toEqual([]);
    expect(res.error).toMatch(/500/);
  });

  it('resetSiteNginx POSTs to /nginx/reset', async () => {
    const calls: Array<[string, RequestInit | undefined]> = [];
    globalThis.fetch = vi.fn(async (url: unknown, init?: RequestInit) => {
      calls.push([String(url), init]);
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const { resetSiteNginx } = await import('./sites');
    const r = await resetSiteNginx('a.test');
    expect(r.ok).toBe(true);
    expect(calls[0][0]).toBe('/api/sites/a.test/nginx/reset');
    expect(calls[0][1]?.method).toBe('POST');
  });

  it('restoreSiteNginx POSTs the requested backup name to /nginx/restore', async () => {
    const calls: Array<[string, RequestInit | undefined]> = [];
    globalThis.fetch = vi.fn(async (url: unknown, init?: RequestInit) => {
      calls.push([String(url), init]);
      return new Response(
        JSON.stringify({
          ok: true,
          restored: 'a.test.conf.bkp.20260528-101010',
          content: '# old\n'
        }),
        { status: 200 }
      );
    }) as unknown as typeof fetch;
    const { restoreSiteNginx } = await import('./sites');
    const r = await restoreSiteNginx('a.test', 'a.test.conf.bkp.20260528-101010');
    expect(calls[0][0]).toBe('/api/sites/a.test/nginx/restore');
    expect(JSON.parse(String(calls[0][1]?.body))).toEqual({
      name: 'a.test.conf.bkp.20260528-101010'
    });
    expect(r.ok).toBe(true);
    expect(r.restored).toBe('a.test.conf.bkp.20260528-101010');
    expect(r.content).toBe('# old\n');
  });

  it('resumeSite POSTs /unpause', async () => {
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: unknown) => {
      calls.push(String(url));
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const { resumeSite } = await import('./sites');
    await resumeSite('a.test');
    expect(calls[0]).toBe('/api/sites/a.test/unpause');
  });

  it('toggleTLS flips between secure/unsecure', async () => {
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: unknown) => {
      calls.push(String(url));
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const { toggleTLS } = await import('./sites');
    await toggleTLS({ domain: 'a.test', tls: false });
    await toggleTLS({ domain: 'a.test', tls: true });
    expect(calls[0]).toBe('/api/sites/a.test/secure');
    expect(calls[1]).toBe('/api/sites/a.test/unsecure');
  });

  it('toggleQueue uses running state', async () => {
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: unknown) => {
      calls.push(String(url));
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const { toggleQueue } = await import('./sites');
    await toggleQueue({ domain: 'a.test', queue_running: false });
    await toggleQueue({ domain: 'a.test', queue_running: true });
    expect(calls[0]).toBe('/api/sites/a.test/queue:start');
    expect(calls[1]).toBe('/api/sites/a.test/queue:stop');
  });

  it('setSiteVersion encodes version in query', async () => {
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: unknown) => {
      calls.push(String(url));
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const { setSiteVersion } = await import('./sites');
    await setSiteVersion({ domain: 'a.test' }, 'php', '8.5');
    expect(calls[0]).toBe('/api/sites/a.test/php?version=8.5');
  });

  it('fpmContainer handles custom/frankenphp/normal', async () => {
    const { fpmContainer } = await import('./sites');
    expect(fpmContainer({ domain: 'a.test', name: 'a', custom_container: true })).toBe('lerd-custom-a');
    expect(fpmContainer({ domain: 'a.test', name: 'a', runtime: 'frankenphp' })).toBe('lerd-fp-a');
    expect(fpmContainer({ domain: 'a.test', php_version: '8.4' })).toBe('lerd-php84-fpm');
  });

  it('siteWorkerFailing checks any worker field', async () => {
    const { siteWorkerFailing } = await import('./sites');
    expect(siteWorkerFailing({ domain: 'a', queue_failing: true })).toBe(true);
    expect(siteWorkerFailing({ domain: 'a', framework_workers: [{ name: 'x', failing: true }] })).toBe(true);
    expect(siteWorkerFailing({ domain: 'a' })).toBe(false);
  });
});
