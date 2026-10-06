import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { m } from '../paraglide/messages.js';

const apiJson = vi.fn();
const apiFetch = vi.fn();
vi.mock('$lib/api', async (orig) => ({
  ...(await orig<typeof import('$lib/api')>()),
  apiJson: (...args: unknown[]) => apiJson(...args),
  apiFetch: (...args: unknown[]) => apiFetch(...args)
}));

import BrowserCaptureSiteToggle from './BrowserCaptureSiteToggle.svelte';

const settings = (enabled: boolean) => ({ enabled, console: ['error', 'warn'], network: [], navigation: true, resources: false, events: [], presets: {} });

describe('BrowserCaptureSiteToggle', () => {
  beforeEach(() => {
    apiJson.mockReset();
    apiFetch.mockReset();
  });

  it('shows the site is on and turns it off', async () => {
    apiJson.mockResolvedValue(settings(true));
    apiFetch.mockResolvedValue({ ok: true, json: async () => settings(false) });
    render(BrowserCaptureSiteToggle, { props: { site: 'shop' } });

    const button = await screen.findByRole('button', { name: m.debug_tab_browser() });
    await waitFor(() => expect(button.getAttribute('aria-pressed')).toBe('true'));
    await fireEvent.click(button);

    await waitFor(() => expect(apiFetch).toHaveBeenCalled());
    const [path, init] = apiFetch.mock.calls[0];
    expect(path).toBe('/api/browser-capture/sites/shop');
    expect(JSON.parse(init.body).enabled).toBe(false);
    await waitFor(() => expect(button.getAttribute('aria-pressed')).toBe('false'));
  });
});
