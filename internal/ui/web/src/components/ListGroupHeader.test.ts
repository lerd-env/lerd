import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ListGroupHeader from './ListGroupHeader.svelte';

describe('ListGroupHeader', () => {
  it('renders the label', () => {
    const { getByText } = render(ListGroupHeader, { props: { label: 'Databases' } });
    expect(getByText('Databases')).toBeTruthy();
  });

  it('draws the top divider by default', () => {
    const { container } = render(ListGroupHeader, { props: { label: 'Cache' } });
    expect(container.querySelector('div')!.className).toMatch(/border-t/);
  });

  it('omits the divider when divider=false', () => {
    const { container } = render(ListGroupHeader, { props: { label: 'Cache', divider: false } });
    expect(container.querySelector('div')!.className).not.toMatch(/border-t/);
  });
});
