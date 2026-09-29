import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import ConfirmSiteServiceRemoveModal from './ConfirmSiteServiceRemoveModal.svelte';
import { modal, openSiteServiceRemoveModal, closeModal } from '$stores/modals';

const { removeSiteService, loadSites } = vi.hoisted(() => ({ removeSiteService: vi.fn(), loadSites: vi.fn() }));
vi.mock('$stores/sites', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/sites')>();
  return { ...actual, removeSiteService, loadSites };
});

describe('ConfirmSiteServiceRemoveModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    removeSiteService.mockResolvedValue({ ok: true });
    loadSites.mockResolvedValue(undefined);
    openSiteServiceRemoveModal({ domain: 'acme.test', name: 'redis' });
  });

  it('names the service and says the service itself keeps running', () => {
    const { getByText } = render(ConfirmSiteServiceRemoveModal);
    expect(getByText('Remove Redis from acme.test?')).toBeTruthy();
    expect(getByText(/stops pointing at it/)).toBeTruthy();
  });

  it('removes nothing and closes when cancelled', async () => {
    const { getByText } = render(ConfirmSiteServiceRemoveModal);
    await fireEvent.click(getByText('Cancel'));
    expect(removeSiteService).not.toHaveBeenCalled();
    expect(get(modal).kind).toBeNull();
  });

  it('removes the service, refreshes the sites and closes when confirmed', async () => {
    const { getByText } = render(ConfirmSiteServiceRemoveModal);
    await fireEvent.click(getByText('Remove'));
    expect(removeSiteService).toHaveBeenCalledWith('acme.test', 'redis');
    await vi.waitFor(() => expect(get(modal).kind).toBeNull());
    expect(loadSites).toHaveBeenCalled();
  });

  it('stays open and shows the error when the server refuses', async () => {
    removeSiteService.mockResolvedValue({ ok: false, error: 'redis is not listed in .lerd.yaml' });
    const { getByText } = render(ConfirmSiteServiceRemoveModal);
    await fireEvent.click(getByText('Remove'));
    await vi.waitFor(() => expect(getByText('redis is not listed in .lerd.yaml')).toBeTruthy());
    expect(loadSites).not.toHaveBeenCalled();
    expect(get(modal).kind).toBe('siteServiceRemove');
    closeModal();
  });
});
