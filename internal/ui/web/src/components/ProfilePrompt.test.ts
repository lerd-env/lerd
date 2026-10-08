import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { m } from '../paraglide/messages.js';

const profileRoute = vi.fn();
vi.mock('$lib/profileRoute', () => ({ profileRoute: (...a: unknown[]) => profileRoute(...a) }));
vi.mock('$stores/dashboard', () => ({ openProfiler: vi.fn(), openProfilerReport: vi.fn() }));

import ProfilePrompt from './ProfilePrompt.svelte';
import { openProfiler, openProfilerReport } from '$stores/dashboard';

describe('ProfilePrompt', () => {
  beforeEach(() => {
    profileRoute.mockReset();
    vi.mocked(openProfiler).mockClear();
  });

  it('profiles the request route with SPX and opens the profiler once it lands', async () => {
    profileRoute.mockResolvedValue(true);
    render(ProfilePrompt, { props: { origin: 'https://shop.test', request: 'GET /users/5?tab=1' } });

    await fireEvent.click(screen.getByRole('button', { name: m.sites_reqstats_profile() }));
    expect(profileRoute.mock.calls[0].slice(0, 3)).toEqual(['shop.test', 'GET /users/:id', 'https://shop.test/users/5?tab=1']);
    await waitFor(() => expect(openProfiler).toHaveBeenCalled());
  });

  it('says so when no profile landed', async () => {
    profileRoute.mockResolvedValue(false);
    render(ProfilePrompt, { props: { origin: 'https://shop.test', request: 'GET /' } });

    await fireEvent.click(screen.getByRole('button', { name: m.sites_reqstats_profile() }));
    await screen.findByText(m.sites_reqstats_profileMissed());
    expect(openProfiler).not.toHaveBeenCalled();
  });

  it('offers nothing for a request it cannot reopen', () => {
    const { container } = render(ProfilePrompt, { props: { origin: 'https://shop.test', request: 'POST /orders' } });
    expect(container.textContent?.trim()).toBe('');
    const none = render(ProfilePrompt, { props: { origin: '', request: 'GET /' } });
    expect(none.container.textContent?.trim()).toBe('');
  });
});

describe('ProfilePrompt for a request SPX already profiled', () => {
  it('opens its flame graph instead of offering to profile again', async () => {
    const onleave = vi.fn();
    render(ProfilePrompt, { props: { origin: 'https://shop.test', request: 'GET /', profileKey: 'spx-full-7', onleave } });

    expect(screen.queryByRole('button', { name: m.sites_reqstats_profile() })).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: m.timeline_flameGraph() }));
    expect(openProfilerReport).toHaveBeenCalledWith('spx-full-7');
    expect(onleave).toHaveBeenCalled();
  });
});
