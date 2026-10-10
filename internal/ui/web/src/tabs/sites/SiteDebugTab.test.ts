import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { writable, get } from 'svelte/store';
import { fakeLensAnswer } from '$lib/fakeLensApi';
import type { DumpEvent } from '$lib/dumpEvent';
import { m } from '../../paraglide/messages.js';

// What lerd-ui holds, read through the faked lens endpoints.
const events: DumpEvent[] = [];
vi.stubGlobal(
  'EventSource',
  class {
    addEventListener() {}
    close() {}
  }
);
vi.mock('$stores/dumps', async (orig) => ({
  ...(await orig<typeof import('$stores/dumps')>()),
  clearDumps: () => Promise.resolve(),
  refreshStatus: () => Promise.resolve()
}));
vi.mock('$stores/queries', async (orig) => ({
  ...(await orig<typeof import('$stores/queries')>()),
  debugCaptureEnabled: writable(true),
  refreshDevtoolsStatus: () => Promise.resolve()
}));
vi.mock('$lib/api', async (orig) => ({
  ...(await orig<typeof import('$lib/api')>()),
  apiJson: (path: string) =>
    path.startsWith('/api/dumps?')
      ? Promise.resolve(events.filter((e) => path.includes(`rid=${e.ctx.rid}`)))
      : path.startsWith('/api/dumps')
        ? Promise.resolve(fakeLensAnswer(events, path))
        : Promise.resolve({ enabled: true, console: [], network: [], navigation: true, resources: false, events: [], presets: {} }),
  apiFetch: () => Promise.resolve({ ok: true, json: async () => ({}) })
}));

import SiteDebugTab from './SiteDebugTab.svelte';
import { debugLens, debugSearch } from '$stores/debugLens';

describe('SiteDebugTab full screen', () => {
  beforeEach(() => debugLens.set('browser'));

  it('fills the window from the lens bar, naming the site, and Escape leaves it', async () => {
    render(SiteDebugTab, { props: { siteName: 'shop', domain: 'shop.test' } });
    expect(screen.queryByText('shop.test')).toBeNull();

    await fireEvent.click(screen.getByRole('button', { name: m.debug_fullscreenTitle() }));
    expect(screen.getByText('shop.test')).toBeTruthy();
    expect(screen.getByRole('button', { name: m.debug_exitFullscreenTitle() })).toBeTruthy();

    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.getByRole('button', { name: m.debug_fullscreenTitle() })).toBeTruthy();
    expect(screen.queryByText('shop.test')).toBeNull();
  });
});

describe('SiteDebugTab pinned to one request', () => {
  it('draws no tab bar when the request recorded nothing beyond its timeline', async () => {
    const { container } = render(SiteDebugTab, { props: { siteName: 'shop', domain: 'shop.test', rid: 'r1' } });
    expect(container.querySelector('[data-tab-bar]')).toBeNull();
  });
});

describe('SiteDebugTab request id', () => {
  it('searches for a request id when it is clicked, which narrows the lenses and opens its timeline', async () => {
    debugLens.set('queries');
    events.splice(0, events.length, ...([
      { v: 1, id: 'e1', ts: new Date().toISOString(), kind: 'query', ctx: { type: 'fpm', site: 'shop', request: 'GET /', rid: 'r1' }, src: {}, data: { sql: 'select 1', time_ms: 1 } }
    ] as DumpEvent[]));
    render(SiteDebugTab, { props: { siteName: 'shop', domain: 'shop.test' } });

    await fireEvent.click(await screen.findByRole('button', { name: 'r1' }));
    expect(get(debugSearch)).toBe('r1');
    expect(screen.getByRole('tab', { name: m.debug_tab_timeline() }).getAttribute('aria-selected')).toBe('true');
    debugSearch.set('');
  });
});
