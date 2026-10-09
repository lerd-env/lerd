import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { Site } from '$stores/sites';

const { worktreeOptions, branchPlan, streamBranchSwitch, closeModal, restoreSnapshot } = vi.hoisted(() => ({
  restoreSnapshot: vi.fn(),
  worktreeOptions: vi.fn(),
  branchPlan: vi.fn(),
  streamBranchSwitch: vi.fn(),
  closeModal: vi.fn()
}));
vi.mock('$stores/worktree', () => ({ worktreeOptions, branchPlan, streamBranchSwitch }));
vi.mock('$stores/modals', () => ({ closeModal }));
vi.mock('$stores/databases', () => ({ restoreSnapshot }));
vi.mock('$stores/sites', () => ({ loadSites: vi.fn() }));

import SwitchBranchModal from './SwitchBranchModal.svelte';

const site = { domain: 'acme.test', branch: 'main' } as Site;

const plan = {
  ahead: 0,
  behind: 3,
  files: 12,
  composer: { label: 'composer install', needed: true, changed: 'composer.lock' },
  js: { label: 'npm ci', needed: false },
  migrate: { label: 'php artisan migrate --force', needed: true },
  migrations_added: 2,
  migrations_missing: 0
};

function finishWith(done: object) {
  streamBranchSwitch.mockImplementation(async (_d: string, _b: string, _s: object, on: (e: object) => void) => {
    on({ line: "Switched to branch 'dev'" });
    on({ done: true, ...done });
  });
}

