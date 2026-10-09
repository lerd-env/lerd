import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';

const { gitRemote, openErrorModal } = vi.hoisted(() => ({ gitRemote: vi.fn(), openErrorModal: vi.fn() }));
vi.mock('$stores/sites', () => ({ gitRemote }));
vi.mock('$stores/modals', () => ({ openErrorModal }));

import GitSync from './GitSync.svelte';

const synced = { staged: 0, modified: 0, untracked: 0, conflicted: 0, ahead: 0, behind: 0, upstream: true };

describe('GitSync', () => {
  beforeEach(() => {
    gitRemote.mockReset();
    openErrorModal.mockReset();
  });

  it('shows how far behind or ahead the checkout is', () => {
    const { unmount } = render(GitSync, { props: { domain: 'acme.test', branch: '', branchLabel: 'main', status: { ...synced, behind: 2 } } });
    expect(screen.getByRole('button', { name: 'Pull from upstream' }).textContent).toContain('2');
    unmount();
    render(GitSync, { props: { domain: 'acme.test', branch: '', branchLabel: 'main', status: { ...synced, ahead: 1 } } });
    expect(screen.getByRole('button', { name: 'Push to upstream' }).textContent).toContain('1');
  });

  // Push with nothing to send would only ever say "Everything up-to-date".
  it('disables push when nothing is ahead', () => {
    render(GitSync, { props: { domain: 'acme.test', branch: '', branchLabel: 'main', status: synced } });
    expect(screen.getByRole('button', { name: 'Push to upstream' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Pull from upstream' })).toBeEnabled();
  });

  it('runs the op on the shown checkout and reports git’s summary', async () => {
    gitRemote.mockResolvedValue({ ok: true, message: 'Already up to date.' });
    const onDone = vi.fn();
    render(GitSync, { props: { domain: 'acme.test', branch: 'feature', branchLabel: 'feature', status: synced, onDone } });

    await fireEvent.click(screen.getByRole('button', { name: 'Pull from upstream' }));
    expect(gitRemote).not.toHaveBeenCalled();
    await fireEvent.click(await screen.findByRole('button', { name: 'Pull' }));

    expect(gitRemote).toHaveBeenCalledWith('acme.test', 'pull', 'feature');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Already up to date.' })).toBeInTheDocument());
    expect(onDone).toHaveBeenCalled();
  });

  // A quiet fetch prints nothing, which still has to read as done.
  it('says up to date when git printed nothing', async () => {
    gitRemote.mockResolvedValue({ ok: true, message: '' });
    render(GitSync, { props: { domain: 'acme.test', branch: '', branchLabel: 'main', status: synced } });

    await fireEvent.click(screen.getByRole('button', { name: 'Fetch from upstream' }));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Up to date' })).toBeInTheDocument());
  });

  it('does nothing when the confirmation is cancelled', async () => {
    render(GitSync, { props: { domain: 'acme.test', branch: '', branchLabel: 'main', status: synced } });
    await fireEvent.click(screen.getByRole('button', { name: 'Pull from upstream' }));
    await fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }));
    expect(gitRemote).not.toHaveBeenCalled();
  });

  // Ahead and behind at once: git refuses both a fast-forward and a push, so
  // the buttons say why instead of failing after a confirmation.
  it('holds pull and push back on a diverged branch, saying why', async () => {
    render(GitSync, { props: { domain: 'acme.test', branch: '', branchLabel: 'main', status: { ...synced, ahead: 2, behind: 3 } } });
    const why = 'Diverged: 2 local and 3 remote commits. Merge or rebase it in your editor or terminal.';
    const [pull, push] = screen.getAllByRole('button', { name: why });
    expect(pull).toHaveAttribute('aria-disabled', 'true');
    await fireEvent.click(pull);
    await fireEvent.click(push);
    expect(screen.queryByText('Pull main?')).toBeNull();
    expect(gitRemote).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Fetch from upstream' })).toBeEnabled();
  });

  // With no upstream only publishing makes sense; it goes to origin and starts tracking.
  it('publishes a branch with no upstream after confirming', async () => {
    gitRemote.mockResolvedValue({ ok: true, message: "branch 'spike' set up to track 'origin/spike'." });
    const status = { ...synced, upstream: false, publishable: true };
    render(GitSync, { props: { domain: 'acme.test', branch: '', branchLabel: 'spike', status } });

    expect(screen.queryByRole('button', { name: 'Pull from upstream' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Fetch from upstream' })).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Publish to origin' }));
    expect(screen.getByText('Publish spike?')).toBeInTheDocument();
    await fireEvent.click(await screen.findByRole('button', { name: 'Publish' }));

    expect(gitRemote).toHaveBeenCalledWith('acme.test', 'push', '');
  });

  it('shows git’s refusal in the error modal', async () => {
    gitRemote.mockResolvedValue({ ok: false, error: 'rejected: fetch first' });
    render(GitSync, { props: { domain: 'acme.test', branch: '', branchLabel: 'main', status: { ...synced, ahead: 1 } } });

    await fireEvent.click(screen.getByRole('button', { name: 'Push to upstream' }));
    expect(await screen.findByText('Push main?')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Push' }));

    await waitFor(() => expect(openErrorModal).toHaveBeenCalledWith('rejected: fetch first', 'Push failed'));
  });
});
