import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import NodePage from './NodePage.svelte';
import { status } from '$stores/status';

vi.mock('$stores/status', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadStatus: vi.fn() };
});
vi.mock('$stores/nodeVersions', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadNodeVersions: vi.fn(), setNodeManager: vi.fn() };
});

function setStatus(patch: Record<string, unknown>) {
  status.update((s) => ({ ...s, ...patch }));
}

describe('NodePage manager switcher', () => {
  beforeEach(() => {
    setStatus({ node_managed_by_lerd: true, using_system_bun: false, node_manager: 'fnm', nvm_available: false });
  });

  it('hides the fnm/nvm switcher when nvm is not installed', () => {
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
