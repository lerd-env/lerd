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

// A site whose "feat" worktree is the active view, carrying whatever tunnel
// state the test needs on the worktree itself.
function withWorktree(wt: Record<string, unknown> = {}): Site {
  return {
    ...site,
    worktrees: [{ branch: 'feat', domain: 'feat.app.test', path: '/home/u/Code/app/wt/feat', ...wt }]
  } as unknown as Site;
}

const toolsPayload = {
  tools: [
    { name: 'ngrok', label: 'ngrok', binary: 'ngrok', installed: false, install_url: 'https://ngrok.com/download' },
    { name: 'cloudflare', label: 'Cloudflare Tunnel', binary: 'cloudflared', installed: true },
    { name: 'serveo', label: 'Serveo', binary: 'ssh', installed: true }
  ],
  auto: 'cloudflare',
  base_domain_answered: true
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

  it('offers the tunnel section on a worktree view', async () => {
    const { container } = render(Harness, { props: { site: withWorktree(), activeWorktreeBranch: 'feat' } });
    await openMenu(container);
    expect(screen.getByText('Public tunnel')).toBeInTheDocument();
    expect(screen.getByText('Local network')).toBeInTheDocument();
  });

  it('starts a worktree tunnel against its branch', async () => {
    const { container } = render(Harness, { props: { site: withWorktree(), activeWorktreeBranch: 'feat' } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByText('Serveo')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('Serveo').closest('button')!);
    await waitFor(() =>
      expect(
        fetchCalls.some((u) => u.includes('tunnel:start?tool=serveo&branch=feat'))
      ).toBe(true)
    );
  });

  // The branch has a tunnel of its own, so the menu must show that one rather
  // than whatever the parent site happens to be running.
  it('shows the worktree tunnel, not the site tunnel', async () => {
    const s = withWorktree({ tunnel_url: 'https://branch.serveo.net', tunnel_tool: 'serveo' });
    (s as unknown as { tunnel_url: string }).tunnel_url = 'https://parent.trycloudflare.com';
    const { container } = render(Harness, { props: { site: s, activeWorktreeBranch: 'feat' } });
    await openMenu(container);
    expect(screen.getByText('https://branch.serveo.net')).toBeInTheDocument();
    expect(screen.queryByText('https://parent.trycloudflare.com')).not.toBeInTheDocument();
  });

  it('stops a worktree tunnel against its branch', async () => {
    const s = withWorktree({ tunnel_url: 'https://branch.serveo.net', tunnel_tool: 'serveo' });
    const { container } = render(Harness, { props: { site: s, activeWorktreeBranch: 'feat' } });
    await openMenu(container);
    await fireEvent.click(screen.getByText('Stop'));
    await waitFor(() =>
      expect(fetchCalls.some((u) => u.includes('tunnel:stop?branch=feat'))).toBe(true)
    );
  });

  // While a tunnel is opening neither tunnelOn nor publicShared is true yet, so
  // an unguarded trigger would fall through and start a LAN share alongside it.
  it('holds the trigger inert while a tunnel is opening', async () => {
    fetchCalls = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        fetchCalls.push(url);
        if (url.includes('tunnel:start')) return new Promise<Response>(() => {});
        return new Response(JSON.stringify(toolsPayload), { status: 200 });
      })
    );
    const fn = vi.fn();
    const { container } = render(Harness, { props: { site, onToggleLan: fn } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByText('Serveo')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('Serveo').closest('button')!);
    await waitFor(() => expect(fetchCalls.some((u) => u.includes('tunnel:start'))).toBe(true));

    const trigger = screen.getByLabelText('Share on LAN');
    expect(trigger).toBeDisabled();
    await fireEvent.click(trigger);
    expect(fn).not.toHaveBeenCalled();
  });

  it('announces stopping the public share while one is live', async () => {
    const shared = { ...site, public_shared: true, public_share_url: 'https://app.dev.example.com' } as unknown as Site;
    render(Harness, { props: { site: shared } });
    expect(screen.getByLabelText('Stop public share')).toBeInTheDocument();
  });

  it('closes on Escape', async () => {
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByTestId('share-menu')).not.toBeInTheDocument();
  });
});

// A fetch that answers each endpoint properly, so the save → start sequence
// actually gets past the save.
function mockShareFetch(tools: Record<string, unknown>) {
  fetchCalls = [];
  const bodies: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      fetchCalls.push(url);
      if (init?.method === 'POST') {
        if (init.body) bodies.push(String(init.body));
        return new Response(JSON.stringify({ ok: true }), { status: 200 });
      }
      return new Response(JSON.stringify(tools), { status: 200 });
    })
  );
  return bodies;
}

const unanswered = { ...toolsPayload, base_domain_answered: false };

async function openCloudflare(container: HTMLElement) {
  await openMenu(container);
  await waitFor(() => expect(screen.getByText('Cloudflare Tunnel')).toBeInTheDocument());
  await fireEvent.click(screen.getByText('Cloudflare Tunnel').closest('button')!);
}

