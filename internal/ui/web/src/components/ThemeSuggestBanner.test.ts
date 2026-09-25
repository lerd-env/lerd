import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import ThemeSuggestBanner from './ThemeSuggestBanner.svelte';
import { configTheme } from '$stores/palettes';
import { palette, palettes } from '$stores/theme';
import { BUILTIN_PALETTES } from '$lib/palettes';

const adwaita = { ...BUILTIN_PALETTES.find((p) => p.id === 'adwaita')!, source: 'desktop' as const };

describe('ThemeSuggestBanner', () => {
  beforeEach(() => {
    palettes.set([...BUILTIN_PALETTES.filter((p) => p.id !== 'adwaita'), adwaita]);
    palette.set('lerd');
    configTheme.set('');
  });

  it('names the desktop theme it suggests', () => {
    const { container } = render(ThemeSuggestBanner);
    expect(container.textContent).toContain('Adwaita');
  });

  it('stays quiet until the daemon has said whether a theme was chosen', () => {
    configTheme.set(null);
    const { container } = render(ThemeSuggestBanner);
    expect(container.querySelector('button')).toBeNull();
  });

  it('stays quiet where there is no desktop to follow', () => {
    palettes.set(BUILTIN_PALETTES);
    const { container } = render(ThemeSuggestBanner);
    expect(container.querySelector('button')).toBeNull();
  });
});
