import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import NginxDetail from './NginxDetail.svelte';
import { status } from '$stores/status';
import { accessMode } from '$stores/accessMode';

// While lerd's own web server serves, nginx is stopped and removed, so the
// panel names neither it nor its image and offers none of its controls.
describe('NginxDetail', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    vi.stubGlobal('EventSource', vi.fn(() => ({ close: vi.fn(), addEventListener: vi.fn() })));
    accessMode.set({ ...get(accessMode), localControl: true });
  });

  it('names nothing about nginx while lerd serves', () => {
    status.update((s) => ({ ...s, web_server: 'go', nginx: { running: true, update_available: true } }));
    const { container } = render(NginxDetail);
    expect(screen.getByText('Web server')).toBeInTheDocument();
    expect(container.textContent?.toLowerCase()).not.toContain('nginx');
    expect(screen.queryByRole('button', { name: /update/i })).toBeNull();
  });

  it('keeps nginx and its controls while nginx serves', () => {
    status.update((s) => ({ ...s, web_server: 'nginx', nginx: { running: true, update_available: true } }));
    render(NginxDetail);
    expect(screen.getByText('Nginx')).toBeInTheDocument();
  });
});