describe('ShareMenu base domain', () => {
  it('asks for a base domain before the first Cloudflare share', async () => {
    mockShareFetch(unanswered);
    const { container } = render(Harness, { props: { site } });
    await openCloudflare(container);
    expect(screen.getByText('Serve this site on your own domain?')).toBeInTheDocument();
    expect(fetchCalls.some((u) => u.includes('tunnel:start'))).toBe(false);
  });

  it('previews the hostname the site would be served on', async () => {
    mockShareFetch(unanswered);
    const { container } = render(Harness, { props: { site } });
    await openCloudflare(container);
    await fireEvent.input(screen.getByLabelText('Base domain'), { target: { value: 'example.com' } });
    expect(screen.getByText('app.example.com')).toBeInTheDocument();
  });

  it('starts on the given domain and remembers the answer', async () => {
    const bodies = mockShareFetch(unanswered);
    const { container } = render(Harness, { props: { site } });
    await openCloudflare(container);
    await fireEvent.input(screen.getByLabelText('Base domain'), { target: { value: 'example.com' } });
    await fireEvent.click(screen.getByLabelText('Remember this answer'));
    await fireEvent.click(screen.getByText('Use this domain'));

    await waitFor(() =>
      expect(
        fetchCalls.some((u) => u.includes('tunnel:start?tool=cloudflare&domain=example.com'))
      ).toBe(true)
    );
    expect(bodies.some((b) => JSON.parse(b).base_domain === 'example.com')).toBe(true);
    expect(bodies.some((b) => JSON.parse(b).remember === true)).toBe(true);
  });

  it('skips straight to a quick tunnel, carrying the remember choice', async () => {
    const bodies = mockShareFetch(unanswered);
    const { container } = render(Harness, { props: { site } });
    await openCloudflare(container);
    await fireEvent.click(screen.getByLabelText('Remember this answer'));
    await fireEvent.click(screen.getByText('Skip'));

    await waitFor(() =>
      expect(fetchCalls.some((u) => u.includes('tunnel:start?tool=cloudflare'))).toBe(true)
    );
    expect(fetchCalls.some((u) => u.includes('domain='))).toBe(false);
    const saved = bodies.map((b) => JSON.parse(b)).find((b) => 'remember' in b);
    expect(saved).toEqual({ base_domain: '', remember: true });
  });

  it('stops asking once the answer has been remembered', async () => {
    mockShareFetch({ ...toolsPayload, base_domain_answered: true, base_domain: 'example.com' });
    const { container } = render(Harness, { props: { site } });
    await openCloudflare(container);
    expect(screen.queryByText('Serve this site on your own domain?')).not.toBeInTheDocument();
    await waitFor(() =>
      expect(fetchCalls.some((u) => u.includes('tunnel:start?tool=cloudflare'))).toBe(true)
    );
  });

  it('shows the hostname a remembered domain will serve the site on', async () => {
    mockShareFetch({ ...toolsPayload, base_domain_answered: true, base_domain: 'example.com' });
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByText('app.example.com')).toBeInTheDocument());
  });

  it('reconfigures the domain from the cog without starting anything', async () => {
    const bodies = mockShareFetch({ ...toolsPayload, base_domain_answered: true, base_domain: 'old.example' });
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByTestId('share-domain-cog')).toBeInTheDocument());
    await fireEvent.click(screen.getByTestId('share-domain-cog'));

    const input = screen.getByLabelText('Base domain') as HTMLInputElement;
    expect(input.value).toBe('old.example');
    await fireEvent.input(input, { target: { value: 'new.example' } });
    await fireEvent.click(screen.getByText('Save'));

    await waitFor(() => expect(bodies.some((b) => JSON.parse(b).base_domain === 'new.example')).toBe(true));
    expect(fetchCalls.some((u) => u.includes('tunnel:start'))).toBe(false);
  });

  it('reports a rejected domain instead of starting', async () => {
    fetchCalls = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        fetchCalls.push(String(input));
        if (init?.method === 'POST') {
          return new Response(JSON.stringify({ error: 'base domain "nope" needs at least one dot' }), { status: 200 });
        }
        return new Response(JSON.stringify(unanswered), { status: 200 });
      })
    );
    const { container } = render(Harness, { props: { site } });
    await openCloudflare(container);
    await fireEvent.input(screen.getByLabelText('Base domain'), { target: { value: 'nope' } });
    await fireEvent.click(screen.getByText('Use this domain'));

    await waitFor(() => expect(screen.getByText(/needs at least one dot/)).toBeInTheDocument());
    expect(fetchCalls.some((u) => u.includes('tunnel:start'))).toBe(false);
  });

  // A worktree is served on its own subdomain, and the hostname follows that
  // domain flattened into one label.
  it('previews the worktree hostname on a branch tab', async () => {
    mockShareFetch(unanswered);
    const { container } = render(Harness, {
      props: { site: withWorktree(), activeWorktreeBranch: 'feat' }
    });
    await openCloudflare(container);
    await fireEvent.input(screen.getByLabelText('Base domain'), { target: { value: 'example.com' } });
    expect(screen.getByText('feat-app.example.com')).toBeInTheDocument();
  });

  it('starts a worktree share on the given domain, against its branch', async () => {
    mockShareFetch(unanswered);
    const { container } = render(Harness, {
      props: { site: withWorktree(), activeWorktreeBranch: 'feat' }
    });
    await openCloudflare(container);
    await fireEvent.input(screen.getByLabelText('Base domain'), { target: { value: 'example.com' } });
    await fireEvent.click(screen.getByText('Use this domain'));
    await waitFor(() =>
      expect(
        fetchCalls.some((u) =>
          u.includes('tunnel:start?tool=cloudflare&branch=feat&domain=example.com')
        )
      ).toBe(true)
    );
  });

  it('says when the running tunnel was started from the CLI', async () => {
    mockShareFetch(toolsPayload);
    const running = {
      ...site,
      tunnel_url: 'https://x.trycloudflare.com',
      tunnel_tool: 'cloudflare',
      tunnel_external: true
    } as unknown as Site;
    const { container } = render(Harness, { props: { site: running } });
    await openMenu(container);
    await waitFor(() =>
      expect(screen.getByText('Tunnel via Cloudflare Tunnel, from the CLI')).toBeInTheDocument()
    );
  });
});

