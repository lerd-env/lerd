import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import ToolsDetail from './ToolsDetail.svelte';
import { status } from '$stores/status';
import { accessMode } from '$stores/accessMode';

const { checkToolUpdates, notifyLocalInfo } = vi.hoisted(() => ({
  checkToolUpdates: vi.fn(),
  notifyLocalInfo: vi.fn()
}));
vi.mock('$stores/status', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/status')>();
  return { ...actual, checkToolUpdates };
});
vi.mock('$lib/notify', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/notify')>();
  return { ...actual, notifyLocalInfo };
});

const tool = (name: string, updateAvailable = false) => ({
  name,
  installed: '1.0.0',
  pinned: updateAvailable ? '1.0.1' : '1.0.0',
  present: true,
  update_available: updateAvailable
});

describe('ToolsDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    accessMode.set({ localControl: true, local: true, lanExposed: false, checked: true });
    status.set({ tools: [tool('composer'), tool('fnm')] } as never);
    checkToolUpdates.mockResolvedValue({ ok: true, tools: [tool('composer'), tool('fnm')] });
  });

  // The pins live in one manifest, so one button covers every card rather than
  // each card growing its own check.
  it('checks every tool with one button', async () => {
    render(ToolsDetail);
    await fireEvent.click(screen.getByRole('button', { name: /check for updates/i }));

    expect(checkToolUpdates).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(notifyLocalInfo).toHaveBeenCalled());
    expect(notifyLocalInfo.mock.calls[0][2]).toMatch(/pinned version/i);
  });

  // The whole point of the check is escaping the 24h cache window, so a check
  // that turns something up has to say so rather than report all clear.
  it('reports a pin that has moved', async () => {
    checkToolUpdates.mockResolvedValue({ ok: true, tools: [tool('composer', true), tool('fnm')] });
    render(ToolsDetail);
    await fireEvent.click(screen.getByRole('button', { name: /check for updates/i }));

    await waitFor(() => expect(notifyLocalInfo).toHaveBeenCalled());
    expect(notifyLocalInfo.mock.calls[0][2]).toMatch(/update is available/i);
  });

  it('reports a check it could not run', async () => {
    checkToolUpdates.mockResolvedValue({ ok: false });
    render(ToolsDetail);
    await fireEvent.click(screen.getByRole('button', { name: /check for updates/i }));

    await waitFor(() => expect(notifyLocalInfo).toHaveBeenCalled());
    expect(notifyLocalInfo.mock.calls[0][2]).toMatch(/failed/i);
  });

  it('hides Update all while every tool is current', () => {
    render(ToolsDetail);
    expect(screen.queryByRole('button', { name: 'Update all' })).toBeNull();
  });

  it('opens the dialog on the tools that have an update', async () => {
    status.set({ tools: [tool('composer', true), tool('fnm')] } as never);
    render(ToolsDetail);

    await fireEvent.click(screen.getByRole('button', { name: 'Update all' }));

    expect(screen.getByText('1.0.0 → 1.0.1')).toBeInTheDocument();
  });

  it('leaves Update all to the loopback viewer', () => {
    accessMode.set({ localControl: false, local: false, lanExposed: true, checked: true });
    status.set({ tools: [tool('composer', true)] } as never);
    render(ToolsDetail);
    expect(screen.queryByRole('button', { name: 'Update all' })).toBeNull();
  });
});
