import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import { readable } from 'svelte/store';
import type { Analytics } from '$stores/analytics';

// Two requests to the same URI in the same millisecond are real (Reverb/websocket
// bursts) and the durable store has no unique constraint, so the Recent list can
// receive rows that share (at_millis, uri). Its {#each} key must stay unique or
// Svelte 5 throws a duplicate-key error at render and blanks the tab.
const analytics: Analytics = {
  site: 'whitewaters',
  range: '1h',
  samples: 2,
  cold_starts: 0,
  median_millis: 5,
  p95_millis: 9,
  status: { c2xx: 2, c3xx: 0, c4xx: 0, c5xx: 0 },
  distribution: [],
  throughput: [],
  routes: [
    {
      route: 'GET /reports/:id',
      method: 'GET',
      example: '/reports/7',
      p50_millis: 40,
      p95_millis: 500,
      recent_p95_millis: 500,
      multiplier: 10,
      samples: 10
    }
  ],
  recent: [
    { at_millis: 1783501663287, method: 'POST', route: 'POST /broadcasting/auth', uri: '/broadcasting/auth', status: 200, millis: 4, cold: false },
    { at_millis: 1783501663287, method: 'POST', route: 'POST /broadcasting/auth', uri: '/broadcasting/auth', status: 200, millis: 6, cold: false }
  ],
  excluded: []
};

const loadSiteAnalytics = vi.fn(async () => analytics);
const removeRecorded = vi.fn(async () => {});
const unexcludeRoute = vi.fn(async () => {});

vi.mock('$stores/analytics', () => ({
  loadSiteAnalytics: (...args: unknown[]) => loadSiteAnalytics(...(args as [])),
  removeRecorded: (...args: unknown[]) => removeRecorded(...(args as [])),
  unexcludeRoute: (...args: unknown[]) => unexcludeRoute(...(args as [])),
  TIME_RANGES: ['15m', '1h', '24h', '7d']
}));

vi.mock('$stores/profiler', async () => {
  const { writable } = await import('svelte/store');
  return {
    profilerEnabled: writable(true),
    setProfiler: vi.fn(async () => {}),
    captureCount: vi.fn(async () => 0),
    waitForCapture: vi.fn(async () => true)
  };
});
vi.mock('$stores/dashboard', () => ({ openProfiler: vi.fn() }));

import SiteRequestTiming from './SiteRequestTiming.svelte';
import { profilerEnabled, setProfiler, captureCount, waitForCapture } from '$stores/profiler';
import { openProfiler } from '$stores/dashboard';
import { m } from '../../paraglide/messages.js';

// The profiler starts armed for the tests that are only about the navigation.
function resetProfilerMocks(enabled = true) {
  profilerEnabled.set(enabled);
  vi.mocked(setProfiler).mockClear();
  vi.mocked(captureCount).mockClear().mockResolvedValue(0);
  vi.mocked(waitForCapture).mockClear().mockResolvedValue(true);
  vi.mocked(openProfiler).mockClear();
}

describe('SiteRequestTiming Recent list', () => {
  it('renders same-millisecond, same-URI rows without a duplicate-key crash', async () => {
    const { getByRole, findByText, getAllByText } = render(SiteRequestTiming, {
      props: { site: { domain: 'whitewaters', can_profile: true } }
    });

    // Wait for the loaded view, then switch to the Recent tab.
    await findByText(m.sites_timing_recent());
    await fireEvent.click(getByRole('button', { name: m.sites_timing_recent() }));

    // Both colliding rows render; without a unique key the keyed each throws.
    await waitFor(() => {
      expect(getAllByText('/broadcasting/auth').length).toBe(2);
    });
  });
});

// A worktree is served from its own subdomain, so the panel must both ask for the
// branch's timing and send its route links there. It used to open the parent's
// domain, profiling a route on the wrong checkout.
describe('SiteRequestTiming on a worktree', () => {
  const site = {
    domain: 'whitewaters.test',
    tls: true,
    can_profile: true,
    worktrees: [{ branch: 'feature-x', domain: 'feature-x.whitewaters.test' }]
  };

  it('loads the branch and opens routes on the worktree domain', async () => {
    loadSiteAnalytics.mockClear();
    resetProfilerMocks();
    const open = vi.fn();
    vi.stubGlobal('open', open);

    const { findByRole } = render(SiteRequestTiming, {
      props: {
        site,
        activeWorktreeBranch: 'feature-x'
      }
    });

    await waitFor(() => {
      expect(loadSiteAnalytics).toHaveBeenCalledWith('whitewaters.test', '1h', 'feature-x');
    });

    // The slow route's own row is the profile trigger; its accessible name is the
    // method and path it renders.
    await fireEvent.click(await findByRole('button', { name: /GET.*\/reports\/:id/ }));
    await waitFor(() => {
      expect(open).toHaveBeenCalledWith('https://feature-x.whitewaters.test/reports/7', '_blank');
    });
  });
});

