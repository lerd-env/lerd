import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

describe('dashboard store', () => {
  beforeEach(() => {
    location.hash = '';
  });

  it('proxied bundled dashboard embeds in the overlay and lists in the sidebar', async () => {
    const { services } = await import('./services');
    const { openDashboard, dashboardOpen, dashboardServices } = await import('./dashboard');

    const rabbit = {
      name: 'rabbitmq',
      status: 'active',
      site_count: 0,
      dashboard: '/_svc/rabbitmq/',
      dashboard_external: false
    };
    services.set([rabbit]);

    expect(get(dashboardServices).some((s) => s.name === 'rabbitmq')).toBe(true);

    const open = vi.spyOn(window, 'open').mockImplementation(() => null);
    openDashboard(rabbit);
    expect(open).not.toHaveBeenCalled();
    expect(get(dashboardOpen)?.dashboard).toBe('/_svc/rabbitmq/');
    expect(location.hash).toBe('#service/rabbitmq');
    open.mockRestore();
  });

  it('user external dashboard still opens in a new tab and is not embedded', async () => {
    const { services } = await import('./services');
    const { openDashboard, dashboardOpen, dashboardServices } = await import('./dashboard');

    const ext = {
      name: 'myadmin',
      status: 'active',
      site_count: 0,
      dashboard: 'http://localhost:9000',
      dashboard_external: true
    };
    services.set([ext]);
    dashboardOpen.set(null);

    expect(get(dashboardServices).some((s) => s.name === 'myadmin')).toBe(false);

    const open = vi.spyOn(window, 'open').mockImplementation(() => null);
    openDashboard(ext);
    expect(open).toHaveBeenCalledWith('http://localhost:9000', '_blank', 'noopener,noreferrer');
    expect(get(dashboardOpen)).toBeNull();
    open.mockRestore();
  });

  it('openDocs opens the built-in documentation on its first page', async () => {
    const { openDocs, dashboardOpen } = await import('./dashboard');

    dashboardOpen.set(null);
    openDocs();
    const cur = get(dashboardOpen);
    expect(cur?.name).toBe('docs');
    // The pages come from the daemon; lerd.sh is only where the header links out.
    expect(cur?.dashboard).toBe('https://lerd.sh');
    expect(location.hash).toBe('#docs/getting-started/requirements');
  });

  it('openMailpitMessage opens overlay with extraPath when mailpit is present', async () => {
    const { services } = await import('./services');
    const { openMailpitMessage, dashboardOpen } = await import('./dashboard');

    services.set([
      { name: 'mailpit', status: 'active', site_count: 0, dashboard: 'http://localhost:8025' }
    ]);

    openMailpitMessage('abc123');
    const cur = get(dashboardOpen);
    expect(cur?.name).toBe('mailpit');
    expect(cur?.dashboard).toBe('http://localhost:8025');
    expect(cur?.extraPath).toBe('/view/abc123');
    expect(location.hash).toBe('#service/mailpit/view/abc123');
  });

  it('openMailpitMessage is a no-op when mailpit has no dashboard', async () => {
    const { services } = await import('./services');
    const { openMailpitMessage, dashboardOpen } = await import('./dashboard');

    services.set([{ name: 'mailpit', status: 'inactive', site_count: 0 }]);
    dashboardOpen.set(null);

    openMailpitMessage('abc');
    expect(get(dashboardOpen)).toBeNull();
  });

  it('encodes ids that contain url-special characters', async () => {
    const { services } = await import('./services');
    const { openMailpitMessage, dashboardOpen } = await import('./dashboard');

    services.set([
      { name: 'mailpit', status: 'active', site_count: 0, dashboard: 'http://localhost:8025' }
    ]);

    openMailpitMessage('id/with spaces');
    const cur = get(dashboardOpen);
    expect(cur?.extraPath).toBe('/view/id%2Fwith%20spaces');
  });

  it('opens an admin on the engine it was opened from, not the first one it finds', async () => {
    const { services } = await import('./services');
    const { openAdminForEngine } = await import('./dashboard');

    services.set([
      { name: 'postgres', status: 'active', site_count: 0, is_database: true },
      {
        name: 'adminer',
        status: 'active',
        site_count: 0,
        dashboard: 'http://localhost:8081',
        preset: 'adminer',
        admin_for: ['mysql', 'postgres']
      }
    ]);

    await openAdminForEngine('postgres');
    expect(location.hash).toBe('#service/adminer/on/postgres');
  });

  it('rehydrates an engine-scoped admin deep-link from the hash', async () => {
    const { services } = await import('./services');
    const { initDashboardRoute, dashboardOpen } = await import('./dashboard');

    services.set([
      { name: 'postgres', status: 'active', site_count: 0, is_database: true },
      {
        name: 'adminer',
        status: 'active',
        site_count: 0,
        dashboard: 'http://localhost:8081',
        preset: 'adminer',
        admin_for: ['mysql', 'postgres']
      }
    ]);
    location.hash = 'service/adminer/on/postgres';
    initDashboardRoute();

    expect(get(dashboardOpen)?.extraPath).toBe('?lerd_server=lerd-postgres');
  });

  it('carries the engine alongside the database when both are known', async () => {
    const { services } = await import('./services');
    const { openDatabaseAdmin, dashboardOpen, initDashboardRoute } = await import('./dashboard');

    services.set([
      { name: 'postgres', status: 'active', site_count: 0, is_database: true },
      {
        name: 'adminer',
        status: 'active',
        site_count: 0,
        dashboard: 'http://localhost:8081',
        preset: 'adminer',
        admin_for: ['mysql', 'postgres']
      }
    ]);

    await openDatabaseAdmin('postgres', 'dating');
    expect(location.hash).toBe('#service/adminer/on/postgres/dating');
    initDashboardRoute();
    expect(get(dashboardOpen)?.extraPath).toBe('?lerd_server=lerd-postgres&db=dating');
  });

  it('opens one entity at the address its preset declares', async () => {
    const { services } = await import('./services');
    const { entities } = await import('./entities');
    const { openEntityInDashboard, dashboardOpen } = await import('./dashboard');

    const rustfs = {
      name: 'rustfs',
      status: 'active' as const,
      site_count: 0,
      dashboard: '/rustfs/console/'
    };
    services.set([rustfs]);
    entities.set({
      rustfs: [
        {
          kind: 'buckets',
          columns: [],
          actions: [],
          rows: [],
          dashboard_link: 'browser/?bucket={{name}}'
        }
      ]
    });

    await openEntityInDashboard(rustfs, 'buckets', 'my bucket');
    expect(get(dashboardOpen)?.extraPath).toBe('browser/?bucket=my%20bucket');
    expect(location.hash).toBe('#service/rustfs/entity/buckets/my%20bucket');
  });

  it('rehydrates an entity deep-link from the hash', async () => {
    const { services } = await import('./services');
    const { entities } = await import('./entities');
    const { initDashboardRoute, dashboardOpen } = await import('./dashboard');

    services.set([{ name: 'rustfs', status: 'active', site_count: 0, dashboard: '/rustfs/console/' }]);
    entities.set({
      rustfs: [
        { kind: 'buckets', columns: [], actions: [], rows: [], dashboard_link: 'browser/?bucket={{name}}' }
      ]
    });
    location.hash = 'service/rustfs/entity/buckets/astrolov';
    initDashboardRoute();

    expect(get(dashboardOpen)?.extraPath).toBe('browser/?bucket=astrolov');
  });

  // A service whose dashboard has no address per entity offers no such link, and
  // the card shows no button.
  it('opens nothing when the entity has no declared address', async () => {
    const { services } = await import('./services');
    const { entities } = await import('./entities');
    const { openEntityInDashboard, dashboardOpen } = await import('./dashboard');

    const svc = { name: 'redis', status: 'active' as const, site_count: 0, dashboard: '/_svc/redis/' };
    services.set([svc]);
    entities.set({ redis: [{ kind: 'keys', columns: [], actions: [], rows: [] }] });
    dashboardOpen.set(null);

    await openEntityInDashboard(svc, 'keys', 'session');
    expect(get(dashboardOpen)).toBeNull();
  });
});
