import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import ToolsUpdateModal from './ToolsUpdateModal.svelte';
import type { ToolStatus } from '$stores/status';

const { updateTool, notifyLocalFailure } = vi.hoisted(() => ({
  updateTool: vi.fn(),
  notifyLocalFailure: vi.fn()
}));
// Stopped at the request, so the real batch runs: order, carrying on, reporting.
vi.mock('$lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api')>();
  return {
    ...actual,
    apiFetch: async (url: string) => {
      const name = decodeURIComponent(url.split('/')[3]);
      return new Response(JSON.stringify(await updateTool(name)));
    }
  };
});
vi.mock('$lib/notify', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/notify')>();
  return { ...actual, notifyLocalFailure };
});

const tool = (name: string, installed: string, pinned: string): ToolStatus => ({
  name,
  installed,
  pinned,
  present: true,
  update_available: true
});
const tools = [tool('composer', '2.10.2', '2.10.3'), tool('fnm', 'v1.4.3', 'v1.4.4')];

describe('ToolsUpdateModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    updateTool.mockResolvedValue({ ok: true });
  });

  it('lists each tool with the version it moves from and to', () => {
    render(ToolsUpdateModal, { props: { tools, onclose: vi.fn() } });
    expect(screen.getByText('composer')).toBeInTheDocument();
    expect(screen.getByText('2.10.2 → 2.10.3')).toBeInTheDocument();
    expect(screen.getByText('v1.4.3 → v1.4.4')).toBeInTheDocument();
  });

  it('updates only the tools still switched on', async () => {
    const onclose = vi.fn();
    render(ToolsUpdateModal, { props: { tools, onclose } });

    await fireEvent.click(screen.getByTitle('fnm'));
    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(onclose).toHaveBeenCalled());
    expect(updateTool.mock.calls).toEqual([['composer']]);
  });

  it('holds the confirm back when every tool is switched off', async () => {
    render(ToolsUpdateModal, { props: { tools, onclose: vi.fn() } });
    for (const t of tools) await fireEvent.click(screen.getByTitle(t.name));
    expect(screen.getByRole('button', { name: 'Update' })).toBeDisabled();
  });

  // A refused checksum names the tool it refused, and the batch carries on.
  it('reports a failed tool by name once the dialog has closed', async () => {
    updateTool.mockImplementation(async (name: string) =>
      name === 'composer' ? { ok: false, error: 'checksum mismatch' } : { ok: true }
    );
    const onclose = vi.fn();
    render(ToolsUpdateModal, { props: { tools, onclose } });

    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(onclose).toHaveBeenCalled());
    expect(updateTool).toHaveBeenCalledTimes(2);
    expect(notifyLocalFailure).toHaveBeenCalledTimes(1);
    expect(notifyLocalFailure.mock.calls[0][2]).toBe('composer: checksum mismatch');
  });

  it('shows the running tool as updating on its own row', async () => {
    let finish: (v: { ok: boolean }) => void = () => {};
    updateTool.mockReturnValueOnce(new Promise((resolve) => (finish = resolve)));
    render(ToolsUpdateModal, { props: { tools, onclose: vi.fn() } });

    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));

    const row = screen.getByText('composer').closest('div.flex') as HTMLElement;
    expect(await within(row).findByText('updating…')).toBeInTheDocument();
    finish({ ok: true });
  });
});
