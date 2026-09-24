import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { sites, sitesLoaded } from '$stores/sites';
import { status } from '$stores/status';
import OnboardingPanel from './OnboardingPanel.svelte';

describe('OnboardingPanel', () => {
  it('welcomes an install with no sites', () => {
    sites.set([]);
    sitesLoaded.set(true);
    status.set({ streaming_mode: false } as never);
    const { container } = render(OnboardingPanel);
    expect(container.textContent).toContain('lerd park');
  });

  it('stays away when streaming mode is what emptied the list', () => {
    sites.set([]);
    sitesLoaded.set(true);
    status.set({ streaming_enabled: true, streaming_mode: true } as never);
    const { container } = render(OnboardingPanel);
    expect(container.textContent).not.toContain('lerd park');
  });
});
