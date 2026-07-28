import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import ToolCard from './ToolCard.svelte';
import type { ToolStatus } from '$stores/status';

const { updateTool } = vi.hoisted(() => ({ updateTool: vi.fn() }));
vi.mock('$stores/status', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/status')>();
  return { ...actual, updateTool };
});

const tool = (over: Partial<ToolStatus>): ToolStatus => ({
  name: 'mkcert',
  installed: 'v1.4.4',
  pinned: 'v1.4.4',
  present: true,
  update_available: false,
  ...over
});

describe('ToolCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    updateTool.mockResolvedValue({ ok: true });
  });

  it('shows the installed version for an up-to-date tool', () => {
    render(ToolCard, { props: { tool: tool({}) } });
    expect(screen.getByText('mkcert')).toBeInTheDocument();
    expect(screen.getByText('v1.4.4')).toBeInTheDocument();
  });

  it('flags an available update with the pinned version', () => {
    render(ToolCard, {
      props: { tool: tool({ name: 'composer', installed: '2.10.2', pinned: '2.10.3', update_available: true }) }
    });
    expect(screen.getByText('2.10.2')).toBeInTheDocument();
    expect(screen.getByText(/2\.10\.3/)).toBeInTheDocument();
  });

  it('dims the pin for a missing tool', () => {
    render(ToolCard, { props: { tool: tool({ present: false, installed: '' }) } });
    const version = screen.getByText('v1.4.4');
    expect(version.className).toContain('text-gray-400');
  });

  it('applies the update from the card and reports the tool that failed', async () => {
    updateTool.mockResolvedValue({ ok: false, error: 'checksum mismatch for composer' });

    render(ToolCard, {
      props: { tool: tool({ name: 'composer', installed: '2.10.2', pinned: '2.10.3', update_available: true }) }
    });

    await fireEvent.click(screen.getByRole('button', { name: /2\.10\.3/ }));

    expect(updateTool).toHaveBeenCalledWith('composer');
    expect(await screen.findByText(/checksum mismatch/)).toBeInTheDocument();
  });

  it('offers no button when nothing is pending', () => {
    render(ToolCard, { props: { tool: tool({}) } });
    expect(screen.queryByRole('button')).toBeNull();
  });
});
