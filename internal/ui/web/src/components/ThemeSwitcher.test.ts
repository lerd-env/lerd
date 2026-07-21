import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import ThemeSwitcher from './ThemeSwitcher.svelte';
import { theme } from '$stores/theme';

describe('ThemeSwitcher', () => {
  beforeEach(() => {
    theme.set('auto');
  });

  it('offers all three modes behind one icon', async () => {
    const { getByLabelText, getByText } = render(ThemeSwitcher);
    await fireEvent.click(getByLabelText(/auto/));
    for (const mode of ['light', 'dark', 'auto']) {
      expect(getByText(mode)).toBeInTheDocument();
    }
  });

  it('applies the chosen mode and closes', async () => {
    const { getByLabelText, getByText, queryByText } = render(ThemeSwitcher);
    await fireEvent.click(getByLabelText(/auto/));
    await fireEvent.click(getByText('dark'));

    expect(get(theme)).toBe('dark');
    expect(queryByText('light')).not.toBeInTheDocument();
  });

  it('names the resolved theme on the trigger while on auto', () => {
    const { getByLabelText } = render(ThemeSwitcher);
    expect(getByLabelText(/auto \((light|dark)\)/)).toBeInTheDocument();
  });
});
