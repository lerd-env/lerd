import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';

const apiJson = vi.fn();
const apiFetch = vi.fn();
vi.mock('$lib/api', async (orig) => ({
  ...(await orig<typeof import('$lib/api')>()),
  apiJson: (...args: unknown[]) => apiJson(...args),
  apiFetch: (...args: unknown[]) => apiFetch(...args)
}));

import BrowserLogsSettings from './BrowserLogsSettings.svelte';

const settings = { enabled: true, console: ['error', 'warn'], network: [], navigation: true, resources: false, events: [], presets: {} };
const presets = [
  { name: 'inertia', label: 'Inertia.js', events: [{ event: 'inertia:invalid', label: '', message: '' }], detected: true, active: true },
  { name: 'htmx', label: 'htmx', events: [{ event: 'htmx:responseError', label: '', message: '' }], detected: false, active: false }
];

describe('BrowserLogsSettings presets', () => {
  beforeEach(() => {
    apiJson.mockReset();
    apiFetch.mockReset();
    apiJson.mockImplementation(async (path: string) => (path.includes('/presets') ? presets : settings));
  });

  it('lists only the presets the site uses, each with a checkbox', async () => {
    render(BrowserLogsSettings, { props: { site: 'shop' } });
    expect(await screen.findByRole('checkbox', { name: /Inertia\.js/ })).toBeChecked();
    expect(screen.queryByRole('checkbox', { name: /htmx/ })).toBeNull();
    expect(screen.queryByText(/Show \d+ more/)).toBeNull();
  });

  it('keeps an undetected preset the site switched on, so it can be switched off', async () => {
    apiJson.mockImplementation(async (path: string) => (path.includes('/presets') ? [presets[0], { ...presets[1], active: true }] : settings));
    render(BrowserLogsSettings, { props: { site: 'shop' } });
    expect(await screen.findByRole('checkbox', { name: /htmx/ })).toBeChecked();
  });

  it('switches a detected preset off', async () => {
    apiFetch.mockResolvedValue({ ok: true, json: async () => presets.map((p) => ({ ...p, active: false })) });
    render(BrowserLogsSettings, { props: { site: 'shop' } });
    await fireEvent.click(await screen.findByRole('checkbox', { name: /Inertia\.js/ }));
    await waitFor(() => expect(apiFetch).toHaveBeenCalled());
    const [path, init] = apiFetch.mock.calls[0];
    expect(path).toBe('/api/browser-logs/presets');
    expect(JSON.parse(init.body)).toEqual({ site: 'shop', name: 'inertia', on: false });
  });
});
