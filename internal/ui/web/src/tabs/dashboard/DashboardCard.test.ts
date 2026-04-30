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
});
