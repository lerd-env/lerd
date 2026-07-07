import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import SiteServiceCard from './SiteServiceCard.svelte';
import { services } from '$stores/services';

describe('SiteServiceCard', () => {
  beforeEach(() => services.set([]));

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
});
