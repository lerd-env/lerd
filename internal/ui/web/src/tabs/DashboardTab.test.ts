import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import DashboardTab from './DashboardTab.svelte';
import { status, statusLoaded } from '$stores/status';
import { unhealthyWorkers } from '$stores/workerHealth';
import { sites } from '$stores/sites';
import { services } from '$stores/services';

vi.mock('./dashboard/SetupCard.svelte', () => import('./services/ServiceDetail.stub.svelte'));
vi.mock('./dashboard/SitesWidget.svelte', () => import('./services/ServiceDetail.stub.svelte'));
vi.mock('./dashboard/ServicesWidget.svelte', () => import('./services/ServiceDetail.stub.svelte'));
vi.mock('./dashboard/WorkersWidget.svelte', () => import('./services/ServiceDetail.stub.svelte'));
vi.mock('./dashboard/SystemHealthWidget.svelte', () => import('./services/ServiceDetail.stub.svelte'));
vi.mock('./dashboard/ResourcesWidget.svelte', () => import('./services/ServiceDetail.stub.svelte'));
vi.mock('./dashboard/LerdInfoWidget.svelte', () => import('./services/ServiceDetail.stub.svelte'));

const allGood = /running smoothly/;
const notAll = /Not everything is running/;

describe('DashboardTab', () => {
  beforeEach(() => {
    statusLoaded.set(true);
    status.update((s) => ({
      ...s,
      dns: { ok: true, status: 'ok', vpn: false, enabled: true, tld: 'test' },
      nginx: { running: true },
      watcher_running: true
    }) as never);
    unhealthyWorkers.set([]);
    sites.set([{ domain: 'up.test', uses_php: true, fpm_running: true }]);
    services.set([{ name: 'mysql', status: 'active', site_count: 1 }]);
  });

  it('says everything is running smoothly when nothing is broken', () => {
    const { getByText } = render(DashboardTab);
    expect(getByText(allGood)).toBeTruthy();
  });

  it('says not everything is running while a site cannot be served', () => {
    sites.set([
      { domain: 'up.test', uses_php: true, fpm_running: true },
      { domain: 'down.test', uses_php: true }
    ]);
    const { queryByText, getByText } = render(DashboardTab);
    expect(queryByText(allGood)).toBeNull();
    expect(getByText(notAll)).toBeTruthy();
  });

  it('says not everything is running while a service has failed', () => {
    services.set([
      { name: 'mysql', status: 'active', site_count: 1 },
      { name: 'mailpit', status: 'failed', site_count: 0 }
    ]);
    const { queryByText, getByText } = render(DashboardTab);
    expect(queryByText(allGood)).toBeNull();
    expect(getByText(notAll)).toBeTruthy();
  });

  it('still says everything is running smoothly with a paused site and a stopped service', () => {
    sites.set([
      { domain: 'up.test', uses_php: true, fpm_running: true },
      { domain: 'paused.test', uses_php: true, paused: true }
    ]);
    services.set([
      { name: 'mysql', status: 'active', site_count: 1 },
      { name: 'redis', status: 'inactive', site_count: 0 }
    ]);
    const { getByText } = render(DashboardTab);
    expect(getByText(allGood)).toBeTruthy();
  });
});
