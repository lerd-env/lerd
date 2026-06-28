import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import StatusPill from './StatusPill.svelte';

describe('StatusPill', () => {
  it('renders label', () => {
    render(StatusPill, { props: { tone: 'ok', label: 'Running' } });
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders a static span (no button) without an onclick', () => {
    const { container } = render(StatusPill, { props: { tone: 'ok', label: '3306' } });
    expect(container.querySelector('button')).toBeNull();
  });

  it('renders an interactive button with title and fires onclick when given one', async () => {
    const onclick = vi.fn();
    render(StatusPill, { props: { tone: 'ok', label: '3307', title: 'reachable at 127.0.0.1:3307', onclick } });
    const btn = screen.getByRole('button', { name: 'reachable at 127.0.0.1:3307' });
    expect(btn).toHaveAttribute('title', 'reachable at 127.0.0.1:3307');
    await fireEvent.click(btn);
    expect(onclick).toHaveBeenCalledOnce();
  });

  it('applies error tone classes', () => {
    const { container } = render(StatusPill, { props: { tone: 'error', label: 'Down' } });
    expect(container.querySelector('span')!.className).toMatch(/text-red-600|bg-red-100/);
  });

  it('applies warn tone classes', () => {
    const { container } = render(StatusPill, { props: { tone: 'warn', label: 'Stale' } });
    expect(container.querySelector('span')!.className).toMatch(/text-yellow-700|bg-yellow-100/);
  });
});
