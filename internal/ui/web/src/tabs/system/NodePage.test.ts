import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import NodePage from './NodePage.svelte';
import { status } from '$stores/status';
import { nodeVersions } from '$stores/nodeVersions';
import { sites, type Site } from '$stores/sites';

vi.mock('$stores/status', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadStatus: vi.fn() };
});
vi.mock('$stores/nodeVersions', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadNodeVersions: vi.fn(), setNodeManager: vi.fn() };
});

import { loadNodeVersions } from '$stores/nodeVersions';

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
