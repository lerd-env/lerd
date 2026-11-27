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

const lerdHealthy = /^Lerd is running smoothly$/;

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

  it('says lerd is running smoothly, with no site or service counts', () => {
    const { getByText } = render(DashboardTab);
    expect(getByText(lerdHealthy)).toBeTruthy();
  });

  // Sites and services report themselves in their own cards.
  it('still says lerd is running smoothly while a site or service is down', () => {
    sites.set([
      { domain: 'up.test', uses_php: true, fpm_running: true },
      { domain: 'down.test', uses_php: true }
    ]);
    services.set([
      { name: 'mysql', status: 'active', site_count: 1 },
      { name: 'mailpit', status: 'failed', site_count: 0 }
    ]);
    const { getByText } = render(DashboardTab);
    expect(getByText(lerdHealthy)).toBeTruthy();
  });

  it('stops saying lerd is running smoothly when part of lerd is down', () => {
    status.update((s) => ({ ...s, nginx: { running: false } }) as never);
    const { queryByText } = render(DashboardTab);
    expect(queryByText(lerdHealthy)).toBeNull();
  });
});
