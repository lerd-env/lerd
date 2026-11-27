import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import ConfirmSiteUnlinkModal from './ConfirmSiteUnlinkModal.svelte';
import { modal, openSiteUnlinkModal, closeModal } from '$stores/modals';
import { routeRest, tab } from '$stores/route';

const { unlinkSite, loadSites, previewSiteDirDelete } = vi.hoisted(() => ({
  unlinkSite: vi.fn(),
  loadSites: vi.fn(),
  previewSiteDirDelete: vi.fn()
}));
vi.mock('$stores/sites', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/sites')>();
  return { ...actual, unlinkSite, loadSites, previewSiteDirDelete };
});
import { sites } from '$stores/sites';
import { accessMode } from '$stores/accessMode';

describe('ConfirmSiteUnlinkModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    unlinkSite.mockResolvedValue({ ok: true });
    loadSites.mockResolvedValue(undefined);
    openSiteUnlinkModal({ domain: 'acme.test' });
    sites.set([{ domain: 'acme.test', name: 'acme', path: '/home/me/Lerd/acme' }]);
    accessMode.set({ localControl: true, local: true, lanExposed: false, checked: true });
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

  it('offers deleting the folder only on the lerd host itself', () => {
    accessMode.set({ localControl: true, local: false, lanExposed: true, checked: true });
    const { queryByLabelText } = render(ConfirmSiteUnlinkModal);
    expect(queryByLabelText(/delete the project folder/)).toBeNull();
  });

  it('names what goes with the folder and waits for the site name before deleting', async () => {
    previewSiteDirDelete.mockResolvedValue({
      path: '/home/me/Lerd/acme',
      inside_worktrees: ['/home/me/Lerd/acme/acme-feat'],
      uncommitted: ['/home/me/Lerd/acme'],
      unpushed: 2,
      unchecked: ['/home/me/Lerd/acme-fix']
    });
    const { getByLabelText, getByText, getByRole } = render(ConfirmSiteUnlinkModal);
    await fireEvent.click(getByLabelText(/delete the project folder/));
    await vi.waitFor(() => expect(getByText(/acme-feat is deleted with it/)).toBeTruthy());
    expect(getByText(/has uncommitted changes/)).toBeTruthy();
    expect(getByText(/2 commits are not on any remote/)).toBeTruthy();
    expect(getByText(/could not read \/home\/me\/Lerd\/acme-fix/)).toBeTruthy();

    const button = getByRole('button', { name: 'Unlink and delete' });
    expect(button).toBeDisabled();
    await fireEvent.input(getByLabelText(/Type acme to confirm/), { target: { value: 'acme' } });
    expect(button).not.toBeDisabled();
    await fireEvent.click(button);
    expect(unlinkSite).toHaveBeenCalledWith('acme.test', 'acme');
  });

  it('says why a folder cannot be deleted and never allows it', async () => {
    previewSiteDirDelete.mockResolvedValue({ path: '/srv/acme', refused: '/srv/acme is outside the folders lerd mounts' });
    const { getByLabelText, getByText, getByRole, queryByLabelText } = render(ConfirmSiteUnlinkModal);
    await fireEvent.click(getByLabelText(/delete the project folder/));
    await vi.waitFor(() => expect(getByText(/outside the folders lerd mounts/)).toBeTruthy());
    expect(queryByLabelText(/to confirm/)).toBeNull();
    expect(getByRole('button', { name: 'Unlink and delete' })).toBeDisabled();
  });
});
