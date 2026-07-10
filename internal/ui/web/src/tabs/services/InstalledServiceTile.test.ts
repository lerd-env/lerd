import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import InstalledServiceTile from './InstalledServiceTile.svelte';
import { services, type Service } from '$stores/services';

const openServiceDashboard = vi.fn();
vi.mock('$stores/dashboard', () => ({
  openServiceDashboard: (s: unknown) => openServiceDashboard(s)
}));

function svc(over: Partial<Service> = {}): Service {
  return {
    name: 'mysql',
    status: 'inactive',
    site_count: 0,
    category: 'databases',
    icon: 'database',
    ...over
  } as Service;
}

const phpmyadmin = {
  name: 'phpmyadmin',
  status: 'active',
  dashboard: 'http://localhost:8080',
  admin_for: ['mysql', 'mariadb']
};

describe('InstalledServiceTile', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    services.set([]);
  });

  it('renders the human label for the service', () => {
    const { getByText } = render(InstalledServiceTile, { props: { svc: svc() } });
    expect(getByText('MySQL')).toBeTruthy();
  });

  it('shows a green dot when active', () => {
    const { container } = render(InstalledServiceTile, { props: { svc: svc({ status: 'active' }) } });
    expect(container.querySelector('.bg-emerald-500')).toBeTruthy();
  });

  it('shows version and site count when present', () => {
    const { getByText } = render(InstalledServiceTile, {
      props: { svc: svc({ version: 'v8.4', site_count: 3 }) }
    });
    expect(getByText('v8.4')).toBeTruthy();
    expect(getByText('3')).toBeTruthy();
  });

  it('shows the update arrow when an update is available', () => {
    const { getByText } = render(InstalledServiceTile, {
      props: { svc: svc({ update_available: true }) }
    });
    expect(getByText('↑')).toBeTruthy();
  });

  // The tile now wears the preset card's shell so the installed and available
  // sections of the services dashboard read as one grid.
  it('renders the preset card shell with a tinted category icon', () => {
    const { container } = render(InstalledServiceTile, { props: { svc: svc() } });
    const card = container.firstElementChild as HTMLElement;
    expect(card.className).toContain('rounded-xl');
    expect(card.className).toContain('p-3');
    // mysql is a database, so it takes the indigo tint every card shares.
    expect(container.querySelector('.w-9.h-9')?.className).toContain('indigo');
  });

  // The navigating button and the dashboard icon must be siblings: a button
  // cannot be nested inside another button.
  it('keeps the dashboard icon outside the navigating button', () => {
    services.set([svc({ status: 'active' }), phpmyadmin] as never);
    const { container, getByLabelText } = render(InstalledServiceTile, {
      props: { svc: svc({ status: 'active' }) }
    });
    const icon = getByLabelText('Open phpMyAdmin');
    expect(icon.closest('button')).toBe(icon);
    expect(container.querySelectorAll('button')).toHaveLength(2);
  });

  it('opens a service that owns its dashboard', () => {
    const rabbit = svc({ name: 'rabbitmq', status: 'active', dashboard: 'http://localhost:15672' });
    services.set([rabbit]);
    const { getByLabelText } = render(InstalledServiceTile, { props: { svc: rabbit } });
    getByLabelText('Dashboard').click();
    expect(openServiceDashboard).toHaveBeenCalledWith(expect.objectContaining({ name: 'rabbitmq' }));
  });

  it('falls back to the suggested admin tool when the service has none', () => {
    services.set([svc({ status: 'active' }), phpmyadmin] as never);
    const { getByLabelText } = render(InstalledServiceTile, {
      props: { svc: svc({ status: 'active' }) }
    });
    getByLabelText('Open phpMyAdmin').click();
    expect(openServiceDashboard).toHaveBeenCalledWith(expect.objectContaining({ name: 'phpmyadmin' }));
  });

  it('renders no dashboard icon when no admin tool is installed', () => {
    services.set([svc({ status: 'active' })]);
    const { container } = render(InstalledServiceTile, { props: { svc: svc({ status: 'active' }) } });
    expect(container.querySelectorAll('button')).toHaveLength(1);
  });

  it('renders no dashboard icon while the service is stopped', () => {
    services.set([svc({ status: 'inactive' }), phpmyadmin] as never);
    const { container } = render(InstalledServiceTile, { props: { svc: svc() } });
    expect(container.querySelectorAll('button')).toHaveLength(1);
  });
});
