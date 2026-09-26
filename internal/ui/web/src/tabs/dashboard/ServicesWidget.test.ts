import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import ServicesWidget from './ServicesWidget.svelte';
import { services, servicesLoaded, type Service } from '$stores/services';
import { accessMode } from '$stores/accessMode';

vi.mock('$stores/dashboard', () => ({
  openServiceDashboard: vi.fn()
}));

function svc(over: Partial<Service> = {}): Service {
  return {
    name: 'mysql',
    status: 'active',
    site_count: 0,
    category: 'databases',
    icon: 'database',
    ...over
  } as Service;
}

// The tile's own label, told apart from the card title by its darker ink.
function tileLabels(container: HTMLElement): string[] {
  return [...container.querySelectorAll('.font-semibold.text-gray-900')].map(
    (n) => n.textContent?.trim() ?? ''
  );
}

describe('ServicesWidget', () => {
  beforeEach(() => {
    services.set([]);
    servicesLoaded.set(true);
    accessMode.set({ localControl: true, lanExposed: false, checked: true });
  });

  it('shows the empty hint when nothing is installed', () => {
    const { getByText } = render(ServicesWidget);
    expect(getByText('No services running.')).toBeTruthy();
  });

  // The widget writes no markup of its own: every service goes through the
  // same tile the services dashboard grid renders.
  it('renders one compact service card per installed service', () => {
    services.set([svc(), svc({ name: 'redis', category: 'cache', icon: 'cache' })]);
    const { container, getByText } = render(ServicesWidget);
    expect(getByText('MySQL')).toBeTruthy();
    expect(getByText('Redis')).toBeTruthy();
    const cards = container.querySelectorAll('.rounded-lg.p-2\\.5');
    expect(cards).toHaveLength(2);
  });

  it('spells out the running state and the version the way the tile does', () => {
    services.set([svc({ version: 'v8.4' }), svc({ name: 'redis', status: 'inactive' })]);
    const { getByText } = render(ServicesWidget);
    expect(getByText('Running')).toBeTruthy();
    expect(getByText('Stopped')).toBeTruthy();
    expect(getByText('v8.4')).toBeTruthy();
  });

  // The card scrolls, so a service with an update can sit out of sight. Float
  // those to the top and the tile's arrow is on screen without a banner.
  it('sorts services with an update ahead of the rest', () => {
    services.set([
      svc({ name: 'redis' }),
      svc({ name: 'mysql', update_available: true }),
      svc({ name: 'mailpit' })
    ]);
    const { container } = render(ServicesWidget);
    expect(tileLabels(container)[0]).toBe('MySQL');
  });

  it('leads with the service the most sites use', () => {
    services.set([
      svc({ name: 'redis', site_count: 1 }),
      svc({ name: 'mysql', site_count: 4 }),
      svc({ name: 'mailpit', site_count: 2 })
    ]);
    const { container } = render(ServicesWidget);
    expect(tileLabels(container)).toEqual(['MySQL', 'Mailpit', 'Redis']);
  });

  it('still floats an update above a busier service', () => {
    services.set([
      svc({ name: 'mysql', site_count: 4 }),
      svc({ name: 'redis', update_available: true })
    ]);
    const { container } = render(ServicesWidget);
    expect(tileLabels(container)[0]).toBe('Redis');
  });

  it('keeps the store order among services with the same site count', () => {
    services.set([svc({ name: 'redis' }), svc({ name: 'mailpit' }), svc({ name: 'mysql' })]);
    const { container } = render(ServicesWidget);
    expect(tileLabels(container)).toEqual(['Redis', 'Mailpit', 'MySQL']);
  });

  // The tile already carries an update arrow, so the banner that used to sit
  // above the list only said the same thing twice.
  it('replaces the update banner with a pill beside the running count', () => {
    services.set([svc({ update_available: true }), svc({ name: 'redis', status: 'inactive' })]);
    const { container, getByText, queryByText } = render(ServicesWidget);
    expect(getByText('1/2 active')).toBeTruthy();
    expect(getByText('1 update(s)')).toBeTruthy();
    expect(queryByText('↑')).toBeTruthy();
    expect(container.querySelector('.bg-yellow-50')).toBeNull();
  });

  it('counts sleeping services in their own badge and keeps the running one green', () => {
    services.set([
      svc({ name: 'mysql', status: 'inactive', idle_suspended: true }),
      svc({ name: 'redis', status: 'inactive', idle_suspended: true }),
      svc({ name: 'mailpit', status: 'active' })
    ]);
    const { getByText } = render(ServicesWidget);
    expect(getByText('2/3 suspended').closest('span')!.className).toMatch(/text-sky-700/);
    expect(getByText('1/3 active').closest('span')!.className).toMatch(/text-emerald-700/);
  });

  it('still turns the running count amber for a service that is simply stopped', () => {
    services.set([svc({ name: 'mysql', status: 'inactive' }), svc({ name: 'redis', status: 'active' })]);
    const { getByText, queryByText } = render(ServicesWidget);
    expect(getByText('1/2 active').closest('span')!.className).toMatch(/text-yellow-700/);
    expect(queryByText(/suspended/)).toBeNull();
  });

  it('shows no update pill when everything is current', () => {
    services.set([svc()]);
    const { queryByText } = render(ServicesWidget);
    expect(queryByText('0 update(s)')).toBeNull();
  });

  it('hides the add button without local control', () => {
    accessMode.set({ localControl: false, lanExposed: false, checked: true });
    services.set([svc()]);
    const { queryByText, getByText } = render(ServicesWidget);
    expect(queryByText('Add')).toBeNull();
    expect(getByText('Open services →')).toBeTruthy();
  });
});
