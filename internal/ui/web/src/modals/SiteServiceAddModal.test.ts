import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import SiteServiceAddModal from './SiteServiceAddModal.svelte';
import { presets, presetsLoaded } from '$stores/presets';
import { modal } from '$stores/modals';
import { services } from '$stores/services';

vi.mock('$stores/presets', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$stores/presets')>()),
  loadPresets: vi.fn()
}));

const { addSuggestedService, loadSites, useSiteDatabase } = vi.hoisted(() => ({
  addSuggestedService: vi.fn(),
  loadSites: vi.fn(),
  useSiteDatabase: vi.fn()
}));
vi.mock('$stores/sites', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$stores/sites')>()),
  addSuggestedService,
  loadSites,
  useSiteDatabase
}));

const { confirmDownload } = vi.hoisted(() => ({ confirmDownload: vi.fn() }));
vi.mock('$stores/downloadConfirm', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$stores/downloadConfirm')>()),
  confirmDownload
}));

const postgres = {
  name: 'postgres',
  family: 'postgres',
  versions: [{ tag: '18' }, { tag: '17' }, { tag: '16' }],
  default_version: '16'
};
const pgvector = {
  name: 'postgres-pgvector',
  family: 'postgres',
  description: 'PostgreSQL with pgvector',
  versions: [{ tag: '18' }, { tag: '17' }],
  default_version: '18'
};

function open(list: unknown[]) {
  presets.set(list as never);
  modal.set({
    kind: 'siteServiceAdd',
    siteServiceAdd: { domain: 'shop.test', name: 'postgres' }
  });
  render(SiteServiceAddModal);
}

describe('SiteServiceAddModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    presetsLoaded.set(true);
    confirmDownload.mockResolvedValue(true);
    addSuggestedService.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    presets.set([]);
    presetsLoaded.set(false);
    modal.set({ kind: null });
  });

  it('starts on the suggested service at its default version', async () => {
    open([postgres, pgvector]);
    await fireEvent.click(screen.getByText('Add'));
    await vi.waitFor(() =>
      expect(addSuggestedService).toHaveBeenCalledWith('shop.test', 'postgres', 'postgres', '16')
    );
    expect(confirmDownload).toHaveBeenCalledWith(expect.any(String), { preset: 'postgres', version: '16' });
    await vi.waitFor(() => expect(loadSites).toHaveBeenCalled());
  });

  // What already runs here adds without a download, so it is the first pick.
  it('starts on a variant this machine already runs', async () => {
    open([postgres, { ...pgvector, installed: true, installed_tags: ['17'] }]);
    await fireEvent.click(screen.getByText('Add'));
    await vi.waitFor(() =>
      expect(addSuggestedService).toHaveBeenCalledWith('shop.test', 'postgres', 'postgres-pgvector', '17')
    );
  });

  // The pick is held while the download prompt is up, and a second click waits.
  it('adds once, what the prompt described', async () => {
    let answer!: (ok: boolean) => void;
    confirmDownload.mockReturnValue(new Promise<boolean>((r) => (answer = r)));
    open([postgres, pgvector]);
    const button = screen.getByText('Add').closest('button')!;
    await fireEvent.click(button);
    await fireEvent.click(button);
    answer(true);
    await vi.waitFor(() => expect(addSuggestedService).toHaveBeenCalled());
    expect(confirmDownload).toHaveBeenCalledTimes(1);
    expect(addSuggestedService).toHaveBeenCalledTimes(1);
    expect(addSuggestedService).toHaveBeenCalledWith('shop.test', 'postgres', 'postgres', '16');
  });

  it('adds nothing when the download is declined', async () => {
    confirmDownload.mockResolvedValue(false);
    open([postgres, pgvector]);
    await fireEvent.click(screen.getByText('Add'));
    await vi.waitFor(() => expect(confirmDownload).toHaveBeenCalled());
    expect(addSuggestedService).not.toHaveBeenCalled();
  });

  it('shows why the add failed and stays open', async () => {
    addSuggestedService.mockResolvedValue({ ok: false, error: 'lerd link: boom' });
    open([postgres, pgvector]);
    await fireEvent.click(screen.getByText('Add'));
    expect(await screen.findByText('lerd link: boom')).toBeInTheDocument();
  });

  it('offers an external server of the same engine and points the site at it', async () => {
    useSiteDatabase.mockResolvedValue({ ok: true });
    services.set([
      { name: 'dbngin', status: 'active', external: 'host.containers.internal:5433', family: 'postgres' },
      { name: 'other', status: 'active', external: 'host.containers.internal:3306', family: 'mariadb' }
    ] as never);
    open([postgres]);
    await fireEvent.click(screen.getAllByRole('button').find((b) => b.getAttribute('aria-haspopup'))!);
    await fireEvent.click(await screen.findByText('dbngin'));
    expect(screen.queryByText('other')).toBeNull();
    const add = screen.getByRole('button', { name: 'Add' }) as HTMLButtonElement;
    expect(add.disabled).toBe(false);
    await fireEvent.click(add);
    expect(useSiteDatabase).toHaveBeenCalledWith('shop.test', 'dbngin');
    expect(addSuggestedService).not.toHaveBeenCalled();
    expect(confirmDownload).not.toHaveBeenCalled();
    services.set([]);
  });
});
