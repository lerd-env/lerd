import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import NodePage from './NodePage.svelte';
import { status } from '$stores/status';
import { nodeVersions, nodeVersionStatuses } from '$stores/nodeVersions';
import { sites, type Site } from '$stores/sites';

vi.mock('$stores/status', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadStatus: vi.fn() };
});
vi.mock('$stores/nodeVersions', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadNodeVersions: vi.fn(), setNodeManager: vi.fn(), updateNodeVersion: vi.fn(), checkNodeUpdates: vi.fn(), setDefaultNode: vi.fn() };
});
vi.mock('$lib/notify', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, notifyLocalInfo: vi.fn() };
});

import { loadNodeVersions, updateNodeVersion, checkNodeUpdates, setDefaultNode } from '$stores/nodeVersions';
import { notifyLocalInfo } from '$lib/notify';
import { accessMode } from '$stores/accessMode';

function setStatus(patch: Record<string, unknown>) {
  status.update((s) => ({ ...s, ...patch }));
}

describe('NodePage manager switcher', () => {
  beforeEach(() => {
    setStatus({ node_managed_by_lerd: true, using_system_bun: false, node_manager: 'fnm', nvm_available: false });
  });

  // mise and fnm are both lerd's to install, so there is always a switch to
  // make even on a host that has never heard of nvm.
  it('shows the switcher without nvm', () => {
    const { getByRole } = render(NodePage);
    const group = getByRole('group', { name: 'Version manager' });
    expect(group.textContent).toContain('mise');
    expect(group.textContent).toContain('fnm');
  });

  it('hides the switcher when the host runs on system bun', () => {
    setStatus({ using_system_bun: true });
    const { queryByRole } = render(NodePage);
    expect(queryByRole('group', { name: 'Version manager' })).toBeNull();
  });

  it('shows the switcher when nvm is installed', () => {
    setStatus({ nvm_available: true });
    const { getByRole } = render(NodePage);
    const group = getByRole('group', { name: 'Version manager' });
    expect(group.textContent).toContain('fnm');
    expect(group.textContent).toContain('nvm');
  });

  it('keeps the switcher when the manager is nvm even if nvm went missing', () => {
    setStatus({ node_manager: 'nvm', nvm_available: false });
    const { getByRole } = render(NodePage);
    expect(getByRole('group', { name: 'Version manager' })).toBeTruthy();
  });
});

// Versions installed from the CLI never reach an open dashboard otherwise: the
// list is fetched once when the app starts.
describe('NodePage refresh', () => {
  it('refetches the versions when the page opens', () => {
    vi.mocked(loadNodeVersions).mockClear();
    render(NodePage);
    expect(loadNodeVersions).toHaveBeenCalled();
  });
});

describe('NodePage sites', () => {
  beforeEach(() => {
    setStatus({ node_managed_by_lerd: true, using_system_bun: false, node_default: '22' });
    nodeVersions.set(['22', '20']);
    sites.set([
      { domain: 'one.test', node_version: '20' } as Site,
      { domain: 'two.test', node_version: '20' } as Site,
      { domain: 'three.test', node_version: '22' } as Site
    ]);
  });

  // A version used by a dozen sites used to paste a dozen chips under its card
  // and push the rest of the page down; the count opens the list on demand.
  it('collapses the sites of a version into a dropdown', () => {
    const { container, queryByText } = render(NodePage);
    expect(queryByText('one.test')).toBeNull();
    const triggers = container.querySelectorAll('button[aria-expanded]');
    expect(Array.from(triggers).some((t) => t.textContent?.includes('2'))).toBe(true);
  });

  it('lists that version\'s sites once the dropdown is opened', async () => {
    const { container, findByText } = render(NodePage);
    const trigger = Array.from(container.querySelectorAll('button[aria-expanded]')).find((t) =>
      t.textContent?.includes('2')
    ) as HTMLButtonElement;
    trigger.click();
    expect(await findByText('one.test')).toBeTruthy();
    expect(await findByText('two.test')).toBeTruthy();
  });

  // Nothing on it: no button at all rather than a dropdown onto an empty list.
  it('shows no sites button for a version nothing uses', () => {
    sites.set([]);
    const { container } = render(NodePage);
    expect(container.querySelector('button[aria-expanded]')).toBeNull();
  });
});

