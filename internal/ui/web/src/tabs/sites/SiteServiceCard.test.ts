import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import SiteServiceCard from './SiteServiceCard.svelte';
import { services } from '$stores/services';

const openServiceDashboard = vi.fn();
vi.mock('$stores/dashboard', () => ({
  openServiceDashboard: (s: unknown) => openServiceDashboard(s)
}));

const phpmyadmin = { name: 'phpmyadmin', status: 'active', dashboard: 'http://localhost:8080' };

describe('SiteServiceCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    services.set([]);
  });

  it('renders the service label', () => {
    const { getByText } = render(SiteServiceCard, { props: { name: 'mysql' } });
    expect(getByText('MySQL')).toBeTruthy();
  });

  it('shows running when the service is active', () => {
    services.set([{ name: 'mysql', status: 'active' } as never]);
    const { getByText } = render(SiteServiceCard, { props: { name: 'mysql' } });
    expect(getByText('Running')).toBeTruthy();
  });

  it('shows stopped when the service is not active', () => {
    services.set([{ name: 'mysql', status: 'inactive' } as never]);
    const { getByText } = render(SiteServiceCard, { props: { name: 'mysql' } });
    expect(getByText('Stopped')).toBeTruthy();
  });

  it('treats an unknown service as stopped', () => {
    const { getByText } = render(SiteServiceCard, { props: { name: 'redis' } });
    expect(getByText('Stopped')).toBeTruthy();
  });

  it('offers a dashboard button when an active service has one', () => {
    services.set([{ name: 'mailpit', status: 'active', dashboard: 'http://localhost:8025' } as never]);
    const { getByLabelText } = render(SiteServiceCard, { props: { name: 'mailpit' } });
    expect(getByLabelText('Dashboard')).toBeTruthy();
  });

  it('hides the dashboard button when the service has no dashboard', () => {
    services.set([{ name: 'mysql', status: 'active' } as never]);
    const { queryByLabelText } = render(SiteServiceCard, { props: { name: 'mysql' } });
    expect(queryByLabelText('Dashboard')).toBeNull();
  });

  it('hides the dashboard button while the service is stopped', () => {
    services.set([{ name: 'mailpit', status: 'inactive', dashboard: 'http://localhost:8025' } as never]);
    const { queryByLabelText } = render(SiteServiceCard, { props: { name: 'mailpit' } });
    expect(queryByLabelText('Dashboard')).toBeNull();
  });

  it('opens a service that owns its dashboard', () => {
    services.set([{ name: 'mailpit', status: 'active', dashboard: 'http://localhost:8025' } as never]);
    const { getByLabelText } = render(SiteServiceCard, { props: { name: 'mailpit' } });
    getByLabelText('Dashboard').click();
    expect(openServiceDashboard).toHaveBeenCalledWith(expect.objectContaining({ name: 'mailpit' }));
  });

  // mysql declares no dashboard of its own, so reaching phpMyAdmin used to mean
  // leaving the site page for the service page.
  it('falls back to the suggested admin tool when the service has none', () => {
    services.set([{ name: 'mysql', status: 'active' }, phpmyadmin] as never);
    const { getByLabelText } = render(SiteServiceCard, { props: { name: 'mysql' } });
    getByLabelText('Open phpMyAdmin').click();
    expect(openServiceDashboard).toHaveBeenCalledWith(expect.objectContaining({ name: 'phpmyadmin' }));
  });

  // A MariaDB service reaches phpMyAdmin through the mysql family, matched on
  // its connection_url rather than its name.
  it('resolves the admin tool through the service family', () => {
    services.set([
      { name: 'mariadb-11-8', status: 'active', connection_url: 'mysql://localhost' },
      phpmyadmin
    ] as never);
    const { getByLabelText } = render(SiteServiceCard, { props: { name: 'mariadb-11-8' } });
    expect(getByLabelText('Open phpMyAdmin')).toBeTruthy();
  });

  // The opener starts a stopped tool, so the card must still offer it.
  it('offers an installed but stopped admin tool', () => {
    services.set([{ name: 'mysql', status: 'active' }, { ...phpmyadmin, status: 'inactive' }] as never);
    const { getByLabelText } = render(SiteServiceCard, { props: { name: 'mysql' } });
    expect(getByLabelText('Open phpMyAdmin')).toBeTruthy();
  });

  it('hides the button when the suggested admin tool is not installed', () => {
    services.set([{ name: 'mysql', status: 'active' }] as never);
    const { queryByLabelText } = render(SiteServiceCard, { props: { name: 'mysql' } });
    expect(queryByLabelText('Open phpMyAdmin')).toBeNull();
  });

  it('hides the admin button while the service itself is stopped', () => {
    services.set([{ name: 'mysql', status: 'inactive' }, phpmyadmin] as never);
    const { queryByLabelText } = render(SiteServiceCard, { props: { name: 'mysql' } });
    expect(queryByLabelText('Open phpMyAdmin')).toBeNull();
  });
});
