import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import PresetModal from './PresetModal.svelte';
import { presets, presetsLoaded } from '$stores/presets';

// onMount runs loadPresets() which hits /api/services/presets; make it fail so
// the seeded store is preserved and the tests drive a known preset list.
const realFetch = globalThis.fetch;

describe('PresetModal search', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn(async () => {
      throw new Error('offline');
    }) as unknown as typeof fetch;
    presets.set([
      { name: 'redis', description: 'In-memory cache' } as never,
      { name: 'postgres', description: 'Relational database' } as never
    ]);
    presetsLoaded.set(true);
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
    presets.set([]);
    presetsLoaded.set(false);
  });

  it('lists every installable preset with no query', () => {
    render(PresetModal);
    expect(screen.getByText('redis')).toBeInTheDocument();
    expect(screen.getByText('postgres')).toBeInTheDocument();
  });

  it('filters the list as you type', async () => {
    render(PresetModal);
    const input = screen.getByPlaceholderText('Search presets…');
    await fireEvent.input(input, { target: { value: 'cache' } });
    expect(screen.getByText('redis')).toBeInTheDocument();
    expect(screen.queryByText('postgres')).not.toBeInTheDocument();
  });

  it('shows an empty state when nothing matches', async () => {
    render(PresetModal);
    const input = screen.getByPlaceholderText('Search presets…');
    await fireEvent.input(input, { target: { value: 'nothing-here' } });
    expect(screen.getByText('No presets match your search.')).toBeInTheDocument();
  });
});