// A localhost site is served over plain HTTP (no mkcert cert), so profiling a
// route must open http://, not https:// which throws a certificate error.
describe('SiteRequestTiming on a localhost site', () => {
  const site = { domain: 'whitewaters.localhost', can_profile: true };

  it('profiles routes over http on an unsecured localhost site', async () => {
    loadSiteAnalytics.mockClear();
    resetProfilerMocks();
    const open = vi.fn();
    vi.stubGlobal('open', open);

    const { findByRole } = render(SiteRequestTiming, { props: { site } });

    await fireEvent.click(await findByRole('button', { name: /GET.*\/reports\/:id/ }));
    await waitFor(() => {
      expect(open).toHaveBeenCalledWith('http://whitewaters.localhost/reports/7', '_blank');
    });
  });
});

// Arming and firing the request are not enough on their own: the report only
// exists once SPX has written it, so the handover to the profiler has to wait for
// the capture rather than happen in the same tick as the navigation.
describe('SiteRequestTiming profiling a slow route', () => {
  const site = { domain: 'whitewaters.test', tls: true, can_profile: true };

  it('opens the profiler only once the request has been captured', async () => {
    resetProfilerMocks();
    let landed: (v: boolean) => void = () => {};
    vi.mocked(captureCount).mockResolvedValue(4);
    vi.mocked(waitForCapture).mockReturnValue(new Promise<boolean>((r) => (landed = r)));
    const open = vi.fn();
    vi.stubGlobal('open', open);

    const { findByRole, findByText } = render(SiteRequestTiming, { props: { site } });
    await fireEvent.click(await findByRole('button', { name: /GET.*\/reports\/:id/ }));

    // The request is out and the wait is on, but SPX has nothing to show yet.
    await findByText(m.sites_reqstats_profileWaiting());
    expect(openProfiler).not.toHaveBeenCalled();
    // One tab, opened at the real URL. A blank tab held open across the arming
    // wait is an about:blank the desktop is asked to find an application for.
    expect(open.mock.calls).toEqual([['https://whitewaters.test/reports/7', '_blank']]);
    // The count taken before the request is what the wait measures against.
    expect(waitForCapture).toHaveBeenCalledWith('whitewaters.test', 'GET /reports/:id', 4);

    landed(true);
    await waitFor(() => expect(openProfiler).toHaveBeenCalled());
  });

  it('reports a request the profiler never saw instead of opening an empty report list', async () => {
    resetProfilerMocks();
    vi.mocked(waitForCapture).mockResolvedValue(false);
    vi.stubGlobal('open', vi.fn(() => ({ location: { href: '' } })));

    const { findByRole, findByText } = render(SiteRequestTiming, { props: { site } });
    await fireEvent.click(await findByRole('button', { name: /GET.*\/reports\/:id/ }));

    await findByText(m.sites_reqstats_profileMissed());
    expect(openProfiler).not.toHaveBeenCalled();
  });

  // The profiler is global and profiles every FPM site while it is on, which is a
  // lot to leave behind for one click on one route.
  it('puts the profiler back when the click was what armed it', async () => {
    resetProfilerMocks(false);
    vi.stubGlobal('open', vi.fn(() => ({ location: { href: '' } })));

    const { findByRole } = render(SiteRequestTiming, { props: { site } });
    await fireEvent.click(await findByRole('button', { name: /GET.*\/reports\/:id/ }));

    await waitFor(() => expect(vi.mocked(setProfiler).mock.calls).toEqual([[true], [false]]));
  });

  it('leaves a profiler that was already armed alone', async () => {
    resetProfilerMocks(true);
    vi.stubGlobal('open', vi.fn(() => ({ location: { href: '' } })));

    const { findByRole } = render(SiteRequestTiming, { props: { site } });
    await fireEvent.click(await findByRole('button', { name: /GET.*\/reports\/:id/ }));

    await waitFor(() => expect(openProfiler).toHaveBeenCalled());
    expect(setProfiler).not.toHaveBeenCalled();
  });
});

// A route with no example, or one that isn't a GET, has no URL to open. Arming is
// global and rewrites every FPM vhost, so it must not happen for a click that
// could only ever land on an empty report.
describe('SiteRequestTiming on a route it cannot open', () => {
  it('does not arm the profiler for a POST route', async () => {
    resetProfilerMocks(false);
    const post = {
      ...analytics,
      routes: [
        { route: 'POST /checkout', method: 'POST', example: '/checkout', p50_millis: 40, p95_millis: 900, recent_p95_millis: 900, multiplier: 10, samples: 10 }
      ]
    } as Analytics;
    loadSiteAnalytics.mockResolvedValueOnce(post);
    const open = vi.fn();
    vi.stubGlobal('open', open);

    const { findAllByText, queryByRole } = render(SiteRequestTiming, {
      props: { site: { domain: 'whitewaters.test', can_profile: true } }
    });

    await findAllByText('/checkout');
    expect(queryByRole('button', { name: /POST.*\/checkout/ })).toBeNull();
    expect(setProfiler).not.toHaveBeenCalled();
    expect(open).not.toHaveBeenCalled();
  });
});