describe('NodePage versions', () => {
  beforeEach(() => {
    setStatus({ node_managed_by_lerd: true, using_system_bun: false, node_manager: 'mise', node_default: '' });
    nodeVersions.set(['22', '24']);
  });

  it('shows the full installed version and the one available', () => {
    nodeVersionStatuses.set([
      { major: '22', installed: '22.23.4', latest: '22.23.4', update_available: false },
      { major: '24', installed: '24.22.2', latest: '24.22.3', update_available: true }
    ]);
    const { getByText, queryByText } = render(NodePage);
    expect(getByText('Node 22')).toBeTruthy();
    expect(getByText('22.23.4')).toBeTruthy();
    expect(getByText('24.22.2')).toBeTruthy();
    expect(getByText('24.22.3 available')).toBeTruthy();
    expect(getByText('current')).toBeTruthy();
    expect(getByText('update')).toBeTruthy();
    expect(queryByText('22.23.4 available')).toBeNull();
  });

  // Without a published list there is no claim to make either way.
  it('calls nothing current while the releases are unknown', () => {
    nodeVersionStatuses.set([{ major: '24', installed: '24.22.2', update_available: false }]);
    const { queryByText } = render(NodePage);
    expect(queryByText('current')).toBeNull();
  });

  // The list of majors arrives first; a failed status call must not blank it.
  it('falls back to the major until the details are in', () => {
    nodeVersionStatuses.set([]);
    const { getByText } = render(NodePage);
    expect(getByText('Node 24')).toBeTruthy();
  });
});

describe('NodePage default version', () => {
  beforeEach(() => {
    vi.mocked(setDefaultNode).mockReset();
    setStatus({ node_managed_by_lerd: true, using_system_bun: false, node_manager: 'mise', node_default: '22' });
    nodeVersions.set(['22', '24']);
    nodeVersionStatuses.set([]);
  });

  it('saves the default as soon as a version is picked', async () => {
    vi.mocked(setDefaultNode).mockResolvedValue(true);
    const { getByRole } = render(NodePage);

    await fireEvent.click(getByRole('button', { name: 'Node 24' }));

    expect(setDefaultNode).toHaveBeenCalledWith('24');
  });

  it('has no save button', () => {
    const { queryByRole } = render(NodePage);
    expect(queryByRole('button', { name: 'Save' })).toBeNull();
  });

  it('does nothing when the default is picked again', async () => {
    const { getByRole } = render(NodePage);

    await fireEvent.click(getByRole('button', { name: 'Node 22' }));

    expect(setDefaultNode).not.toHaveBeenCalled();
  });

  it('goes back to the real default and says so when saving fails', async () => {
    vi.mocked(setDefaultNode).mockResolvedValue(false);
    const { getByRole, findByText } = render(NodePage);

    await fireEvent.click(getByRole('button', { name: 'Node 24' }));

    expect(await findByText('Failed')).toBeTruthy();
    expect(getByRole('button', { name: 'Node 22' }).getAttribute('aria-pressed')).toBe('true');
    expect(getByRole('button', { name: 'Node 24' }).getAttribute('aria-pressed')).toBe('false');
  });
});

