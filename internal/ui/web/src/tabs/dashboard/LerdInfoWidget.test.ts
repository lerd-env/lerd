import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import LerdInfoWidget from './LerdInfoWidget.svelte';
import { version } from '$stores/version';
import { modal } from '$stores/modals';

const notes = 'v1.34.3\n- a change worth reading\n- another one';

describe('LerdInfoWidget', () => {
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

  it('marks a dev build next to its version', () => {
    version.update((v) => ({ ...v, current: '1.35.0-66-g7ef9e61e-dirty' }));
    render(LerdInfoWidget);

    expect(screen.getByRole('button', { name: /7ef9e61e/ }).textContent?.trim()).toBe('dev');
    expect(screen.getByText('v1.35.0')).toBeInTheDocument();
    expect(screen.queryByText(/g7ef9e61e/)).toBeNull();
  });

  // Expanding the notes in place pushed the rest of the dashboard down, so the
  // card only offers a way in and the text lives in the modal.
  it('keeps the release notes out of the card', async () => {
    render(LerdInfoWidget);
    await fireEvent.click(screen.getByRole('button', { name: /what's new/i }));

    expect(screen.queryByText(/a change worth reading/)).toBeNull();
    expect(get(modal).kind).toBe('changelog');
  });

  it('offers no way in without notes', () => {
    version.update((v) => ({ ...v, changelog: '' }));
    render(LerdInfoWidget);

    expect(screen.queryByRole('button', { name: /what's new/i })).toBeNull();
  });
});
