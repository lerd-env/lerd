import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import RemoveWorktreeModal from './RemoveWorktreeModal.svelte';
import { sites, type Site } from '$stores/sites';

type Result = { ok: boolean; error?: string };
let answer: (r: Result) => void;
vi.mock('$stores/worktree', () => ({
  removeWorktree: () => new Promise<Result>((resolve) => (answer = resolve))
}));
vi.mock('$stores/sites', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/sites')>();
  return { ...actual, loadSites: vi.fn() };
});
const closeModal = vi.fn();
vi.mock('$stores/modals', () => ({ closeModal: () => closeModal() }));

const without = { domain: 'acme.test', path: '/srv/acme', worktrees: [] } as unknown as Site;
const site = {
  ...without,
  worktrees: [{ branch: 'wip', domain: 'wip.acme.test', path: '/srv/acme-wip' }]
} as unknown as Site;

async function startRemoval() {
  render(RemoveWorktreeModal, { props: { site, branch: 'wip' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
}

function expectRemoved() {
  expect(screen.getByRole('heading', { name: 'Worktree removed' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Cancel' })).toBeNull();
  expect(screen.queryByRole('button', { name: /^Remov/ })).toBeNull();
  for (const b of screen.getAllByRole('button', { name: 'Close' })) expect(b).toBeEnabled();
}

describe('RemoveWorktreeModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sites.set([site]);
  });

  it('says the worktree is removed when the live update lands before the answer', async () => {
    await startRemoval();
    sites.set([without]);
    await waitFor(expectRemoved);
    expect(screen.queryByText('No worktrees yet.')).toBeNull();
  });

  it('stays open on the result once the removal is answered', async () => {
    await startRemoval();
    answer({ ok: true });
    await waitFor(expectRemoved);
    expect(closeModal).not.toHaveBeenCalled();
  });

  it('keeps asking, with the reason, when git refuses', async () => {
    await startRemoval();
    answer({ ok: false, error: 'contains modified files' });
    await waitFor(() => expect(screen.getByText('contains modified files')).toBeInTheDocument());
    expect(screen.getByRole('heading', { name: 'Remove worktree wip?' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Remove' })).toBeEnabled();
  });

  it('shows a failure that arrives after the worktree is gone', async () => {
    await startRemoval();
    sites.set([without]);
    answer({ ok: false, error: 'Request failed' });
    await waitFor(() => expect(screen.getByText('Request failed')).toBeInTheDocument());
    expectRemoved();
  });
});
