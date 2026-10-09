import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';

const { pullPlan, streamPull, closeModal, restoreSnapshot } = vi.hoisted(() => ({
  pullPlan: vi.fn(),
  streamPull: vi.fn(),
  closeModal: vi.fn(),
  restoreSnapshot: vi.fn()
}));
vi.mock('$stores/worktree', () => ({ pullPlan, streamPull }));
vi.mock('$stores/modals', () => ({ closeModal }));
vi.mock('$stores/databases', () => ({ restoreSnapshot }));

import PullModal from './PullModal.svelte';

const plan = {
  ahead: 0,
  behind: 3,
  files: 7,
  composer: { label: 'composer install', needed: true, changed: 'composer.lock' },
  js: { label: 'npm ci', needed: false },
  migrate: { label: 'php artisan migrate --force', needed: true },
  migrations_added: 1,
  migrations_missing: 0,
  conflicts: [],
  db: { service: 'mysql', database: 'acme' }
};

const props = { domain: 'acme.test', branch: 'feature', branchLabel: 'feature' };

function finishWith(done: object) {
  streamPull.mockImplementation(async (_d: string, _b: string, _s: object, on: (e: object) => void) => {
    on({ line: 'Fast-forward' });
    on({ done: true, ...done });
  });
}

describe('PullModal', () => {
  beforeEach(() => {
    for (const f of [pullPlan, streamPull, closeModal, restoreSnapshot]) f.mockReset();
    pullPlan.mockResolvedValue(plan);
  });

  // The same picture as a branch switch: what comes in, and what it calls for.
  it('shows what the pull brings in and ticks what it calls for', async () => {
    render(PullModal, { props });
    expect(await screen.findByRole('switch', { name: 'composer install' })).toBeChecked();
    expect(screen.getByRole('switch', { name: 'npm ci' })).not.toBeChecked();
    expect(screen.getByRole('switch', { name: 'php artisan migrate --force' })).toBeChecked();
    expect(screen.getByRole('switch', { name: 'Snapshot the database first' })).toBeChecked();
    expect(screen.getByText('composer.lock changed')).toBeInTheDocument();
    expect(screen.getByText('Behind')).toBeInTheDocument();
    expect(pullPlan).toHaveBeenCalledWith('acme.test', 'feature');
  });

  it('pulls with only what stays ticked, then refreshes and closes', async () => {
    finishWith({ ok: true });
    const onDone = vi.fn();
    render(PullModal, { props: { ...props, onDone } });
    await fireEvent.click(await screen.findByRole('switch', { name: 'composer install' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Pull' }));

    expect(streamPull).toHaveBeenCalledWith('acme.test', 'feature', { composer: false, js: false, migrate: true, snapshot: true }, expect.any(Function));
    await waitFor(() => expect(closeModal).toHaveBeenCalled());
    expect(onDone).toHaveBeenCalled();
  });

  it('holds the pull back when it would overwrite uncommitted work', async () => {
    pullPlan.mockResolvedValue({ ...plan, conflicts: ['composer.lock'] });
    render(PullModal, { props });
    expect(await screen.findByText(/would be overwritten by the pull/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pull' })).toBeDisabled();
  });

  it('says so when there is nothing to pull', async () => {
    pullPlan.mockResolvedValue({ ...plan, behind: 0 });
    render(PullModal, { props });
    expect(await screen.findByText('Up to date')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pull' })).toBeDisabled();
  });

  it('shows why the plan failed', async () => {
    pullPlan.mockResolvedValue({ error: 'this branch has no upstream to pull from' });
    render(PullModal, { props });
    expect(await screen.findByText('this branch has no upstream to pull from')).toBeInTheDocument();
  });

  it('puts the database back when a step after the snapshot failed', async () => {
    finishWith({ ok: false, error: 'migrate failed', snapshot: 'before-switch-feature' });
    restoreSnapshot.mockResolvedValue({ ok: true });
    render(PullModal, { props });
    await screen.findByRole('switch', { name: 'composer install' });
    await fireEvent.click(screen.getByRole('button', { name: 'Pull' }));

    expect(await screen.findByText('migrate failed')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Restore snapshot' }));
    expect(restoreSnapshot).toHaveBeenCalledWith('mysql', 'acme', 'before-switch-feature');
    expect(await screen.findByText('Database restored from before-switch-feature.')).toBeInTheDocument();
  });
});
