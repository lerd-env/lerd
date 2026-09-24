import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { status } from '$stores/status';
import StreamingToggle from './StreamingToggle.svelte';

describe('StreamingToggle', () => {
  it('is absent while streaming mode is disabled', () => {
    status.set({ streaming_enabled: false } as never);
    const { queryByRole } = render(StreamingToggle);
    expect(queryByRole('button')).toBeNull();
  });

  it('shows once streaming mode is enabled', () => {
    status.set({ streaming_enabled: true, streaming_mode: false } as never);
    const { getByRole } = render(StreamingToggle);
    expect(getByRole('button', { pressed: false })).toBeTruthy();
  });
});
