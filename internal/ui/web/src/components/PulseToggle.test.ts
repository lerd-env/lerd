import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import Harness from './PulseToggle.test.svelte';

describe('PulseToggle', () => {
  it('renders the icon child', () => {
    const { container } = render(Harness, { props: { enabled: false } });
    expect(container.querySelector('[data-testid="icon"]')).toBeInTheDocument();
  });

  it('shows the pulsing dot only when enabled', () => {
    const off = render(Harness, { props: { enabled: false } });
    expect(off.container.querySelector('.lerd-pulse-ping')).toBeNull();

    const on = render(Harness, { props: { enabled: true } });
    expect(on.container.querySelector('.lerd-pulse-ping')).not.toBeNull();
  });

  it('paints emerald text when enabled', () => {
    const { container } = render(Harness, { props: { enabled: true } });
    expect(container.querySelector('button')!.className).toMatch(/text-emerald-600/);
  });

  it('disables the button while busy', () => {
    const { container } = render(Harness, { props: { enabled: false, busy: true } });
    expect((container.querySelector('button') as HTMLButtonElement).disabled).toBe(true);
  });

  it('forwards clicks', () => {
    const onclick = vi.fn();
    render(Harness, { props: { enabled: false, title: 'T', onclick } });
    screen.getByTitle('T').click();
    expect(onclick).toHaveBeenCalledOnce();
  });
});