describe('SwitchBranchModal', () => {
  beforeEach(() => {
    for (const f of [worktreeOptions, branchPlan, streamBranchSwitch, closeModal, restoreSnapshot]) f.mockReset();
    const daysAgo = (d: number) => Math.floor(Date.now() / 1000) - d * 86_400;
    worktreeOptions.mockResolvedValue({
      local_branches: ['dev'],
      remote_branches: ['origin/release'],
      branch_dates: { dev: daysAgo(2), 'origin/release': daysAgo(9) }
    });
    branchPlan.mockResolvedValue(plan);
  });

  // What the diff says is due starts ticked; the rest is the user's choice.
  it('ticks the steps the diff calls for', async () => {
    render(SwitchBranchModal, { props: { site } });
    const composer = await screen.findByRole('switch', { name: 'composer install' });
    expect(composer).toBeChecked();
    expect(screen.getByRole('switch', { name: 'npm ci' })).not.toBeChecked();
    expect(screen.getByRole('switch', { name: 'php artisan migrate --force' })).toBeChecked();
    expect(screen.getByText('composer.lock changed')).toBeInTheDocument();
    expect(branchPlan).toHaveBeenCalledWith('acme.test', 'dev');
  });

  it('runs only what stays ticked', async () => {
    finishWith({ ok: true });
    render(SwitchBranchModal, { props: { site } });
    await fireEvent.click(await screen.findByRole('switch', { name: 'php artisan migrate --force' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Switch' }));

    expect(streamBranchSwitch).toHaveBeenCalledWith(
      'acme.test',
      'dev',
      { composer: true, js: false, migrate: false, snapshot: false, restore: '', create: false, base: '' },
      expect.any(Function)
    );
    await waitFor(() => expect(closeModal).toHaveBeenCalled());
  });

  it('shows a loader while the branches are compared', async () => {
    branchPlan.mockReturnValue(new Promise(() => {}));
    render(SwitchBranchModal, { props: { site } });
    expect(await screen.findByRole('status')).toHaveTextContent('Comparing branches…');
    expect(screen.getByRole('button', { name: 'Switch' })).toBeDisabled();
  });

  it('warns when the database is ahead of the branch', async () => {
    branchPlan.mockResolvedValue({ ...plan, migrations_missing: 3 });
    render(SwitchBranchModal, { props: { site } });
    expect(await screen.findByText(/already ran \(3\)/)).toBeInTheDocument();
  });

  it('shows when each branch last moved', async () => {
    render(SwitchBranchModal, { props: { site } });
    await screen.findByRole('switch', { name: 'composer install' });
    await fireEvent.click(document.querySelector<HTMLElement>('[aria-haspopup="listbox"]')!);
    expect(await screen.findByText('2d ago')).toBeInTheDocument();
    expect(screen.getByText('remote · 9d ago')).toBeInTheDocument();
  });

  // git would refuse anyway; saying which files, before trying, is the point.
  it('names the files a switch would overwrite and holds the switch back', async () => {
    branchPlan.mockResolvedValue({ ...plan, conflicts: ['composer.lock', 'app/Models/User.php'] });
    render(SwitchBranchModal, { props: { site } });
    expect(await screen.findByRole('alert')).toHaveTextContent('app/Models/User.php');
    expect(screen.getByRole('button', { name: 'Switch' })).toBeDisabled();
  });

  describe('with a database that takes snapshots', () => {
    const db = { service: 'mysql', database: 'acme', restore: { name: 'before-switch-20261001', created: new Date(Date.now() - 2 * 86_400_000).toISOString() } };

    // The schema moves on this switch, so a copy is taken by default; going
    // back to the target branch's own copy is a choice, never a default.
    it('offers a snapshot first and the target branch’s copy, in that order', async () => {
      branchPlan.mockResolvedValue({ ...plan, db });
      render(SwitchBranchModal, { props: { site } });
      const snap = await screen.findByRole('switch', { name: 'Snapshot the database first' });
      expect(snap).toBeChecked();
      expect(screen.getByText("acme is kept as main's copy")).toBeInTheDocument();
      const restore = screen.getByRole('switch', { name: "Restore dev's database" });
      expect(restore).not.toBeChecked();
      expect(screen.getByText('from a snapshot taken 2d ago')).toBeInTheDocument();
      const order = screen.getAllByRole('switch').map((el) => el.getAttribute('aria-label'));
      expect(order.slice(0, 3)).toEqual(['Snapshot the database first', "Restore dev's database", 'composer install']);
    });

    it('sends the chosen snapshot to restore', async () => {
      branchPlan.mockResolvedValue({ ...plan, db });
      finishWith({ ok: true });
      render(SwitchBranchModal, { props: { site } });
      await fireEvent.click(await screen.findByRole('switch', { name: "Restore dev's database" }));
      await fireEvent.click(screen.getByRole('button', { name: 'Switch' }));
      expect(streamBranchSwitch.mock.calls[0][2]).toMatchObject({ snapshot: true, restore: 'before-switch-20261001' });
    });

    it('puts the database back when a step after the snapshot failed', async () => {
      branchPlan.mockResolvedValue({ ...plan, db });
      finishWith({ ok: false, error: 'php artisan migrate --force: exit status 1', snapshot: 'before-switch-20261009' });
      restoreSnapshot.mockResolvedValue({ ok: true });
      render(SwitchBranchModal, { props: { site } });
      await screen.findByRole('switch', { name: 'Snapshot the database first' });
      await fireEvent.click(screen.getByRole('button', { name: 'Switch' }));

      await fireEvent.click(await screen.findByRole('button', { name: 'Restore snapshot' }));
      expect(restoreSnapshot).toHaveBeenCalledWith('mysql', 'acme', 'before-switch-20261009');
      expect(await screen.findByText('Database restored from before-switch-20261009.')).toBeInTheDocument();
    });
  });

  describe('new branch', () => {
    it('creates the branch from the current commit, compared with HEAD', async () => {
      finishWith({ ok: true });
      render(SwitchBranchModal, { props: { site } });
      await screen.findByRole('switch', { name: 'composer install' });
      await fireEvent.click(screen.getByRole('tab', { name: 'New branch' }));
      await fireEvent.input(screen.getByRole('textbox', { name: 'New branch' }), { target: { value: ' feature/payments ' } });
      await waitFor(() => expect(branchPlan).toHaveBeenLastCalledWith('acme.test', 'HEAD'));
      await fireEvent.click(await screen.findByRole('button', { name: 'Create and switch' }));

      expect(streamBranchSwitch.mock.calls[0][1]).toBe('feature/payments');
      expect(streamBranchSwitch.mock.calls[0][2]).toMatchObject({ create: true, base: '', restore: '' });
    });

    it('lists only what is due when branching off the current commit', async () => {
      branchPlan.mockImplementation(async (_d: string, ref: string) =>
        ref === 'HEAD'
          ? { ...plan, behind: 0, files: 0, composer: { label: 'composer install', needed: false }, js: { label: 'npm ci', needed: true, missing: true }, migrate: { label: 'php artisan migrate --force', needed: false }, migrations_added: 0, db: { service: 'mysql', database: 'acme' } }
          : plan
      );
      render(SwitchBranchModal, { props: { site } });
      await screen.findByRole('switch', { name: 'composer install' });
      await fireEvent.click(screen.getByRole('tab', { name: 'New branch' }));
      await waitFor(() => expect(screen.queryByRole('switch', { name: 'composer install' })).toBeNull());
      expect(screen.getAllByRole('switch').map((el) => el.getAttribute('aria-label'))).toEqual(['npm ci']);
      expect(screen.queryByText('Files changed')).toBeNull();
    });

    // A branch open in a worktree can't be switched to here, but it is a fine base.
    it('offers every branch as a base, including ones a worktree has open', async () => {
      worktreeOptions.mockResolvedValue({
        local_branches: ['dev'],
        remote_branches: [],
        branch_dates: {},
        base_local_branches: ['dev', 'main', 'in-worktree'],
        base_remote_branches: ['origin/main']
      });
      render(SwitchBranchModal, { props: { site } });
      await screen.findByRole('switch', { name: 'composer install' });
      await fireEvent.click(screen.getByRole('tab', { name: 'New branch' }));
      const pickers = document.querySelectorAll<HTMLElement>('[aria-haspopup="listbox"]');
      await fireEvent.click(pickers[pickers.length - 1]);
      const shown = Array.from(document.querySelectorAll('[role="option"]')).map((o) => o.querySelector('span.block')?.textContent?.trim());
      expect(shown).toEqual(['main (current)', 'dev', 'in-worktree', 'origin/main']);
    });

    it('refuses a name that is already a branch', async () => {
      render(SwitchBranchModal, { props: { site } });
      await screen.findByRole('switch', { name: 'composer install' });
      await fireEvent.click(screen.getByRole('tab', { name: 'New branch' }));
      await fireEvent.input(screen.getByRole('textbox', { name: 'New branch' }), { target: { value: 'dev' } });
      expect(await screen.findByText('A branch named dev already exists.')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Create and switch' })).toBeDisabled();
    });

    // A fresh repo with only main has nothing to switch to, but can branch.
    it('is offered even when there is no other branch', async () => {
      worktreeOptions.mockResolvedValue({ local_branches: [], remote_branches: [], branch_dates: {} });
      render(SwitchBranchModal, { props: { site } });
      expect(await screen.findByText('No other branch to switch to.')).toBeInTheDocument();
      await fireEvent.click(screen.getByRole('tab', { name: 'New branch' }));
      expect(screen.getByRole('textbox', { name: 'New branch' })).toBeInTheDocument();
    });
  });

  // Its error and snapshot restore only live in the dialog, so it stays put.
  it('ignores close requests while the switch runs', async () => {
    streamBranchSwitch.mockReturnValue(new Promise(() => {}));
    render(SwitchBranchModal, { props: { site } });
    await screen.findByRole('switch', { name: 'composer install' });
    await fireEvent.click(screen.getByRole('button', { name: 'Switch' }));
    await waitFor(() => expect(streamBranchSwitch).toHaveBeenCalled());

    await fireEvent.keyDown(document, { key: 'Escape' });
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(closeModal).not.toHaveBeenCalled();
  });

  it('shows git’s refusal', async () => {
    finishWith({ ok: false, error: 'error: Your local changes would be overwritten' });
    render(SwitchBranchModal, { props: { site } });
    await screen.findByRole('switch', { name: 'composer install' });
    await fireEvent.click(screen.getByRole('button', { name: 'Switch' }));
    expect(await screen.findByText(/would be overwritten/)).toBeInTheDocument();
    expect(closeModal).not.toHaveBeenCalled();
  });
});
