import { cleanup, render, screen, fireEvent } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import SiteServiceLinkModal from './SiteServiceLinkModal.svelte';
import { modal } from '$stores/modals';

const { linkableServices, linkSiteService, loadSites } = vi.hoisted(() => ({
  linkableServices: vi.fn(),
  linkSiteService: vi.fn(),
  loadSites: vi.fn()
}));
vi.mock('$stores/sites', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$stores/sites')>()),
  linkableServices,
  linkSiteService,
  loadSites
}));

function open() {
  modal.set({ kind: 'siteServiceLink', siteServiceLink: { domain: 'shop.test' } });
  render(SiteServiceLinkModal);
}
const addButton = () => screen.getByText('Add').closest('button') as HTMLButtonElement;

describe('SiteServiceLinkModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    linkableServices.mockResolvedValue({ names: ['mailpit', 'mariadb-12-3'], writesEnv: true });
    linkSiteService.mockResolvedValue({ ok: true });
  });
  afterEach(() => modal.set({ kind: null }));

  it('adds the picked service, reloads the sites and closes', async () => {
    open();
    await fireEvent.click(await screen.findByText('Mailpit'));
    await fireEvent.click(await screen.findByText('mariadb-12-3'));
    await fireEvent.click(addButton());
    await vi.waitFor(() => expect(get(modal).kind).toBeNull());
    expect(linkSiteService).toHaveBeenCalledWith('shop.test', 'mariadb-12-3');
    expect(loadSites).toHaveBeenCalled();
  });

  it('stays open with the reason when adding fails', async () => {
    linkSiteService.mockResolvedValue({ ok: false, error: 'lerd link: boom' });
    open();
    await screen.findByText('Mailpit');
    await fireEvent.click(addButton());
    expect(await screen.findByText('lerd link: boom')).toBeTruthy();
    expect(get(modal).kind).toBe('siteServiceLink');
    expect(loadSites).not.toHaveBeenCalled();
  });

  it('only promises to point the env file where lerd writes one', async () => {
    open();
    expect(await screen.findByText(/the env file is pointed at it/)).toBeTruthy();
    cleanup();
    linkableServices.mockResolvedValue({ names: ['mailpit'], writesEnv: false });
    open();
    expect(await screen.findByText(/Lerd does not write this site's env file/)).toBeTruthy();
    expect(screen.queryByText(/the env file is pointed at it/)).toBeNull();
  });

  it('says so when there is nothing to add', async () => {
    linkableServices.mockResolvedValue({ names: [], writesEnv: true });
    open();
    expect(await screen.findByText(/Every installed service/)).toBeTruthy();
    expect(addButton().disabled).toBe(true);
  });

  it('shows why the list could not be read', async () => {
    linkableServices.mockRejectedValue(new Error('site not found'));
    open();
    expect(await screen.findByText('site not found')).toBeTruthy();
    expect(addButton().disabled).toBe(true);
  });
});
