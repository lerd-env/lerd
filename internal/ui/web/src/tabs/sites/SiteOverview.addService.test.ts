import { render, fireEvent } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import SiteOverview from './SiteOverview.svelte';
import { modal } from '$stores/modals';
import { services, type Service } from '$stores/services';
import type { Site } from '$stores/sites';

globalThis.fetch = vi.fn(async () => new Response('{}')) as unknown as typeof fetch;

function overview(used: string[], installed: string[], extra: Partial<Service> = {}) {
  services.set(installed.map((name) => ({ name, status: 'active', site_count: 0, ...extra })));
  const site = { name: 'shop', domain: 'shop.test', path: '/srv/shop', services: used } as unknown as Site;
  return render(SiteOverview, { props: { site } });
}

describe('adding a service from the site overview', () => {
  afterEach(() => {
    services.set([]);
    modal.set({ kind: null });
  });

  it('opens the picker for a site that uses no services yet', async () => {
    const { getByText } = overview([], ['mailpit']);
    await fireEvent.click(getByText('Add service'));
    expect(get(modal)).toEqual({ kind: 'siteServiceLink', siteServiceLink: { domain: 'shop.test' } });
  });

  it('offers nothing when the site uses every installed service', () => {
    const { getByText, queryByText } = overview(['mailpit'], ['mailpit']);
    expect(getByText('Services')).toBeTruthy();
    expect(queryByText('Add service')).toBeNull();
  });

  it('hides the section when there is nothing to show or add, workers not counting', () => {
    const { queryByText } = overview([], ['queue-shop'], { queue_site: 'shop' });
    expect(queryByText('Services')).toBeNull();
  });
});
