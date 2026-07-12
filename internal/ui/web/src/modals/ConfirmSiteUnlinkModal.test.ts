import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import ConfirmSiteUnlinkModal from './ConfirmSiteUnlinkModal.svelte';
import { modal, openSiteUnlinkModal, closeModal } from '$stores/modals';

const { unlinkSite, loadSites } = vi.hoisted(() => ({ unlinkSite: vi.fn(), loadSites: vi.fn() }));
vi.mock('$stores/sites', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/sites')>();
  return { ...actual, unlinkSite, loadSites };
});

describe('ConfirmSiteUnlinkModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    unlinkSite.mockResolvedValue({ ok: true });
    loadSites.mockResolvedValue(undefined);
    openSiteUnlinkModal({ domain: 'acme.test' });
  });

  it('names the site and says the files survive', () => {
    const { getByText } = render(ConfirmSiteUnlinkModal);
    expect(getByText('Unlink acme.test?')).toBeTruthy();
    expect(getByText(/files stay on disk/)).toBeTruthy();
  });

  it('unlinks nothing and closes when cancelled', async () => {
    const { getByText } = render(ConfirmSiteUnlinkModal);
    await fireEvent.click(getByText('Cancel'));
    expect(unlinkSite).not.toHaveBeenCalled();
    expect(get(modal).kind).toBeNull();
  });

  it('unlinks the site, refreshes the list and closes when confirmed', async () => {
    const { getByText } = render(ConfirmSiteUnlinkModal);
    await fireEvent.click(getByText('Unlink'));
    expect(unlinkSite).toHaveBeenCalledWith('acme.test');
    await vi.waitFor(() => expect(get(modal).kind).toBeNull());
    expect(loadSites).toHaveBeenCalled();
  });

  it('stays open and shows the error when the server refuses', async () => {
    unlinkSite.mockResolvedValue({ ok: false, error: 'site is not linked' });
    const { getByText } = render(ConfirmSiteUnlinkModal);
    await fireEvent.click(getByText('Unlink'));
    await vi.waitFor(() => expect(getByText('site is not linked')).toBeTruthy());
    expect(loadSites).not.toHaveBeenCalled();
    expect(get(modal).kind).toBe('siteUnlink');
    closeModal();
  });
});
