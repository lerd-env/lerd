import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import ToolsDetail from './ToolsDetail.svelte';
import { status } from '$stores/status';

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
    status.set({ tools: [tool('composer'), tool('mkcert')] } as never);
    checkToolUpdates.mockResolvedValue({ ok: true, tools: [tool('composer'), tool('mkcert')] });
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
    checkToolUpdates.mockResolvedValue({ ok: true, tools: [tool('composer', true), tool('mkcert')] });
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
});