describe('ShareMenu ngrok token', () => {
  it('offers a cog on ngrok whether or not it is installed', async () => {
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByTestId('share-token-cog')).toBeInTheDocument());
  });

  it('saves a token from the cog without starting anything', async () => {
    const bodies = mockShareFetch(toolsPayload);
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByTestId('share-token-cog')).toBeInTheDocument());
    await fireEvent.click(screen.getByTestId('share-token-cog'));

    const input = screen.getByTestId('share-token-input') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: '2abcXYZ' } });
    await fireEvent.click(screen.getByText('Save'));

    await waitFor(() => expect(bodies.some((b) => JSON.parse(b).ngrok_token === '2abcXYZ')).toBe(true));
    expect(fetchCalls.some((u) => u.includes('tunnel:start'))).toBe(false);
  });

  // The token is a credential, so the field is never a readable one.
  it('masks the token field', async () => {
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByTestId('share-token-cog')).toBeInTheDocument());
    await fireEvent.click(screen.getByTestId('share-token-cog'));

    expect((screen.getByTestId('share-token-input') as HTMLInputElement).type).toBe('password');
  });

  // A stored token is never sent back, so the form starts empty and says so
  // rather than showing a masked value that cannot be edited.
  it('starts empty when a token is already stored, and offers to clear it', async () => {
    mockShareFetch({ ...toolsPayload, ngrok_token_set: true });
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByTestId('share-token-cog')).toBeInTheDocument());
    await fireEvent.click(screen.getByTestId('share-token-cog'));

    expect((screen.getByTestId('share-token-input') as HTMLInputElement).value).toBe('');
    expect(screen.getByTestId('share-token-clear')).toBeInTheDocument();
  });

  it('clears the stored token', async () => {
    const bodies = mockShareFetch({ ...toolsPayload, ngrok_token_set: true });
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByTestId('share-token-cog')).toBeInTheDocument());
    await fireEvent.click(screen.getByTestId('share-token-cog'));
    await fireEvent.click(screen.getByTestId('share-token-clear'));

    await waitFor(() => expect(bodies.some((b) => JSON.parse(b).ngrok_token === '')).toBe(true));
  });

  // With a token stored, ngrok runs from its image, so the menu offers it
  // instead of sending the user to an install page they do not need.
  it('offers a containerised ngrok as usable', async () => {
    mockShareFetch({
      ...toolsPayload,
      tools: [
        { name: 'ngrok', label: 'ngrok', binary: 'ngrok', installed: true, containerised: true },
        ...toolsPayload.tools.slice(1)
      ],
      ngrok_token_set: true
    });
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByText('Runs as a container')).toBeInTheDocument());
    expect(screen.queryByText('Not installed · ngrok.com')).not.toBeInTheDocument();
  });
});

describe('ShareMenu ngrok without a binary or a token', () => {
  const needsToken = {
    ...toolsPayload,
    tools: [
      { name: 'ngrok', label: 'ngrok', binary: 'ngrok', installed: false, needs_token: true, install_url: 'https://ngrok.com/download' },
      ...toolsPayload.tools.slice(1)
    ]
  };

  // Pointing at an install page hides the fact that a token alone is enough.
  it('asks for a token rather than sending the user to install ngrok', async () => {
    mockShareFetch(needsToken);
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByText('Needs an auth token')).toBeInTheDocument());
    expect(screen.queryByText('Not installed · ngrok.com')).not.toBeInTheDocument();
  });

  it('opens the token form from the row instead of starting a tunnel', async () => {
    mockShareFetch(needsToken);
    const { container } = render(Harness, { props: { site } });
    await openMenu(container);
    await waitFor(() => expect(screen.getByTestId('share-tool-needs-token')).toBeInTheDocument());
    await fireEvent.click(screen.getByTestId('share-tool-needs-token'));

    expect(screen.getByTestId('share-token-input')).toBeInTheDocument();
    expect(fetchCalls.some((u) => u.includes('tunnel:start'))).toBe(false);
  });
});
