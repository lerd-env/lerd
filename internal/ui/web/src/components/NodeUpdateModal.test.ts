import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import NodeUpdateModal from './NodeUpdateModal.svelte';
import type { NodeVersionStatus } from '$stores/nodeVersions';

const { post, notifyLocalFailure } = vi.hoisted(() => ({ post: vi.fn(), notifyLocalFailure: vi.fn() }));
// Stopped at the request, so the real batch runs: order, carrying on, reporting.
vi.mock('$lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api')>();
  return {
    ...actual,
    apiJson: async () => [],
    apiFetch: async (url: string) => new Response(JSON.stringify(await post(url)))
  };
});
vi.mock('$lib/notify', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/notify')>();
  return { ...actual, notifyLocalFailure };
});

const version = (major: string, installed: string, latest: string): NodeVersionStatus => ({
  major,
  installed,
  latest,
  update_available: true
});
const versions = [version('22', '22.1.0', '22.2.0'), version('24', '24.1.0', '24.2.0')];

describe('NodeUpdateModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    post.mockResolvedValue({ ok: true });
  });

  it('lists each major with the version it moves from and to', () => {
    render(NodeUpdateModal, { props: { versions, onclose: vi.fn() } });
    expect(screen.getByText('Node 22')).toBeInTheDocument();
    expect(screen.getByText('22.1.0 → 22.2.0')).toBeInTheDocument();
    expect(screen.getByText('24.1.0 → 24.2.0')).toBeInTheDocument();
  });

  it('updates each version one after the other', async () => {
    const onclose = vi.fn();
    render(NodeUpdateModal, { props: { versions, onclose } });
    await fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(onclose).toHaveBeenCalled());
    expect(post.mock.calls.map((c) => c[0])).toEqual([
      '/api/node-versions/22/update',
      '/api/node-versions/24/update'
    ]);
  });

  it('carries on past a failure and names the version that failed', async () => {
    post.mockResolvedValueOnce({ ok: false, error: 'download failed' });
    const onclose = vi.fn();
    render(NodeUpdateModal, { props: { versions, onclose } });
    await fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(onclose).toHaveBeenCalled());
    expect(post).toHaveBeenCalledTimes(2);
    expect(notifyLocalFailure).toHaveBeenCalledWith(expect.anything(), expect.anything(), '22: download failed');
  });
});
