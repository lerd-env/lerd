import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import ExternalDbUseModal from './ExternalDbUseModal.svelte';
import { sites } from '$stores/sites';
import type { Service } from '$stores/services';

const { useSiteDatabase, loadSites } = vi.hoisted(() => ({ useSiteDatabase: vi.fn(), loadSites: vi.fn() }));
vi.mock('$stores/sites', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$stores/sites')>()),
  useSiteDatabase,
  loadSites
}));

const svc = { name: 'dbngin', status: 'active', external: 'host.containers.internal:3306' } as Service;

async function pick(name: string) {
  await fireEvent.click(screen.getAllByRole('button').find((b) => b.getAttribute('aria-haspopup'))!);
  await fireEvent.click(await screen.findByText(name));
}

describe('ExternalDbUseModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sites.set([
      { domain: 'shop.test', name: 'shop' },
      { domain: 'on.test', name: 'on', external_db: 'dbngin' }
    ] as never);
  });
  afterEach(() => sites.set([]));

  it('points a picked site at the server and closes, offering only sites not on it', async () => {
    useSiteDatabase.mockResolvedValue({ ok: true });
    const onclose = vi.fn();
    render(ExternalDbUseModal, { props: { open: true, svc, onclose } });
    await fireEvent.click(screen.getAllByRole('button').find((b) => b.getAttribute('aria-haspopup'))!);
    expect(screen.queryByText('on.test')).toBeNull();
    await fireEvent.click(await screen.findByText('shop.test'));
    await fireEvent.click(screen.getByRole('button', { name: 'Use' }));
    expect(useSiteDatabase).toHaveBeenCalledWith('shop.test', 'dbngin');
    await waitFor(() => expect(onclose).toHaveBeenCalled());
  });

  it('stays open with the reason when the site cannot move', async () => {
    useSiteDatabase.mockResolvedValue({ ok: false, error: 'this project uses nothing' });
    const onclose = vi.fn();
    render(ExternalDbUseModal, { props: { open: true, svc, onclose } });
    await pick('shop.test');
    await fireEvent.click(screen.getByRole('button', { name: 'Use' }));
    await waitFor(() => expect(screen.getByText('this project uses nothing')).toBeTruthy());
    expect(onclose).not.toHaveBeenCalled();
  });

  it('has nothing to do until a site is picked', () => {
    render(ExternalDbUseModal, { props: { open: true, svc, onclose: vi.fn() } });
    expect((screen.getByRole('button', { name: 'Use' }) as HTMLButtonElement).disabled).toBe(true);
  });
});
