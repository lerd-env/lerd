import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import SiteIndicators from './SiteIndicators.svelte';
import type { Site } from '$stores/sites';

// The moon path is distinctive enough to detect "is the sleep indicator shown".
const MOON_PATH = 'M21.752 15.002';

function hasMoon(container: HTMLElement): boolean {
  return Array.from(container.querySelectorAll('path')).some((p) =>
    (p.getAttribute('d') || '').startsWith(MOON_PATH)
  );
}

function site(overrides: Partial<Site>): Site {
  return { domain: 'app.test', name: 'app', has_queue_worker: true, ...overrides } as Site;
}

describe('SiteIndicators sleep moon', () => {
  it('shows the moon when workers are actually suspended', () => {
    const { container } = render(SiteIndicators, {
      props: { site: site({ idle_suspended: true, idle_suspended_workers: ['queue'] }) }
    });
    expect(hasMoon(container)).toBe(true);
  });

  it('hides the moon the instant workers resume, even while the idle timeout flag is still stale', () => {
    // This is the bug: on resume the engine clears idle_suspended and publishes
    // immediately, but `idle` (read from the watcher's activity file) stays true
    // for up to a tick. The moon must follow idle_suspended, not idle.
    const { container } = render(SiteIndicators, {
      props: { site: site({ idle: true, idle_suspended: false, idle_suspended_workers: [] }) }
    });
    expect(hasMoon(container)).toBe(false);
  });

  it('shows no moon for a fully running site', () => {
    const { container } = render(SiteIndicators, {
      props: { site: site({ idle: false, idle_suspended: false, queue_running: true }) }
    });
    expect(hasMoon(container)).toBe(false);
  });
});

describe('SiteIndicators share badges', () => {
  it('marks a site that is shared through a public tunnel', () => {
    const { getByTitle } = render(SiteIndicators, {
      props: { site: site({ tunnel_url: 'https://abc.trycloudflare.com' }) }
    });
    expect(getByTitle('Shared publicly on https://abc.trycloudflare.com')).toBeInTheDocument();
  });

  it('marks a site that is shared on the local network', () => {
    const { getByTitle } = render(SiteIndicators, {
      props: { site: site({ lan_share_url: 'http://192.168.1.10:8080' }) }
    });
    expect(getByTitle('Shared on your network at http://192.168.1.10:8080')).toBeInTheDocument();
  });

  // The list has one row per site, so a share on a branch still has to show up
  // against the site it belongs to.
  it('counts a share on one of the site worktrees', () => {
    const { getByTitle } = render(SiteIndicators, {
      props: {
        site: site({
          worktrees: [{ branch: 'feat', tunnel_url: 'https://branch.serveo.net' }]
        } as Partial<Site>)
      }
    });
    expect(getByTitle('Shared publicly on https://branch.serveo.net')).toBeInTheDocument();
  });

  it('shows nothing for a site that is not shared', () => {
    const { queryByTitle } = render(SiteIndicators, { props: { site: site({}) } });
    expect(queryByTitle(/^Shared/)).not.toBeInTheDocument();
  });
});
