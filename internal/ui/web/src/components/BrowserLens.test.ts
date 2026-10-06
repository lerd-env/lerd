import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { writable, get, type Writable } from 'svelte/store';
import { modal } from '$stores/modals';
import { m } from '../paraglide/messages.js';

// The real stream opens an EventSource; the lens only needs the event list.
vi.mock('$stores/dumps', async (orig) => ({
  ...(await orig<typeof import('$stores/dumps')>()),
  dumps: writable<unknown[]>([]),
  startDumpsStream: () => {},
  stopDumpsStream: () => {},
  clearDumps: () => {}
}));

const apiJson = vi.fn();
const apiFetch = vi.fn();
vi.mock('$lib/api', async (orig) => ({
  ...(await orig<typeof import('$lib/api')>()),
  apiJson: (...args: unknown[]) => apiJson(...args),
  apiFetch: (...args: unknown[]) => apiFetch(...args)
}));

import BrowserLens from './BrowserLens.svelte';
import { dumps as dumpsStore } from '$stores/dumps';

const dumps = dumpsStore as unknown as Writable<unknown[]>;

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
    dumps.set([]);
  });

  it('says a site that has not opted in is off, pointing to the header toggle', async () => {
    apiJson.mockResolvedValue(settings(false));
    render(BrowserLens, { props: { siteScope: 'shop' } });

    expect(await screen.findByText(m.browser_disabled_body())).toBeTruthy();
    expect(apiFetch).not.toHaveBeenCalled();
  });

  it('still lists events captured before the site was turned off', async () => {
    apiJson.mockResolvedValue(settings(false));
    dumps.set([browserEvent]);
    render(BrowserLens, { props: { siteScope: 'shop' } });

    expect(await screen.findByText('old-boom')).toBeTruthy();
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
    dumps.set([load, browserEvent]);
    render(BrowserLens, { props: { siteScope: 'shop' } });

    await screen.findByText('old-boom');
    expect(screen.queryByText('load')).toBeNull();
  });

  it('opens the site settings in the shared modal', async () => {
    apiJson.mockResolvedValue(settings(true));
    render(BrowserLens, { props: { siteScope: 'shop' } });

    await fireEvent.click(await screen.findByRole('button', { name: m.common_settings() }));
    expect(get(modal)).toMatchObject({ kind: 'browserCapture', browserCaptureSite: 'shop' });
  });
});

