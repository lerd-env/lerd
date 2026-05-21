import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import ProfilerToggle from './ProfilerToggle.svelte';
import { profilerEnabled } from '../stores/profiler';

// Mock fetch so the onMount loadProfilerStatus call resolves quietly and
// matches whatever state the test sets on the store.
function mockStatus(enabled: boolean) {
  globalThis.fetch = vi.fn(
    async () =>
      new Response(JSON.stringify({ enabled }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
  ) as unknown as typeof fetch;
}

describe('ProfilerToggle', () => {
  const realFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  beforeEach(() => {
    profilerEnabled.set(false);
  });

  it('reflects the off state with no pulsing dot', () => {
    mockStatus(false);
    const { container } = render(ProfilerToggle);
    expect(container.querySelector('button')!.getAttribute('title')).toMatch(/off/i);
    expect(container.querySelector('.lerd-pulse-ping')).toBeNull();
  });

  it('reflects the on state with a pulsing dot', () => {
    mockStatus(true);
    profilerEnabled.set(true);
    const { container } = render(ProfilerToggle);
    expect(container.querySelector('button')!.getAttribute('title')).toMatch(/on,/i);
    expect(container.querySelector('.lerd-pulse-ping')).not.toBeNull();
  });
});
