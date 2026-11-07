import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';

const { loadGitChanges, loadGitVersions, loadCommitFiles, loadCommitVersions, stageFiles, unstageFiles, stageHunk, commitChanges, loadLastMessage, closeModal } =
  vi.hoisted(() => ({
    stageHunk: vi.fn(),
    loadGitChanges: vi.fn(),
    loadGitVersions: vi.fn(),
    loadCommitFiles: vi.fn(),
    loadCommitVersions: vi.fn(),
    stageFiles: vi.fn(),
    unstageFiles: vi.fn(),
    commitChanges: vi.fn(),
    loadLastMessage: vi.fn(),
    closeModal: vi.fn()
  }));
vi.mock('$lib/gitDiff', async (orig) => ({
  ...(await orig<object>()),
  loadGitChanges,
  loadGitVersions,
  loadCommitFiles,
  loadCommitVersions,
  stageFiles,
  unstageFiles,
  stageHunk,
  commitChanges,
  loadLastMessage
}));
vi.mock('$stores/modals', () => ({ closeModal }));
// Monaco does not run under jsdom; the stub shows what the modal hands it.
vi.mock('$components/MonacoDiffEditor.svelte', async () => ({ default: (await import('./MonacoDiffEditor.stub.svelte')).default }));

import GitDiffModal from './GitDiffModal.svelte';

const props = { domain: 'acme.test', path: '/srv/acme', branch: 'main' };
const text = (original: string, modified: string) => ({ original, modified, binary: false, too_large: false, unreadable: false });

