import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { status } from '$stores/status';
import WorkspaceHideToggle from './WorkspaceHideToggle.svelte';

const setWorkspacePrivate = vi.fn();
vi.mock('$stores/workspaces', () => ({
  setWorkspacePrivate: (n: string, p: boolean) => setWorkspacePrivate(n, p)
}));

function withPrivate(names: string[]) {
  status.set({ streaming_enabled: true, private_workspaces: names } as never);
}

describe('WorkspaceHideToggle', () => {
  beforeEach(() => {
    setWorkspacePrivate.mockReset();
    withPrivate([]);
  });

  it('offers to hide a visible workspace, only on hover', async () => {
    setWorkspacePrivate.mockResolvedValue({ ok: true });
    const { getByRole } = render(WorkspaceHideToggle, { props: { workspace: 'Clients' } });
    const btn = getByRole('button', { name: 'Hide while streaming' });
    expect(btn.classList).toContain('invisible');
    await fireEvent.click(btn);
    expect(setWorkspacePrivate).toHaveBeenCalledWith('Clients', true);
  });

  it('stays visible on a hidden workspace and offers to show it', async () => {
    withPrivate(['Clients']);
    setWorkspacePrivate.mockResolvedValue({ ok: true });
    const { getByRole } = render(WorkspaceHideToggle, { props: { workspace: 'Clients' } });
    const btn = getByRole('button', { name: 'Show while streaming' });
    expect(btn.classList).not.toContain('invisible');
    await fireEvent.click(btn);
    expect(setWorkspacePrivate).toHaveBeenCalledWith('Clients', false);
  });

  it('flips as soon as it is clicked, before the server answers', async () => {
    setWorkspacePrivate.mockReturnValue(new Promise(() => {}));
    const { getByRole } = render(WorkspaceHideToggle, { props: { workspace: 'Clients' } });
    await fireEvent.click(getByRole('button', { name: 'Hide while streaming' }));
    expect(getByRole('button', { name: 'Show while streaming' })).toBeTruthy();
  });

  it('flips back when the server refuses', async () => {
    setWorkspacePrivate.mockResolvedValue({ ok: false, error: 'nope' });
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const { getByRole } = render(WorkspaceHideToggle, { props: { workspace: 'Clients' } });
    await fireEvent.click(getByRole('button', { name: 'Hide while streaming' }));
    await waitFor(() => expect(getByRole('button', { name: 'Hide while streaming' })).toBeTruthy());
  });

  it('stays out of the header while streaming mode is disabled', () => {
    status.set({ streaming_enabled: false, private_workspaces: ['Clients'] } as never);
    const { queryByRole } = render(WorkspaceHideToggle, { props: { workspace: 'Clients' } });
    expect(queryByRole('button')).toBeNull();
  });
});
