import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import Harness from './ServiceCardShell.test.svelte';

function root(container: HTMLElement): HTMLElement {
  return container.firstElementChild as HTMLElement;
}

describe('ServiceCardShell', () => {
  it('renders its children', () => {
    const { getByText } = render(Harness);
    expect(getByText('card body')).toBeInTheDocument();
  });

  it('draws the full card by default', () => {
    const { container } = render(Harness);
    expect(root(container).className).toContain('rounded-xl');
    expect(root(container).className).toContain('p-3');
    expect(root(container).className).toContain('hover:-translate-y-0.5');
  });

  it('draws a tighter card when compact', () => {
    const { container } = render(Harness, { props: { compact: true } });
    expect(root(container).className).toContain('rounded-lg');
    expect(root(container).className).toContain('p-2.5');
    expect(root(container).className).not.toContain('hover:-translate-y-0.5');
  });

  // Both variants stay a hover group so the icon can scale with the card.
  it('is a hover group in either variant', () => {
    expect(root(render(Harness).container).className).toContain('group');
    expect(root(render(Harness, { props: { compact: true } }).container).className).toContain('group');
  });
});
