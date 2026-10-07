import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { writable } from 'svelte/store';
import { m } from '../../paraglide/messages.js';

// The real streams open an EventSource; the tab only needs the event list.
vi.mock('$stores/dumps', async (orig) => ({
  ...(await orig<typeof import('$stores/dumps')>()),
  dumps: writable<unknown[]>([]),
  startDumpsStream: () => {},
  stopDumpsStream: () => {},
  clearDumps: () => {},
  refreshStatus: () => Promise.resolve()
}));
vi.mock('$stores/queries', async (orig) => ({
  ...(await orig<typeof import('$stores/queries')>()),
  debugCaptureEnabled: writable(true),
  refreshDevtoolsStatus: () => Promise.resolve()
}));
vi.mock('$lib/api', async (orig) => ({
  ...(await orig<typeof import('$lib/api')>()),
  apiJson: () => Promise.resolve({ enabled: true, console: [], network: [], navigation: true, resources: false, events: [], presets: {} }),
  apiFetch: () => Promise.resolve({ ok: true, json: async () => ({}) })
}));

import SiteDebugTab from './SiteDebugTab.svelte';
import { debugLens } from '$stores/debugLens';

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
