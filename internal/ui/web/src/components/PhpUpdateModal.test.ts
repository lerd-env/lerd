import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import PhpUpdateModal from './PhpUpdateModal.svelte';
import { phpRuntime } from '$stores/phpRuntime';

const { rebuild, notifyLocalFailure, loadStatus } = vi.hoisted(() => ({
  rebuild: vi.fn(),
  notifyLocalFailure: vi.fn(),
  loadStatus: vi.fn()
}));
// Stopped at the request, so the real stream parsing and batch run.
vi.mock('$lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api')>();
  return {
    ...actual,
    apiFetch: async (url: string) => new Response(await rebuild(decodeURIComponent(url.split('/')[3])))
  };
});
vi.mock('$stores/status', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/status')>();
  return { ...actual, loadStatus };
});
vi.mock('$lib/notify', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/notify')>();
  return { ...actual, notifyLocalFailure };
});

const sse = (payload: object, ...lines: string[]) =>
  lines.map((l) => `data: ${l}\n\n`).join('') + `event: done\ndata: ${JSON.stringify(payload)}\n\n`;

const versions = ['8.3', '8.4'];

describe('PhpUpdateModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    phpRuntime.set('container');
    rebuild.mockResolvedValue(sse({ ok: true }));
  });

  it('lists each version', () => {
    render(PhpUpdateModal, { props: { versions, onclose: vi.fn() } });
    expect(screen.getByText('PHP 8.3')).toBeInTheDocument();
    expect(screen.getByText('PHP 8.4')).toBeInTheDocument();
  });

  it('rebuilds only the versions still switched on, then re-reads the status', async () => {
    const onclose = vi.fn();
    render(PhpUpdateModal, { props: { versions, onclose } });

    await fireEvent.click(screen.getByTitle('PHP 8.4'));
    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(onclose).toHaveBeenCalled());
    expect(rebuild.mock.calls).toEqual([['8.3']]);
    expect(loadStatus).toHaveBeenCalled();
  });

  // The server answers a failed rebuild with ok:false inside a 200 stream.
  it('carries on past a failed version and reports it by name', async () => {
    rebuild.mockImplementation(async (v: string) =>
      v === '8.3' ? sse({ ok: false, error: 'PHP 8.3 is already building' }) : sse({ ok: true })
    );
    const onclose = vi.fn();
    render(PhpUpdateModal, { props: { versions, onclose } });

    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(onclose).toHaveBeenCalled());
    expect(rebuild).toHaveBeenCalledTimes(2);
    expect(notifyLocalFailure).toHaveBeenCalledTimes(1);
    expect(notifyLocalFailure.mock.calls[0][2]).toBe('PHP 8.3: PHP 8.3 is already building');
  });

  // A stream cut before its done frame is not a success.
  it('counts a stream that ends without a result as a failure', async () => {
    rebuild.mockResolvedValue('data: Step 1/4\n\n');
    const onclose = vi.fn();
    render(PhpUpdateModal, { props: { versions: ['8.3'], onclose } });

    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(onclose).toHaveBeenCalled());
    expect(notifyLocalFailure).toHaveBeenCalledTimes(1);
  });

  it('shows the running version the latest build line on its row', async () => {
    const enc = new TextEncoder();
    let feed: ReadableStreamDefaultController<Uint8Array> | undefined;
    rebuild.mockReturnValueOnce(new ReadableStream({ start: (c) => void (feed = c) }));
    render(PhpUpdateModal, { props: { versions, onclose: vi.fn() } });

    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));
    await waitFor(() => expect(feed).toBeDefined());
    feed!.enqueue(enc.encode(sse({ ok: true }, 'Step 1/4', 'Step 2/4').split('event:')[0]));

    expect(await screen.findByText('Step 2/4')).toBeInTheDocument();
    feed!.close();
  });

  // Matches `lerd php:rebuild`, which builds every version at once.
  it('starts every rebuild at once on the container runtime', async () => {
    let finish: (v: string) => void = () => {};
    const held = new Promise<string>((resolve) => (finish = resolve));
    rebuild.mockReturnValue(held);
    const onclose = vi.fn();
    render(PhpUpdateModal, { props: { versions, onclose } });

    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));

    await waitFor(() => expect(rebuild).toHaveBeenCalledTimes(2));
    finish(sse({ ok: true }));
    await waitFor(() => expect(onclose).toHaveBeenCalled());
  });

  // `lerd php:update` takes native builds in turn, so the dashboard does too.
  it('updates native versions one at a time', async () => {
    phpRuntime.set('native');
    let finish: (v: string) => void = () => {};
    rebuild.mockReturnValueOnce(new Promise<string>((resolve) => (finish = resolve)));
    const onclose = vi.fn();
    render(PhpUpdateModal, { props: { versions, onclose } });

    await fireEvent.click(screen.getByRole('button', { name: 'Update' }));
    await waitFor(() => expect(rebuild).toHaveBeenCalledTimes(1));
    await new Promise((r) => setTimeout(r));
    expect(rebuild).toHaveBeenCalledTimes(1);

    finish(sse({ ok: true }));
    await waitFor(() => expect(rebuild).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(onclose).toHaveBeenCalled());
  });

  // There is no image on the native runtime: the same action downloads a build.
  it('describes a download on the native runtime', () => {
    phpRuntime.set('native');
    render(PhpUpdateModal, { props: { versions, onclose: vi.fn() } });
    expect(screen.getByText(/newest published build/)).toBeInTheDocument();
    expect(screen.queryByText(/base image/i)).toBeNull();
  });
});
