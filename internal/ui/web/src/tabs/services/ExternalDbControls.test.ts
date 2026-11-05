import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import ExternalDbControls from './ExternalDbControls.svelte';
import { sites } from '$stores/sites';
import type { Service } from '$stores/services';

const { apiFetch, useSiteDatabase, loadSites } = vi.hoisted(() => ({
  apiFetch: vi.fn(),
  useSiteDatabase: vi.fn(),
  loadSites: vi.fn()
}));
vi.mock('$lib/api', async (importOriginal) => ({ ...(await importOriginal<typeof import('$lib/api')>()), apiFetch }));
vi.mock('$stores/sites', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$stores/sites')>()),
  useSiteDatabase,
  loadSites
}));

const svc = { name: 'dbngin', status: 'active', external: 'host.containers.internal:3306' } as Service;

describe('ExternalDbControls', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sites.set([
      { domain: 'shop.test', name: 'shop' },
      { domain: 'on.test', name: 'on', external_db: 'dbngin' }
    ] as never);
  });
  afterEach(() => sites.set([]));

  it('shows the server', () => {
    render(ExternalDbControls, { props: { svc } });
    expect(screen.getByText('Server: host.containers.internal:3306')).toBeTruthy();
  });

  it('points a picked site at the server, offering only sites not on it', async () => {
    useSiteDatabase.mockResolvedValue({ ok: true });
    render(ExternalDbControls, { props: { svc } });
    await fireEvent.click(screen.getAllByRole('button').find((b) => b.getAttribute('aria-haspopup'))!);
    expect(screen.queryByText('on.test')).toBeNull();
    await fireEvent.click(await screen.findByText('shop.test'));
    await fireEvent.click(screen.getByRole('button', { name: 'Use' }));
    expect(useSiteDatabase).toHaveBeenCalledWith('shop.test', 'dbngin');
    await waitFor(() => expect(loadSites).toHaveBeenCalled());
  });

  it('keeps the form open with the reason when a new password fails', async () => {
    apiFetch.mockResolvedValue({ ok: false, text: async () => 'could not connect: access denied' });
    render(ExternalDbControls, { props: { svc } });
    await fireEvent.click(screen.getByText('Change password'));
    await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'typo' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(screen.getByText('could not connect: access denied')).toBeTruthy());
    expect(screen.getByLabelText('Password')).toBeTruthy();
  });

  it('confirms a changed password', async () => {
    apiFetch.mockResolvedValue({ ok: true });
    render(ExternalDbControls, { props: { svc } });
    await fireEvent.click(screen.getByText('Change password'));
    await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'n3w' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(screen.queryByLabelText('Password')).toBeNull());
    expect(JSON.parse(apiFetch.mock.calls[0][1].body)).toEqual({ name: 'dbngin', password: 'n3w' });
  });
});