// Removing history is destructive and the checkbox in the same modal is what
// makes it permanent, so the confirmation has to be what sends either one.
describe('SiteRequestTiming removing recorded traffic', () => {
  const site = { domain: 'whitewaters.test', tls: true, can_profile: true };

  function resetRemoveMocks() {
    loadSiteAnalytics.mockClear().mockResolvedValue(analytics);
    removeRecorded.mockClear();
    unexcludeRoute.mockClear();
  }

  it('drops a route only after the modal is confirmed', async () => {
    resetRemoveMocks();
    const { findAllByRole, findByRole } = render(SiteRequestTiming, { props: { site } });

    const buttons = await findAllByRole('button', { name: m.sites_timing_removeRow() });
    await fireEvent.click(buttons[0]);
    expect(removeRecorded).not.toHaveBeenCalled();

    await fireEvent.click(await findByRole('button', { name: m.sites_timing_removeConfirm() }));
    await waitFor(() => {
      expect(removeRecorded).toHaveBeenCalledWith('whitewaters.test', {
        route: 'GET /reports/:id',
        branch: '',
        exclude: false
      });
    });
  });

  it('sends the exclusion when the checkbox is ticked', async () => {
    resetRemoveMocks();
    const { findAllByRole, findByRole } = render(SiteRequestTiming, { props: { site } });

    await fireEvent.click((await findAllByRole('button', { name: m.sites_timing_removeRow() }))[0]);
    await fireEvent.click(await findByRole('checkbox'));
    await fireEvent.click(await findByRole('button', { name: m.sites_timing_removeConfirm() }));

    await waitFor(() => {
      expect(removeRecorded).toHaveBeenCalledWith('whitewaters.test', {
        route: 'GET /reports/:id',
        branch: '',
        exclude: true
      });
    });
  });

  // A recent row is one request, so its button must name that request and not
  // sweep away every other hit on the same route.
  it('removes a single request from the recent list', async () => {
    resetRemoveMocks();
    const { getByRole, findByText, findAllByRole, findByRole } = render(SiteRequestTiming, {
      props: { site }
    });
    await findByText(m.sites_timing_recent());
    await fireEvent.click(getByRole('button', { name: m.sites_timing_recent() }));

    const buttons = await findAllByRole('button', { name: m.sites_timing_removeRow() });
    await fireEvent.click(buttons[buttons.length - 1]);
    await fireEvent.click(await findByRole('button', { name: m.sites_timing_removeConfirm() }));

    await waitFor(() => {
      expect(removeRecorded).toHaveBeenCalledWith('whitewaters.test', {
        route: 'POST /broadcasting/auth',
        branch: '',
        exclude: false,
        uri: '/broadcasting/auth',
        at_millis: 1783501663287
      });
    });
  });

  // Excluded routes are a rare housekeeping list, so they live behind the cog
  // rather than as a card sitting under the panel on every visit.
  it('lists excluded routes behind the cog and puts one back under observation', async () => {
    resetRemoveMocks();
    loadSiteAnalytics.mockResolvedValue({ ...analytics, excluded: ['GET /health'] } as Analytics);

    const { findByRole, queryByText } = render(SiteRequestTiming, { props: { site } });
    await findByRole('button', { name: m.sites_timing_excludedManage() });
    expect(queryByText('GET /health')).toBeNull();

    await fireEvent.click(await findByRole('button', { name: m.sites_timing_excludedManage() }));
    await fireEvent.click(await findByRole('button', { name: m.sites_timing_unexclude() }));
    await waitFor(() => {
      expect(unexcludeRoute).toHaveBeenCalledWith('whitewaters.test', 'GET /health', '');
    });
  });
});

// SPX lives in the FPM image, so a site served by FrankenPHP, a custom container
// or a host-proxy dev server has nothing to profile. The slow routes still read,
// they just aren't a profile trigger.
describe('SiteRequestTiming on a site SPX cannot profile', () => {
  it('lists the slow route without making it clickable', async () => {
    const { findAllByText, queryByRole } = render(SiteRequestTiming, {
      props: { site: { domain: 'nuxtapp.test', can_profile: false } }
    });

    await findAllByText('/reports/:id');
    expect(queryByRole('button', { name: /GET.*\/reports\/:id/ })).toBeNull();
  });
});
