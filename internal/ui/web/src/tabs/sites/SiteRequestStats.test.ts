import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import SiteRequestStats from './SiteRequestStats.svelte';
import { profilerEnabled } from '../../stores/profiler';

const SNAPSHOT = {
  site: 'acme',
  median_millis: 40,
  samples: 30,
  slow: [
    { route: 'GET /reports/:id', method: 'GET', example: '/reports/7', p95_millis: 460, multiplier: 11.2, samples: 10 }
  ]
};

function mockFetch(body: unknown) {
  globalThis.fetch = vi.fn(
    async () =>
      new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
  ) as unknown as typeof fetch;
}

describe('SiteRequestStats', () => {
  const realFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = realFetch;
    vi.restoreAllMocks();
  });
  beforeEach(() => {
    profilerEnabled.set(false);
  });

  it('renders the slow route with a Profile button', async () => {
    mockFetch(SNAPSHOT);
    const { findByText } = render(SiteRequestStats, { props: { domain: 'acme.test' } });
    expect(await findByText('GET /reports/:id')).toBeTruthy();
    expect(await findByText('Profile')).toBeTruthy();
  });

  it('shows an all-good message when there is traffic but no slow routes', async () => {
    mockFetch({ site: 'acme', median_millis: 38, samples: 40, slow: [] });
    const { findByText } = render(SiteRequestStats, { props: { domain: 'acme.test' } });
    expect(await findByText(/within the typical range/i)).toBeTruthy();
  });

  // The API sends slow as null (not []) when nothing is flagged; the panel must
  // still render the all-good state rather than crash on null.length.
  it('handles a null slow list without crashing', async () => {
    mockFetch({ site: 'acme', median_millis: 38, samples: 40, slow: null });
    const { findByText } = render(SiteRequestStats, { props: { domain: 'acme.test' } });
    expect(await findByText(/within the typical range/i)).toBeTruthy();
  });

  // A slow response for the previous site must not paint its numbers over the
  // site the user has since switched to.
  it('drops a stale response after the domain changes', async () => {
    let resolveA!: (r: Response) => void;
    const aPending = new Promise<Response>((res) => (resolveA = res));
    globalThis.fetch = vi.fn((url: string | URL | Request) => {
      if (String(url).includes('aaa.test')) return aPending;
      return Promise.resolve(
        new Response(JSON.stringify({ site: 'bbb', median_millis: 99, samples: 5, slow: [] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' }
        })
      );
    }) as unknown as typeof fetch;

    const { rerender, findByText, queryByText } = render(SiteRequestStats, {
      props: { domain: 'aaa.test' }
    });
    await rerender({ domain: 'bbb.test' });
    await findByText(/within the typical range/i);

    // Resolve the now-stale aaa request with slow data; it must be ignored.
    resolveA(
      new Response(
        JSON.stringify({
          site: 'aaa',
          median_millis: 5,
          samples: 50,
          slow: [{ route: 'GET /old', method: 'GET', example: '/old', p95_millis: 999, multiplier: 20, samples: 10 }]
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    );
    await new Promise((r) => setTimeout(r, 0));
    expect(queryByText('GET /old')).toBeNull();
  });

  it('links a GET route to its concrete example URL', async () => {
    mockFetch(SNAPSHOT);
    const { findByText } = render(SiteRequestStats, { props: { domain: 'acme.test' } });
    const link = (await findByText('GET /reports/:id')).closest('a');
    expect(link?.getAttribute('href')).toBe('https://acme.test/reports/7');
    expect(link?.getAttribute('target')).toBe('_blank');
  });

  it('arms the profiler, then opens the route and switches to the profiler', async () => {
    const calls: string[] = [];
    globalThis.fetch = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const u = String(url);
      calls.push(`${init?.method ?? 'GET'} ${u}`);
      if (u.includes('/toggle')) return new Response(JSON.stringify({ enabled: true }), { status: 200 });
      return new Response(JSON.stringify(SNAPSHOT), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }) as unknown as typeof fetch;

    // A pre-opened blank tab whose location is navigated once profiling is armed.
    const tab = { location: { href: '' }, close: vi.fn() };
    const openSpy = vi.spyOn(window, 'open').mockReturnValue(tab as unknown as Window);

    const { findByText } = render(SiteRequestStats, { props: { domain: 'acme.test' } });
    await fireEvent.click(await findByText('Profile'));

    expect(openSpy).toHaveBeenCalledWith('', '_blank');
    expect(calls.some((c) => c.startsWith('POST') && c.includes('/api/profiler/toggle'))).toBe(true);
    // Route is navigated only after the toggle resolves (i.e. after it's armed).
    await waitFor(() => expect(tab.location.href).toBe('https://acme.test/reports/7'));
  });
});
