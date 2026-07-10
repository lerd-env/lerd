import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import ConfirmWorkspaceDeleteModal from './ConfirmWorkspaceDeleteModal.svelte';
import { modal, openWorkspaceDeleteModal, closeModal } from '$stores/modals';

const deleteWorkspace = vi.hoisted(() => vi.fn());
vi.mock('$stores/workspaces', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/workspaces')>();
  return { ...actual, deleteWorkspace };
});

describe('ConfirmWorkspaceDeleteModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    deleteWorkspace.mockResolvedValue({ ok: true });
    openWorkspaceDeleteModal({ name: 'Client Work', siteCount: 2 });
  });

  it('names the workspace and says its sites survive', () => {
    const { getByText } = render(ConfirmWorkspaceDeleteModal);
    expect(getByText('Delete "Client Work"?')).toBeTruthy();
    expect(getByText(/stay linked and become ungrouped/)).toBeTruthy();
    expect(getByText('Sites affected: 2')).toBeTruthy();
  });

  // The count reads the same for one site as for many, so it needs no plural.
  it('states the count for a single site', () => {
    openWorkspaceDeleteModal({ name: 'Client Work', siteCount: 1 });
    const { getByText } = render(ConfirmWorkspaceDeleteModal);
    expect(getByText('Sites affected: 1')).toBeTruthy();
  });

  it('omits the site count for an empty workspace', () => {
    openWorkspaceDeleteModal({ name: 'Empty', siteCount: 0 });
    const { queryByText } = render(ConfirmWorkspaceDeleteModal);
    expect(queryByText(/Sites affected/)).toBeNull();
  });

  it('deletes nothing and closes when cancelled', async () => {
    const { getByText } = render(ConfirmWorkspaceDeleteModal);
    await fireEvent.click(getByText('Cancel'));
    expect(deleteWorkspace).not.toHaveBeenCalled();
    expect(get(modal).kind).toBeNull();
  });

  it('deletes the workspace and closes when confirmed', async () => {
    const { getByText } = render(ConfirmWorkspaceDeleteModal);
    await fireEvent.click(getByText('Remove'));
    expect(deleteWorkspace).toHaveBeenCalledWith('Client Work');
    await vi.waitFor(() => expect(get(modal).kind).toBeNull());
  });

  it('stays open and shows the error when the server refuses', async () => {
    deleteWorkspace.mockResolvedValue({ ok: false, error: 'workspace not found' });
    const { getByText } = render(ConfirmWorkspaceDeleteModal);
    await fireEvent.click(getByText('Remove'));
    await vi.waitFor(() => expect(getByText('workspace not found')).toBeTruthy());
    expect(get(modal).kind).toBe('workspaceDelete');
    closeModal();
  });
});
