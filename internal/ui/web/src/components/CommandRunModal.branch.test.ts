import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi, afterEach } from 'vitest';

// Spy on executeCommand while keeping the rest of the store real, so clicking
// Run Anyway exercises the real component wiring end to end. vi.hoisted lets the
// hoisted vi.mock factory reference the spy without a temporal-dead-zone error.
const { executeCommand } = vi.hoisted(() => ({ executeCommand: vi.fn() }));
vi.mock('$stores/commands', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/commands')>();
  return { ...actual, executeCommand };
});

import { currentRun, closeRun } from '$stores/commands';
import Harness from './CommandRunModal.test.svelte';

afterEach(() => {
  closeRun();
  executeCommand.mockClear();
});

describe('CommandRunModal Run Anyway', () => {
  // Regression for the wrong-database bug: clicking Run Anyway on a worktree
  // must forward that worktree's branch, not an empty branch that resolves to
  // the parent checkout.
  it('forwards the worktree branch to executeCommand', async () => {
    render(Harness);
    currentRun.set({
      kind: 'confirm',
      domain: 'acme.test',
      cmd: { name: 'migrate:fresh', label: 'Fresh migrate', command: 'php artisan migrate:fresh --force', confirm: true },
      branch: 'feat-x'
    });
    await new Promise((r) => setTimeout(r, 0));
    screen.getByText(/Run anyway/i).click();
    expect(executeCommand).toHaveBeenCalledWith(
      'acme.test',
      expect.objectContaining({ name: 'migrate:fresh' }),
      'feat-x',
      true
    );
  });

  it('passes an empty branch for a parent-site run', async () => {
    render(Harness);
    currentRun.set({
      kind: 'confirm',
      domain: 'acme.test',
      cmd: { name: 'x', label: 'X', command: 'true', confirm: true },
      branch: ''
    });
    await new Promise((r) => setTimeout(r, 0));
    screen.getByText(/Run anyway/i).click();
    expect(executeCommand).toHaveBeenCalledWith('acme.test', expect.anything(), '', true);
  });
});
