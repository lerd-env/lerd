import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import PaletteSwitcher from './PaletteSwitcher.svelte';
import { palette, palettes } from '$stores/theme';
import { BUILTIN_PALETTES, resolvePalette } from '$lib/palettes';

describe('PaletteSwitcher', () => {
  beforeEach(() => {
    palette.set('lerd');
    palettes.set(BUILTIN_PALETTES);
  });

  it('offers every built-in theme', async () => {
    const { getByRole, getAllByText, getByText } = render(PaletteSwitcher);
    await fireEvent.click(getByRole('button'));

    // The chosen theme names the trigger as well as its own option.
    expect(getAllByText('lerd')).toHaveLength(2);
    expect(getByText('muted')).toBeInTheDocument();
  });

  it('applies the chosen theme', async () => {
    const { getByRole, getByText } = render(PaletteSwitcher);
    await fireEvent.click(getByRole('button'));
    await fireEvent.click(getByText('muted'));

    expect(get(palette)).toBe('muted');
  });

  it('names a user theme by its own name', async () => {
    palettes.set([
      ...BUILTIN_PALETTES,
      resolvePalette({ id: 'lagoon', name: 'Lagoon', accent: '#3b7ea1' })!
    ]);
    const { getByRole, getByText } = render(PaletteSwitcher);
    await fireEvent.click(getByRole('button'));

    expect(getByText('Lagoon')).toBeInTheDocument();
  });
});
