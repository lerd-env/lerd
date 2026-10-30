import { render, fireEvent, within } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { tick } from 'svelte';
import ServicesDashboard from './ServicesDashboard.svelte';
import { services, servicesLoaded, type Service } from '$stores/services';
import { accessMode } from '$stores/accessMode';
import { presets, presetsLoaded } from '$stores/presets';
import { routeRest } from '$stores/route';

const updateServices = vi.fn();
vi.mock('$stores/services', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/services')>();
  return { ...actual, updateServices: (...args: unknown[]) => updateServices(...args) };
});
vi.mock('$stores/presets', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/presets')>();
  return { ...actual, loadPresets: vi.fn() };
});

function svc(over: Partial<Service> & { name: string }): Service {
  return { status: 'active', site_count: 0, ...over };
}

describe('ServicesDashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    routeRest.set('');
    servicesLoaded.set(true);
    accessMode.set({ localControl: true } as never);
    presetsLoaded.set(true);
    presets.set([
      { name: 'valkey', description: 'Redis-compatible store', category: 'cache' },
      { name: 'mongo', description: 'Document database', category: 'databases' },
      { name: 'kafka', description: 'Event streaming broker', category: 'messaging' }
    ] as never);
    services.set([
      svc({ name: 'mysql', version: 'v8.4' }),
      svc({ name: 'mailpit', version: 'v1.27', update_available: true, latest_version: 'v1.28' }),
      svc({ name: 'redis', version: 'v7', update_available: true, latest_version: 'v7.4' }),
      svc({ name: 'rustfs', version: 'latest', update_available: true, latest_version: 'latest' })
    ]);
  });

  it('keeps the header out of the scrolling area', () => {
    const { container } = render(ServicesDashboard);
    const header = container.querySelector('.page-header')!;
    expect(header.closest('.overflow-y-auto')).toBeNull();
    expect(container.querySelector('.overflow-y-auto')).not.toBeNull();
  });

  it('opens the update dialog from the update count in the header', async () => {
    const { getByRole } = render(ServicesDashboard);
    await fireEvent.click(getByRole('button', { name: '3 updates' }));
    expect(within(getByRole('dialog')).getByText('v1.27 → v1.28')).toBeTruthy();
  });

  it('says one update in the singular', () => {
    services.set([svc({ name: 'mysql', update_available: true })]);
    const { getByRole } = render(ServicesDashboard);
    expect(getByRole('button', { name: '1 update' })).toBeTruthy();
  });

  it('offers no update all when nothing has an update', () => {
    services.set([svc({ name: 'mysql' })]);
    const { queryByText } = render(ServicesDashboard);
    expect(queryByText('Update all')).toBeNull();
  });

  it('offers no update all without local control', () => {
    accessMode.set({ localControl: false } as never);
    const { queryByText } = render(ServicesDashboard);
    expect(queryByText('Update all')).toBeNull();
  });

  // Installed services share one compact grid ordered by type, with the ones
  // waiting on an update leading their type, and a chip narrows it to a type.
  it('lists installed services in one grid and filters them by type', async () => {
    services.set([
      svc({ name: 'mysql', category: 'databases' }),
      svc({ name: 'postgres', category: 'databases', update_available: true }),
      svc({ name: 'redis', category: 'cache' })
    ]);
    const { container, getByRole, queryByRole } = render(ServicesDashboard);
    const names = () => [...container.querySelectorAll('[data-testid="installed-grid"] .font-semibold')].map((n) => n.textContent);
    expect(names()).toEqual(['PostgreSQL', 'MySQL', 'Redis']);
    expect(container.querySelector('section section h3')).toBeNull();
    expect(queryByRole('button', { name: 'Messaging' })).toBeNull();
    await fireEvent.click(getByRole('button', { name: 'Cache' }));
    expect(names()).toEqual(['Redis']);
  });

  // The two halves are separate views, so the long preset list no longer
  // pushes the installed services up the page.
  it('shows installed services only until Discover is opened', async () => {
    const { getByText, queryByText, getByRole } = render(ServicesDashboard);
    expect(getByText('MySQL')).toBeTruthy();
    expect(queryByText('Valkey')).toBeNull();
    await fireEvent.click(getByRole('button', { name: 'Add a service' }));
    expect(location.hash).toBe('#services/_discover');
  });

  // A preset already run stays on offer while it has versions left to add.
  it('offers the other versions of an installed preset in Discover', () => {
    presets.update((list) => [
      ...list,
      { name: 'mysql', category: 'databases', installed: true, installed_tags: ['8.4'], versions: [{ tag: '8.4' }, { tag: '8.0' }] },
      { name: 'redis', category: 'cache', installed: true }
    ] as never);
    routeRest.set('_discover');
    const { getByText, queryByText } = render(ServicesDashboard);
    expect(getByText('MySQL')).toBeTruthy();
    expect(queryByText('Redis')).toBeNull();
  });

  // A chip whose last preset went away must not leave the list filtered by a
  // type with no chip left to clear it.
  it('falls back to every type when the picked one runs out of presets', async () => {
    routeRest.set('_discover');
    const { getByRole, getByText } = render(ServicesDashboard);
    await fireEvent.click(getByRole('button', { name: 'Cache' }));
    presets.update((list) => list.filter((p) => p.name !== 'valkey'));
    await tick();
    expect(getByText('Kafka')).toBeTruthy();
    expect(getByText('MongoDB')).toBeTruthy();
  });

  it('finds a preset in Discover by search or by type', async () => {
    routeRest.set('_discover');
    const { getByText, queryByText, getByLabelText, getByRole } = render(ServicesDashboard);
    expect(queryByText('MySQL')).toBeNull();
    expect(getByText('Valkey')).toBeTruthy();
    await fireEvent.input(getByLabelText('Search presets…'), { target: { value: 'stream' } });
    expect(getByText('Kafka')).toBeTruthy();
    expect(queryByText('Valkey')).toBeNull();
    await fireEvent.input(getByLabelText('Search presets…'), { target: { value: '' } });
    await fireEvent.click(getByRole('button', { name: 'Databases' }));
    expect(getByText('MongoDB')).toBeTruthy();
    expect(queryByText('Kafka')).toBeNull();
  });

  it('confirms the batch, updates only the outdated services and reports each failure', async () => {
    updateServices.mockResolvedValue([
      { name: 'mailpit', ok: true },
      { name: 'redis', ok: false, error: 'pull failed' },
      { name: 'rustfs', ok: true }
    ]);
    const { getByText, getByRole, findByText } = render(ServicesDashboard);

    await fireEvent.click(getByText('Update all'));
    expect(getByText('v1.27 → v1.28')).toBeTruthy();
    expect(getByText('v7 → v7.4')).toBeTruthy();
    expect(updateServices).not.toHaveBeenCalled();

    await fireEvent.click(getByRole('button', { name: 'Update' }));
    expect(updateServices).toHaveBeenCalledWith(['mailpit', 'redis', 'rustfs'], expect.any(Function));
    expect(await findByText('2 updated')).toBeTruthy();
    expect(getByText('Redis: pull failed')).toBeTruthy();
  });

  // Every outdated service starts switched on, even when there is only one,
  // and the user can leave any of them out of the batch.
  it('updates only the services left switched on', async () => {
    updateServices.mockResolvedValue([]);
    const { getByText, getByRole, getAllByRole } = render(ServicesDashboard);

    await fireEvent.click(getByText('Update all'));
    const switches = getAllByRole('switch');
    expect(switches.map((s) => s.getAttribute('aria-label'))).toEqual(['Mailpit', 'Redis', 'RustFS']);
    expect(switches.every((s) => s.getAttribute('aria-checked') === 'true')).toBe(true);

    await fireEvent.click(getByRole('switch', { name: 'Redis' }));
    await fireEvent.click(getByRole('button', { name: 'Update' }));
    expect(updateServices).toHaveBeenCalledWith(['mailpit', 'rustfs'], expect.any(Function));
  });

  it('holds the update back while every service is switched off', async () => {
    services.set([svc({ name: 'mailpit', version: 'latest', update_available: true, latest_version: 'latest' })]);
    const { getByText, getByRole } = render(ServicesDashboard);

    await fireEvent.click(getByText('Update all'));
    expect(getByRole('switch', { name: 'Mailpit' }).getAttribute('aria-checked')).toBe('true');
    await fireEvent.click(getByRole('switch', { name: 'Mailpit' }));
    expect((getByRole('button', { name: 'Update' }) as HTMLButtonElement).disabled).toBe(true);
  });

  // A row's live progress is cleared once its update ends, so while the rest
  // of the batch is still pulling the row has to keep its own outcome.
  it('keeps each finished row on its outcome while the batch runs on', async () => {
    updateServices.mockImplementation((_names: string[], onresult: (r: unknown) => void) => {
      onresult({ name: 'mailpit', ok: true });
      onresult({ name: 'redis', ok: false, error: 'pull failed' });
      return new Promise(() => {});
    });
    const { getByText, getByRole, findByText } = render(ServicesDashboard);

    await fireEvent.click(getByText('Update all'));
    await fireEvent.click(getByRole('button', { name: 'Update' }));
    expect(await findByText('Updated')).toBeTruthy();
    expect(getByText('pull failed')).toBeTruthy();
    expect(within(getByRole('dialog')).getByText('latest')).toBeTruthy();
  });
});
