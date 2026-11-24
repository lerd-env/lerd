import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import SiteTile from './SiteTile.svelte';
import type { Site } from '$stores/sites';

function site(over: Partial<Site> = {}): Site {
  return { domain: 'app.test', ...over } as Site;
}

describe('SiteTile', () => {
  it('renders the site domain', () => {
    const { getByText } = render(SiteTile, { props: { site: site() } });
    expect(getByText('app.test')).toBeTruthy();
  });

  it('builds a subtitle from framework and php version', () => {
    const { getByText } = render(SiteTile, {
      props: { site: site({ framework_label: 'Laravel', php_version: '8.3' }) }
    });
    expect(getByText('Laravel · PHP 8.3')).toBeTruthy();
  });

  it('falls back to the node version when there is no php', () => {
    const { getByText } = render(SiteTile, {
      props: { site: site({ framework_label: 'Next.js', node_version: '20' }) }
    });
    expect(getByText('Next.js · Node 20')).toBeTruthy();
  });

  it('uses the Laravel app_name as the title and drops the domain into the subline', () => {
    const { getByText } = render(SiteTile, {
      props: { site: site({ app_name: 'My Shop', framework_label: 'Laravel', php_version: '8.3' }) }
    });
    expect(getByText('My Shop')).toBeTruthy();
    expect(getByText('app.test · Laravel · PHP 8.3')).toBeTruthy();
  });

  it('offers an open-in-browser button for active sites', () => {
    const { getByLabelText } = render(SiteTile, { props: { site: site() } });
    expect(getByLabelText('Open in browser')).toBeTruthy();
  });

  it('hides the open-in-browser button and dims the tile when paused', () => {
    const { container, queryByLabelText } = render(SiteTile, {
      props: { site: site({ paused: true }) }
    });
    expect(queryByLabelText('Open in browser')).toBeNull();
    expect(container.querySelector('.opacity-60')).toBeTruthy();
  });

  // A stopped site's icon is often its favicon or framework mark, which never
  // changes, so the subline says whether PHP is serving it, as a service does.
  it('leads the subline with Running when PHP is serving the site', () => {
    const { getByText, container } = render(SiteTile, {
      props: { site: site({ fpm_running: true, framework_label: 'Laravel', php_version: '8.5' }) }
    });
    expect(getByText('Running')).toBeTruthy();
    expect(getByText('Laravel · PHP 8.5')).toBeTruthy();
    expect(container.querySelector('.bg-emerald-500')).toBeTruthy();
  });

  it('says Stopped with a grey dot when PHP is not serving the site', () => {
    const { getByText, queryByText, container } = render(SiteTile, {
      props: { site: site({ fpm_running: false, php_version: '8.4' }) }
    });
    expect(getByText('Stopped')).toBeTruthy();
    expect(queryByText('Running')).toBeNull();
    expect(container.querySelector('.bg-gray-300')).toBeTruthy();
  });

  // Paused sites already sit in their own section with a count of their own.
  it('shows no running state on a paused tile', () => {
    const { queryByText } = render(SiteTile, {
      props: { site: site({ paused: true, php_version: '8.5' }) }
    });
    expect(queryByText('Running')).toBeNull();
    expect(queryByText('Stopped')).toBeNull();
  });

  // The dashboard widget renders the same tile in a much narrower column, so
  // it asks for a tighter card rather than restyling the tile.
  it('tightens the card and the icon box when compact', () => {
    const { container } = render(SiteTile, { props: { site: site(), compact: true } });
    const card = container.firstElementChild as HTMLElement;
    expect(card.className).toContain('rounded-lg');
    expect(card.className).toContain('p-2.5');
    expect(container.querySelector('.w-8.h-8')).toBeTruthy();
  });

  it('keeps the roomier card by default', () => {
    const { container } = render(SiteTile, { props: { site: site() } });
    const card = container.firstElementChild as HTMLElement;
    expect(card.className).toContain('rounded-xl');
    expect(card.className).toContain('p-3');
    expect(container.querySelector('.w-9.h-9')).toBeTruthy();
  });

  it('shows a running worker dot when a worker is active', () => {
    const { container } = render(SiteTile, { props: { site: site({ queue_running: true }) } });
    expect(container.querySelector('.bg-amber-400')).toBeTruthy();
  });

  it('shows a pulsing red dot when a worker is failing', () => {
    const { container } = render(SiteTile, { props: { site: site({ queue_failing: true }) } });
    expect(container.querySelector('.bg-red-500')).toBeTruthy();
  });
});
