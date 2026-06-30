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
