import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import ConfirmSiteUnlinkModal from './ConfirmSiteUnlinkModal.svelte';
import { modal, openSiteUnlinkModal, closeModal } from '$stores/modals';
import { routeRest, tab } from '$stores/route';

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

  it('leaves the unlinked site page for the sites list', async () => {
    location.hash = '#sites/acme.test/env';
    await vi.waitFor(() => expect(get(routeRest)).toBe('acme.test/env'));
    const { getByText } = render(ConfirmSiteUnlinkModal);
    await fireEvent.click(getByText('Unlink'));
    await vi.waitFor(() => expect(get(routeRest)).toBe(''));
    expect(get(tab)).toBe('sites');
    expect(location.hash).toBe('#sites');
  });

  it('stays on another site page when a different site is unlinked', async () => {
    location.hash = '#sites/other.test';
    await vi.waitFor(() => expect(get(routeRest)).toBe('other.test'));
    const { getByText } = render(ConfirmSiteUnlinkModal);
    await fireEvent.click(getByText('Unlink'));
    await vi.waitFor(() => expect(get(modal).kind).toBeNull());
    expect(location.hash).toBe('#sites/other.test');
  });

  it('stays open and shows the error when the server refuses', async () => {
    location.hash = '#sites/acme.test';
    await vi.waitFor(() => expect(get(routeRest)).toBe('acme.test'));
    unlinkSite.mockResolvedValue({ ok: false, error: 'site is not linked' });
    const { getByText } = render(ConfirmSiteUnlinkModal);
    await fireEvent.click(getByText('Unlink'));
    await vi.waitFor(() => expect(getByText('site is not linked')).toBeTruthy());
    expect(loadSites).not.toHaveBeenCalled();
    expect(get(modal).kind).toBe('siteUnlink');
    expect(location.hash).toBe('#sites/acme.test');
    closeModal();
  });
});
