import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, afterEach } from 'vitest';
import SiteLogs from './SiteLogs.svelte';
import { routeRest } from '$stores/route';
import type { Site } from '$stores/sites';

function site(over: Partial<Site> = {}): Site {
  return { domain: 'app.test', name: 'app', uses_php: true, ...over } as Site;
}

describe('SiteLogs tabs', () => {
  afterEach(() => routeRest.set(''));

  it('keeps a stopped worker tab so its logs stay readable', () => {
    const { getByText } = render(SiteLogs, {
      props: {
        site: site({
          has_queue_worker: true,
          queue_running: false,
          framework_workers: [{ name: 'vite', label: 'Vite', running: false }]
        })
      }
    });
    expect(getByText('Queue')).toBeTruthy();
    expect(getByText('Vite')).toBeTruthy();
  });

  it('marks a failing worker so it stands out from a stopped one', () => {
    const { getByText } = render(SiteLogs, {
      props: {
        site: site({
          has_queue_worker: true,
          queue_failing: true,
          framework_workers: [{ name: 'vite', label: 'Vite', running: false }]
        })
      }
    });
    expect(getByText('Queue !')).toBeTruthy();
  });

  it('offers no worker tab for a site that has none', () => {
    const { queryByText } = render(SiteLogs, { props: { site: site() } });
    expect(queryByText('Queue')).toBeNull();
    expect(queryByText('Vite')).toBeNull();
  });
});

describe('SiteLogs source deep link', () => {
  afterEach(() => routeRest.set(''));

  it('opens the source the route names, so a worker toggle can link at it', () => {
    routeRest.set('app.test/logs/worker:vite');
    const { getByText } = render(SiteLogs, {
      props: {
        site: site({
          has_queue_worker: true,
          framework_workers: [{ name: 'vite', label: 'Vite', running: true }]
        })
      }
    });
    expect(getByText('Vite').className).toContain('text-lerd-red');
    expect(getByText('Queue').className).not.toContain('text-lerd-red');
  });

  it('mirrors a tab click into the hash so the link stays live', async () => {
    routeRest.set('app.test/logs/worker:vite');
    const { getByText } = render(SiteLogs, {
      props: {
        site: site({
          has_queue_worker: true,
          framework_workers: [{ name: 'vite', label: 'Vite', running: true }]
        })
      }
    });
    await fireEvent.click(getByText('Queue'));
    expect(location.hash).toBe('#sites/app.test/logs/queue');
  });
});
