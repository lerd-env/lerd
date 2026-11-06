import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import PresetModal from './PresetModal.svelte';
import { presets, presetsLoaded } from '$stores/presets';
import { serviceIcons } from '$stores/serviceIcons';
import { modal } from '$stores/modals';
import { get } from 'svelte/store';

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
    serviceIcons.set({});
  });

  it('lists every installable preset with no query', () => {
    render(PresetModal);
    expect(screen.getByText('redis')).toBeInTheDocument();
    expect(screen.getByText('postgres')).toBeInTheDocument();
  });

  it('is titled Add a service', () => {
    render(PresetModal);
    expect(screen.getByRole('heading', { name: 'Add a service' })).toBeTruthy();
  });

  it('offers an external database server beside the presets', async () => {
    render(PresetModal);
    expect(screen.getByText('External database server')).toBeTruthy();
    const row = screen.getByText('External database server').closest('.rounded-lg') as HTMLElement;
    await fireEvent.click(row.querySelector('button') as HTMLButtonElement);
    expect(get(modal).kind).toBe('externalDb');
    modal.set({ kind: null } as never);
  });

  it('keeps the external server when every preset is installed', () => {
    presets.set([]);
    render(PresetModal);
    expect(screen.getByText('External database server')).toBeTruthy();
  });

  it('finds the external server by search, and hides it otherwise', async () => {
    render(PresetModal);
    const search = screen.getByPlaceholderText('Search presets…');
    await fireEvent.input(search, { target: { value: 'dbngin' } });
    expect(screen.getByText('External database server')).toBeTruthy();
    await fireEvent.input(search, { target: { value: 'redis' } });
    expect(screen.queryByText('External database server')).toBeNull();
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

describe('PresetModal icons', () => {
  const MARK = '<svg viewBox="0 0 24 24"><path d="M3 3h18v18H3z"/></svg>';

  beforeEach(() => {
    globalThis.fetch = vi.fn(async () => {
      throw new Error('offline');
    }) as unknown as typeof fetch;
    presetsLoaded.set(true);
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
    presets.set([]);
    presetsLoaded.set(false);
    serviceIcons.set({});
  });

  it('leads each row with the mark the preset ships, in its declared colour', () => {
    serviceIcons.set({ redis: MARK });
    presets.set([{ name: 'redis', category: 'cache', icon: 'cache', color: '#ff4438' }]);
    const { container } = render(PresetModal);
    expect(container.querySelector('.mark-glyph path')?.getAttribute('d')).toBe('M3 3h18v18H3z');
    expect(container.querySelector('.mark-tint')?.getAttribute('style')).toContain(
      '--mark-tint: #ff4438'
    );
  });

  it('falls back to the declared glyph and category tint with no mark', () => {
    presets.set([{ name: 'beanstalkd', category: 'messaging', icon: 'queue' }]);
    const { container } = render(PresetModal);
    expect(container.querySelector('.mark-glyph')).toBeNull();
    expect(container.querySelector('.text-violet-600')).not.toBeNull();
  });
});
