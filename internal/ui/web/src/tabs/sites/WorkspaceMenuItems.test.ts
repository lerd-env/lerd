import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import WorkspaceMenuItems from './WorkspaceMenuItems.svelte';
import { status } from '$stores/status';
import type { Site } from '$stores/sites';

const assignSiteWorkspace = vi.hoisted(() => vi.fn());
vi.mock('$stores/workspaces', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/workspaces')>();
  return { ...actual, assignSiteWorkspace };
});

function site(over: Partial<Site> = {}): Site {
  return { domain: 'app.test', name: 'app', ...over } as Site;
}

function setWorkspaces(names: string[]) {
  status.update((s) => ({ ...s, workspaces: names }));
}

describe('WorkspaceMenuItems', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    assignSiteWorkspace.mockResolvedValue({ ok: true });
    setWorkspaces(['Client Work', 'Side Projects']);
  });

  it('lists the workspaces, None, and a way to make a new one', async () => {
    const { getByText } = render(WorkspaceMenuItems, { props: { site: site(), onDone: vi.fn() } });
    expect(getByText('Client Work')).toBeTruthy();
    expect(getByText('Side Projects')).toBeTruthy();
    expect(getByText('None')).toBeTruthy();
    expect(getByText('New workspace…')).toBeTruthy();
  });


  it('marks the current workspace as checked', async () => {
    const { getByRole } = render(WorkspaceMenuItems, { props: { site: site({ workspace: 'Client Work' }), onDone: vi.fn() } });
    expect(getByRole('menuitemradio', { name: 'Client Work' }).getAttribute('aria-checked')).toBe('true');
    expect(getByRole('menuitemradio', { name: 'None' }).getAttribute('aria-checked')).toBe('false');
  });

  it('assigns the site to the picked workspace', async () => {
    const { getByText } = render(WorkspaceMenuItems, { props: { site: site(), onDone: vi.fn() } });
    await fireEvent.click(getByText('Side Projects'));
    expect(assignSiteWorkspace).toHaveBeenCalledWith(['app'], 'Side Projects', false);
  });

  it('ungroups the site with None', async () => {
    const { getByText } = render(WorkspaceMenuItems, { props: { site: site({ workspace: 'Client Work' }), onDone: vi.fn() } });
    await fireEvent.click(getByText('None'));
    expect(assignSiteWorkspace).toHaveBeenCalledWith(['app'], '', false);
  });

  // Creating and assigning is one round trip, so the server never leaves an
  // empty workspace behind if the assign half were to fail.
  it('creates and assigns a new workspace in one call', async () => {
    const { getByText, getByPlaceholderText } = render(WorkspaceMenuItems, { props: { site: site(), onDone: vi.fn() } });
    await fireEvent.click(getByText('New workspace…'));
    await fireEvent.input(getByPlaceholderText('Workspace name'), { target: { value: '  Fresh  ' } });
    await fireEvent.click(getByText('Add'));
    expect(assignSiteWorkspace).toHaveBeenCalledWith(['app'], 'Fresh', true);
  });

  it('ignores a blank new-workspace name', async () => {
    const { getByText, getByPlaceholderText } = render(WorkspaceMenuItems, { props: { site: site(), onDone: vi.fn() } });
    await fireEvent.click(getByText('New workspace…'));
    await fireEvent.input(getByPlaceholderText('Workspace name'), { target: { value: '   ' } });
    await fireEvent.click(getByText('Add'));
    expect(assignSiteWorkspace).not.toHaveBeenCalled();
  });

});
