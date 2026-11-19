import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import NginxDetail from './NginxDetail.svelte';
import { status } from '$stores/status';
import { accessMode } from '$stores/accessMode';

const { updateNginx, checkNginxUpdate, loadStatus, notifyLocalInfo, notifyLocalFailure } = vi.hoisted(() => ({
  updateNginx: vi.fn(),
  checkNginxUpdate: vi.fn(),
  loadStatus: vi.fn(),
  notifyLocalInfo: vi.fn(),
  notifyLocalFailure: vi.fn()
}));
vi.mock('$stores/status', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/status')>();
  return { ...actual, updateNginx, checkNginxUpdate, loadStatus };
});
vi.mock('$lib/notify', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/notify')>();
  return { ...actual, notifyLocalInfo, notifyLocalFailure };
});

const withNginx = (nginx: { running: boolean; update_available?: boolean }) =>
  status.set({ nginx, php_fpms: [], tools: [] } as never);

describe('NginxDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    accessMode.set({ localControl: true, local: true, lanExposed: false, checked: true });
    withNginx({ running: true });
  });

  it('offers no Update while nginx runs the newest image', () => {
    render(NginxDetail);
    expect(screen.queryByRole('button', { name: 'Update' })).toBeNull();
  });

  // A remote session without host control sees the state but cannot act on it.
  it('hides Update from a session without local control', () => {
    withNginx({ running: true, update_available: true });
    accessMode.set({ localControl: false, local: false, lanExposed: true, checked: true });
    render(NginxDetail);
    expect(screen.queryByRole('button', { name: 'Update' })).toBeNull();
  });

  it('names the reason when an update fails', async () => {
    withNginx({ running: true, update_available: true });
    updateNginx.mockResolvedValue({ ok: false, error: 'pull refused' });
    render(NginxDetail);
    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(notifyLocalFailure).toHaveBeenCalled());
    expect(notifyLocalFailure.mock.calls[0][2]).toBe('pull refused');
  });

  // Offline must never read as "up to date".
  it('reports a check it could not run as failed', async () => {
    checkNginxUpdate.mockResolvedValue({ ok: false });
    render(NginxDetail);
    await fireEvent.click(screen.getByRole('button', { name: /check for updates/i }));

    await waitFor(() => expect(notifyLocalInfo).toHaveBeenCalled());
    expect(notifyLocalInfo.mock.calls[0][2]).toMatch(/failed/i);
  });

  it('reports the newest image after a clean check', async () => {
    checkNginxUpdate.mockResolvedValue({ ok: true, update_available: false });
    render(NginxDetail);
    await fireEvent.click(screen.getByRole('button', { name: /check for updates/i }));

    await waitFor(() => expect(notifyLocalInfo).toHaveBeenCalled());
    expect(notifyLocalInfo.mock.calls[0][2]).toMatch(/newest image/i);
  });

  // Without a websocket (the lerd.sh demo) nothing else re-reads status, so the
  // flag and the Update button would outlive the update.
  it('re-reads status after a successful update', async () => {
    withNginx({ running: true, update_available: true });
    updateNginx.mockResolvedValue({ ok: true, updated: true });
    render(NginxDetail);
    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(loadStatus).toHaveBeenCalled());
  });
});