describe('GitDiffModal', () => {
  beforeEach(() => {
    closeModal.mockReset();
    localStorage.clear();
    loadGitChanges.mockReset();
    loadGitVersions.mockReset();
    for (const f of [stageFiles, unstageFiles, stageHunk, commitChanges, loadLastMessage]) f.mockReset();
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

  it('passes the whitespace switch to the editor, and keeps the choice', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'c.yaml', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('    - x\n', '  - x\n'));
    const first = render(GitDiffModal, { props });
    expect((await screen.findByTestId('diff')).dataset.ignoreSpace).toBe('false');
    await fireEvent.click(screen.getByRole('switch', { name: 'Hide whitespace' }));
    expect(screen.getByTestId('diff').dataset.ignoreSpace).toBe('true');
    first.unmount();

    render(GitDiffModal, { props });
    expect((await screen.findByTestId('diff')).dataset.ignoreSpace).toBe('true');
  });

  it('closes from full screen and opens full screen next time', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'a.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    const first = render(GitDiffModal, { props });
    await screen.findByTestId('diff');

    await fireEvent.click(screen.getByRole('button', { name: 'Enter full screen' }));
    expect(screen.getByRole('dialog').className).toContain('h-dvh');
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(closeModal).toHaveBeenCalledOnce();
    first.unmount();

    render(GitDiffModal, { props });
    await screen.findByTestId('diff');
    expect(screen.getByRole('dialog').className).toContain('h-dvh');
    await fireEvent.click(screen.getByRole('button', { name: 'Exit full screen' }));
    expect(screen.getByRole('dialog').className).toContain('h-[85vh]');
  });

  it('switches between side by side and unified, and keeps the choice', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'a.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    const first = render(GitDiffModal, { props });
    expect((await screen.findByTestId('diff')).dataset.sideBySide).toBe('true');

    await fireEvent.click(screen.getByRole('button', { name: 'Unified view' }));
    expect(screen.getByTestId('diff').dataset.sideBySide).toBe('false');
    first.unmount();

    render(GitDiffModal, { props });
    expect((await screen.findByTestId('diff')).dataset.sideBySide).toBe('false');
    expect(screen.getByRole('button', { name: 'Side by side view' })).toBeTruthy();
  });

  it('hides the file list and brings it back', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'a.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');

    // The list slides shut rather than unmounting, so it keeps its scroll; inert keeps it out of reach.
    const list = screen.getByTestId('file-list');
    await fireEvent.click(screen.getByRole('button', { name: 'Hide file list' }));
    expect(list.inert).toBe(true);
    expect(list.className).toContain('md:w-0');
    await fireEvent.click(screen.getByRole('button', { name: 'Show file list' }));
    expect(list.inert).toBe(false);
    expect(list.className).toContain('md:w-72');
  });

  it('says why a binary file has no diff', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [], untracked: [{ path: 'logo.png', status: '?' }] });
    loadGitVersions.mockResolvedValue({ original: '', modified: '', binary: true, too_large: false, unreadable: false });
    render(GitDiffModal, { props });
    expect(await screen.findByText('Binary file, there is no text to compare.')).toBeTruthy();
    expect(screen.queryByTestId('diff')).toBeNull();
  });

  it('says when git cannot hand over a file to compare', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'c.php', status: 'U' }], untracked: [] });
    loadGitVersions.mockResolvedValue({ original: '', modified: '', binary: false, too_large: false, unreadable: true });
    render(GitDiffModal, { props });
    expect(await screen.findByText(/can't be compared here/)).toBeTruthy();
    expect(screen.queryByTestId('diff')).toBeNull();
  });

  // Answers can land out of order; a failure for a file left behind must not
  // cover the diff of the one now open.
  it('ignores a late failure for a file no longer selected', async () => {
    loadGitChanges.mockResolvedValue({
      staged: [],
      unstaged: [{ path: 'slow.php', status: 'M' }, { path: 'fast.php', status: 'M' }],
      untracked: []
    });
    let failSlow: (e: Error) => void = () => {};
    loadGitVersions.mockImplementation((_d: string, _p: string, _s: string, file: string) =>
      file === 'slow.php' ? new Promise((_, reject) => (failSlow = reject)) : Promise.resolve(text('a', 'fast'))
    );
    render(GitDiffModal, { props });
    await waitFor(() => expect(loadGitVersions).toHaveBeenCalledWith('acme.test', '/srv/acme', 'unstaged', 'slow.php'));

    await fireEvent.click(screen.getByText('fast.php'));
    await waitFor(() => expect(screen.getByTestId('diff').dataset.modified).toBe('fast'));
    failSlow(new Error('500 slow failed'));
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.queryByText('500 slow failed')).toBeNull();
    expect(screen.getByTestId('diff').dataset.modified).toBe('fast');
  });

  it('says so when the tree is clean', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [], untracked: [] });
    render(GitDiffModal, { props });
    await waitFor(() => expect(screen.getByText('No uncommitted changes.')).toBeTruthy());
    expect(loadGitVersions).not.toHaveBeenCalled();
  });

  it('shows a commit as one list and closes through its own handler', async () => {
    loadCommitFiles.mockResolvedValue([{ path: 'a.php', status: 'M' }, { path: 'b.php', status: 'A' }]);
    loadCommitVersions.mockImplementation(async (_d: string, _p: string, _h: string, file: string) => text('', file));
    const onclose = vi.fn();
    render(GitDiffModal, { props: { ...props, commit: { hash: 'abc1234def', subject: 'feat: x' }, onclose } });

    expect(await screen.findByText('abc1234 feat: x')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId('diff').dataset.modified).toBe('a.php'));
    expect(loadCommitFiles).toHaveBeenCalledWith('acme.test', '/srv/acme', 'abc1234def');
    expect(screen.queryByText(/Staged|Unstaged|Untracked/)).toBeNull();
    expect(loadGitChanges).not.toHaveBeenCalled();

    await fireEvent.click(screen.getByText('b.php'));
    await waitFor(() => expect(screen.getByTestId('diff').dataset.modified).toBe('b.php'));
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onclose).toHaveBeenCalledOnce();
    expect(closeModal).not.toHaveBeenCalled();
  });

  it('lists files under their folders and collapses a folder on click', async () => {
    loadCommitFiles.mockResolvedValue([{ path: 'app/Models/User.php', status: 'M' }, { path: 'README.md', status: 'M' }]);
    loadCommitVersions.mockImplementation(async (_d: string, _p: string, _h: string, file: string) => text('', file));
    render(GitDiffModal, { props: { ...props, commit: { hash: 'abc1234def', subject: 'feat: x' } } });

    const folder = await screen.findByRole('button', { name: 'app/Models' });
    expect(screen.getByText('User.php')).toBeInTheDocument();
    await fireEvent.click(folder);
    expect(folder).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('User.php')).toBeNull();
    expect(screen.getByText('README.md')).toBeInTheDocument();
  });

  it('shows how many lines each file adds and removes', async () => {
    loadGitChanges.mockResolvedValue({
      staged: [],
      unstaged: [{ path: 'a.php', status: 'M', added: 4, removed: 3 }, { path: 'img.png', status: 'M' }],
      untracked: [{ path: 'new.php', status: '?', added: 5 }]
    });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');
    const row = (name: string) => within(screen.getByTestId('file-list')).getByText(name).closest('div')!;
    expect(row('a.php')).toHaveTextContent('+4 −3');
    expect(row('new.php')).toHaveTextContent('+5');
    expect(row('new.php')).not.toHaveTextContent('−');
    expect(row('img.png')).not.toHaveTextContent(/[+−]\d/);
  });

  it('totals the lines each section adds and removes', async () => {
    loadGitChanges.mockResolvedValue({
      staged: [{ path: 's.php', status: 'D', removed: 7 }],
      unstaged: [{ path: 'a.php', status: 'M', added: 4, removed: 3 }, { path: 'b.php', status: 'M', added: 2, removed: 1 }, { path: 'img.png', status: 'M' }],
      untracked: [{ path: 'n.php', status: '?', added: 5 }]
    });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');
    const head = (label: RegExp) => screen.getByText(label).parentElement!;
    expect(head(/Unstaged · 3/)).toHaveTextContent('+6 −4');
    // One file already shows its own counts on its row.
    expect(head(/Staged · 1/)).not.toHaveTextContent('−7');
    expect(head(/Untracked · 1/)).not.toHaveTextContent('+5');
  });

  it('keeps a dragged split for the next file, but not for the next dialog', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'a.php', status: 'M' }, { path: 'b.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockImplementation(async (_d: string, _p: string, _s: string, file: string) => text('old', `new ${file}`));
    const first = render(GitDiffModal, { props });
    expect((await screen.findByTestId('diff')).dataset.split).toBe('undefined');

    await fireEvent.click(screen.getByRole('button', { name: 'drag split' }));
    await fireEvent.click(within(screen.getByTestId('file-list')).getByText('b.php'));
    await waitFor(() => expect(screen.getByTestId('diff').dataset.modified).toBe('new b.php'));
    expect(screen.getByTestId('diff').dataset.split).toBe('0.3');

    first.unmount();
    render(GitDiffModal, { props });
    expect((await screen.findByTestId('diff')).dataset.split).toBe('undefined');
  });

  it('says so when a commit changes no files', async () => {
    loadCommitFiles.mockResolvedValue([]);
    render(GitDiffModal, { props: { ...props, commit: { hash: 'abc1234def', subject: 'merge' } } });
    expect(await screen.findByText('This commit changes no files.')).toBeInTheDocument();
  });

  it('stages and unstages a file, then lists it in its new section', async () => {
    loadGitChanges
      .mockResolvedValueOnce({ staged: [], unstaged: [{ path: 'a.php', status: 'M' }], untracked: [] })
      .mockResolvedValueOnce({ staged: [{ path: 'a.php', status: 'M' }], unstaged: [], untracked: [] })
      .mockResolvedValueOnce({ staged: [], unstaged: [{ path: 'a.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    stageFiles.mockResolvedValue({ ok: true });
    unstageFiles.mockResolvedValue({ ok: true });
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');

    await fireEvent.click(screen.getByRole('button', { name: 'Stage a.php' }));
    expect(stageFiles).toHaveBeenCalledWith('acme.test', '/srv/acme', ['a.php']);
    expect(await screen.findByText(/Staged · 1/)).toBeInTheDocument();
    await waitFor(() => expect(loadGitVersions).toHaveBeenLastCalledWith('acme.test', '/srv/acme', 'staged', 'a.php'));

    await fireEvent.click(screen.getByRole('button', { name: 'Unstage a.php' }));
    expect(unstageFiles).toHaveBeenCalledWith('acme.test', '/srv/acme', ['a.php']);
    expect(await screen.findByText(/Unstaged · 1/)).toBeInTheDocument();
  });

  it('stages or unstages a whole section from its header', async () => {
    const changes = {
      staged: [{ path: 's.php', status: 'M' }],
      unstaged: [{ path: 'a.php', status: 'M' }, { path: 'app/b.php', status: 'D' }],
      untracked: [{ path: 'n1.md', status: '?' }, { path: 'n2.md', status: '?' }]
    };
    loadGitChanges.mockResolvedValue(changes);
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    stageFiles.mockResolvedValue({ ok: true });
    unstageFiles.mockResolvedValue({ ok: true });
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');

    await fireEvent.click(screen.getByRole('button', { name: 'Stage all Untracked' }));
    expect(stageFiles).toHaveBeenLastCalledWith('acme.test', '/srv/acme', ['n1.md', 'n2.md']);
    await fireEvent.click(screen.getByRole('button', { name: 'Stage all Unstaged' }));
    expect(stageFiles).toHaveBeenLastCalledWith('acme.test', '/srv/acme', ['a.php', 'app/b.php']);
    await fireEvent.click(screen.getByRole('button', { name: 'Unstage all Staged' }));
    expect(unstageFiles).toHaveBeenLastCalledWith('acme.test', '/srv/acme', ['s.php']);
    await waitFor(() => expect(loadGitChanges).toHaveBeenCalledTimes(4));
  });

  it('shows why a section action failed', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [], untracked: [{ path: 'n.md', status: '?' }] });
    loadGitVersions.mockResolvedValue(text('', 'b'));
    stageFiles.mockResolvedValue({ error: 'index.lock exists' });
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');

    await fireEvent.click(screen.getByRole('button', { name: 'Stage all Untracked' }));
    expect(await screen.findByText('index.lock exists')).toBeInTheDocument();
  });

  it('offers no section action on a commit', async () => {
    loadCommitFiles.mockResolvedValue([{ path: 'a.php', status: 'M' }]);
    loadCommitVersions.mockResolvedValue(text('a', 'b'));
    render(GitDiffModal, { props: { ...props, commit: { hash: 'abc1234def', subject: 'fix' } } });
    await screen.findByTestId('diff');
    expect(screen.queryByRole('button', { name: /Stage all|Unstage all/ })).toBeNull();
  });

  it('commits what is staged, then shows what is left', async () => {
    loadGitChanges
      .mockResolvedValueOnce({ staged: [{ path: 'a.php', status: 'M' }], unstaged: [{ path: 'b.php', status: 'M' }], untracked: [] })
      .mockResolvedValueOnce({ staged: [], unstaged: [{ path: 'b.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    commitChanges.mockResolvedValue({ ok: true });
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');

    const commit = screen.getByRole('button', { name: 'Commit' });
    expect(commit).toBeDisabled();
    await fireEvent.input(screen.getByPlaceholderText('Commit message'), { target: { value: 'fix: a' } });
    await fireEvent.click(commit);
    expect(commitChanges).toHaveBeenCalledWith('acme.test', '/srv/acme', 'fix: a', false, false);
    await waitFor(() => expect(screen.queryByText(/Staged · /)).toBeNull());
    expect((screen.getByPlaceholderText('Commit message') as HTMLTextAreaElement).value).toBe('');
  });

  it('cannot commit with nothing staged, and pushes when asked', async () => {
    loadGitChanges.mockResolvedValue({ staged: [], unstaged: [{ path: 'b.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    loadLastMessage.mockResolvedValue('feat: earlier');
    commitChanges.mockResolvedValue({ ok: true });
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');
    await fireEvent.input(screen.getByPlaceholderText('Commit message'), { target: { value: 'x' } });
    expect(screen.getByRole('button', { name: 'Commit' })).toBeDisabled();

    // Amending rewrites the last commit, so its message comes in to edit.
    await fireEvent.input(screen.getByPlaceholderText('Commit message'), { target: { value: '' } });
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Amend last commit' }));
    await waitFor(() => expect((screen.getByPlaceholderText('Commit message') as HTMLTextAreaElement).value).toBe('feat: earlier'));
    await fireEvent.click(screen.getByRole('button', { name: 'Commit and push' }));
    expect(commitChanges).toHaveBeenCalledWith('acme.test', '/srv/acme', 'feat: earlier', true, true);
  });

  it('keeps a commit whose push failed, and says so', async () => {
    loadGitChanges
      .mockResolvedValueOnce({ staged: [{ path: 'a.php', status: 'M' }], unstaged: [], untracked: [] })
      .mockResolvedValueOnce({ staged: [], unstaged: [], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    commitChanges.mockResolvedValue({ ok: true, warning: 'Committed, but the push failed: no remote' });
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');
    await fireEvent.input(screen.getByPlaceholderText('Commit message'), { target: { value: 'fix: a' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Commit and push' }));
    expect(await screen.findByText('Committed, but the push failed: no remote')).toBeInTheDocument();
    expect(screen.getByText('No uncommitted changes.')).toBeInTheDocument();
  });

  // The last message arrives after a round trip; text typed meanwhile wins.
  it('does not overwrite a message typed while amend loads', async () => {
    loadGitChanges.mockResolvedValue({ staged: [{ path: 'a.php', status: 'M' }], unstaged: [], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    let answer: (m: string) => void = () => {};
    loadLastMessage.mockReturnValue(new Promise((r) => (answer = r)));
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Amend last commit' }));
    await fireEvent.input(screen.getByPlaceholderText('Commit message'), { target: { value: 'typed' } });
    answer('feat: earlier');
    await new Promise((r) => setTimeout(r, 0));
    expect((screen.getByPlaceholderText('Commit message') as HTMLTextAreaElement).value).toBe('typed');
  });

  it('shows why a commit failed and keeps the message', async () => {
    loadGitChanges.mockResolvedValue({ staged: [{ path: 'a.php', status: 'M' }], unstaged: [], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    commitChanges.mockResolvedValue({ error: 'pre-commit hook failed' });
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');
    await fireEvent.input(screen.getByPlaceholderText('Commit message'), { target: { value: 'fix: a' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Commit' }));
    expect(await screen.findByText('pre-commit hook failed')).toBeInTheDocument();
    expect((screen.getByPlaceholderText('Commit message') as HTMLTextAreaElement).value).toBe('fix: a');
  });

  it('offers no commit box for a commit already made', async () => {
    loadCommitFiles.mockResolvedValue([{ path: 'a.php', status: 'M' }]);
    loadCommitVersions.mockResolvedValue(text('a', 'b'));
    render(GitDiffModal, { props: { ...props, commit: { hash: 'abc1234def', subject: 'feat: x', time: 0 } } });
    await screen.findByTestId('diff');
    expect(screen.queryByPlaceholderText('Commit message')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Stage a.php' })).toBeNull();
  });

  it('stages one change block and stays on the part left unstaged', async () => {
    loadGitChanges
      .mockResolvedValueOnce({ staged: [], unstaged: [{ path: 'a.php', status: 'M' }], untracked: [{ path: 'n.md', status: '?' }] })
      .mockResolvedValueOnce({ staged: [{ path: 'a.php', status: 'M' }], unstaged: [{ path: 'a.php', status: 'M' }], untracked: [] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    stageHunk.mockResolvedValue({ ok: true });
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');

    await fireEvent.click(screen.getByRole('button', { name: 'Stage this change' }));
    expect(stageHunk).toHaveBeenCalledWith('acme.test', '/srv/acme', 'a.php', expect.objectContaining({ modified: 'b' }), false);
    await waitFor(() => expect(screen.getByText(/Staged · 1/)).toBeInTheDocument());
    expect(loadGitVersions).toHaveBeenLastCalledWith('acme.test', '/srv/acme', 'unstaged', 'a.php');
  });

  it('unstages a block from the staged side, and offers none for a new file', async () => {
    loadGitChanges.mockResolvedValue({ staged: [{ path: 'a.php', status: 'M' }], unstaged: [], untracked: [{ path: 'n.md', status: '?' }] });
    loadGitVersions.mockResolvedValue(text('a', 'b'));
    stageHunk.mockResolvedValue({ ok: true });
    render(GitDiffModal, { props });
    await screen.findByTestId('diff');
    await fireEvent.click(screen.getByRole('button', { name: 'Unstage this change' }));
    expect(stageHunk).toHaveBeenCalledWith('acme.test', '/srv/acme', 'a.php', expect.any(Object), true);
    await waitFor(() => expect(loadGitVersions).toHaveBeenCalledTimes(2));

    await fireEvent.click(screen.getByText('n.md'));
    await waitFor(() => expect(loadGitVersions).toHaveBeenLastCalledWith('acme.test', '/srv/acme', 'untracked', 'n.md'));
    expect(screen.queryByRole('button', { name: /this change/ })).toBeNull();
  });
});
