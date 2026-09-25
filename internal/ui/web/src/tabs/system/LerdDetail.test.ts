import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import LerdDetail from './LerdDetail.svelte';
import { version } from '$stores/version';
import { modal } from '$stores/modals';
import { status } from '$stores/status';
import { accessMode } from '$stores/accessMode';

const notes = 'v1.34.3\n- a change worth reading\n- another one';

describe('LerdDetail', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    modal.set({ kind: null });
    version.set({
      current: '1.34.2',
      latest: '1.34.3',
      hasUpdate: true,
      checked: true,
      checking: false,
      changelog: notes
    });
  });

  // Release notes run long enough to push the rest of the settings off screen,
  // so the card only offers a way in and the text lives in the modal.
  it('marks a dev build next to its version', () => {
    version.update((v) => ({ ...v, current: '1.35.0-66-g7ef9e61e-dirty' }));
    render(LerdDetail);

    expect(screen.getByRole('button', { name: /7ef9e61e/ }).textContent?.trim()).toBe('dev');
    expect(screen.getByText('v1.35.0')).toBeInTheDocument();
    expect(screen.queryByText(/g7ef9e61e/)).toBeNull();
  });

  it('marks a beta next to its version', () => {
    version.update((v) => ({ ...v, current: '1.35.0-beta.4' }));
    render(LerdDetail);

    expect(screen.getByText('beta')).toBeInTheDocument();
    expect(screen.getByText('v1.35.0')).toBeInTheDocument();
  });

  it('adds no badge to a release', () => {
    render(LerdDetail);

    expect(screen.queryByText('dev')).toBeNull();
    expect(screen.queryByText('beta')).toBeNull();
  });

  it('keeps the release notes out of the card', () => {
    render(LerdDetail);

    expect(screen.queryByText(/a change worth reading/)).toBeNull();
    expect(screen.getByRole('button', { name: /what's new/i })).toBeInTheDocument();
  });

  it('opens the changelog modal', async () => {
    render(LerdDetail);
    await fireEvent.click(screen.getByRole('button', { name: /what's new/i }));

    expect(get(modal).kind).toBe('changelog');
  });

  it('offers no way in without notes', () => {
    version.update((v) => ({ ...v, changelog: '' }));
    render(LerdDetail);

    expect(screen.queryByRole('button', { name: /what's new/i })).toBeNull();
  });

  it('prefills the theme share posts for X and Bluesky', () => {
    render(LerdDetail);

    const x = screen.getByRole('link', { name: 'X' }) as HTMLAnchorElement;
    const bluesky = screen.getByRole('link', { name: 'Bluesky' }) as HTMLAnchorElement;

    expect(x.href).toContain('x.com/intent/post?text=');
    expect(decodeURIComponent(x.href)).toContain('@lerdphp');
    expect(bluesky.href).toContain('bsky.app/intent/compose?text=');
    expect(decodeURIComponent(bluesky.href)).toContain('@lerdphp.bsky.social');

    const reddit = screen.getByRole('link', { name: 'Reddit' }) as HTMLAnchorElement;
    expect(reddit.href).toContain('reddit.com/r/lerd/submit?title=');
  });

  it('enables streaming mode from its card', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ json: async () => ({ ok: true }) });
    vi.stubGlobal('fetch', fetchMock);
    accessMode.update((a) => ({ ...a, localControl: true }));
    status.update((st) => ({ ...st, streaming_enabled: false }));
    render(LerdDetail);

    await fireEvent.click(screen.getByTitle('Enable streaming mode'));
    const call = fetchMock.mock.calls.find(([url]) => String(url).includes('/api/settings/streaming-enabled'));
    expect(call).toBeTruthy();
    expect(JSON.parse(call![1].body)).toEqual({ enabled: true });
  });
});
