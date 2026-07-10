import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import ServiceIcon from './ServiceIcon.svelte';
import { presets } from '$stores/presets';
import { services } from '$stores/services';

function box(container: HTMLElement): HTMLElement {
  return container.firstElementChild as HTMLElement;
}

beforeEach(() => {
  presets.set([{ name: 'mysql', category: 'databases', icon: 'database' }]);
  services.set([]);
});

describe('ServiceIcon', () => {
  // A caller with only a name resolves the declared metadata through the registry.
  it('tints from the metadata the named preset declares', () => {
    const { container } = render(ServiceIcon, { props: { name: 'mysql' } });
    expect(box(container).className).toContain('indigo');
  });

  it('tints as other when nothing declares the name', () => {
    const { container } = render(ServiceIcon, { props: { name: 'totally-unknown' } });
    expect(box(container).className).toContain('gray');
  });

  it('prefers an explicit category over the registry', () => {
    const { container } = render(ServiceIcon, { props: { name: 'mysql', category: 'cache' } });
    expect(box(container).className).toContain('amber');
    expect(box(container).className).not.toContain('indigo');
  });

  it('draws a w-9 box that scales with its card by default', () => {
    const { container } = render(ServiceIcon, { props: { name: 'redis' } });
    expect(box(container).className).toContain('w-9');
    expect(box(container).className).toContain('group-hover:scale-105');
    expect(container.querySelector('svg')?.getAttribute('class')).toContain('w-5');
  });

  it('draws a w-8 box that holds still when compact', () => {
    const { container } = render(ServiceIcon, { props: { name: 'redis', compact: true } });
    expect(box(container).className).toContain('w-8');
    expect(box(container).className).not.toContain('group-hover:scale-105');
    expect(container.querySelector('svg')?.getAttribute('class')).toContain('w-4');
  });

  it('renders the service glyph', () => {
    const { container } = render(ServiceIcon, { props: { name: 'mysql' } });
    expect(container.querySelector('svg')?.innerHTML.length).toBeGreaterThan(0);
  });
});
