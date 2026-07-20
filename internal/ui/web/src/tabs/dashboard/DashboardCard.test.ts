import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import Harness from './DashboardCard.test.svelte';

describe('DashboardCard', () => {
  it('renders title and body', () => {
    render(Harness, { props: { title: 'Sites' } });
    expect(screen.getByText('Sites')).toBeInTheDocument();
    expect(screen.getByText('body content')).toBeInTheDocument();
  });

  it('omits badge slot when not provided', () => {
    render(Harness, { props: { title: 'Sites' } });
    expect(screen.queryByTestId('badge')).not.toBeInTheDocument();
  });

  it('renders badge snippet when provided', () => {
    render(Harness, { props: { title: 'Sites', withBadge: true } });
    expect(screen.getByTestId('badge')).toBeInTheDocument();
  });

  it('renders footer snippet when provided', () => {
    render(Harness, { props: { title: 'Sites', withFooter: true } });
    expect(screen.getByTestId('footer')).toBeInTheDocument();
  });

  it('renders all three slots together', () => {
    render(Harness, { props: { title: 'Sites', withBadge: true, withFooter: true } });
    expect(screen.getByText('Sites')).toBeInTheDocument();
    expect(screen.getByTestId('badge')).toBeInTheDocument();
    expect(screen.getByText('body content')).toBeInTheDocument();
    expect(screen.getByTestId('footer')).toBeInTheDocument();
  });

  it('applies critical tone accent', () => {
    const { container } = render(Harness, { props: { title: 'Workers', tone: 'critical' } });
    const root = container.querySelector('div');
    expect(root!.className).toMatch(/border-l-4/);
    expect(root!.className).toMatch(/border-l-red-500/);
  });

  it('applies warn tone accent', () => {
    const { container } = render(Harness, { props: { title: 'Lerd', tone: 'warn' } });
    const root = container.querySelector('div');
    expect(root!.className).toMatch(/border-l-4/);
    expect(root!.className).toMatch(/border-l-yellow-500/);
  });

  it('omits accent in default tone', () => {
    const { container } = render(Harness, { props: { title: 'Sites' } });
    const root = container.querySelector('div');
    expect(root!.className).not.toMatch(/border-l-4/);
  });

  it('caps height on narrow layouts but stretches to fill the row at xl', () => {
    const { container } = render(Harness, { props: { title: 'Sites' } });
    const root = container.querySelector('div');
    // Narrow layouts stack into many rows, so the cap stays; at xl the card
    // fills its share instead of leaving dead space.
    expect(root!.className).toMatch(/max-h-\[340px\]/);
    expect(root!.className).toMatch(/xl:max-h-none/);
  });

  it('keeps a height floor below xl and drops it where it stretches', () => {
    const { container } = render(Harness, { props: { title: 'Sites' } });
    const root = container.querySelector('div');
    // Narrow layouts stack and scroll, so a floor keeps each card readable.
    // At xl the floor would push the rows past a short viewport and clip
    // them, so the card shrinks with its row and its body scrolls instead.
    expect(root!.className).toMatch(/min-h-\[280px\]/);
    expect(root!.className).toMatch(/xl:min-h-0/);
  });
});
