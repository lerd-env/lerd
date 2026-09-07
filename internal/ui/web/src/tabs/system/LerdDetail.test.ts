import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import LerdDetail from './LerdDetail.svelte';
import { version } from '$stores/version';
import { modal } from '$stores/modals';

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
});
