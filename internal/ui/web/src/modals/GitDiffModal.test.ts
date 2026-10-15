import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';

const { loadGitChanges, loadGitVersions, closeModal } = vi.hoisted(() => ({
  loadGitChanges: vi.fn(),
  loadGitVersions: vi.fn(),
  closeModal: vi.fn()
}));
vi.mock('$lib/gitDiff', async (orig) => ({ ...(await orig<object>()), loadGitChanges, loadGitVersions }));
vi.mock('$stores/modals', () => ({ closeModal }));
// Monaco does not run under jsdom; the stub shows what the modal hands it.
vi.mock('$components/MonacoDiffEditor.svelte', async () => ({ default: (await import('./MonacoDiffEditor.stub.svelte')).default }));

import GitDiffModal from './GitDiffModal.svelte';

const props = { domain: 'acme.test', path: '/srv/acme', branch: 'main' };
const text = (original: string, modified: string) => ({ original, modified, binary: false, too_large: false });

describe('GitDiffModal', () => {
  beforeEach(() => {
    closeModal.mockReset();
    loadGitChanges.mockReset();
    loadGitVersions.mockReset();
  });

  it('opens the first changed file and loads another on click', async () => {
    loadGitChanges.mockResolvedValue({
      staged: [],
      unstaged: [{ path: 'app.php', status: 'M' }],
      untracked: [{ path: 'notes.md', status: '?' }]
    });
    loadGitVersions.mockImplementation(async (_d: string, _p: string, _s: string, file: string) => text('old', `new ${file}`));
    render(GitDiffModal, { props });

    const editor = await screen.findByTestId('diff');
    await waitFor(() => expect(editor.dataset.modified).toBe('new app.php'));
    expect(editor.dataset.language).toBe('php');
    expect(loadGitVersions).toHaveBeenCalledWith('acme.test', '/srv/acme', 'unstaged', 'app.php');
    expect(screen.queryByText(/Staged/)).toBeNull();

    await fireEvent.click(screen.getByText('notes.md'));
    await waitFor(() => expect(screen.getByTestId('diff').dataset.modified).toBe('new notes.md'));
    expect(loadGitVersions).toHaveBeenLastCalledWith('acme.test', '/srv/acme', 'untracked', 'notes.md');
  });

  it('passes the whitespace switch to the editor', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'c.yaml', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('    - x\n', '  - x\n'));
    render(GitDiffModal, { props });
    expect((await screen.findByTestId('diff')).dataset.ignoreSpace).toBe('false');
    await fireEvent.click(screen.getByRole('switch', { name: 'Hide whitespace' }));
    expect(screen.getByTestId('diff').dataset.ignoreSpace).toBe('true');
  });

  it('fills the window, and Escape leaves full screen before it closes the modal', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'a.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');

    await fireEvent.click(screen.getByRole('button', { name: 'Enter full screen' }));
    expect(screen.getByRole('dialog').className).toContain('h-dvh');
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.getByRole('dialog').className).toContain('h-[85vh]');
    expect(closeModal).not.toHaveBeenCalled();

    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(closeModal).toHaveBeenCalledOnce();
  });

  it('hides the file list and brings it back', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'a.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');

    // The list slides shut rather than unmounting, so it keeps its scroll; inert keeps it out of reach.
    const list = screen.getByText('a.php', { selector: 'nav *' }).closest('nav')!.parentElement!;
    await fireEvent.click(screen.getByRole('button', { name: 'Hide file list' }));
    expect(list.inert).toBe(true);
    expect(list.className).toContain('md:w-0');
    await fireEvent.click(screen.getByRole('button', { name: 'Show file list' }));
    expect(list.inert).toBe(false);
    expect(list.className).toContain('md:w-72');
  });

  it('says why a binary file has no diff', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [], untracked: [{ path: 'logo.png', status: '?' }] });
    loadGitVersions.mockResolvedValue({ original: '', modified: '', binary: true, too_large: false });
    render(GitDiffModal, { props });
    expect(await screen.findByText('Binary file, there is no text to compare.')).toBeTruthy();
    expect(screen.queryByTestId('diff')).toBeNull();
  });

  it('says so when the tree is clean', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [], untracked: [] });
    render(GitDiffModal, { props });
    await waitFor(() => expect(screen.getByText('No uncommitted changes.')).toBeTruthy());
    expect(loadGitVersions).not.toHaveBeenCalled();
  });
});
