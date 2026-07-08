import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
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
  routes: [],
  recent: [
    { at_millis: 1783501663287, method: 'POST', route: 'POST /broadcasting/auth', uri: '/broadcasting/auth', status: 200, millis: 4, cold: false },
    { at_millis: 1783501663287, method: 'POST', route: 'POST /broadcasting/auth', uri: '/broadcasting/auth', status: 200, millis: 6, cold: false }
  ]
};

vi.mock('$stores/analytics', () => ({
  loadSiteAnalytics: vi.fn(async () => analytics),
  TIME_RANGES: ['15m', '1h', '24h', '7d']
}));

import SiteRequestTiming from './SiteRequestTiming.svelte';
import { m } from '../../paraglide/messages.js';

describe('SiteRequestTiming Recent list', () => {
  it('renders same-millisecond, same-URI rows without a duplicate-key crash', async () => {
    const { getByRole, findByText, getAllByText } = render(SiteRequestTiming, {
      props: { site: { domain: 'whitewaters' } }
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
