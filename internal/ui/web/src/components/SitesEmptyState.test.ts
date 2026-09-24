import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { status } from '$stores/status';
import SitesEmptyState from './SitesEmptyState.svelte';

describe('SitesEmptyState', () => {
  it('points at lerd park when there are no sites', () => {
    status.set({ streaming_enabled: true, streaming_mode: false } as never);
    const { container } = render(SitesEmptyState);
    expect(container.textContent).toContain('lerd park');
  });

  // Every site can sit in a private workspace, and then the list is empty only
  // because streaming mode hides it, with no workspace left to unhide from.
  it('offers the way back when streaming mode hides every site', () => {
    status.set({ streaming_enabled: true, streaming_mode: true } as never);
    const { container, getByRole } = render(SitesEmptyState);
    expect(container.textContent).not.toContain('lerd park');
    expect(getByRole('button', { pressed: true })).toBeTruthy();
  });
});
