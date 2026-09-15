import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';

import MobileBackBar from './MobileBackBar.svelte';
import { tab, routeRest } from '$stores/route';

describe('MobileBackBar', () => {
  beforeEach(() => {
    tab.set('sites');
    routeRest.set('');
  });

  // The bar names what you drilled into, not the route you took. The tab within
  // a site already has its own strip right below, so repeating it here only
  // leaked the hash format into the heading.
  it('names the site, not the route', () => {
    routeRest.set('shop.test/overview');
    const { getByText } = render(MobileBackBar);
    expect(getByText('shop.test')).toBeInTheDocument();
  });

  it('names the site for a nested route', () => {
    routeRest.set('shop.test/logs/queue');
    const { getByText } = render(MobileBackBar);
    expect(getByText('shop.test')).toBeInTheDocument();
  });

  it('names the service on the services tab', () => {
    tab.set('services');
    routeRest.set('mysql');
    const { getByText } = render(MobileBackBar);
    expect(getByText('mysql')).toBeInTheDocument();
  });

  it('falls back to the tab when nothing is selected', () => {
    tab.set('system');
    routeRest.set('');
    const { getByText } = render(MobileBackBar);
    expect(getByText('system')).toBeInTheDocument();
  });
});
