import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import NavRail from './NavRail.svelte';
import { services } from '$stores/services';
import { accessMode } from '$stores/accessMode';
import { dashboardOpen } from '$stores/dashboard';

const PLUGINS = [
  { name: 'hello', page: { title: 'Hello', url: '/plugins/hello/ui/index.html', icon: '/plugins/hello/ui/icon.svg' } },
  { name: 'quiet', site_tab: { title: 'Quiet', url: '/plugins/quiet/ui/site.html' } }
];

beforeEach(() => {
  globalThis.fetch = (async (url: string) =>
    new Response(JSON.stringify(String(url).endsWith('/api/plugins') ? PLUGINS : {}), {
      status: 200
    })) as unknown as typeof fetch;
  accessMode.set({ localControl: true, local: true, lanExposed: false, checked: true });
  dashboardOpen.set(null);
  services.set([]);
});

// Only a plugin with a page earns a rail button, and it opens in the overlay as
// a plugin page rather than a service dashboard.
it('draws a launcher for each plugin page and opens it', async () => {
  const { findByLabelText, queryByLabelText } = render(NavRail);
  const button = await findByLabelText('Hello');
  expect(queryByLabelText('Quiet')).toBeNull();

  await fireEvent.click(button);
  await waitFor(() => expect(get(dashboardOpen)?.plugin).toBe('hello'));
  expect(get(dashboardOpen)?.dashboard).toBe('/plugins/hello/ui/index.html');
  expect(location.hash).toBe('#plugin/hello');
});
