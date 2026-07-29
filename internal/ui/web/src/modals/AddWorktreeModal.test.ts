import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import AddWorktreeModal from './AddWorktreeModal.svelte';
import type { Site } from '$stores/sites';

const worktreeOptions = vi.fn();
const streamWorktreeAdd = vi.fn();
vi.mock('$stores/worktree', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/worktree')>();
  return {
    ...actual,
    worktreeOptions: (...args: unknown[]) => worktreeOptions(...args),
    streamWorktreeAdd: (...args: unknown[]) => streamWorktreeAdd(...args)
  };
});
vi.mock('$stores/sites', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/sites')>();
  return { ...actual, loadSites: vi.fn() };
});

const site = { domain: 'acme.test', path: '/home/u/acme' } as Site;

const options = {
  local_branches: ['demo/gui', 'feature/x'],
  remote_branches: [],
  default_branch_label: 'main',
  build_options: [],
  build_default: 'auto',
  db_options: [
    { value: 'share', label: "Share parent's database" },
    { value: 'empty', label: 'Isolated database, empty schema' }
  ],
  can_migrate: false
};

// Dropdown triggers are the only aria-haspopup elements in the modal, in
// document order: [0] is "Based on", [1] is the database picker.
function dropdown(i: number): HTMLElement {
  return Array.from(
    document.querySelectorAll<HTMLElement>('[aria-haspopup="listbox"]')
  )[i];
}

async function pick(trigger: HTMLElement, text: string) {
  await fireEvent.click(trigger);
  const menu = document.querySelector('[role="listbox"]');
  if (!menu) throw new Error('menu did not open');
  const option = Array.from(menu.querySelectorAll<HTMLElement>('[role="option"]')).find((o) =>
    o.textContent?.includes(text)
  );
  if (!option) throw new Error(`option ${text} not offered`);
  await fireEvent.click(option);
}

describe('AddWorktreeModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    worktreeOptions.mockResolvedValue(options);
    streamWorktreeAdd.mockImplementation(
      async (_d: string, _p: unknown, onEvent: (e: unknown) => void) => {
        onEvent({ done: true, ok: true, branch: 'wip', warnings: [] });
      }
    );
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('names the current branch as the default base instead of showing a blank picker', async () => {
    render(AddWorktreeModal, { props: { site } });
    await waitFor(() => expect(screen.getByText('Based on')).toBeInTheDocument());
    expect(dropdown(0).textContent).toContain('main (current)');
  });

  it('keeps the chosen base branch while the branch name is edited', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    render(AddWorktreeModal, { props: { site } });
    await waitFor(() => expect(screen.getByText('Based on')).toBeInTheDocument());

    await pick(dropdown(0), 'demo/gui');
    const input = screen.getByPlaceholderText(/branch/i);
    await fireEvent.input(input, { target: { value: 'wip' } });
    await vi.advanceTimersByTimeAsync(500);
    await fireEvent.input(input, { target: { value: '' } });
    await vi.advanceTimersByTimeAsync(500);
    await fireEvent.input(input, { target: { value: 'wip-2' } });
    await vi.advanceTimersByTimeAsync(500);

    expect(dropdown(0).textContent).toContain('demo/gui');
  });

  it('creates the worktree from the branch the user picked', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    render(AddWorktreeModal, { props: { site } });
    await waitFor(() => expect(screen.getByText('Based on')).toBeInTheDocument());

    await fireEvent.input(screen.getByPlaceholderText(/branch/i), { target: { value: 'wip' } });
    await pick(dropdown(0), 'demo/gui');
    await pick(dropdown(1), 'empty');
    await vi.advanceTimersByTimeAsync(500);
    await fireEvent.click(screen.getByText('Create worktree'));

    expect(streamWorktreeAdd).toHaveBeenCalledWith(
      'acme.test',
      expect.objectContaining({ newBranch: 'wip', baseRef: 'demo/gui', db: 'empty' }),
      expect.any(Function)
    );
  });

  it('ignores a stale options response that lands after a newer one', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    let releaseStale: (() => void) | undefined;
    worktreeOptions
      .mockResolvedValueOnce(options)
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            releaseStale = () => resolve({ ...options, db_options: [{ value: 'share', label: 'x' }] });
          })
      )
      .mockResolvedValue({ ...options, local_branches: ['demo/gui'] });

    render(AddWorktreeModal, { props: { site } });
    await waitFor(() => expect(screen.getByText('Based on')).toBeInTheDocument());

    const input = screen.getByPlaceholderText(/branch/i);
    await fireEvent.input(input, { target: { value: 'wi' } });
    await vi.advanceTimersByTimeAsync(500);
    await fireEvent.input(input, { target: { value: 'wip' } });
    await vi.advanceTimersByTimeAsync(500);
    await pick(dropdown(1), 'empty');
    releaseStale?.();
    await vi.advanceTimersByTimeAsync(50);

    expect(dropdown(1).textContent).toContain('Isolated database, empty schema');
  });
});
