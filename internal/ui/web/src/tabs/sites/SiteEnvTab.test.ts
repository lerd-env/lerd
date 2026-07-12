import { render, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import SiteEnvTab from './SiteEnvTab.svelte';
import type { Site } from '$stores/sites';

// The tab must open the file the framework actually reads, which the server
// sorts first, rather than assuming a root .env. A Symfony site lists .env.local
// ahead of the committed .env; a CakePHP site lists only config/.env.
const loadSiteEnvFiles = vi.fn(async () => ['.env.local', '.env']);
const loadSiteEnv = vi.fn(async (_d: string, _b: string, file: string) => `FROM=${file}\n`);
const loadSiteEnvBackups = vi.fn(async () => []);
const proposeSiteEnv = vi.fn(async () => ({
  file: '.env.local',
  current: '',
  merged: '',
  added: [],
  addedLines: [],
  required: [],
  optional: [],
  entries: []
}));

vi.mock('$stores/sites', () => ({
  loadSiteEnvFiles: (...a: unknown[]) => loadSiteEnvFiles(...(a as [])),
  loadSiteEnv: (...a: unknown[]) => loadSiteEnv(...(a as [string, string, string])),
  loadSiteEnvBackups: (...a: unknown[]) => loadSiteEnvBackups(...(a as [])),
  loadSiteEnvBackupContent: vi.fn(async () => ''),
  proposeSiteEnv: (...a: unknown[]) => proposeSiteEnv(...(a as []))
}));

vi.mock('$stores/modals', () => ({
  openEnvSaveModal: vi.fn(),
  openEnvRestoreModal: vi.fn(),
  openEnvProposeModal: vi.fn()
}));

const site = { domain: 'sf.test', path: '/home/u/Code/sf', worktrees: [] } as unknown as Site;

describe('SiteEnvTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('opens the framework dotenv the server sorted first, not a root .env', async () => {
    const { getByText } = render(SiteEnvTab, { props: { site, branch: '' } });

    await waitFor(() => expect(loadSiteEnv).toHaveBeenCalled());
    expect(loadSiteEnv.mock.calls[0][2]).toBe('.env.local');
    await waitFor(() => getByText('.env.local'));
  });

  it('fetches the content once, after the file list names a file', async () => {
    render(SiteEnvTab, { props: { site, branch: '' } });

    await waitFor(() => expect(loadSiteEnv).toHaveBeenCalled());
    // Settle any follow-up effects before counting.
    await new Promise((r) => setTimeout(r, 0));

    // Firing on the empty initial selection would load the server-resolved file
    // and then reload the same file again once the list snapped it into place.
    expect(loadSiteEnv).toHaveBeenCalledTimes(1);
    expect(loadSiteEnvBackups).toHaveBeenCalledTimes(1);
    expect(loadSiteEnv.mock.calls.every(([, , f]) => f !== '')).toBe(true);
  });

  it('shows the absolute path of the selected dotenv', async () => {
    const { getByText } = render(SiteEnvTab, { props: { site, branch: '' } });

    await waitFor(() => getByText('/home/u/Code/sf/.env.local'));
  });

  it('shows the worktree path when a worktree branch is active', async () => {
    const wtSite = {
      ...site,
      worktrees: [{ branch: 'feat', path: '/home/u/Code/sf-feat' }]
    } as unknown as Site;

    const { getByText } = render(SiteEnvTab, { props: { site: wtSite, branch: 'feat' } });

    await waitFor(() => getByText('/home/u/Code/sf-feat/.env.local'));
  });

  it('opens a nested dotenv when that is the only file the framework has', async () => {
    loadSiteEnvFiles.mockResolvedValueOnce(['config/.env']);
    proposeSiteEnv.mockResolvedValueOnce({
      file: 'config/.env',
      current: '',
      merged: '',
      added: [],
      addedLines: [],
      required: [],
      optional: [],
      entries: []
    });

    render(SiteEnvTab, { props: { site, branch: '' } });

    await waitFor(() => expect(loadSiteEnv).toHaveBeenCalled());
    expect(loadSiteEnv.mock.calls[0][2]).toBe('config/.env');
  });
});
