import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api', () => ({ apiJson: vi.fn() }));
import { apiJson } from '$lib/api';
import { loadStats, stats, statsLoaded } from './stats';

describe('loadStats', () => {
  beforeEach(() => {
    statsLoaded.set(false);
    vi.mocked(apiJson).mockReset();
  });
  afterEach(() => vi.restoreAllMocks());

  it('marks the fetch as attempted on success', async () => {
    vi.mocked(apiJson).mockResolvedValue({ containers: [], available: true });
    await loadStats();
    expect(get(statsLoaded)).toBe(true);
    expect(get(stats).available).toBe(true);
  });

  // A failed fetch used to leave statsLoaded false forever, so the widget sat on
  // its loading message claiming to be reading stats it had given up on.
  it('marks the fetch as attempted on failure', async () => {
    vi.mocked(apiJson).mockRejectedValue(new Error('boom'));
    await loadStats();
    expect(get(statsLoaded)).toBe(true);
  });
});
