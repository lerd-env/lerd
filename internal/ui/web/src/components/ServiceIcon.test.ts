import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ServiceIcon from './ServiceIcon.svelte';

function box(container: HTMLElement): HTMLElement {
  return container.firstElementChild as HTMLElement;
}

describe('ServiceIcon', () => {
  it('tints from the service name', () => {
    const { container } = render(ServiceIcon, { props: { name: 'mysql' } });
    expect(box(container).className).toContain('indigo');
  });

  it('prefers an explicit category over the name', () => {
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
