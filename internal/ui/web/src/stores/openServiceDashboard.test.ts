import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// openServiceDashboard is shared by the service page and the site overview card.
// A stopped dashboard service must be started before the overlay opens, otherwise
// it embeds a URL nothing is listening on. serviceAction refreshes the services
// store itself when the start succeeds, so the mocks below do the same.
// Kept out of dashboard.test.ts because mocking './services' there would leak
// into the tests that exercise the real store.
vi.mock('./services', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./services')>();
  return { ...actual, serviceAction: vi.fn() };
});

describe('openServiceDashboard', () => {
  beforeEach(() => {
    location.hash = '';
    vi.clearAllMocks();
  });

  it('opens a running dashboard service without starting it', async () => {
    const { services, serviceAction } = await import('./services');
    const { openServiceDashboard, dashboardOpen } = await import('./dashboard');

    const pma = { name: 'phpmyadmin', status: 'active', site_count: 0, dashboard: '/_svc/phpmyadmin/' };
    services.set([pma]);
    dashboardOpen.set(null);

    await openServiceDashboard(pma);

    expect(serviceAction).not.toHaveBeenCalled();
    expect(get(dashboardOpen)?.name).toBe('phpmyadmin');
  });

  it('starts a stopped dashboard service and opens the refreshed record', async () => {
    const { services, serviceAction } = await import('./services');
    const { openServiceDashboard, dashboardOpen } = await import('./dashboard');

    const stopped = { name: 'phpmyadmin', status: 'inactive', site_count: 0, dashboard: '/_svc/phpmyadmin/' };
    services.set([stopped]);
    dashboardOpen.set(null);

    // The start flips the record to active; the helper must open that one, not
    // the stale object it was handed.
    vi.mocked(serviceAction).mockImplementation(async () => {
      services.set([{ ...stopped, status: 'active', dashboard: '/_svc/phpmyadmin/?fresh' }]);
      return true;
    });

    await openServiceDashboard(stopped);

    expect(serviceAction).toHaveBeenCalledWith('phpmyadmin', 'start');
    expect(get(dashboardOpen)?.dashboard).toBe('/_svc/phpmyadmin/?fresh');
  });

  it('opens nothing when the start fails', async () => {
    const { services, serviceAction } = await import('./services');
    const { openServiceDashboard, dashboardOpen } = await import('./dashboard');

    const stopped = { name: 'phpmyadmin', status: 'inactive', site_count: 0, dashboard: '/_svc/phpmyadmin/' };
    services.set([stopped]);
    dashboardOpen.set(null);
    vi.mocked(serviceAction).mockResolvedValue(false);

    await openServiceDashboard(stopped);

    expect(get(dashboardOpen)).toBeNull();
    expect(location.hash).toBe('');
  });

  it('falls back to the passed service when the reload does not return it', async () => {
    const { services, serviceAction } = await import('./services');
    const { openServiceDashboard, dashboardOpen } = await import('./dashboard');

    const stopped = { name: 'pgadmin', status: 'inactive', site_count: 0, dashboard: '/_svc/pgadmin/' };
    services.set([stopped]);
    dashboardOpen.set(null);
    vi.mocked(serviceAction).mockImplementation(async () => {
      services.set([]);
      return true;
    });

    await openServiceDashboard(stopped);

    expect(get(dashboardOpen)?.dashboard).toBe('/_svc/pgadmin/');
  });
});
