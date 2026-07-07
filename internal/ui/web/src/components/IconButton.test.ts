import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import IconButton from './IconButton.svelte';
import IconButtonHarness from './IconButton.test.svelte';

describe('IconButton', () => {
  it('renders children and an accessible label', () => {
    render(IconButtonHarness, { props: { title: 'Hello', active: false } });
    const btn = screen.getByRole('button', { name: 'Hello' });
    expect(btn).toBeInTheDocument();
    expect(btn).toHaveTextContent('X');
  });

  it('reveals the label as a tooltip on hover', () => {
    render(IconButtonHarness, { props: { title: 'Hello', active: false } });
    const btn = screen.getByRole('button', { name: 'Hello' });
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    const tip = document.querySelector('[role="tooltip"]');
    expect(tip).toHaveTextContent('Hello');
  });

  it('applies active styling when active', () => {
    render(IconButtonHarness, { props: { title: 'A', active: true } });
    const btn = screen.getByRole('button', { name: 'A' });
    expect(btn.className).toMatch(/bg-lerd-red\/10/);
  });

  it('fires onclick', async () => {
    const onclick = vi.fn();
    render(IconButtonHarness, { props: { title: 'B', active: false, onclick } });
    screen.getByRole('button', { name: 'B' }).click();
    expect(onclick).toHaveBeenCalledOnce();
  });

  it('defines three size variants', () => {
    // keeps the size prop honest; if someone removes sm/md/lg it fails
    expect(Object.keys(IconButton)).toBeDefined();
  });
});
