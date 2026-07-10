import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import SitesTab from './SitesTab.svelte';
import { sites, sitesLoaded, type Site } from '$stores/sites';
import { status } from '$stores/status';
import { sitesSort } from '$stores/sitesSort';
import { accessMode } from '$stores/accessMode';
import { workspaceCollapse } from '$stores/workspaces';
import { modal, closeModal } from '$stores/modals';

const saveWorkspaceLayout = vi.hoisted(() => vi.fn());
const createWorkspace = vi.hoisted(() => vi.fn());

vi.mock('$stores/workspaces', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/workspaces')>();
  return { ...actual, saveWorkspaceLayout, createWorkspace };
});

function site(over: Partial<Site> = {}): Site {
  return { domain: 'app.test', name: 'app', ...over } as Site;
}

function setWorkspaces(names: string[]) {
  status.update((s) => ({ ...s, workspaces: names }));
}

describe('SitesTab workspace sections', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    saveWorkspaceLayout.mockResolvedValue({ ok: true });
    createWorkspace.mockResolvedValue({ ok: true });
    closeModal();
    localStorage.clear();
    workspaceCollapse.set([]);
    sitesSort.set('manual');
    sitesLoaded.set(true);
    accessMode.set({ loopback: true } as never);
    setWorkspaces([]);
    sites.set([]);
  });

  it('renders no section headers when no workspace is configured', () => {
    sites.set([site({ domain: 'a.test', name: 'a' })]);
    const { getByText, queryByLabelText } = render(SitesTab);
    expect(getByText('a.test')).toBeTruthy();
    expect(queryByLabelText('Collapse section')).toBeNull();
  });

  it('groups sites under their workspace, leaving the rest unlabelled', () => {
    setWorkspaces(['Client Work']);
    sites.set([
      site({ domain: 'a.test', name: 'a', workspace: 'Client Work' }),
      site({ domain: 'b.test', name: 'b' })
    ]);
    const { getByText, queryByText } = render(SitesTab);
    expect(getByText('Client Work')).toBeTruthy();
    expect(getByText('a.test')).toBeTruthy();
    expect(getByText('b.test')).toBeTruthy();
    expect(queryByText('Ungrouped')).toBeNull();
  });

  it('lists the ungrouped sites below the workspace sections, above paused', () => {
    setWorkspaces(['Client Work']);
    sites.set([
      site({ domain: 'a.test', name: 'a', workspace: 'Client Work' }),
      site({ domain: 'b.test', name: 'b' }),
      site({ domain: 'old.test', name: 'old', paused: true })
    ]);
    const { container } = render(SitesTab);
    const text = container.textContent ?? '';
    expect(text.indexOf('Client Work')).toBeLessThan(text.indexOf('b.test'));
    expect(text.indexOf('b.test')).toBeLessThan(text.indexOf('Paused'));
  });

  it('renders an empty workspace as its own section', () => {
    setWorkspaces(['Empty']);
    sites.set([site({ domain: 'a.test', name: 'a' })]);
    const { getByText } = render(SitesTab);
    expect(getByText('Empty')).toBeTruthy();
  });

  it('a site whose workspace no longer exists falls back to the ungrouped list', () => {
    setWorkspaces(['Client Work']);
    sites.set([site({ domain: 'a.test', name: 'a', workspace: 'Deleted' })]);
    const { getByText, container } = render(SitesTab);
    expect(getByText('a.test')).toBeTruthy();
    // Client Work reports zero members; the site sits in Ungrouped.
    const counts = Array.from(container.querySelectorAll('button[aria-expanded]')).map((b) => b.textContent?.trim());
    expect(counts.some((t) => t?.startsWith('Client Work') && t?.endsWith('0'))).toBe(true);
  });

  it('collapsing a section hides its rows and persists', async () => {
    setWorkspaces(['Client Work']);
    sites.set([site({ domain: 'a.test', name: 'a', workspace: 'Client Work' })]);
    const { getByText, queryByText } = render(SitesTab);

    await fireEvent.click(getByText('Client Work'));
    expect(queryByText('a.test')).toBeNull();
    expect(get(workspaceCollapse)).toContain('Client Work');
    expect(JSON.parse(localStorage.getItem('lerd:workspaceCollapse') ?? '[]')).toContain('Client Work');
  });

  it('collapsing the paused section hides the paused sites', async () => {
    sites.set([site({ domain: 'a.test', name: 'a' }), site({ domain: 'old.test', name: 'old', paused: true })]);
    const { getByText, queryByText } = render(SitesTab);
    expect(getByText('old.test')).toBeTruthy();

    await fireEvent.click(getByText('Paused'));
    expect(queryByText('old.test')).toBeNull();
    expect(getByText('a.test')).toBeTruthy();
  });

  it('the add-workspace button creates a workspace', async () => {
    sites.set([site()]);
    const { getByLabelText, getByPlaceholderText, getByText } = render(SitesTab);

    await fireEvent.click(getByLabelText('Add workspace'));
    await fireEvent.input(getByPlaceholderText('Workspace name'), { target: { value: '  Client Work  ' } });
    await fireEvent.click(getByText('Add'));

    expect(createWorkspace).toHaveBeenCalledWith('Client Work');
  });

  it('hides the workspace controls when the dashboard is read-only', () => {
    accessMode.set({ loopback: false } as never);
    setWorkspaces(['Client Work']);
    sites.set([site()]);
    const { queryByLabelText } = render(SitesTab);
    expect(queryByLabelText('Add workspace')).toBeNull();
    expect(queryByLabelText('Workspace actions')).toBeNull();
  });

  // Deleting opens a confirmation modal; nothing is destroyed from the menu
  // itself, and the modal is told how many sites are about to be ungrouped.
  it('deleting a workspace opens the confirmation modal instead of deleting', async () => {
    setWorkspaces(['Client Work']);
    sites.set([
      site({ domain: 'a.test', name: 'a', workspace: 'Client Work' }),
      site({ domain: 'b.test', name: 'b', workspace: 'Client Work' })
    ]);
    const { getByLabelText, getByText } = render(SitesTab);

    await fireEvent.click(getByLabelText('Workspace actions'));
    await fireEvent.click(getByText('Delete workspace'));

    expect(get(modal)).toMatchObject({
      kind: 'workspaceDelete',
      workspaceDelete: { name: 'Client Work', siteCount: 2 }
    });
    expect(getByText('a.test')).toBeTruthy();
    closeModal();
  });

  // Picking up a row must never expand a section the user collapsed.
  it('leaves collapsed sections collapsed while a row is being dragged', async () => {
    setWorkspaces(['Client Work', 'Side Projects']);
    sites.set([
      site({ domain: 'a.test', name: 'a', workspace: 'Client Work' }),
      site({ domain: 'b.test', name: 'b', workspace: 'Side Projects' })
    ]);
    const { getByText, queryByText, getAllByLabelText } = render(SitesTab);

    await fireEvent.click(getByText('Side Projects'));
    expect(queryByText('b.test')).toBeNull();

    await fireEvent.mouseDown(getAllByLabelText('Drag to reorder')[0]);
    expect(queryByText('b.test')).toBeNull();
    expect(get(workspaceCollapse)).toEqual(['Side Projects']);
  });

  // A press on the grip that never moves is a click, not a drag. It must not
  // latch the drag state, which would freeze live updates.
  it('releases the drag state when a grip press never becomes a drag', async () => {
    setWorkspaces(['Client Work']);
    sites.set([site({ domain: 'a.test', name: 'a', workspace: 'Client Work' })]);
    const { getByText, getAllByLabelText, queryByText } = render(SitesTab);

    await fireEvent.mouseDown(getAllByLabelText('Drag to reorder')[0]);
    await fireEvent.mouseUp(window);

    // The live snapshot still reaches the list: a new site shows up.
    sites.set([
      site({ domain: 'a.test', name: 'a', workspace: 'Client Work' }),
      site({ domain: 'fresh.test', name: 'fresh' })
    ]);
    await vi.waitFor(() => expect(queryByText('fresh.test')).toBeTruthy());
    expect(getByText('a.test')).toBeTruthy();
  });

  // A cross-section drop finalizes the source and the target zone in the same
  // tick. Both must fold into one write, built from both zones.
  it('coalesces the two finalize events of a cross-section drop into one write', async () => {
    setWorkspaces(['Client Work']);
    sites.set([
      site({ domain: 'a.test', name: 'a', workspace: 'Client Work' }),
      site({ domain: 'b.test', name: 'b' })
    ]);
    const { container } = render(SitesTab);
    const [wsZone, ungroupedZone] = Array.from(container.querySelectorAll('section'));

    const moved = { id: 'b.test', site: { domain: 'b.test', name: 'b' } };
    const detail = (items: unknown[]) => ({ items, info: { source: 'pointer', trigger: 'droppedIntoZone' } });
    wsZone.dispatchEvent(
      new CustomEvent('finalize', { detail: detail([{ id: 'a.test', site: { domain: 'a.test', name: 'a' } }, moved]) })
    );
    ungroupedZone.dispatchEvent(new CustomEvent('finalize', { detail: detail([]) }));

    await vi.waitFor(() => expect(saveWorkspaceLayout).toHaveBeenCalledTimes(1));
    const [layout] = saveWorkspaceLayout.mock.calls[0];
    expect(layout).toEqual([{ name: 'Client Work', sites: ['a', 'b'] }]);
  });

  // A secondary displays in its main's workspace and is never a zone item, so a
  // count taken from the zone would miss it.
  it('counts the group secondaries drawn under a main in the section header', () => {
    setWorkspaces(['Client Work']);
    sites.set([
      site({ domain: 'astrolov.test', name: 'astrolov', workspace: 'Client Work', group: 'astrolov' }),
      site({
        domain: 'admin.astrolov.test',
        name: 'admin',
        workspace: 'Client Work',
        group: 'astrolov',
        group_subdomain: 'admin'
      })
    ]);
    const { getByText } = render(SitesTab);
    expect(getByText('Client Work').closest('button')?.textContent?.trim()).toMatch(/2$/);
  });

  // Deleting ungroups every member, paused ones included, so the confirmation
  // has to count them rather than the rows on screen.
  it('counts paused members when confirming a workspace delete', async () => {
    setWorkspaces(['Client Work']);
    sites.set([
      site({ domain: 'a.test', name: 'a', workspace: 'Client Work' }),
      site({ domain: 'old.test', name: 'old', workspace: 'Client Work', paused: true })
    ]);
    const { getByLabelText, getByText } = render(SitesTab);

    await fireEvent.click(getByLabelText('Workspace actions'));
    await fireEvent.click(getByText('Delete workspace'));

    expect(get(modal)).toMatchObject({ workspaceDelete: { name: 'Client Work', siteCount: 2 } });
    closeModal();
  });

  // Only a main's membership is stored. Listing a secondary too would be state
  // nothing reads, and it would strand the site in a workspace the day its
  // group is dissolved.
  it('persists only the mains, never the group secondaries', async () => {
    setWorkspaces(['Client Work']);
    sites.set([
      site({ domain: 'astrolov.test', name: 'astrolov', group: 'astrolov' }),
      site({ domain: 'admin.astrolov.test', name: 'admin', group: 'astrolov', group_subdomain: 'admin' })
    ]);
    const { container } = render(SitesTab);
    const [wsZone, ungroupedZone] = Array.from(container.querySelectorAll('section'));

    const detail = (items: unknown[]) => ({ items, info: { source: 'pointer', trigger: 'droppedIntoZone' } });
    wsZone.dispatchEvent(
      new CustomEvent('finalize', {
        detail: detail([{ id: 'astrolov.test', site: { domain: 'astrolov.test', name: 'astrolov', group: 'astrolov' } }])
      })
    );
    ungroupedZone.dispatchEvent(new CustomEvent('finalize', { detail: detail([]) }));

    await vi.waitFor(() => expect(saveWorkspaceLayout).toHaveBeenCalledTimes(1));
    const [layout] = saveWorkspaceLayout.mock.calls[0];
    expect(layout).toEqual([{ name: 'Client Work', sites: ['astrolov'] }]);
  });

  // The optimistic update has to apply the server's rule, or the secondary
  // flickers into the ungrouped list until the websocket push corrects it.
  it('moves a secondary with its main in the optimistic update', async () => {
    setWorkspaces(['Client Work']);
    sites.set([
      site({ domain: 'astrolov.test', name: 'astrolov', group: 'astrolov' }),
      site({ domain: 'admin.astrolov.test', name: 'admin', group: 'astrolov', group_subdomain: 'admin' })
    ]);
    const { container } = render(SitesTab);
    const [wsZone, ungroupedZone] = Array.from(container.querySelectorAll('section'));

    const detail = (items: unknown[]) => ({ items, info: { source: 'pointer', trigger: 'droppedIntoZone' } });
    wsZone.dispatchEvent(
      new CustomEvent('finalize', {
        detail: detail([{ id: 'astrolov.test', site: { domain: 'astrolov.test', name: 'astrolov', group: 'astrolov' } }])
      })
    );
    ungroupedZone.dispatchEvent(new CustomEvent('finalize', { detail: detail([]) }));

    await vi.waitFor(() => expect(saveWorkspaceLayout).toHaveBeenCalledTimes(1));
    const admin = get(sites).find((s) => s.name === 'admin');
    expect(admin?.workspace).toBe('Client Work');
  });

  // The header is the drag target, so it must not carry a separate handle, and
  // clicking it still has to toggle the section rather than start a drag.
  it('drags workspace headers directly, with no grip and only when several exist', async () => {
    setWorkspaces(['One']);
    sites.set([site()]);
    const single = render(SitesTab);
    const lone = single.getByText('One').closest('button')!;
    expect(lone.className).not.toContain('cursor-grab');
    single.unmount();

    setWorkspaces(['One', 'Two']);
    const many = render(SitesTab);
    const header = many.getByText('One').closest('button')!;
    expect(header.className).toContain('cursor-grab');
    expect(header.querySelector('svg')).toBeTruthy(); // the chevron, not a grip

    await fireEvent.click(header);
    expect(get(workspaceCollapse)).toContain('One');
  });
});
