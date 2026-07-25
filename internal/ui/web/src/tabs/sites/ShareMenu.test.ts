import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import Harness from './ShareMenu.test.svelte';
import type { Site } from '$stores/sites';

const site = {
  domain: 'app.test',
  domains: ['app.test'],
  path: '/home/u/Code/app',
  worktrees: []
} as unknown as Site;

const toolsPayload = {
  tools: [
    { name: 'ngrok', label: 'ngrok', binary: 'ngrok', installed: false, install_url: 'https://ngrok.com/download' },
    { name: 'cloudflare', label: 'Cloudflare Tunnel', binary: 'cloudflared', installed: true },
    { name: 'serveo', label: 'Serveo', binary: 'ssh', installed: true }
  ],
  auto: 'cloudflare'
};

let fetchCalls: string[];

function mockFetch(payload: unknown = toolsPayload) {
  fetchCalls = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      fetchCalls.push(String(input));
      return new Response(JSON.stringify(payload), { status: 200 });
    })
  );
}

async function openMenu(container: HTMLElement) {
  const wrapper = container.querySelector('div.relative');
  await fireEvent.mouseEnter(wrapper!);
  return screen.getByTestId('share-menu');
}

beforeEach(() => mockFetch());

describe('ShareMenu', () => {
  it('keeps the click path a plain LAN toggle', async () => {
    const fn = vi.fn();
    render(Harness, { props: { site, onToggleLan: fn } });
    await fireEvent.click(screen.getByLabelText('Share on LAN'));
    expect(fn).toHaveBeenCalledOnce();
  });

  it('stops the tunnel on click while one is running, without touching LAN', async () => {
    const fn = vi.fn();
    const running = { ...site, tunnel_url: 'https://x.trycloudflare.com', tunnel_tool: 'cloudflare' } as unknown as Site;
    render(Harness, { props: { site: running, onToggleLan: fn } });
    await fireEvent.click(screen.getByLabelText('Stop tunnel'));
    await waitFor(() => expect(fetchCalls.some((u) => u.includes('/api/sites/app.test/tunnel:stop'))).toBe(true));
    expect(fn).not.toHaveBeenCalled();
  });

  it('opens on hover and lists the tunnel tools with the auto pick', async () => {
    const { container } = render(Harness, { props: { site } });
    expect(screen.queryByTestId('share-menu')).not.toBeInTheDocument();
    await openMenu(container);
    await waitFor(() => expect(screen.getByText('Cloudflare Tunnel')).toBeInTheDocument());
    expect(screen.getByText('Auto picks Cloudflare Tunnel')).toBeInTheDocument();
    expect(fetchCalls.some((u) => u.includes('/api/share-tools'))).toBe(true);
  });

  it('disables tools that are not installed and shows an install hint', async () => {
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByText('ngrok')).toBeInTheDocument());
    const entry = screen.getByText('ngrok').closest('button');
    expect(entry).toBeDisabled();
    expect(entry?.textContent).toContain('ngrok.com');
  });

  it('starts a tunnel with the picked tool', async () => {
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByText('Serveo')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('Serveo').closest('button')!);
    await waitFor(() =>
      expect(fetchCalls.some((u) => u.includes('/api/sites/app.test/tunnel:start?tool=serveo'))).toBe(true)
    );
  });

  it('shows the running tunnel with its URL and a stop action', async () => {
    const running = { ...site, tunnel_url: 'https://x.trycloudflare.com', tunnel_tool: 'cloudflare' } as unknown as Site;
    const { container } = render(Harness, { props: { site: running } });
    await openMenu(container);
    expect(screen.getByText('https://x.trycloudflare.com')).toBeInTheDocument();
    await fireEvent.click(screen.getByText('Stop'));
    await waitFor(() => expect(fetchCalls.some((u) => u.includes('/api/sites/app.test/tunnel:stop'))).toBe(true));
  });

  it('hides the tunnel section on a worktree view', async () => {
    const { container } = render(Harness, { props: { site, activeWorktreeBranch: 'feat' } });
    await openMenu(container);
    expect(screen.queryByText('Public tunnel')).not.toBeInTheDocument();
    expect(screen.getByText('Local network')).toBeInTheDocument();
  });

  it('closes on Escape', async () => {
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByTestId('share-menu')).not.toBeInTheDocument();
  });
});
