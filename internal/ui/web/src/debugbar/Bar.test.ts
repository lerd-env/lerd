import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi, afterEach } from 'vitest';
import Bar from './Bar.svelte';

vi.mock('$stores/requests', async (orig) => ({
  ...(await orig<object>()),
  loadRequest: () => Promise.resolve({ rid: 'r1', type: 'page', method: 'GET', uri: '/demo', status: 200, time_ms: 90, started: '', counts: {}, problems: [], events: { request: [{ data: {} }], query: [{}, {}] } })
}));

const config = { style: 'dock', edge: 'bottom', corner: 'bottom-right', theme: 'auto', palette: '', base: '/_lerd/browser/bar/', site: 'shop', path: '/srv/shop', roots: ['/srv/shop'], local: true, themes: [] };

describe('Bar', () => {
  afterEach(() => {
    localStorage.clear();
    vi.useRealTimers();
  });

  it('shows it is loading while the request is on the way', () => {
    localStorage.setItem('lerd:debugbar:open', '1');
    render(Bar, { props: { rid: '', config: { ...config, style: 'dock' as const, edge: 'bottom' as const, corner: 'bottom-right' as const, theme: 'auto' as const } } });
    expect(screen.getByRole('status')).toHaveTextContent('Loading');
  });

  it('shows the mark while the request is not in yet', () => {
    render(Bar, { props: { rid: '', config: { ...config, style: 'dock' as const, edge: 'bottom' as const, corner: 'bottom-right' as const, theme: 'auto' as const } } });
    expect(screen.getByRole('button', { name: /Show the debug bar/ })).toBeInTheDocument();
  });

  it('shows the request behind the page', async () => {
    localStorage.setItem('lerd:debugbar:open', '1');
    render(Bar, { props: { rid: 'r1', config: { ...config, style: 'dock' as const, edge: 'bottom' as const, corner: 'bottom-right' as const, theme: 'auto' as const } } });
    expect(await screen.findByText('/demo')).toBeInTheDocument();
  });
});
