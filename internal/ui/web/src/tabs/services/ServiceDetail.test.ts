import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';

// The children do IO on mount (log SSE, the loopback-only databases fetch);
// swap them for stubs so these tests stay on the tab-visibility logic.
import { vi } from 'vitest';
vi.mock('./ServiceHeader.svelte', () => import('./ServiceDetail.stub.svelte'));
vi.mock('./ServiceSiteBadges.svelte', () => import('./ServiceDetail.stub.svelte'));
vi.mock('./PresetSuggestionBanner.svelte', () => import('./ServiceDetail.stub.svelte'));
vi.mock('./ServiceDatabasesTab.svelte', () => import('./ServiceDetail.stub.svelte'));
vi.mock('$components/LogViewer.svelte', () => import('./ServiceDetail.stub.svelte'));

import ServiceDetail from './ServiceDetail.svelte';
import { accessMode } from '$stores/accessMode';
import type { Service } from '$stores/services';

function dbService(): Service {
  return {
    name: 'mysql',
    status: 'active',
    site_count: 0,
    preset_owned: true,
    is_database: true
  } as Service;
}

describe('ServiceDetail databases tab', () => {
  beforeEach(() => accessMode.set({ loopback: true, lanExposed: false, checked: true }));

  it('shows the Databases tab on the loopback host', () => {
    const { getByRole } = render(ServiceDetail, { props: { svc: dbService() } });
    expect(getByRole('button', { name: 'Databases' })).toBeInTheDocument();
  });

  // The whole /api/databases subtree is loopback-only, so on a LAN-exposed
  // dashboard the tab would report a running engine as stopped and 403 every
  // action. It must be hidden there instead.
  it('hides the Databases tab on a LAN-exposed dashboard', () => {
    accessMode.set({ loopback: false, lanExposed: true, checked: true });
    const { queryByRole } = render(ServiceDetail, { props: { svc: dbService() } });
    expect(queryByRole('button', { name: 'Databases' })).toBeNull();
  });
});
