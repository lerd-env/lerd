import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { modal } from '$stores/modals';
import { debugSearch } from '$stores/debugLens';
import { m } from '../paraglide/messages.js';

import { fakeLensAnswer } from '$lib/fakeLensApi';
import type { DumpEvent } from '$lib/dumpEvent';

// What lerd-ui holds; the lens reads it through the faked lens endpoints, and
// the site's browser-logs settings come from apiJson's mock.
const events: DumpEvent[] = [];
const apiJson = vi.fn();
const apiFetch = vi.fn();
vi.mock('$lib/api', async (orig) => ({
  ...(await orig<typeof import('$lib/api')>()),
  apiJson: (path: string, ...rest: unknown[]) =>
    path.startsWith('/api/dumps') ? Promise.resolve(fakeLensAnswer(events, path)) : apiJson(path, ...rest),
  apiFetch: (...args: unknown[]) => apiFetch(...args)
}));

import BrowserLens from './BrowserLens.svelte';

const setEvents = (evs: unknown[]) => events.splice(0, events.length, ...(evs as DumpEvent[]));
vi.stubGlobal(
  'EventSource',
  class {
    addEventListener() {}
    close() {}
  }
);

const settings = (enabled: boolean) => ({ enabled, console: ['error'], network: [], navigation: true, resources: false, events: [], presets: {} });

const browserEvent = {
  v: 1,
  id: 'e1',
  ts: new Date().toISOString(),
  kind: 'browser',
  label: 'error',
  ctx: { type: 'browser', site: 'shop', request: 'https://shop.test/', rid: 'p1' },
  src: {},
  data: { type: 'error', message: 'old-boom', url: 'https://shop.test/' }
};

describe('BrowserLens per-site opt-in', () => {
  beforeEach(() => {
    apiJson.mockReset();
    apiFetch.mockReset();
    setEvents([]);
  });

  it('says a site that has not opted in is off, pointing to the header toggle', async () => {
    apiJson.mockResolvedValue(settings(false));
    render(BrowserLens, { props: { siteScope: 'shop' } });

    expect(await screen.findByText(m.browser_disabled_body())).toBeTruthy();
    expect(screen.queryByPlaceholderText(m.debug_searchPlaceholder())).toBeNull();
    expect(apiFetch).not.toHaveBeenCalled();
  });

  it('turns capture on from the off notice', async () => {
    apiJson.mockResolvedValue(settings(false));
    apiFetch.mockResolvedValue({ ok: true, json: async () => settings(true) });
    render(BrowserLens, { props: { siteScope: 'shop' } });

    await fireEvent.click(await screen.findByRole('button', { name: m.debug_enable() }));
    const [path, init] = apiFetch.mock.calls[0];
    expect(path).toBe('/api/browser-logs/sites/shop');
    expect(JSON.parse(init.body)).toMatchObject({ enabled: true, console: ['error'] });
    await screen.findByText(m.browser_waiting_body());
  });

  it('still lists events captured before the site was turned off', async () => {
    apiJson.mockResolvedValue(settings(false));
    setEvents([browserEvent]);
    render(BrowserLens, { props: { siteScope: 'shop' } });

    expect(await screen.findByText('old-boom')).toBeTruthy();
    expect(screen.getByPlaceholderText(m.debug_searchPlaceholder())).toBeTruthy();
  });

  it('waits for a page load once the site is on', async () => {
    apiJson.mockResolvedValue(settings(true));
    render(BrowserLens, { props: { siteScope: 'shop' } });

    await screen.findByText(m.browser_waiting_body());
    expect(screen.queryByText(m.browser_disabled_body())).toBeNull();
  });

  it('lists what happened on a page without a row for the page load itself', async () => {
    apiJson.mockResolvedValue(settings(true));
    const load = { ...browserEvent, id: 'e0', label: 'navigation', data: { type: 'navigation', nav: 'load', message: 'https://shop.test/', url: 'https://shop.test/' } };
    setEvents([load, browserEvent]);
    render(BrowserLens, { props: { siteScope: 'shop' } });

    await screen.findByText('old-boom');
    expect(screen.queryByText('load')).toBeNull();
  });

  it('opens the site settings in the shared modal', async () => {
    apiJson.mockResolvedValue(settings(true));
    render(BrowserLens, { props: { siteScope: 'shop' } });

    await fireEvent.click(await screen.findByRole('button', { name: m.common_settings() }));
    expect(get(modal)).toMatchObject({ kind: 'browserLogs', browserLogsSite: 'shop' });
  });

  it('drops the toolbar inside one request, so a search left on the Debug tab hides nothing', async () => {
    apiJson.mockResolvedValue(settings(true));
    debugSearch.set('no-such-text');
    setEvents([browserEvent]);
    render(BrowserLens, { props: { siteScope: 'shop', pinned: true } });

    expect(await screen.findByText('old-boom')).toBeTruthy();
    expect(screen.queryByPlaceholderText(m.debug_searchPlaceholder())).toBeNull();
    expect(screen.queryByRole('button', { name: m.common_clear() })).toBeNull();
    // The dialog's title already names the request, so the group header does not.
    expect(screen.queryByText('p1')).toBeNull();
    debugSearch.set('');
  });
});

// The system Debug window has no request timeline, so an id clicked there
// becomes the lens search.
describe('BrowserLens across every site', () => {
  it('searches for a request id when it is clicked', async () => {
    apiJson.mockResolvedValue(settings(true));
    setEvents([browserEvent]);
    render(BrowserLens, { props: {} });

    await fireEvent.click(await screen.findByRole('button', { name: 'p1' }));
    expect((screen.getByPlaceholderText(m.debug_searchPlaceholder()) as HTMLInputElement).value).toBe('p1');
  });
});
