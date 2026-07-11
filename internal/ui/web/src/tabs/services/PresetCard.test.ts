import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import PresetCard from './PresetCard.svelte';
import type { Preset } from '$stores/presets';

function preset(over: Partial<Preset> = {}): Preset {
  return { name: 'postgres', ...over } as Preset;
}

describe('PresetCard', () => {
  it('renders the human label and the description blurb', () => {
    const { getByText } = render(PresetCard, {
      props: { preset: preset({ description: 'Relational database with extensions' }) }
    });
    expect(getByText('PostgreSQL')).toBeTruthy();
    expect(getByText('Relational database with extensions')).toBeTruthy();
  });

  it('renders a service glyph icon', () => {
    const { container } = render(PresetCard, { props: { preset: preset() } });
    expect(container.querySelector('svg')).toBeTruthy();
  });

  it('disables the Add button and shows a spinner while installing', () => {
    const { container } = render(PresetCard, { props: { preset: preset({ installing: true }) } });
    const btn = container.querySelector('button')!;
    expect(btn.disabled).toBe(true);
    expect(container.querySelector('.animate-spin')).toBeTruthy();
  });

  it('shows the install error when present', () => {
    const { getByText } = render(PresetCard, { props: { preset: preset({ error: 'boom' }) } });
    expect(getByText('boom')).toBeTruthy();
  });

  it('tints the icon by the given category', () => {
    const { container } = render(PresetCard, {
      props: { preset: preset(), category: 'databases' }
    });
    expect(container.innerHTML).toMatch(/text-indigo-600/);
  });

  it('uses the category the preset declares when none is passed', () => {
    const { container } = render(PresetCard, {
      props: { preset: preset({ name: 'redis', category: 'cache' }) }
    });
    expect(container.innerHTML).toMatch(/text-amber-600/);
  });
});