describe('NodePage update check', () => {
  beforeEach(() => {
    vi.mocked(checkNodeUpdates).mockReset();
    vi.mocked(notifyLocalInfo).mockClear();
    setStatus({ node_managed_by_lerd: true, using_system_bun: false, node_manager: 'mise', node_default: '' });
    nodeVersions.set(['24']);
    nodeVersionStatuses.set([]);
  });

  const check = (container: ReturnType<typeof render>) => container.getByRole('button', { name: 'Check for updates' });

  it('says when a newer patch turned up', async () => {
    vi.mocked(checkNodeUpdates).mockResolvedValue({
      ok: true,
      versions: [{ major: '24', installed: '24.1.0', latest: '24.2.0', update_available: true }]
    });
    await fireEvent.click(check(render(NodePage)));
    await waitFor(() =>
      expect(notifyLocalInfo).toHaveBeenCalledWith('update_check', 'Node.js', 'An update is available')
    );
  });

  it('says when every version is current', async () => {
    vi.mocked(checkNodeUpdates).mockResolvedValue({ ok: true, versions: [] });
    await fireEvent.click(check(render(NodePage)));
    await waitFor(() =>
      expect(notifyLocalInfo).toHaveBeenCalledWith('update_check', 'Node.js', 'Every Node version is on its newest patch')
    );
  });

  it('does not call a failed check up to date', async () => {
    vi.mocked(checkNodeUpdates).mockResolvedValue({ ok: false });
    await fireEvent.click(check(render(NodePage)));
    await waitFor(() => expect(notifyLocalInfo).toHaveBeenCalledWith('update_check', 'Node.js', 'Check failed'));
  });

  it('has nothing to check when lerd does not manage Node', () => {
    setStatus({ node_managed_by_lerd: false });
    const { queryByRole } = render(NodePage);
    expect(queryByRole('button', { name: 'Check for updates' })).toBeNull();
  });
});

describe('NodePage updates', () => {
  const behind = { major: '24', installed: '24.1.0', latest: '24.2.0', update_available: true };
  const current = { major: '22', installed: '22.2.0', latest: '22.2.0', update_available: false };

  beforeEach(() => {
    vi.mocked(updateNodeVersion).mockReset();
    setStatus({ node_managed_by_lerd: true, using_system_bun: false, node_manager: 'mise', node_default: '' });
    accessMode.set({ localControl: true, local: true, lanExposed: false, checked: true });
    nodeVersions.set(['22', '24']);
    nodeVersionStatuses.set([current, behind]);
  });

  it('updates from the card and reports a failure there', async () => {
    vi.mocked(updateNodeVersion).mockResolvedValue({ ok: false, error: 'download failed' });
    const { getByRole, findByText } = render(NodePage);

    await fireEvent.click(getByRole('button', { name: 'Update' }));

    expect(updateNodeVersion).toHaveBeenCalledWith('24');
    expect(await findByText('download failed')).toBeTruthy();
  });

  it('refreshes the versions after an update', async () => {
    vi.mocked(updateNodeVersion).mockResolvedValue({ ok: true });
    vi.mocked(loadNodeVersions).mockClear();
    const { getByRole } = render(NodePage);
    vi.mocked(loadNodeVersions).mockClear();

    await fireEvent.click(getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(loadNodeVersions).toHaveBeenCalled());
  });

  it('offers no update for a version that is current', () => {
    nodeVersionStatuses.set([current]);
    const { queryByRole } = render(NodePage);
    expect(queryByRole('button', { name: 'Update' })).toBeNull();
    expect(queryByRole('button', { name: 'Update all' })).toBeNull();
  });

  it('opens the batch dialog from the header', async () => {
    const { getByRole, findByText } = render(NodePage);

    await fireEvent.click(getByRole('button', { name: 'Update all' }));

    expect(await findByText('24.1.0 → 24.2.0')).toBeTruthy();
  });

  it('tells a remote viewer about the update without offering to run it', () => {
    accessMode.set({ localControl: false, local: false, lanExposed: true, checked: true });
    const { getByText, queryByRole } = render(NodePage);
    expect(getByText('24.2.0 available')).toBeTruthy();
    expect(queryByRole('button', { name: 'Update' })).toBeNull();
    expect(queryByRole('button', { name: 'Update all' })).toBeNull();
  });
});
